// Package app wires repositories, services, API handlers and background jobs.
package app

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/lenchik/logmonitor/config"
	collectioncron "github.com/lenchik/logmonitor/crons/collection"
	discoverycron "github.com/lenchik/logmonitor/crons/discovery"
	integritycron "github.com/lenchik/logmonitor/crons/integrity"
	"github.com/lenchik/logmonitor/crons/locks"
	"github.com/lenchik/logmonitor/crons/scheduler"
	"github.com/lenchik/logmonitor/internal/api"
	jobqueue "github.com/lenchik/logmonitor/internal/jobs"
	"github.com/lenchik/logmonitor/internal/repository"
	"github.com/lenchik/logmonitor/internal/repository/memory"
	"github.com/lenchik/logmonitor/internal/repository/postgres"
	"github.com/lenchik/logmonitor/internal/runtimeinfo"
	"github.com/lenchik/logmonitor/internal/security"
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

// App owns the runtime dependencies and controls application startup/shutdown.
type App struct {
	cfg       *config.Config
	logger    *slog.Logger
	repo      repository.Repository
	jobs      *jobqueue.Manager
	apiServer *api.Server
	scheduler *scheduler.Scheduler

	discovery *discoveryservice.Service
	collector *collectservice.Service
	integrity *integrityservice.Service
	health    *healthservice.Service
	locks     *locks.Manager
	runtime   *runtimeinfo.State
}

// New wires repositories, services, API server and cron jobs into one application.
func New(cfg *config.Config) (*App, error) {
	log := logger.New("info")
	runtimeState := runtimeinfo.NewState()
	runtimeState.SetDryRun(cfg.Runtime.DryRun)
	runtimeState.SetEnvChecks(cfg.EnvChecks)

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
	serverService := serverappservice.NewServiceWithHealthAndLocker(store, discoveryService, healthService, lockManager)
	logFileService := logfileappservice.NewServiceWithHealthAndLocker(store, store, collectorService, healthService, lockManager)
	entryService := entryappservice.NewService(store)
	checkService := checkappservice.NewServiceWithHealthAndLocker(store, store, store, integrityService, healthService, lockManager)

	address := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	app := &App{
		cfg:       cfg,
		logger:    log,
		repo:      store,
		jobs:      jobqueue.NewManager(log, jobqueue.Options{Workers: cfg.Jobs.Workers, QueueSize: cfg.Jobs.QueueSize, HistoryLimit: cfg.Jobs.HistoryLimit}),
		scheduler: scheduler.New(),
		discovery: discoveryService,
		collector: collectorService,
		integrity: integrityService,
		health:    healthService,
		locks:     lockManager,
		runtime:   runtimeState,
	}
	apiServer := api.NewServer(address, log, cfg.API.AuthToken, serverService, logFileService, entryService, checkService, app.jobs, runtimeState, app.readiness)

	app.apiServer = apiServer

	if err := app.seedServers(context.Background()); err != nil {
		return nil, err
	}
	if err := app.registerJobs(); err != nil {
		return nil, err
	}
	app.runtime.SetSchedulerEnabled(!cfg.Runtime.DryRun)

	return app, nil
}

// buildLockManager creates shared cron isolation locks when enabled in config.
func buildLockManager(cfg *config.Config) *locks.Manager {
	if cfg.Workers.PerServerIsolation == nil || !*cfg.Workers.PerServerIsolation {
		return nil
	}
	return locks.NewManager()
}

// buildRepository selects PostgreSQL when database settings are provided and falls back to memory otherwise.
func buildRepository(cfg *config.Config) (repository.Repository, string, error) {
	if cfg.Runtime.DryRun {
		return memory.New(), "memory", nil
	}
	if cfg.Database.Host == "" || cfg.Database.User == "" || cfg.Database.DBName == "" {
		return memory.New(), "memory", nil
	}

	authCipher, err := security.NewStringCipher(cfg.Security.AuthValueEncryptionKey)
	if err != nil {
		return nil, "", fmt.Errorf("app: create auth value cipher: %w", err)
	}

	store, err := postgres.OpenWithOptions(cfg.Database.DSN(), postgres.Options{
		MaxConns:      cfg.Database.MaxConns,
		MinConns:      cfg.Database.MinConns,
		MigrationsDir: cfg.Database.MigrationsDir,
		AuthCipher:    authCipher,
	})
	if err != nil {
		return nil, "", fmt.Errorf("app: open postgres repository: %w", err)
	}

	return store, "postgres", nil
}

// Run starts background jobs and the API server until the context is canceled.
func (a *App) Run(ctx context.Context) (err error) {
	a.jobs.Start(ctx)
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if shutdownErr := a.jobs.Shutdown(shutdownCtx); err == nil && shutdownErr != nil {
			err = fmt.Errorf("app: shutdown jobs: %w", shutdownErr)
		}
	}()

	if !a.cfg.Runtime.DryRun {
		a.scheduler.Start(ctx)
		defer a.scheduler.Stop()
	}
	defer func() {
		if closeErr := a.repo.Close(); err == nil && closeErr != nil {
			err = fmt.Errorf("app: close repository: %w", closeErr)
		}
	}()

	return a.apiServer.Run(ctx)
}

