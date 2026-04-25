// Package httpapp wires the long-running HTTP server application.
package httpapp

import (
	"context"
	"fmt"
	"time"

	"github.com/lenchik/logmonitor/config"
	collectioncron "github.com/lenchik/logmonitor/crons/collection"
	discoverycron "github.com/lenchik/logmonitor/crons/discovery"
	integritycron "github.com/lenchik/logmonitor/crons/integrity"
	"github.com/lenchik/logmonitor/crons/scheduler"
	generalapp "github.com/lenchik/logmonitor/internal/app/general"
	jobqueue "github.com/lenchik/logmonitor/internal/jobs"
	"github.com/lenchik/logmonitor/internal/runtimeinfo"
	httptransport "github.com/lenchik/logmonitor/internal/transport/http"
)

// App owns HTTP transport, async jobs and scheduler lifecycle.
type App struct {
	cfg       *config.Config
	runtime   *generalapp.Runtime
	jobs      *jobqueue.Manager
	apiServer *httptransport.Server
	scheduler *scheduler.Scheduler
}

// New wires repositories, services, HTTP transport and cron jobs into one application.
func New(cfg *config.Config) (*App, error) {
	runtime, err := generalapp.NewRuntime(cfg)
	if err != nil {
		return nil, err
	}

	jobs := jobqueue.NewManager(runtime.Logger, jobqueue.Options{
		Workers:      cfg.Jobs.Workers,
		QueueSize:    cfg.Jobs.QueueSize,
		HistoryLimit: cfg.Jobs.HistoryLimit,
	})

	address := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	app := &App{
		cfg:       cfg,
		runtime:   runtime,
		jobs:      jobs,
		scheduler: scheduler.New(),
	}

	app.apiServer = httptransport.NewServer(
		address,
		runtime.Logger,
		cfg.API.AuthToken,
		runtime.ServerService,
		runtime.LogFileService,
		runtime.EntryService,
		runtime.CheckService,
		jobs,
		runtime.RuntimeState,
		app.readiness,
	)

	if err := app.registerJobs(); err != nil {
		_ = runtime.Close()
		return nil, err
	}
	app.runtime.SetSchedulerEnabled(!cfg.Runtime.DryRun)

	return app, nil
}

// Run starts background jobs and the HTTP API until the context is canceled.
func (a *App) Run(ctx context.Context) (err error) {
	a.jobs.Start(ctx)
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if shutdownErr := a.jobs.Shutdown(shutdownCtx); err == nil && shutdownErr != nil {
			err = fmt.Errorf("http app: shutdown jobs: %w", shutdownErr)
		}
	}()

	if !a.cfg.Runtime.DryRun {
		a.scheduler.Start(ctx)
		defer a.scheduler.Stop()
	}
	defer func() {
		if closeErr := a.runtime.Close(); err == nil && closeErr != nil {
			err = fmt.Errorf("http app: close repository: %w", closeErr)
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

	discoveryRunner := discoverycron.NewRunnerWithHealthAndOptions(
		a.runtime.Logger,
		a.runtime.Repo,
		a.runtime.DiscoveryService(),
		a.runtime.HealthService(),
		a.runtime.LockManager(),
		discoverycron.Options{MaxServerWorkers: a.cfg.Workers.DiscoveryServers},
	)
	if err := a.scheduler.AddFunc("discovery", discoveryInterval, discoveryRunner.Run); err != nil {
		return err
	}

	collectionRunner := collectioncron.NewRunnerWithHealthAndOptions(
		a.runtime.Logger,
		a.runtime.Repo,
		a.runtime.Repo,
		a.runtime.CollectorService(),
		a.runtime.HealthService(),
		a.runtime.LockManager(),
		collectioncron.Options{
			MaxServerWorkers:         a.cfg.Workers.CollectionServers,
			MaxLogFileWorkersPerHost: a.cfg.Workers.CollectionLogFilesPerHost,
		},
	)
	if err := a.scheduler.AddFunc("collection", collectionInterval, collectionRunner.Run); err != nil {
		return err
	}

	integrityRunner := integritycron.NewRunnerWithHealthAndOptions(
		a.runtime.Logger,
		a.runtime.Repo,
		a.runtime.Repo,
		a.runtime.IntegrityService(),
		a.runtime.HealthService(),
		a.runtime.LockManager(),
		integritycron.Options{
			MaxServerWorkers:         a.cfg.Workers.IntegrityServers,
			MaxLogFileWorkersPerHost: a.cfg.Workers.IntegrityLogFilesPerHost,
		},
	)
	if err := a.scheduler.AddFunc("integrity", integrityInterval, integrityRunner.Run); err != nil {
		return err
	}

	return nil
}

// readiness builds the HTTP-specific readiness payload on top of the shared runtime checks.
func (a *App) readiness(ctx context.Context) runtimeinfo.Readiness {
	base := a.runtime.Readiness(ctx)
	checks := append([]runtimeinfo.Check{}, base.Checks...)
	checks = append(checks,
		runtimeinfo.Check{
			Name:    "api",
			Ready:   true,
			Message: "http api is initialized",
		},
	)
	if a.jobs.Started() {
		checks = append(checks, runtimeinfo.Check{
			Name:    "jobs",
			Ready:   true,
			Message: "async job queue workers are running",
		})
	} else {
		checks = append(checks, runtimeinfo.Check{
			Name:    "jobs",
			Ready:   false,
			Message: "async job queue workers are not running",
		})
	}

	if a.cfg.Runtime.DryRun {
		checks = append(checks, runtimeinfo.Check{
			Name:    "scheduler",
			Ready:   true,
			Message: "scheduler is intentionally disabled in dry-run mode",
		})
	} else if a.scheduler.Started() {
		checks = append(checks, runtimeinfo.Check{
			Name:    "scheduler",
			Ready:   true,
			Message: "scheduler workers are running",
		})
	} else {
		checks = append(checks, runtimeinfo.Check{
			Name:    "scheduler",
			Ready:   false,
			Message: "scheduler workers are not running",
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
