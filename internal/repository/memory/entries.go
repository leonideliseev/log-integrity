package memory

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"time"

	"github.com/lenchik/logmonitor/internal/repository"
	"github.com/lenchik/logmonitor/models"
)

// CreateLogEntry stores one log entry.
func (s *Storage) CreateLogEntry(_ context.Context, entry *models.LogEntry) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.createLogEntryLocked(entry)
}

// CreateLogEntries stores a batch of log entries.
func (s *Storage) CreateLogEntries(_ context.Context, entries []*models.LogEntry) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Validate everything first so we do not partially apply the batch.
	for _, entry := range entries {
		if err := s.validateLogEntryLocked(entry); err != nil {
			return err
		}
	}

	for _, entry := range entries {
		if entry.ID == "" {
			entry.ID = newID("entry")
		}
		if entry.CollectedAt.IsZero() {
			entry.CollectedAt = time.Now().UTC()
		}
		s.entries[entry.ID] = cloneLogEntry(entry)
		if s.entriesByLogFile[entry.LogFileID] == nil {
			s.entriesByLogFile[entry.LogFileID] = make(map[int64]string)
		}
		s.entriesByLogFile[entry.LogFileID][entry.LineNumber] = entry.ID
	}

	return nil
}

// GetLogEntryByLine returns an entry by log file and line number.
func (s *Storage) GetLogEntryByLine(_ context.Context, logFileID string, lineNumber int64) (*models.LogEntry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	entryID, ok := s.entriesByLogFile[logFileID][lineNumber]
	if !ok {
		return nil, fmt.Errorf("%w: log entry %q line %d", repository.ErrNotFound, logFileID, lineNumber)
	}

	return cloneLogEntry(s.entries[entryID]), nil
}

// ListLogEntries returns entries for a log file with offset and limit.
func (s *Storage) ListLogEntries(_ context.Context, logFileID string, offset, limit int) ([]*models.LogEntry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	items := s.listLogEntriesLocked(logFileID)
	return paginate(items, offset, limit), nil
}

// ListLogEntriesFiltered returns a filtered and paginated log entry page.
func (s *Storage) ListLogEntriesFiltered(_ context.Context, filter repository.LogEntryListFilter) (repository.Page[*models.LogEntry], error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	items := make([]*models.LogEntry, 0)
	for _, item := range s.entries {
		if filter.LogFileID != "" && item.LogFileID != filter.LogFileID {
			continue
		}
		if filter.FromLine > 0 && item.LineNumber < filter.FromLine {
			continue
		}
		if filter.ToLine > 0 && item.LineNumber > filter.ToLine {
			continue
		}
		if !matchesSearch(filter.Q, item.ID, item.LogFileID, item.Content, item.Hash, strconv.FormatInt(item.LineNumber, 10)) {
			continue
		}
		items = append(items, cloneLogEntry(item))
	}

	sort.Slice(items, func(i, j int) bool {
		less := compareLogEntries(items[i], items[j], filter.Sort)
		if ascending(filter.Order) {
			return less
		}
		return compareLogEntries(items[j], items[i], filter.Sort)
	})

	return paged(items, filter.Offset, filter.Limit), nil
}

func compareLogEntries(left, right *models.LogEntry, sortBy string) bool {
	switch sortBy {
	case "collected_at":
		return left.CollectedAt.Before(right.CollectedAt)
	default:
		if left.LineNumber == right.LineNumber {
			return left.ID < right.ID
		}
		return left.LineNumber < right.LineNumber
	}
}

// ListLogEntriesByLineRange returns entries whose line numbers fit the inclusive range.
func (s *Storage) ListLogEntriesByLineRange(_ context.Context, logFileID string, fromLine, toLine int64) ([]*models.LogEntry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	items := make([]*models.LogEntry, 0)
	for _, entry := range s.listLogEntriesLocked(logFileID) {
		if entry.LineNumber >= fromLine && entry.LineNumber <= toLine {
			items = append(items, entry)
		}
	}
	return items, nil
}

// GetMaxLineNumber returns the highest stored line number for a log file.
func (s *Storage) GetMaxLineNumber(_ context.Context, logFileID string) (int64, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var maxLine int64
	for lineNumber := range s.entriesByLogFile[logFileID] {
		if lineNumber > maxLine {
			maxLine = lineNumber
		}
	}
	return maxLine, nil
}

// CountLogEntries returns the number of stored entries for a log file.
func (s *Storage) CountLogEntries(_ context.Context, logFileID string) (int64, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return int64(len(s.entriesByLogFile[logFileID])), nil
}

// DeleteLogEntriesByLogFile removes all entries linked to one log file.
func (s *Storage) DeleteLogEntriesByLogFile(_ context.Context, logFileID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, entryID := range s.entriesByLineIDs(logFileID) {
		delete(s.entries, entryID)
	}
	delete(s.entriesByLogFile, logFileID)
	return nil
}

// createLogEntryLocked stores one entry while assuming the write lock is already held.
func (s *Storage) createLogEntryLocked(entry *models.LogEntry) error {
	if err := s.validateLogEntryLocked(entry); err != nil {
		return err
	}
	s.storeLogEntryLocked(entry)
	return nil
}

// storeLogEntryLocked stores one entry while assuming validation and write locking are already handled.
func (s *Storage) storeLogEntryLocked(entry *models.LogEntry) {
	if entry.ID == "" {
		entry.ID = newID("entry")
	}
	if entry.CollectedAt.IsZero() {
		entry.CollectedAt = time.Now().UTC()
	}

	s.entries[entry.ID] = cloneLogEntry(entry)
	if s.entriesByLogFile[entry.LogFileID] == nil {
		s.entriesByLogFile[entry.LogFileID] = make(map[int64]string)
	}
	s.entriesByLogFile[entry.LogFileID][entry.LineNumber] = entry.ID
}

// validateLogEntryLocked checks entry consistency while the write lock is held.
func (s *Storage) validateLogEntryLocked(entry *models.LogEntry) error {
	if _, ok := s.logFiles[entry.LogFileID]; !ok {
		return fmt.Errorf("%w: log file %q", repository.ErrNotFound, entry.LogFileID)
	}
	if _, exists := s.entries[entry.ID]; entry.ID != "" && exists {
		return fmt.Errorf("%w: log entry %q already exists", repository.ErrConflict, entry.ID)
	}
	if existingID, exists := s.entriesByLogFile[entry.LogFileID][entry.LineNumber]; exists {
		return fmt.Errorf("%w: log entry line %d already exists as %q", repository.ErrConflict, entry.LineNumber, existingID)
	}
	return nil
}

// entriesByLineIDs returns entry identifiers for a log file.
func (s *Storage) entriesByLineIDs(logFileID string) []string {
	byLine := s.entriesByLogFile[logFileID]
	ids := make([]string, 0, len(byLine))
	for _, id := range byLine {
		ids = append(ids, id)
	}
	return ids
}

// listLogEntriesLocked returns ordered entry models while the read or write lock is held.
func (s *Storage) listLogEntriesLocked(logFileID string) []*models.LogEntry {
	items := make([]*models.LogEntry, 0, len(s.entriesByLogFile[logFileID]))
	for _, entryID := range s.entriesByLogFile[logFileID] {
		items = append(items, cloneLogEntry(s.entries[entryID]))
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].LineNumber < items[j].LineNumber
	})
	return items
}
