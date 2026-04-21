// Package collectioncron runs scheduled log collection jobs.
package collectioncron

import (
	"context"
	"log/slog"
	"sync"

	"github.com/lenchik/logmonitor/crons/locks"
	"github.com/lenchik/logmonitor/internal/repository"
	collectservice "github.com/lenchik/logmonitor/internal/service/collector"
	"github.com/lenchik/logmonitor/models"
)

// Options controls bounded concurrency for collection jobs.
type Options struct {
	MaxServerWorkers         int
	MaxLogFileWorkersPerHost int
}

// Runner executes bounded concurrent collection sweeps.
type Runner struct {
	logger      *slog.Logger
	servers     repository.ServerRepository
	logFiles    repository.LogFileRepository
	collector   *collectservice.Service
	lockManager *locks.Manager
	options     Options
}

// NewRunner creates a cron runner for log collection jobs.
func NewRunner(logger *slog.Logger, servers repository.ServerRepository, logFiles repository.LogFileRepository, collector *collectservice.Service) *Runner {
	return NewRunnerWithOptions(logger, servers, logFiles, collector, nil, Options{})
}

// NewRunnerWithOptions creates a cron runner with explicit concurrency settings.
func NewRunnerWithOptions(logger *slog.Logger, servers repository.ServerRepository, logFiles repository.LogFileRepository, collector *collectservice.Service, lockManager *locks.Manager, options Options) *Runner {
	options = normalizeOptions(options)
	return &Runner{
		logger:      logger,
		servers:     servers,
		logFiles:    logFiles,
		collector:   collector,
		lockManager: lockManager,
		options:     options,
	}
}

// Run executes one collection sweep for all active server log files.
func (r *Runner) Run(ctx context.Context) {
	servers, err := r.servers.ListServers(ctx)
	if err != nil {
		r.logger.Error("list servers for collection", "error", err)
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

// processServer collects logs for one server under an optional isolation lock.
func (r *Runner) processServer(ctx context.Context, serverModel *models.Server) {
	if serverModel.Status == models.ServerStatusInactive {
		r.logger.Debug("skip collection because server is inactive", "server", serverModel.Name)
		return
	}

	unlock, ok := r.tryLockServer(serverModel)
	if !ok {
		r.logger.Debug("skip collection because server is locked", "server", serverModel.Name)
		return
	}
	defer unlock()

	logFiles, err := r.logFiles.ListLogFilesByServer(ctx, serverModel.ID)
	if err != nil {
		_ = r.servers.UpdateServerStatus(ctx, serverModel.ID, models.ServerStatusError)
		r.logger.Error("list log files", "server", serverModel.Name, "error", err)
		return
	}

	active := make([]*models.LogFile, 0, len(logFiles))
	for _, logFile := range logFiles {
		if logFile.IsActive {
			active = append(active, logFile)
		}
	}

	if r.runLogFileWorkers(ctx, serverModel, active) {
		_ = r.servers.UpdateServerStatus(ctx, serverModel.ID, models.ServerStatusActive)
		return
	}
	_ = r.servers.UpdateServerStatus(ctx, serverModel.ID, models.ServerStatusError)
}

// runLogFileWorkers collects log files of one server with a bounded worker pool.
func (r *Runner) runLogFileWorkers(ctx context.Context, serverModel *models.Server, logFiles []*models.LogFile) bool {
	sem := make(chan struct{}, r.options.MaxLogFileWorkersPerHost)
	var wg sync.WaitGroup
	var mu sync.Mutex
	success := true

	for _, logFile := range logFiles {
		if ctx.Err() != nil {
			break
		}
		sem <- struct{}{}
		wg.Add(1)
		go func(logFile *models.LogFile) {
			defer wg.Done()
			defer func() { <-sem }()
			if _, err := r.collector.CollectLogFile(ctx, serverModel, logFile); err != nil {
				mu.Lock()
				success = false
				mu.Unlock()
				r.logger.Error("collect log entries", "server", serverModel.Name, "log_file", logFile.Path, "error", err)
			}
		}(logFile)
	}

	wg.Wait()
	return success
}

// tryLockServer acquires a non-blocking server lock when isolation is enabled.
func (r *Runner) tryLockServer(serverModel *models.Server) (func(), bool) {
	if r.lockManager == nil {
		return func() {}, true
	}
	return r.lockManager.TryLock("server:" + serverModel.ID)
}

// normalizeOptions applies safe defaults for collection worker settings.
func normalizeOptions(options Options) Options {
	if options.MaxServerWorkers <= 0 {
		options.MaxServerWorkers = 1
	}
	if options.MaxLogFileWorkersPerHost <= 0 {
		options.MaxLogFileWorkersPerHost = 1
	}
	return options
}
