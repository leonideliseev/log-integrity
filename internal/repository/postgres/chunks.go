package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/lenchik/logmonitor/internal/repository"
	"github.com/lenchik/logmonitor/models"
)

// CreateLogChunk stores one aggregate chunk hash.
func (s *Storage) CreateLogChunk(ctx context.Context, chunk *models.LogChunk) error {
	if chunk.ID == "" {
		chunk.ID = newID("chunk")
	}
	if chunk.CreatedAt.IsZero() {
		chunk.CreatedAt = time.Now().UTC()
	}

	_, err := s.pool.Exec(
		ctx,
		`INSERT INTO log_chunks (id, log_file_id, chunk_number, from_line_number, to_line_number, from_byte_offset, to_byte_offset, entries_count, hash, hash_algorithm, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`,
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
	)
	if err != nil {
		if isForeignKeyViolation(err) {
			return logFileNotFoundError(chunk.LogFileID)
		}
		if isUniqueViolation(err) {
			return conflictError(fmt.Sprintf("log chunk number %d already exists", chunk.ChunkNumber))
		}
		return fmt.Errorf("postgres: create log chunk %q: %w", chunk.ID, err)
	}

	return nil
}

// CreateLogChunks stores aggregate chunk hashes through pgx CopyFrom.
func (s *Storage) CreateLogChunks(ctx context.Context, chunks []*models.LogChunk) error {
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

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("postgres: begin create log chunks transaction: %w", err)
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	_, err = tx.CopyFrom(
		ctx,
		pgx.Identifier{"log_chunks"},
		[]string{"id", "log_file_id", "chunk_number", "from_line_number", "to_line_number", "from_byte_offset", "to_byte_offset", "entries_count", "hash", "hash_algorithm", "created_at"},
		pgx.CopyFromRows(rows),
	)
	if err != nil {
		if isForeignKeyViolation(err) {
			return logFileNotFoundError(chunks[0].LogFileID)
		}
		if isUniqueViolation(err) {
			return conflictError("log chunk number already exists")
		}
		return fmt.Errorf("postgres: copy log chunks: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("postgres: commit create log chunks transaction: %w", err)
	}

	return nil
}

// ListLogChunks returns chunks for a log file ordered by chunk number.
func (s *Storage) ListLogChunks(ctx context.Context, logFileID string, offset, limit int) ([]*models.LogChunk, error) {
	offset = normalizeOffset(offset)

	var rows pgx.Rows
	var err error
	if limit <= 0 {
		rows, err = s.pool.Query(
			ctx,
			`SELECT id, log_file_id, chunk_number, from_line_number, to_line_number, from_byte_offset, to_byte_offset, entries_count, hash, hash_algorithm, created_at
			 FROM log_chunks
			 WHERE log_file_id = $1
			 ORDER BY chunk_number ASC
			 OFFSET $2`,
			logFileID,
			offset,
		)
	} else {
		rows, err = s.pool.Query(
			ctx,
			`SELECT id, log_file_id, chunk_number, from_line_number, to_line_number, from_byte_offset, to_byte_offset, entries_count, hash, hash_algorithm, created_at
			 FROM log_chunks
			 WHERE log_file_id = $1
			 ORDER BY chunk_number ASC
			 OFFSET $2
			 LIMIT $3`,
			logFileID,
			offset,
			limit,
		)
	}
	if err != nil {
		return nil, fmt.Errorf("postgres: list log chunks for %q: %w", logFileID, err)
	}
	defer rows.Close()

	items := make([]*models.LogChunk, 0)
	for rows.Next() {
		var chunk models.LogChunk
		if err := rows.Scan(
			&chunk.ID,
			&chunk.LogFileID,
			&chunk.ChunkNumber,
			&chunk.FromLineNumber,
			&chunk.ToLineNumber,
			&chunk.FromByteOffset,
			&chunk.ToByteOffset,
			&chunk.EntriesCount,
			&chunk.Hash,
			&chunk.HashAlgorithm,
			&chunk.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("postgres: scan log chunk: %w", err)
		}
		items = append(items, cloneLogChunk(&chunk))
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("postgres: iterate log chunks for %q: %w", logFileID, err)
	}

	return items, nil
}

// GetLatestLogChunk returns the newest chunk for a log file.
func (s *Storage) GetLatestLogChunk(ctx context.Context, logFileID string) (*models.LogChunk, error) {
	row := s.pool.QueryRow(
		ctx,
		`SELECT id, log_file_id, chunk_number, from_line_number, to_line_number, from_byte_offset, to_byte_offset, entries_count, hash, hash_algorithm, created_at
		 FROM log_chunks
		 WHERE log_file_id = $1
		 ORDER BY chunk_number DESC
		 LIMIT 1`,
		logFileID,
	)

	var chunk models.LogChunk
	if err := row.Scan(
		&chunk.ID,
		&chunk.LogFileID,
		&chunk.ChunkNumber,
		&chunk.FromLineNumber,
		&chunk.ToLineNumber,
		&chunk.FromByteOffset,
		&chunk.ToByteOffset,
		&chunk.EntriesCount,
		&chunk.Hash,
		&chunk.HashAlgorithm,
		&chunk.CreatedAt,
	); err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("%w: latest log chunk for log file %q", repository.ErrNotFound, logFileID)
		}
		return nil, fmt.Errorf("postgres: get latest log chunk for %q: %w", logFileID, err)
	}

	return cloneLogChunk(&chunk), nil
}

// DeleteLogChunksByLogFile removes all chunk hashes linked to one log file.
func (s *Storage) DeleteLogChunksByLogFile(ctx context.Context, logFileID string) error {
	if _, err := s.pool.Exec(ctx, `DELETE FROM log_chunks WHERE log_file_id = $1`, logFileID); err != nil {
		return fmt.Errorf("postgres: delete log chunks for %q: %w", logFileID, err)
	}
	return nil
}
