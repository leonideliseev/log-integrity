package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/lenchik/logmonitor/internal/repository"
	"github.com/lenchik/logmonitor/models"
)

// CreateLogEntry stores one log entry.
func (s *Storage) CreateLogEntry(ctx context.Context, entry *models.LogEntry) error {
	if entry.ID == "" {
		entry.ID = newID("entry")
	}
	if entry.CollectedAt.IsZero() {
		entry.CollectedAt = time.Now().UTC()
	}

	_, err := s.pool.Exec(
		ctx,
		`INSERT INTO log_entries (id, log_file_id, line_number, content, hash, collected_at)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		entry.ID,
		entry.LogFileID,
		entry.LineNumber,
		entry.Content,
		entry.Hash,
		entry.CollectedAt,
	)
	if err != nil {
		if isForeignKeyViolation(err) {
			return logFileNotFoundError(entry.LogFileID)
		}
		if isUniqueViolation(err) {
			return conflictError(fmt.Sprintf("log entry line %d already exists", entry.LineNumber))
		}
		return fmt.Errorf("postgres: create log entry %q: %w", entry.ID, err)
	}

	return nil
}

// CreateLogEntries stores a batch of log entries.
func (s *Storage) CreateLogEntries(ctx context.Context, entries []*models.LogEntry) error {
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

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("postgres: begin create log entries transaction: %w", err)
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	_, err = tx.CopyFrom(
		ctx,
		pgx.Identifier{"log_entries"},
		[]string{"id", "log_file_id", "line_number", "content", "hash", "collected_at"},
		pgx.CopyFromRows(rows),
	)
	if err != nil {
		if isForeignKeyViolation(err) {
			return logFileNotFoundError(entries[0].LogFileID)
		}
		if isUniqueViolation(err) {
			return conflictError("log entry line already exists")
		}
		return fmt.Errorf("postgres: copy log entries: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("postgres: commit create log entries transaction: %w", err)
	}

	return nil
}

// GetLogEntryByLine returns an entry by log file and line number.
func (s *Storage) GetLogEntryByLine(ctx context.Context, logFileID string, lineNumber int64) (*models.LogEntry, error) {
	row := s.pool.QueryRow(
		ctx,
		`SELECT id, log_file_id, line_number, content, hash, collected_at
		 FROM log_entries
		 WHERE log_file_id = $1 AND line_number = $2`,
		logFileID,
		lineNumber,
	)

	var entry models.LogEntry
	if err := row.Scan(
		&entry.ID,
		&entry.LogFileID,
		&entry.LineNumber,
		&entry.Content,
		&entry.Hash,
		&entry.CollectedAt,
	); err != nil {
		if err == pgx.ErrNoRows {
			return nil, logEntryNotFoundError(logFileID, lineNumber)
		}
		return nil, fmt.Errorf("postgres: get log entry %q line %d: %w", logFileID, lineNumber, err)
	}

	return cloneLogEntry(&entry), nil
}

// ListLogEntries returns entries for a log file with offset and limit.
func (s *Storage) ListLogEntries(ctx context.Context, logFileID string, offset, limit int) ([]*models.LogEntry, error) {
	offset = normalizeOffset(offset)

	var rows pgx.Rows
	var err error
	if limit <= 0 {
		rows, err = s.pool.Query(
			ctx,
			`SELECT id, log_file_id, line_number, content, hash, collected_at
			 FROM log_entries
			 WHERE log_file_id = $1
			 ORDER BY line_number ASC
			 OFFSET $2`,
			logFileID,
			offset,
		)
	} else {
		rows, err = s.pool.Query(
			ctx,
			`SELECT id, log_file_id, line_number, content, hash, collected_at
			 FROM log_entries
			 WHERE log_file_id = $1
			 ORDER BY line_number ASC
			 OFFSET $2
			 LIMIT $3`,
			logFileID,
			offset,
			limit,
		)
	}
	if err != nil {
		return nil, fmt.Errorf("postgres: list log entries for %q: %w", logFileID, err)
	}
	defer rows.Close()

	items := make([]*models.LogEntry, 0)
	for rows.Next() {
		var entry models.LogEntry
		if err := rows.Scan(
			&entry.ID,
			&entry.LogFileID,
			&entry.LineNumber,
			&entry.Content,
			&entry.Hash,
			&entry.CollectedAt,
		); err != nil {
			return nil, fmt.Errorf("postgres: scan log entry: %w", err)
		}
		items = append(items, cloneLogEntry(&entry))
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("postgres: iterate log entries for %q: %w", logFileID, err)
	}

	return items, nil
}

// ListLogEntriesFiltered returns a filtered and paginated log entry page.
func (s *Storage) ListLogEntriesFiltered(ctx context.Context, filter repository.LogEntryListFilter) (repository.Page[*models.LogEntry], error) {
	filter.ListOptions = normalizePage(filter.ListOptions)

	sqlFilter := sqlFilter{}
	if filter.LogFileID != "" {
		sqlFilter.add("log_file_id = $%d", filter.LogFileID)
	}
	if filter.FromLine > 0 {
		sqlFilter.add("line_number >= $%d", filter.FromLine)
	}
	if filter.ToLine > 0 {
		sqlFilter.add("line_number <= $%d", filter.ToLine)
	}
	sqlFilter.addSearch([]string{"id", "log_file_id", "content", "hash", "line_number::text"}, filter.Q)

	var total int
	if err := s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM log_entries`+sqlFilter.whereSQL(), sqlFilter.args...).Scan(&total); err != nil {
		return repository.Page[*models.LogEntry]{}, fmt.Errorf("postgres: count filtered log entries: %w", err)
	}

	args := append([]any{}, sqlFilter.args...)
	args = append(args, filter.Offset, filter.Limit)
	orderSQL := orderBy(filter.Sort, map[string]string{
		"line_number":  "line_number",
		"collected_at": "collected_at",
	}, "line_number", filter.Order, "ASC")
	rows, err := s.pool.Query(
		ctx,
		`SELECT id, log_file_id, line_number, content, hash, collected_at
		 FROM log_entries`+sqlFilter.whereSQL()+orderSQL+fmt.Sprintf(", id ASC OFFSET $%d LIMIT $%d", len(args)-1, len(args)),
		args...,
	)
	if err != nil {
		return repository.Page[*models.LogEntry]{}, fmt.Errorf("postgres: list filtered log entries: %w", err)
	}
	defer rows.Close()

	items := make([]*models.LogEntry, 0)
	for rows.Next() {
		var entry models.LogEntry
		if err := rows.Scan(&entry.ID, &entry.LogFileID, &entry.LineNumber, &entry.Content, &entry.Hash, &entry.CollectedAt); err != nil {
			return repository.Page[*models.LogEntry]{}, fmt.Errorf("postgres: scan filtered log entry: %w", err)
		}
		items = append(items, cloneLogEntry(&entry))
	}
	if err := rows.Err(); err != nil {
		return repository.Page[*models.LogEntry]{}, fmt.Errorf("postgres: iterate filtered log entries: %w", err)
	}

	return repository.Page[*models.LogEntry]{Items: items, Total: total, Offset: filter.Offset, Limit: filter.Limit}, nil
}

