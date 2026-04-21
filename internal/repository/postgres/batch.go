package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/lenchik/logmonitor/models"
)

// CreateLogEntriesWithChunks stores entries and chunks in one database transaction.
func (s *Storage) CreateLogEntriesWithChunks(ctx context.Context, entries []*models.LogEntry, chunks []*models.LogChunk) error {
	if len(entries) == 0 && len(chunks) == 0 {
		return nil
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("postgres: begin create log batch transaction: %w", err)
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	if err := copyLogEntries(ctx, tx, entries); err != nil {
		if len(entries) > 0 && isForeignKeyViolation(err) {
			return logFileNotFoundError(entries[0].LogFileID)
		}
		if isUniqueViolation(err) {
			return conflictError("log entry line already exists")
		}
		return fmt.Errorf("postgres: copy log entries: %w", err)
	}

	if err := copyLogChunks(ctx, tx, chunks); err != nil {
		if len(chunks) > 0 && isForeignKeyViolation(err) {
			return logFileNotFoundError(chunks[0].LogFileID)
		}
		if isUniqueViolation(err) {
			return conflictError("log chunk number already exists")
		}
		return fmt.Errorf("postgres: copy log chunks: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("postgres: commit create log batch transaction: %w", err)
	}

	return nil
}

// copyLogEntries bulk inserts entries into the current transaction.
func copyLogEntries(ctx context.Context, tx pgx.Tx, entries []*models.LogEntry) error {
	if len(entries) == 0 {
		return nil
	}

	now := time.Now().UTC()
	rows := make([][]any, 0, len(entries))
	for _, entry := range entries {
		if entry.ID == "" {
			entry.ID = newID("entry")
		}
		if entry.CollectedAt.IsZero() {
			entry.CollectedAt = now
		}

		rows = append(rows, []any{
			entry.ID,
			entry.LogFileID,
			entry.LineNumber,
			entry.Content,
			entry.Hash,
			entry.CollectedAt,
		})
	}

	_, err := tx.CopyFrom(
		ctx,
		pgx.Identifier{"log_entries"},
		[]string{"id", "log_file_id", "line_number", "content", "hash", "collected_at"},
		pgx.CopyFromRows(rows),
	)
	return err
}

// copyLogChunks bulk inserts aggregate chunks into the current transaction.
func copyLogChunks(ctx context.Context, tx pgx.Tx, chunks []*models.LogChunk) error {
	if len(chunks) == 0 {
		return nil
	}

	now := time.Now().UTC()
	rows := make([][]any, 0, len(chunks))
	for _, chunk := range chunks {
		if chunk.ID == "" {
			chunk.ID = newID("chunk")
		}
		if chunk.CreatedAt.IsZero() {
			chunk.CreatedAt = now
		}
		rows = append(rows, []any{
			chunk.ID,
			chunk.LogFileID,
			chunk.ChunkNumber,
			chunk.FromLineNumber,
			chunk.ToLineNumber,
			chunk.FromByteOffset,
			chunk.ToByteOffset,
			chunk.EntriesCount,
			chunk.Hash,
			chunk.HashAlgorithm,
			chunk.CreatedAt,
		})
	}

	_, err := tx.CopyFrom(
		ctx,
		pgx.Identifier{"log_chunks"},
		[]string{"id", "log_file_id", "chunk_number", "from_line_number", "to_line_number", "from_byte_offset", "to_byte_offset", "entries_count", "hash", "hash_algorithm", "created_at"},
		pgx.CopyFromRows(rows),
	)
	return err
}
