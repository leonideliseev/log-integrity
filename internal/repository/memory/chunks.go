package memory

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/lenchik/logmonitor/internal/repository"
	"github.com/lenchik/logmonitor/models"
)

// CreateLogChunk stores one aggregate chunk hash.
func (s *Storage) CreateLogChunk(_ context.Context, chunk *models.LogChunk) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.createLogChunkLocked(chunk)
}

// CreateLogChunks stores aggregate chunk hashes as one validated batch.
func (s *Storage) CreateLogChunks(_ context.Context, chunks []*models.LogChunk) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, chunk := range chunks {
		if err := s.validateLogChunkLocked(chunk); err != nil {
			return err
		}
	}

	for _, chunk := range chunks {
		if err := s.createLogChunkLocked(chunk); err != nil {
			return err
		}
	}

	return nil
}

// ListLogChunks returns chunks for a log file ordered by chunk number.
func (s *Storage) ListLogChunks(_ context.Context, logFileID string, offset, limit int) ([]*models.LogChunk, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	items := s.listLogChunksLocked(logFileID)
	return paginate(items, offset, limit), nil
}

// GetLatestLogChunk returns the newest chunk for a log file.
func (s *Storage) GetLatestLogChunk(_ context.Context, logFileID string) (*models.LogChunk, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	items := s.listLogChunksLocked(logFileID)
	if len(items) == 0 {
		return nil, fmt.Errorf("%w: latest log chunk for log file %q", repository.ErrNotFound, logFileID)
	}
	return items[len(items)-1], nil
}

// DeleteLogChunksByLogFile removes all chunk hashes linked to one log file.
func (s *Storage) DeleteLogChunksByLogFile(_ context.Context, logFileID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, chunkID := range s.chunksByLogFile[logFileID] {
		delete(s.chunks, chunkID)
	}
	delete(s.chunksByLogFile, logFileID)
	return nil
}

// createLogChunkLocked stores one chunk while assuming the write lock is already held.
func (s *Storage) createLogChunkLocked(chunk *models.LogChunk) error {
	if err := s.validateLogChunkLocked(chunk); err != nil {
		return err
	}
	s.storeLogChunkLocked(chunk)
	return nil
}

// storeLogChunkLocked stores one chunk while assuming validation and write locking are already handled.
func (s *Storage) storeLogChunkLocked(chunk *models.LogChunk) {
	if chunk.ID == "" {
		chunk.ID = newID("chunk")
	}
	if chunk.CreatedAt.IsZero() {
		chunk.CreatedAt = time.Now().UTC()
	}

	s.chunks[chunk.ID] = cloneLogChunk(chunk)
	s.chunksByLogFile[chunk.LogFileID] = append(s.chunksByLogFile[chunk.LogFileID], chunk.ID)
}

// validateLogChunkLocked checks chunk consistency while the write lock is held.
func (s *Storage) validateLogChunkLocked(chunk *models.LogChunk) error {
	if _, ok := s.logFiles[chunk.LogFileID]; !ok {
		return fmt.Errorf("%w: log file %q", repository.ErrNotFound, chunk.LogFileID)
	}
	if _, exists := s.chunks[chunk.ID]; chunk.ID != "" && exists {
		return fmt.Errorf("%w: log chunk %q already exists", repository.ErrConflict, chunk.ID)
	}
	for _, chunkID := range s.chunksByLogFile[chunk.LogFileID] {
		existing := s.chunks[chunkID]
		if existing != nil && existing.ChunkNumber == chunk.ChunkNumber {
			return fmt.Errorf("%w: log chunk number %d already exists", repository.ErrConflict, chunk.ChunkNumber)
		}
	}
	return nil
}

// listLogChunksLocked returns chunk models while the read or write lock is held.
func (s *Storage) listLogChunksLocked(logFileID string) []*models.LogChunk {
	items := make([]*models.LogChunk, 0, len(s.chunksByLogFile[logFileID]))
	for _, chunkID := range s.chunksByLogFile[logFileID] {
		items = append(items, cloneLogChunk(s.chunks[chunkID]))
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].ChunkNumber == items[j].ChunkNumber {
			return items[i].ID < items[j].ID
		}
		return items[i].ChunkNumber < items[j].ChunkNumber
	})
	return items
}
