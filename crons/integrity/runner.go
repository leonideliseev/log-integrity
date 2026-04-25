// Package integritycron runs scheduled integrity check jobs.
package integritycron

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"github.com/lenchik/logmonitor/crons/locks"
	"github.com/lenchik/logmonitor/internal/repository"
	healthservice "github.com/lenchik/logmonitor/internal/service/health"
	integrityservice "github.com/lenchik/logmonitor/internal/service/integrity"
	"github.com/lenchik/logmonitor/models"
)

// Options controls bounded concurrency for integrity jobs.
type Options struct {
	MaxServerWorkers         int
	MaxLogFileWorkersPerHost int
}

// Runner executes bounded concurrent integrity sweeps.
type Runner struct {
	logger      *slog.Logger
	servers     repository.ServerRepository
	logFiles    repository.LogFileRepository
	integrity   *integrityservice.Service
	health      *healthservice.Service
	lockManager *locks.Manager
	options     Options
}

// NewRunner creates a cron runner for integrity check jobs.
func NewRunner(logger *slog.Logger, servers repository.ServerRepository, logFiles repository.LogFileRepository, integrity *integrityservice.Service) *Runner {
	return NewRunnerWithOptions(logger, servers, logFiles, integrity, nil, Options{})
}

// NewRunnerWithOptions creates a cron runner with explicit concurrency settings.
func NewRunnerWithOptions(logger *slog.Logger, servers repository.ServerRepository, logFiles repository.LogFileRepository, integrity *integrityservice.Service, lockManager *locks.Manager, options Options) *Runner {
	return NewRunnerWithHealthAndOptions(logger, servers, logFiles, integrity, healthservice.NewService(servers, healthservice.Options{}), lockManager, options)
}

// NewRunnerWithHealthAndOptions creates a cron runner with health tracking and explicit concurrency settings.
func NewRunnerWithHealthAndOptions(logger *slog.Logger, servers repository.ServerRepository, logFiles repository.LogFileRepository, integrity *integrityservice.Service, health *healthservice.Service, lockManager *locks.Manager, options Options) *Runner {
	options = normalizeOptions(options)
	return &Runner{
		logger:      logger,
		servers:     servers,
		logFiles:    logFiles,
		integrity:   integrity,
		health:      health,
		lockManager: lockManager,
		options:     options,
	}
}

// Run executes integrity checks for all active log files across all servers.
func (r *Runner) Run(ctx context.Context) {
	servers, err := r.servers.ListServers(ctx)
	if err != nil {
		r.logger.Error("list servers for integrity", "error", err)
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

// processServer runs integrity checks for one server under an optional isolation lock.
func (r *Runner) processServer(ctx context.Context, serverModel *models.Server) {
	if serverModel.Status == models.ServerStatusInactive {
		r.logger.Debug("skip integrity because server is inactive", "server", serverModel.Name)
		return
	}
	if r.health.ShouldSkip(serverModel) {
		r.logger.Debug("skip integrity because server is in backoff", "server", serverModel.Name, "backoff_until", serverModel.BackoffUntil)
		return
	}

	unlock, ok := r.tryLockServer(serverModel)
	if !ok {
		r.logger.Debug("skip integrity because server is locked", "server", serverModel.Name)
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
		r.logger.Debug("skip integrity because server has no active log files", "server", serverModel.Name)
		return
	}

	summary := r.runLogFileWorkers(ctx, serverModel, active)
	switch {
	case summary.failureCount == 0 && summary.tamperedCount == 0:
		_ = r.health.RecordSuccess(ctx, serverModel.ID)
	case summary.successCount == 0:
		_ = r.health.RecordFailure(ctx, serverModel, summary.firstErr)
	default:
		_ = r.health.RecordDegraded(ctx, serverModel.ID, buildIntegrityDegradedMessage(summary.failureCount, summary.tamperedCount, len(active)))
	}
}

// runLogFileWorkers checks log files of one server with a bounded worker pool.
func (r *Runner) runLogFileWorkers(ctx context.Context, serverModel *models.Server, logFiles []*models.LogFile) integritySummary {
	sem := make(chan struct{}, r.options.MaxLogFileWorkersPerHost)
	var wg sync.WaitGroup
	var mu sync.Mutex
	summary := integritySummary{}

	for _, logFile := range logFiles {
		if ctx.Err() != nil {
			break
		}
		sem <- struct{}{}
		wg.Add(1)
		go func(logFile *models.LogFile) {
			defer wg.Done()
			defer func() { <-sem }()
			result, _, err := r.integrity.CheckLogFile(ctx, serverModel, logFile)
			if err != nil {
				mu.Lock()
				if summary.firstErr == nil {
					summary.firstErr = err
				}
				summary.failureCount++
				mu.Unlock()
				r.logger.Error("integrity check", "server", serverModel.Name, "log_file", logFile.Path, "error", err)
				return
			}
			mu.Lock()
			summary.successCount++
			if result != nil && result.Status == models.CheckStatusTampered {
				summary.tamperedCount++
			}
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

// normalizeOptions applies safe defaults for integrity worker settings.
func normalizeOptions(options Options) Options {
	if options.MaxServerWorkers <= 0 {
		options.MaxServerWorkers = 1
	}
	if options.MaxLogFileWorkersPerHost <= 0 {
		options.MaxLogFileWorkersPerHost = 1
	}
	return options
}

// buildIntegrityDegradedMessage summarizes partial integrity issues on a reachable server.
func buildIntegrityDegradedMessage(failureCount, tamperedCount, total int) string {
	switch {
	case failureCount > 0 && tamperedCount > 0:
		return fmt.Sprintf("integrity completed with %d check errors and %d tampered log files out of %d", failureCount, tamperedCount, total)
	case failureCount > 0:
		return fmt.Sprintf("integrity partially failed for %d of %d log files", failureCount, total)
	default:
		return fmt.Sprintf("integrity detected tampering in %d of %d log files", tamperedCount, total)
	}
}

// integritySummary aggregates one integrity cycle outcome for a server.
type integritySummary struct {
	successCount  int
	failureCount  int
	tamperedCount int
	firstErr      error
}