// ListLogEntriesByLineRange returns entries whose line numbers fit the inclusive range.
func (s *Storage) ListLogEntriesByLineRange(ctx context.Context, logFileID string, fromLine, toLine int64) ([]*models.LogEntry, error) {
	rows, err := s.pool.Query(
		ctx,
		`SELECT id, log_file_id, line_number, content, hash, collected_at
		 FROM log_entries
		 WHERE log_file_id = $1 AND line_number >= $2 AND line_number <= $3
		 ORDER BY line_number ASC`,
		logFileID,
		fromLine,
		toLine,
	)
	if err != nil {
		return nil, fmt.Errorf("postgres: list log entries for %q range %d-%d: %w", logFileID, fromLine, toLine, err)
	}
	defer rows.Close()

	items := make([]*models.LogEntry, 0)
	for rows.Next() {
		var entry models.LogEntry
		if err := rows.Scan(
			&entry.ID,
			&entry.LogFileID,
			&entry.LineNumber,
			&entry.Content,
			&entry.Hash,
			&entry.CollectedAt,
		); err != nil {
			return nil, fmt.Errorf("postgres: scan log entry range: %w", err)
		}
		items = append(items, cloneLogEntry(&entry))
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("postgres: iterate log entries for %q range %d-%d: %w", logFileID, fromLine, toLine, err)
	}

	return items, nil
}

// GetMaxLineNumber returns the highest stored line number for a log file.
func (s *Storage) GetMaxLineNumber(ctx context.Context, logFileID string) (int64, error) {
	var maxLineNumber int64
	if err := s.pool.QueryRow(
		ctx,
		`SELECT COALESCE(MAX(line_number), 0)
		 FROM log_entries
		 WHERE log_file_id = $1`,
		logFileID,
	).Scan(&maxLineNumber); err != nil {
		return 0, fmt.Errorf("postgres: get max line number for %q: %w", logFileID, err)
	}

	return maxLineNumber, nil
}

// CountLogEntries returns the number of stored entries for a log file.
func (s *Storage) CountLogEntries(ctx context.Context, logFileID string) (int64, error) {
	var count int64
	if err := s.pool.QueryRow(
		ctx,
		`SELECT COUNT(*)
		 FROM log_entries
		 WHERE log_file_id = $1`,
		logFileID,
	).Scan(&count); err != nil {
		return 0, fmt.Errorf("postgres: count log entries for %q: %w", logFileID, err)
	}

	return count, nil
}

// DeleteLogEntriesByLogFile removes all entries linked to one log file.
func (s *Storage) DeleteLogEntriesByLogFile(ctx context.Context, logFileID string) error {
	if _, err := s.pool.Exec(ctx, `DELETE FROM log_entries WHERE log_file_id = $1`, logFileID); err != nil {
		return fmt.Errorf("postgres: delete log entries for %q: %w", logFileID, err)
	}
	return nil
}
