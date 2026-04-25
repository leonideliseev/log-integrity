// Package app wires repositories, services, API handlers and background jobs.
package app

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/lenchik/logmonitor/config"
	"github.com/lenchik/logmonitor/crons/locks"
	jobqueue "github.com/lenchik/logmonitor/internal/jobs"
	"github.com/lenchik/logmonitor/internal/repository"
	"github.com/lenchik/logmonitor/internal/runtimeinfo"
	checkappservice "github.com/lenchik/logmonitor/internal/service/check"
	collectservice "github.com/lenchik/logmonitor/internal/service/collector"
	discoveryservice "github.com/lenchik/logmonitor/internal/service/discovery"
	entryappservice "github.com/lenchik/logmonitor/internal/service/entry"
	healthservice "github.com/lenchik/logmonitor/internal/service/health"
	integrityservice "github.com/lenchik/logmonitor/internal/service/integrity"
	logfileappservice "github.com/lenchik/logmonitor/internal/service/logfile"
	serverappservice "github.com/lenchik/logmonitor/internal/service/server"
	sshclient "github.com/lenchik/logmonitor/internal/ssh"
	"github.com/lenchik/logmonitor/models"
	"github.com/lenchik/logmonitor/pkg/logger"
)

// Runtime holds reusable application dependencies shared by server and CLI entrypoints.
type Runtime struct {
	Config       *config.Config
	Logger       *slog.Logger
	Repo         repository.Repository
	Jobs         *jobqueue.Manager
	RuntimeState *runtimeinfo.State

	ServerService  *serverappservice.Service
	LogFileService *logfileappservice.Service
	EntryService   *entryappservice.Service
	CheckService   *checkappservice.Service

	discovery *discoveryservice.Service
	collector *collectservice.Service
	integrity *integrityservice.Service
	health    *healthservice.Service
	locks     *locks.Manager
}

// NewRuntime builds repositories, services and shared runtime state without starting transports.
func NewRuntime(cfg *config.Config) (*Runtime, error) {
	log := logger.New("info")
	runtimeState := runtimeinfo.NewState()
	runtimeState.SetDryRun(cfg.Runtime.DryRun)
	runtimeState.SetEnvChecks(cfg.EnvChecks)
	runtimeState.SetSchedulerEnabled(false)

	store, backend, err := buildRepository(cfg)
	if err != nil {
		return nil, err
	}
	runtimeState.SetStorageBackend(backend)
	if cfg.Runtime.DryRun {
		runtimeState.AddWarning("dry-run-enabled", "dry-run mode is enabled: background jobs are disabled and the in-memory repository is used")
	}
	if cfg.Runtime.DryRun && databaseConfigured(cfg) {
		runtimeState.AddWarning("dry-run-database-skipped", "database configuration was ignored because dry-run mode is enabled")
	}

	sshFactory, err := sshclient.NewClientFactoryWithOptions(sshclient.Options{
		ConnectTimeout:        time.Duration(cfg.SSH.ConnectTimeoutSeconds) * time.Second,
		CommandTimeout:        time.Duration(cfg.SSH.CommandTimeoutSeconds) * time.Second,
		KnownHostsPath:        cfg.SSH.KnownHostsPath,
		InsecureIgnoreHostKey: *cfg.SSH.InsecureIgnoreHostKey,
	})
	if err != nil {
		return nil, fmt.Errorf("app: create ssh client factory: %w", err)
	}

	discoveryService := discoveryservice.NewServiceWithServerRepository(sshFactory, store, store, nil)
	collectorService := collectservice.NewServiceWithOptions(sshFactory, store, store, store, collectservice.Options{
		BatchSize:        cfg.Collector.BatchSize,
		ChunkSize:        cfg.Collector.ChunkSize,
		StoreRawContent:  *cfg.Collector.StoreRawContent,
		ChunkHashAlgo:    cfg.Collector.ChunkHashAlgo,
		IntegrityHMACKey: cfg.Security.IntegrityHMACKey,
	})
	integrityService := integrityservice.NewServiceWithOptions(sshFactory, store, store, store, integrityservice.Options{
		IntegrityHMACKey: cfg.Security.IntegrityHMACKey,
	})
	healthService := healthservice.NewService(store, healthservice.Options{
		FailureThreshold:   cfg.Health.FailureThreshold,
		BackoffBase:        time.Duration(cfg.Health.BackoffBaseSeconds) * time.Second,
		BackoffMax:         time.Duration(cfg.Health.BackoffMaxSeconds) * time.Second,
		LastErrorMaxLength: cfg.Health.LastErrorMaxLength,
	})
	lockManager := buildLockManager(cfg)

	runtime := &Runtime{
		Config:       cfg,
		Logger:       log,
		Repo:         store,
		Jobs:         jobqueue.NewManager(log, jobqueue.Options{Workers: cfg.Jobs.Workers, QueueSize: cfg.Jobs.QueueSize, HistoryLimit: cfg.Jobs.HistoryLimit}),
		RuntimeState: runtimeState,
		discovery:    discoveryService,
		collector:    collectorService,
		integrity:    integrityService,
		health:       healthService,
		locks:        lockManager,
	}

	runtime.ServerService = serverappservice.NewServiceWithHealthAndLocker(store, discoveryService, healthService, lockManager)
	runtime.LogFileService = logfileappservice.NewServiceWithHealthAndLocker(store, store, collectorService, healthService, lockManager)
	runtime.EntryService = entryappservice.NewService(store)
	runtime.CheckService = checkappservice.NewServiceWithHealthAndLocker(store, store, store, integrityService, healthService, lockManager)

	if err := runtime.seedServers(context.Background()); err != nil {
		_ = runtime.Close()
		return nil, err
	}

	return runtime, nil
}

