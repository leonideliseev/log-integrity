package memory

import (
	"context"
	"fmt"

	"github.com/lenchik/logmonitor/internal/repository"
	"github.com/lenchik/logmonitor/models"
)

type entryLineKey struct {
	logFileID  string
	lineNumber int64
}

type chunkNumberKey struct {
	logFileID   string
	chunkNumber int64
}

// CreateLogEntriesWithChunks stores entries and chunks atomically in memory.
func (s *Storage) CreateLogEntriesWithChunks(_ context.Context, entries []*models.LogEntry, chunks []*models.LogChunk) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.validateLogEntryBatchLocked(entries); err != nil {
		return err
	}
	if err := s.validateLogChunkBatchLocked(chunks); err != nil {
		return err
	}

	for _, entry := range entries {
		s.storeLogEntryLocked(entry)
	}
	for _, chunk := range chunks {
		s.storeLogChunkLocked(chunk)
	}

	return nil
}

// validateLogEntryBatchLocked checks entries without partially applying the batch.
func (s *Storage) validateLogEntryBatchLocked(entries []*models.LogEntry) error {
	ids := make(map[string]struct{}, len(entries))
	lines := make(map[entryLineKey]struct{}, len(entries))

	for _, entry := range entries {
		if entry == nil {
			return fmt.Errorf("%w: log entry is nil", repository.ErrConflict)
		}
		if err := s.validateLogEntryLocked(entry); err != nil {
			return err
		}
		if entry.ID != "" {
			if _, exists := ids[entry.ID]; exists {
				return fmt.Errorf("%w: log entry %q is duplicated in batch", repository.ErrConflict, entry.ID)
			}
			ids[entry.ID] = struct{}{}
		}

		key := entryLineKey{logFileID: entry.LogFileID, lineNumber: entry.LineNumber}
		if _, exists := lines[key]; exists {
			return fmt.Errorf("%w: log entry line %d is duplicated in batch", repository.ErrConflict, entry.LineNumber)
		}
		lines[key] = struct{}{}
	}

	return nil
}

// validateLogChunkBatchLocked checks chunks without partially applying the batch.
func (s *Storage) validateLogChunkBatchLocked(chunks []*models.LogChunk) error {
	ids := make(map[string]struct{}, len(chunks))
	numbers := make(map[chunkNumberKey]struct{}, len(chunks))

	for _, chunk := range chunks {
		if chunk == nil {
			return fmt.Errorf("%w: log chunk is nil", repository.ErrConflict)
		}
		if err := s.validateLogChunkLocked(chunk); err != nil {
			return err
		}
		if chunk.ID != "" {
			if _, exists := ids[chunk.ID]; exists {
				return fmt.Errorf("%w: log chunk %q is duplicated in batch", repository.ErrConflict, chunk.ID)
			}
			ids[chunk.ID] = struct{}{}
		}

		key := chunkNumberKey{logFileID: chunk.LogFileID, chunkNumber: chunk.ChunkNumber}
		if _, exists := numbers[key]; exists {
			return fmt.Errorf("%w: log chunk number %d is duplicated in batch", repository.ErrConflict, chunk.ChunkNumber)
		}
		numbers[key] = struct{}{}
	}

	return nil
}
