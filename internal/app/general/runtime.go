// Package general builds the shared application runtime used by both HTTP and CLI entrypoints.
package general

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/lenchik/logmonitor/config"
	"github.com/lenchik/logmonitor/crons/locks"
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

// Runtime holds reusable application dependencies shared by server and CLI entrypoints.
type Runtime struct {
	Config       *config.Config
	Logger       *slog.Logger
	Repo         repository.Repository
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

// Readiness returns core readiness checks shared by all startup modes.
func (r *Runtime) Readiness(ctx context.Context) runtimeinfo.Readiness {
	checks := make([]runtimeinfo.Check, 0, 2)

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
		Name:    "services",
		Ready:   true,
		Message: "core services are initialized",
	})

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

// DiscoveryService exposes the shared discovery implementation to server-only orchestration.
func (r *Runtime) DiscoveryService() *discoveryservice.Service {
	return r.discovery
}

// CollectorService exposes the shared collector implementation to server-only orchestration.
func (r *Runtime) CollectorService() *collectservice.Service {
	return r.collector
}

// IntegrityService exposes the shared integrity implementation to server-only orchestration.
func (r *Runtime) IntegrityService() *integrityservice.Service {
	return r.integrity
}

// HealthService exposes the shared health implementation to server-only orchestration.
func (r *Runtime) HealthService() *healthservice.Service {
	return r.health
}

// LockManager exposes the shared isolation manager to server-only orchestration.
func (r *Runtime) LockManager() *locks.Manager {
	return r.locks
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