// seedServers imports server definitions from config into the repository.
func (a *App) seedServers(ctx context.Context) error {
	existing, err := a.repo.ListServers(ctx)
	if err != nil {
		return fmt.Errorf("app: list existing servers: %w", err)
	}

	for _, item := range a.cfg.Servers {
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
			if err := a.repo.UpdateServer(ctx, serverModel); err != nil {
				a.runtime.AddWarning("seed-server-skipped", fmt.Sprintf("server %q was skipped during bootstrap: %v", item.Name, err))
				a.logger.Warn("skip server bootstrap update", "server", item.Name, "error", err)
				continue
			}
			existing = replaceServer(existing, serverModel)
			continue
		}

		serverModel := serverModelFromConfig(item)

		if err := a.repo.CreateServer(ctx, serverModel); err != nil {
			a.runtime.AddWarning("seed-server-skipped", fmt.Sprintf("server %q was skipped during bootstrap: %v", item.Name, err))
			a.logger.Warn("skip server bootstrap create", "server", item.Name, "error", err)
			continue
		}
		existing = append(existing, serverModel)
	}

	return nil
}

// registerJobs registers all configured cron jobs in the scheduler.
func (a *App) registerJobs() error {
	if a.cfg.Runtime.DryRun {
		return nil
	}
	discoveryInterval, err := scheduler.ParseInterval(a.cfg.Scheduler.DiscoveryCron)
	if err != nil {
		return err
	}
	collectionInterval, err := scheduler.ParseInterval(a.cfg.Scheduler.CollectionCron)
	if err != nil {
		return err
	}
	integrityInterval, err := scheduler.ParseInterval(a.cfg.Scheduler.IntegrityCron)
	if err != nil {
		return err
	}

	discoveryRunner := discoverycron.NewRunnerWithHealthAndOptions(a.logger, a.repo, a.discovery, a.health, a.locks, discoverycron.Options{
		MaxServerWorkers: a.cfg.Workers.DiscoveryServers,
	})
	if err := a.scheduler.AddFunc("discovery", discoveryInterval, discoveryRunner.Run); err != nil {
		return err
	}
	collectionRunner := collectioncron.NewRunnerWithHealthAndOptions(a.logger, a.repo, a.repo, a.collector, a.health, a.locks, collectioncron.Options{
		MaxServerWorkers:         a.cfg.Workers.CollectionServers,
		MaxLogFileWorkersPerHost: a.cfg.Workers.CollectionLogFilesPerHost,
	})
	if err := a.scheduler.AddFunc("collection", collectionInterval, collectionRunner.Run); err != nil {
		return err
	}
	integrityRunner := integritycron.NewRunnerWithHealthAndOptions(a.logger, a.repo, a.repo, a.integrity, a.health, a.locks, integritycron.Options{
		MaxServerWorkers:         a.cfg.Workers.IntegrityServers,
		MaxLogFileWorkersPerHost: a.cfg.Workers.IntegrityLogFilesPerHost,
	})
	if err := a.scheduler.AddFunc("integrity", integrityInterval, integrityRunner.Run); err != nil {
		return err
	}

	return nil
}

// serverModelFromConfig converts a configured server into a repository model.
func serverModelFromConfig(item config.ServerEntry) *models.Server {
	serverModel := &models.Server{
		Name:      item.Name,
		Host:      item.Host,
		Port:      item.Port,
		Username:  item.Username,
		AuthType:  models.AuthType(item.AuthType),
		AuthValue: item.AuthValue,
		OSType:    models.OSType(item.OSType),
		Status:    models.ServerStatusActive,
		ManagedBy: models.ServerManagedByConfig,
	}
	if serverModel.Port == 0 {
		serverModel.Port = 22
	}
	return serverModel
}

// isConfigManagedServer reports whether the config bootstrap is allowed to update a server.
func isConfigManagedServer(serverModel *models.Server) bool {
	return serverModel.ManagedBy == "" || serverModel.ManagedBy == models.ServerManagedByConfig
}

// replaceServer keeps the local seed snapshot aligned after an upsert.
func replaceServer(servers []*models.Server, replacement *models.Server) []*models.Server {
	for index, item := range servers {
		if item.ID == replacement.ID {
			servers[index] = replacement
			return servers
		}
	}
	return append(servers, replacement)
}

// findServerByNameOrHost returns an existing server with the same configured identity.
func findServerByNameOrHost(servers []*models.Server, name, host string) *models.Server {
	for _, item := range servers {
		if strings.EqualFold(item.Name, name) || strings.EqualFold(item.Host, host) {
			return item
		}
	}
	return nil
}

// databaseConfigured reports whether PostgreSQL settings are present enough to attempt a real connection.
func databaseConfigured(cfg *config.Config) bool {
	return cfg.Database.Host != "" && cfg.Database.User != "" && cfg.Database.DBName != ""
}

// readiness reports whether the process is ready to serve traffic and background tasks.
func (a *App) readiness(ctx context.Context) runtimeinfo.Readiness {
	checks := make([]runtimeinfo.Check, 0, 3)

	if err := a.repo.Ping(ctx); err != nil {
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

	if a.cfg.Runtime.DryRun {
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
