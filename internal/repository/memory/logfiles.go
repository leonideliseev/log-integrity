package memory

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/lenchik/logmonitor/internal/repository"
	"github.com/lenchik/logmonitor/models"
)

// CreateLogFile saves a new log file for a given server.
func (s *Storage) CreateLogFile(_ context.Context, logFile *models.LogFile) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.servers[logFile.ServerID]; !ok {
		return fmt.Errorf("%w: server %q", repository.ErrNotFound, logFile.ServerID)
	}
	if logFile.ID == "" {
		logFile.ID = newID("log")
	}
	if _, exists := s.logFiles[logFile.ID]; exists {
		return fmt.Errorf("%w: log file %q already exists", repository.ErrConflict, logFile.ID)
	}

	byPath := s.logFilesByServer[logFile.ServerID]
	if byPath == nil {
		byPath = make(map[string]string)
		s.logFilesByServer[logFile.ServerID] = byPath
	}
	if existingID, exists := byPath[logFile.Path]; exists {
		return fmt.Errorf("%w: log file path %q already exists as %q", repository.ErrConflict, logFile.Path, existingID)
	}

	if logFile.CreatedAt.IsZero() {
		logFile.CreatedAt = time.Now().UTC()
	}

	s.logFiles[logFile.ID] = cloneLogFile(logFile)
	byPath[logFile.Path] = logFile.ID
	return nil
}

// GetLogFileByID returns a log file by its identifier.
func (s *Storage) GetLogFileByID(_ context.Context, id string) (*models.LogFile, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	logFile, ok := s.logFiles[id]
	if !ok {
		return nil, fmt.Errorf("%w: log file %q", repository.ErrNotFound, id)
	}

	return cloneLogFile(logFile), nil
}

// ListLogFilesByServer returns all log files bound to a server.
func (s *Storage) ListLogFilesByServer(_ context.Context, serverID string) ([]*models.LogFile, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var items []*models.LogFile
	for _, item := range s.logFiles {
		if item.ServerID == serverID {
			items = append(items, cloneLogFile(item))
		}
	}

	sort.Slice(items, func(i, j int) bool {
		if items[i].Path == items[j].Path {
			return items[i].ID < items[j].ID
		}
		return items[i].Path < items[j].Path
	})

	return items, nil
}

// ListActiveLogFiles returns only active log files.
func (s *Storage) ListActiveLogFiles(_ context.Context) ([]*models.LogFile, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var items []*models.LogFile
	for _, item := range s.logFiles {
		if item.IsActive {
			items = append(items, cloneLogFile(item))
		}
	}

	sort.Slice(items, func(i, j int) bool {
		if items[i].ServerID == items[j].ServerID {
			return items[i].Path < items[j].Path
		}
		return items[i].ServerID < items[j].ServerID
	})

	return items, nil
}

// ListLogFilesFiltered returns a filtered and paginated log file page.
func (s *Storage) ListLogFilesFiltered(_ context.Context, filter repository.LogFileListFilter) (repository.Page[*models.LogFile], error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	items := make([]*models.LogFile, 0, len(s.logFiles))
	for _, item := range s.logFiles {
		if filter.ServerID != "" && item.ServerID != filter.ServerID {
			continue
		}
		if filter.Active != nil && item.IsActive != *filter.Active {
			continue
		}
		if filter.LogType != "" && item.LogType != filter.LogType {
			continue
		}
		if !matchesSearch(filter.Q, item.ID, item.ServerID, item.Path, string(item.LogType)) {
			continue
		}
		items = append(items, cloneLogFile(item))
	}

	sort.Slice(items, func(i, j int) bool {
		less := compareLogFiles(items[i], items[j], filter.Sort)
		if ascending(filter.Order) {
			return less
		}
		return compareLogFiles(items[j], items[i], filter.Sort)
	})

	return paged(items, filter.Offset, filter.Limit), nil
}

func compareLogFiles(left, right *models.LogFile, sortBy string) bool {
	switch sortBy {
	case "last_scanned":
		return timeValue(left.LastScannedAt).Before(timeValue(right.LastScannedAt))
	case "last_line":
		if left.LastLineNumber == right.LastLineNumber {
			return left.Path < right.Path
		}
		return left.LastLineNumber < right.LastLineNumber
	case "created":
		return left.CreatedAt.Before(right.CreatedAt)
	default:
		if left.Path == right.Path {
			return left.ID < right.ID
		}
		return left.Path < right.Path
	}
}

// UpdateLogFile overwrites an existing log file model.
func (s *Storage) UpdateLogFile(_ context.Context, logFile *models.LogFile) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	current, ok := s.logFiles[logFile.ID]
	if !ok {
		return fmt.Errorf("%w: log file %q", repository.ErrNotFound, logFile.ID)
	}

	if current.ServerID != logFile.ServerID {
		return fmt.Errorf("%w: log file %q server id cannot change", repository.ErrConflict, logFile.ID)
	}

	byPath := s.logFilesByServer[logFile.ServerID]
	if byPath == nil {
		byPath = make(map[string]string)
		s.logFilesByServer[logFile.ServerID] = byPath
	}

	if current.Path != logFile.Path {
		if existingID, exists := byPath[logFile.Path]; exists && existingID != logFile.ID {
			return fmt.Errorf("%w: log file path %q already exists as %q", repository.ErrConflict, logFile.Path, existingID)
		}
		delete(byPath, current.Path)
		byPath[logFile.Path] = logFile.ID
	}

	s.logFiles[logFile.ID] = cloneLogFile(logFile)
	return nil
}

// DeleteLogFile removes a log file and all data linked to it.
func (s *Storage) DeleteLogFile(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.logFiles[id]; !ok {
		return fmt.Errorf("%w: log file %q", repository.ErrNotFound, id)
	}

	s.deleteLogFileLocked(id)
	return nil
}

// UpdateLastScanned stores the last scan timestamp for a log file.
func (s *Storage) UpdateLastScanned(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	logFile, ok := s.logFiles[id]
	if !ok {
		return fmt.Errorf("%w: log file %q", repository.ErrNotFound, id)
	}

	now := time.Now().UTC()
	logFile.LastScannedAt = &now
	return nil
}

// deleteLogFileLocked performs a cascading delete while the write lock is held.
func (s *Storage) deleteLogFileLocked(id string) {
	logFile := s.logFiles[id]
	if logFile == nil {
		return
	}

	for _, entryID := range s.entriesByLineIDs(id) {
		delete(s.entries, entryID)
	}
	delete(s.entriesByLogFile, id)

	for _, chunkID := range s.chunksByLogFile[id] {
		delete(s.chunks, chunkID)
	}
	delete(s.chunksByLogFile, id)

	for _, checkID := range s.checksByLogFile[id] {
		delete(s.checks, checkID)
	}
	delete(s.checksByLogFile, id)

	if byPath := s.logFilesByServer[logFile.ServerID]; byPath != nil {
		delete(byPath, logFile.Path)
		if len(byPath) == 0 {
			delete(s.logFilesByServer, logFile.ServerID)
		}
	}

	delete(s.logFiles, id)
}
