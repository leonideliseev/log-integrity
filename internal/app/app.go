// Package app wires repositories, services, API handlers and background jobs.
package app

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/lenchik/logmonitor/config"
	collectioncron "github.com/lenchik/logmonitor/crons/collection"
	discoverycron "github.com/lenchik/logmonitor/crons/discovery"
	integritycron "github.com/lenchik/logmonitor/crons/integrity"
	"github.com/lenchik/logmonitor/crons/locks"
	"github.com/lenchik/logmonitor/crons/scheduler"
	"github.com/lenchik/logmonitor/internal/repository"
	"github.com/lenchik/logmonitor/internal/repository/memory"
	"github.com/lenchik/logmonitor/internal/repository/postgres"
	"github.com/lenchik/logmonitor/internal/security"
	httptransport "github.com/lenchik/logmonitor/internal/transport/http"
	"github.com/lenchik/logmonitor/models"
)

// App owns the runtime dependencies and controls application startup/shutdown.
type App struct {
	cfg       *config.Config
	runtime   *Runtime
	apiServer *httptransport.Server
	scheduler *scheduler.Scheduler
}

// New wires repositories, services, API server and cron jobs into one application.
func New(cfg *config.Config) (*App, error) {
	runtime, err := NewRuntime(cfg)
	if err != nil {
		return nil, err
	}

	address := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	app := &App{
		cfg:       cfg,
		runtime:   runtime,
		scheduler: scheduler.New(),
	}
	apiServer := httptransport.NewServer(
		address,
		runtime.Logger,
		cfg.API.AuthToken,
		runtime.ServerService,
		runtime.LogFileService,
		runtime.EntryService,
		runtime.CheckService,
		runtime.Jobs,
		runtime.RuntimeState,
		runtime.readiness,
	)

	app.apiServer = apiServer

	if err := app.registerJobs(); err != nil {
		_ = runtime.Close()
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
	a.runtime.Jobs.Start(ctx)
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if shutdownErr := a.runtime.Jobs.Shutdown(shutdownCtx); err == nil && shutdownErr != nil {
			err = fmt.Errorf("app: shutdown jobs: %w", shutdownErr)
		}
	}()

	if !a.cfg.Runtime.DryRun {
		a.scheduler.Start(ctx)
		defer a.scheduler.Stop()
	}
	defer func() {
		if closeErr := a.runtime.Close(); err == nil && closeErr != nil {
			err = fmt.Errorf("app: close repository: %w", closeErr)
		}
	}()

	return a.apiServer.Run(ctx)
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

	discoveryRunner := discoverycron.NewRunnerWithHealthAndOptions(a.runtime.Logger, a.runtime.Repo, a.runtime.discovery, a.runtime.health, a.runtime.locks, discoverycron.Options{
		MaxServerWorkers: a.cfg.Workers.DiscoveryServers,
	})
	if err := a.scheduler.AddFunc("discovery", discoveryInterval, discoveryRunner.Run); err != nil {
		return err
	}
	collectionRunner := collectioncron.NewRunnerWithHealthAndOptions(a.runtime.Logger, a.runtime.Repo, a.runtime.Repo, a.runtime.collector, a.runtime.health, a.runtime.locks, collectioncron.Options{
		MaxServerWorkers:         a.cfg.Workers.CollectionServers,
		MaxLogFileWorkersPerHost: a.cfg.Workers.CollectionLogFilesPerHost,
	})
	if err := a.scheduler.AddFunc("collection", collectionInterval, collectionRunner.Run); err != nil {
		return err
	}
	integrityRunner := integritycron.NewRunnerWithHealthAndOptions(a.runtime.Logger, a.runtime.Repo, a.runtime.Repo, a.runtime.integrity, a.runtime.health, a.runtime.locks, integritycron.Options{
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
