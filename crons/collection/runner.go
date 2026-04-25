// Package collectioncron runs scheduled log collection jobs.
package collectioncron

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"github.com/lenchik/logmonitor/crons/locks"
	"github.com/lenchik/logmonitor/internal/repository"
	collectservice "github.com/lenchik/logmonitor/internal/service/collector"
	healthservice "github.com/lenchik/logmonitor/internal/service/health"
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
	health      *healthservice.Service
	lockManager *locks.Manager
	options     Options
}

// NewRunner creates a cron runner for log collection jobs.
func NewRunner(logger *slog.Logger, servers repository.ServerRepository, logFiles repository.LogFileRepository, collector *collectservice.Service) *Runner {
	return NewRunnerWithOptions(logger, servers, logFiles, collector, nil, Options{})
}

// NewRunnerWithOptions creates a cron runner with explicit concurrency settings.
func NewRunnerWithOptions(logger *slog.Logger, servers repository.ServerRepository, logFiles repository.LogFileRepository, collector *collectservice.Service, lockManager *locks.Manager, options Options) *Runner {
	return NewRunnerWithHealthAndOptions(logger, servers, logFiles, collector, healthservice.NewService(servers, healthservice.Options{}), lockManager, options)
}

// NewRunnerWithHealthAndOptions creates a cron runner with health tracking and explicit concurrency settings.
func NewRunnerWithHealthAndOptions(logger *slog.Logger, servers repository.ServerRepository, logFiles repository.LogFileRepository, collector *collectservice.Service, health *healthservice.Service, lockManager *locks.Manager, options Options) *Runner {
	options = normalizeOptions(options)
	return &Runner{
		logger:      logger,
		servers:     servers,
		logFiles:    logFiles,
		collector:   collector,
		health:      health,
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
	if r.health.ShouldSkip(serverModel) {
		r.logger.Debug("skip collection because server is in backoff", "server", serverModel.Name, "backoff_until", serverModel.BackoffUntil)
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
		_ = r.health.RecordFailure(ctx, serverModel, err)
		r.logger.Error("list log files", "server", serverModel.Name, "error", err)
		return
	}

	active := make([]*models.LogFile, 0, len(logFiles))
	for _, logFile := range logFiles {
		if logFile.IsActive {
			active = append(active, logFile)
		}
	}
	if len(active) == 0 {
		r.logger.Debug("skip collection because server has no active log files", "server", serverModel.Name)
		return
	}

	summary := r.runLogFileWorkers(ctx, serverModel, active)
	switch {
	case summary.failureCount == 0:
		_ = r.health.RecordSuccess(ctx, serverModel.ID)
	case summary.successCount == 0:
		_ = r.health.RecordFailure(ctx, serverModel, summary.firstErr)
	default:
		_ = r.health.RecordDegraded(ctx, serverModel.ID, fmt.Sprintf("collection partially failed for %d of %d log files", summary.failureCount, len(active)))
	}
}

// runLogFileWorkers collects log files of one server with a bounded worker pool.
func (r *Runner) runLogFileWorkers(ctx context.Context, serverModel *models.Server, logFiles []*models.LogFile) collectionSummary {
	sem := make(chan struct{}, r.options.MaxLogFileWorkersPerHost)
	var wg sync.WaitGroup
	var mu sync.Mutex
	summary := collectionSummary{}

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
				if summary.firstErr == nil {
					summary.firstErr = err
				}
				summary.failureCount++
				mu.Unlock()
				r.logger.Error("collect log entries", "server", serverModel.Name, "log_file", logFile.Path, "error", err)
				return
			}
			mu.Lock()
			summary.successCount++
			mu.Unlock()
		}(logFile)
	}

	wg.Wait()
	return summary
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

// collectionSummary aggregates one collection cycle outcome for a server.
type collectionSummary struct {
	successCount int
	failureCount int
	firstErr     error
}
