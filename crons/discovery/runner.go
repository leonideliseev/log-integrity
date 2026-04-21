// Package discoverycron runs scheduled log discovery jobs.
package discoverycron

import (
	"context"
	"log/slog"
	"sync"

	"github.com/lenchik/logmonitor/crons/locks"
	"github.com/lenchik/logmonitor/internal/repository"
	discoveryservice "github.com/lenchik/logmonitor/internal/service/discovery"
	"github.com/lenchik/logmonitor/models"
)

// Options controls bounded concurrency for discovery jobs.
type Options struct {
	MaxServerWorkers int
}

// Runner executes bounded concurrent discovery sweeps.
type Runner struct {
	logger      *slog.Logger
	servers     repository.ServerRepository
	discovery   *discoveryservice.Service
	lockManager *locks.Manager
	options     Options
}

// NewRunner creates a cron runner for discovery jobs.
func NewRunner(logger *slog.Logger, servers repository.ServerRepository, discovery *discoveryservice.Service) *Runner {
	return NewRunnerWithOptions(logger, servers, discovery, nil, Options{})
}

// NewRunnerWithOptions creates a cron runner with explicit concurrency settings.
func NewRunnerWithOptions(logger *slog.Logger, servers repository.ServerRepository, discovery *discoveryservice.Service, lockManager *locks.Manager, options Options) *Runner {
	options = normalizeOptions(options)
	return &Runner{
		logger:      logger,
		servers:     servers,
		discovery:   discovery,
		lockManager: lockManager,
		options:     options,
	}
}

// Run executes one discovery sweep for all registered servers.
func (r *Runner) Run(ctx context.Context) {
	servers, err := r.servers.ListServers(ctx)
	if err != nil {
		r.logger.Error("list servers for discovery", "error", err)
		return
	}

	r.runServerWorkers(ctx, servers)
}

// runServerWorkers processes servers with a bounded worker pool.
func (r *Runner) runServerWorkers(ctx context.Context, servers []*models.Server) {
	sem := make(chan struct{}, r.options.MaxServerWorkers)
	var wg sync.WaitGroup

	for _, serverModel := range servers {
		if ctx.Err() != nil {
			break
		}
		sem <- struct{}{}
		wg.Add(1)
		go func(serverModel *models.Server) {
			defer wg.Done()
			defer func() { <-sem }()
			r.processServer(ctx, serverModel)
		}(serverModel)
	}

	wg.Wait()
}

// processServer runs discovery for one server under an optional isolation lock.
func (r *Runner) processServer(ctx context.Context, serverModel *models.Server) {
	if serverModel.Status == models.ServerStatusInactive {
		r.logger.Debug("skip discovery because server is inactive", "server", serverModel.Name)
		return
	}

	unlock, ok := r.tryLockServer(serverModel)
	if !ok {
		r.logger.Debug("skip discovery because server is locked", "server", serverModel.Name)
		return
	}
	defer unlock()

	if _, err := r.discovery.DiscoverAndSync(ctx, serverModel); err != nil {
		_ = r.servers.UpdateServerStatus(ctx, serverModel.ID, models.ServerStatusError)
		r.logger.Error("discover logs", "server", serverModel.Name, "error", err)
		return
	}
	_ = r.servers.UpdateServerStatus(ctx, serverModel.ID, models.ServerStatusActive)
}

// tryLockServer acquires a non-blocking server lock when isolation is enabled.
func (r *Runner) tryLockServer(serverModel *models.Server) (func(), bool) {
	if r.lockManager == nil {
		return func() {}, true
	}
	return r.lockManager.TryLock("server:" + serverModel.ID)
}

// normalizeOptions applies safe defaults for discovery worker settings.
func normalizeOptions(options Options) Options {
	if options.MaxServerWorkers <= 0 {
		options.MaxServerWorkers = 1
	}
	return options
}