// Close releases runtime resources such as the repository connection.
func (r *Runtime) Close() error {
	if r == nil || r.Repo == nil {
		return nil
	}
	return r.Repo.Close()
}

// SetSchedulerEnabled stores the effective scheduler availability in runtime state.
func (r *Runtime) SetSchedulerEnabled(enabled bool) {
	if r == nil || r.RuntimeState == nil {
		return
	}
	r.RuntimeState.SetSchedulerEnabled(enabled)
}

// Readiness returns the current readiness snapshot for transports and CLI commands.
func (r *Runtime) Readiness(ctx context.Context) runtimeinfo.Readiness {
	return r.readiness(ctx)
}

// seedServers imports server definitions from config into the repository.
func (r *Runtime) seedServers(ctx context.Context) error {
	existing, err := r.Repo.ListServers(ctx)
	if err != nil {
		return fmt.Errorf("app: list existing servers: %w", err)
	}

	for _, item := range r.Config.Servers {
		if current := findServerByNameOrHost(existing, item.Name, item.Host); current != nil {
			if !isConfigManagedServer(current) {
				return fmt.Errorf(
					"%w: app: config server %q (%s) conflicts with API-managed server %q",
					repository.ErrConflict,
					item.Name,
					item.Host,
					current.ID,
				)
			}

			serverModel := serverModelFromConfig(item)
			serverModel.ID = current.ID
			serverModel.CreatedAt = current.CreatedAt
			serverModel.Status = current.Status
			serverModel.SuccessCount = current.SuccessCount
			serverModel.FailureCount = current.FailureCount
			serverModel.LastError = current.LastError
			serverModel.LastSeenAt = current.LastSeenAt
			serverModel.BackoffUntil = current.BackoffUntil
			if serverModel.Status == "" {
				serverModel.Status = models.ServerStatusActive
			}
			if err := r.Repo.UpdateServer(ctx, serverModel); err != nil {
				r.RuntimeState.AddWarning("seed-server-skipped", fmt.Sprintf("server %q was skipped during bootstrap: %v", item.Name, err))
				r.Logger.Warn("skip server bootstrap update", "server", item.Name, "error", err)
				continue
			}
			existing = replaceServer(existing, serverModel)
			continue
		}

		serverModel := serverModelFromConfig(item)
		if err := r.Repo.CreateServer(ctx, serverModel); err != nil {
			r.RuntimeState.AddWarning("seed-server-skipped", fmt.Sprintf("server %q was skipped during bootstrap: %v", item.Name, err))
			r.Logger.Warn("skip server bootstrap create", "server", item.Name, "error", err)
			continue
		}
		existing = append(existing, serverModel)
	}

	return nil
}

// readiness reports whether the process is ready to serve traffic and background tasks.
func (r *Runtime) readiness(ctx context.Context) runtimeinfo.Readiness {
	checks := make([]runtimeinfo.Check, 0, 4)

	if err := r.Repo.Ping(ctx); err != nil {
		checks = append(checks, runtimeinfo.Check{
			Name:    "repository",
			Ready:   false,
			Message: err.Error(),
		})
	} else {
		checks = append(checks, runtimeinfo.Check{
			Name:    "repository",
			Ready:   true,
			Message: "repository is reachable",
		})
	}

	checks = append(checks, runtimeinfo.Check{
		Name:    "api",
		Ready:   true,
		Message: "http api is initialized",
	})
	checks = append(checks, runtimeinfo.Check{
		Name:    "jobs",
		Ready:   true,
		Message: "async job queue is configured",
	})

	if r.Config.Runtime.DryRun {
		checks = append(checks, runtimeinfo.Check{
			Name:    "scheduler",
			Ready:   true,
			Message: "scheduler is intentionally disabled in dry-run mode",
		})
	} else {
		checks = append(checks, runtimeinfo.Check{
			Name:    "scheduler",
			Ready:   true,
			Message: "scheduler is configured",
		})
	}

	ready := true
	for _, check := range checks {
		if !check.Ready {
			ready = false
			break
		}
	}

	return runtimeinfo.Readiness{
		Ready:  ready,
		Checks: checks,
	}
}
