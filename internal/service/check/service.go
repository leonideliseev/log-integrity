// Package check contains application operations for integrity check history and execution.
package check

import (
	"context"
	"errors"
	"fmt"

	"github.com/lenchik/logmonitor/internal/repository"
	healthservice "github.com/lenchik/logmonitor/internal/service/health"
	integrityservice "github.com/lenchik/logmonitor/internal/service/integrity"
	"github.com/lenchik/logmonitor/models"
)

// RunResult contains integrity check output for one log file.
type RunResult struct {
	Result          *models.CheckResult    `json:"result,omitempty"`
	TamperedEntries []models.TamperedEntry `json:"tampered_entries,omitempty"`
	Error           string                 `json:"error,omitempty"`
}

// Service provides check-related application operations for the HTTP layer.
type Service struct {
	servers   repository.ServerRepository
	logFiles  repository.LogFileRepository
	checks    repository.CheckResultRepository
	integrity *integrityservice.Service
	health    *healthservice.Service
	locker    ServerLocker
}

// ServerLocker provides optional per-server isolation for manual operations.
type ServerLocker interface {
	TryLock(key string) (func(), bool)
}

// NewService creates a check application service.
func NewService(servers repository.ServerRepository, logFiles repository.LogFileRepository, checks repository.CheckResultRepository, integrity *integrityservice.Service) *Service {
	return NewServiceWithLocker(servers, logFiles, checks, integrity, nil)
}

// NewServiceWithLocker creates a check application service with optional server isolation.
func NewServiceWithLocker(servers repository.ServerRepository, logFiles repository.LogFileRepository, checks repository.CheckResultRepository, integrity *integrityservice.Service, locker ServerLocker) *Service {
	return NewServiceWithHealthAndLocker(servers, logFiles, checks, integrity, healthservice.NewService(servers, healthservice.Options{}), locker)
}

// NewServiceWithHealthAndLocker creates a check service with health tracking and optional isolation.
func NewServiceWithHealthAndLocker(servers repository.ServerRepository, logFiles repository.LogFileRepository, checks repository.CheckResultRepository, integrity *integrityservice.Service, health *healthservice.Service, locker ServerLocker) *Service {
	return &Service{
		servers:   servers,
		logFiles:  logFiles,
		checks:    checks,
		integrity: integrity,
		health:    health,
		locker:    locker,
	}
}

// List returns integrity check history for a log file.
func (s *Service) List(ctx context.Context, logFileID string, offset, limit int) ([]*models.CheckResult, error) {
	return s.checks.ListCheckResults(ctx, logFileID, offset, limit)
}

// ListFiltered returns a filtered check result page for API list screens.
func (s *Service) ListFiltered(ctx context.Context, filter repository.CheckResultListFilter) (repository.Page[*models.CheckResult], error) {
	return s.checks.ListCheckResultsFiltered(ctx, filter)
}

// Run launches integrity checks for one log file or for all server log files.
func (s *Service) Run(ctx context.Context, serverID, logFileID string) (map[string]RunResult, error) {
	serverModel, err := s.servers.GetServerByID(ctx, serverID)
	if err != nil {
		return nil, err
	}
	unlock, ok := s.tryLockServer(serverModel.ID)
	if !ok {
		return nil, fmt.Errorf("check: server %q is busy", serverModel.Name)
	}
	defer unlock()

	logFiles, err := s.resolveLogFiles(ctx, serverID, logFileID)
	if err != nil {
		return nil, err
	}
	if len(logFiles) == 0 {
		return map[string]RunResult{}, nil
	}

	result := make(map[string]RunResult, len(logFiles))
	successCount := 0
	failureCount := 0
	tamperedCount := 0
	var firstErr error
	for _, logFile := range logFiles {
		checkResult, tamperedEntries, checkErr := s.integrity.CheckLogFile(ctx, serverModel, logFile)
		if checkErr != nil {
			if firstErr == nil {
				firstErr = checkErr
			}
			failureCount++
			result[logFile.ID] = RunResult{Error: checkErr.Error()}
			continue
		}
		if checkResult != nil && checkResult.Status == models.CheckStatusError {
			if firstErr == nil {
				firstErr = errors.New(checkResult.ErrorMessage)
			}
			failureCount++
			result[logFile.ID] = RunResult{
				Result: checkResult,
				Error:  checkResult.ErrorMessage,
			}
			continue
		}
		successCount++
		if checkResult != nil && checkResult.Status == models.CheckStatusTampered {
			tamperedCount++
		}
		result[logFile.ID] = RunResult{
			Result:          checkResult,
			TamperedEntries: tamperedEntries,
		}
	}

	switch {
	case failureCount == 0 && tamperedCount == 0:
		_ = s.health.RecordSuccess(ctx, serverModel.ID)
	case successCount == 0:
		_ = s.health.RecordFailure(ctx, serverModel, firstErr)
	default:
		_ = s.health.RecordDegraded(ctx, serverModel.ID, buildIntegrityDegradedMessage(failureCount, tamperedCount, len(logFiles)))
	}

	return result, nil
}

// resolveLogFiles returns either one selected log file or the whole server set.
func (s *Service) resolveLogFiles(ctx context.Context, serverID, logFileID string) ([]*models.LogFile, error) {
	if logFileID != "" {
		logFile, err := s.logFiles.GetLogFileByID(ctx, logFileID)
		if err != nil {
			return nil, err
		}
		if logFile.ServerID != serverID {
			return nil, fmt.Errorf("%w: check: log file %q does not belong to server %q", repository.ErrConflict, logFileID, serverID)
		}
		return []*models.LogFile{logFile}, nil
	}
	return s.logFiles.ListLogFilesByServer(ctx, serverID)
}

// tryLockServer acquires optional non-blocking isolation for manual checks.
func (s *Service) tryLockServer(serverID string) (func(), bool) {
	if s.locker == nil {
		return func() {}, true
	}
	return s.locker.TryLock("server:" + serverID)
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
