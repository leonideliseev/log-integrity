// Package logfile contains application operations for discovered log files.
package logfile

import (
	"context"
	"fmt"

	"github.com/lenchik/logmonitor/internal/repository"
	collectservice "github.com/lenchik/logmonitor/internal/service/collector"
	"github.com/lenchik/logmonitor/models"
)

// CollectResult contains collection output for one log file.
type CollectResult struct {
	CollectedEntries int    `json:"collected_entries,omitempty"`
	Error            string `json:"error,omitempty"`
}

// Service provides log file application operations for the HTTP layer.
type Service struct {
	servers   repository.ServerRepository
	logFiles  repository.LogFileRepository
	collector *collectservice.Service
	locker    ServerLocker
}

// ServerLocker provides optional per-server isolation for manual operations.
type ServerLocker interface {
	TryLock(key string) (func(), bool)
}

// NewService creates a log file application service.
func NewService(servers repository.ServerRepository, logFiles repository.LogFileRepository, collector *collectservice.Service) *Service {
	return NewServiceWithLocker(servers, logFiles, collector, nil)
}

// NewServiceWithLocker creates a log file application service with optional server isolation.
func NewServiceWithLocker(servers repository.ServerRepository, logFiles repository.LogFileRepository, collector *collectservice.Service, locker ServerLocker) *Service {
	return &Service{
		servers:   servers,
		logFiles:  logFiles,
		collector: collector,
		locker:    locker,
	}
}

// List returns active log files or the log files of a concrete server.
func (s *Service) List(ctx context.Context, serverID string) ([]*models.LogFile, error) {
	if serverID == "" {
		return s.logFiles.ListActiveLogFiles(ctx)
	}
	return s.logFiles.ListLogFilesByServer(ctx, serverID)
}

// Collect reads remote log files and persists newly discovered entries.
func (s *Service) Collect(ctx context.Context, serverID, logFileID string) (map[string]CollectResult, error) {
	serverModel, err := s.servers.GetServerByID(ctx, serverID)
	if err != nil {
		return nil, err
	}
	unlock, ok := s.tryLockServer(serverModel.ID)
	if !ok {
		return nil, fmt.Errorf("logfile: server %q is busy", serverModel.Name)
	}
	defer unlock()

	logFiles, err := s.resolveLogFiles(ctx, serverID, logFileID)
	if err != nil {
		return nil, err
	}

	result := make(map[string]CollectResult, len(logFiles))
	for _, logFile := range logFiles {
		count, collectErr := s.collector.CollectLogFile(ctx, serverModel, logFile)
		if collectErr != nil {
			result[logFile.ID] = CollectResult{Error: collectErr.Error()}
			continue
		}
		result[logFile.ID] = CollectResult{CollectedEntries: count}
	}

	return result, nil
}

// resolveLogFiles returns either one concrete log file or the full list for a server.
func (s *Service) resolveLogFiles(ctx context.Context, serverID, logFileID string) ([]*models.LogFile, error) {
	if logFileID != "" {
		logFile, err := s.logFiles.GetLogFileByID(ctx, logFileID)
		if err != nil {
			return nil, err
		}
		if logFile.ServerID != serverID {
			return nil, fmt.Errorf("%w: logfile: log file %q does not belong to server %q", repository.ErrConflict, logFileID, serverID)
		}
		return []*models.LogFile{logFile}, nil
	}
	return s.logFiles.ListLogFilesByServer(ctx, serverID)
}

// tryLockServer acquires optional non-blocking isolation for manual collection.
func (s *Service) tryLockServer(serverID string) (func(), bool) {
	if s.locker == nil {
		return func() {}, true
	}
	return s.locker.TryLock("server:" + serverID)
}
