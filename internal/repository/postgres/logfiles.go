package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/lenchik/logmonitor/internal/repository"
	"github.com/lenchik/logmonitor/models"
)

// CreateLogFile stores a new log file for a given server.
func (s *Storage) CreateLogFile(ctx context.Context, logFile *models.LogFile) error {
	if logFile.ID == "" {
		logFile.ID = newID("log")
	}
	if logFile.CreatedAt.IsZero() {
		logFile.CreatedAt = time.Now().UTC()
	}
	if logFile.Meta == nil {
		logFile.Meta = map[string]string{}
	}

	fileIdentity, err := encodeJSON(logFile.FileIdentity)
	if err != nil {
		return err
	}
	meta, err := encodeJSON(logFile.Meta)
	if err != nil {
		return err
	}

	_, err = s.pool.Exec(
		ctx,
		`INSERT INTO log_files (id, server_id, path, log_type, file_identity, meta, last_scanned_at, last_line_number, last_byte_offset, is_active, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`,
		logFile.ID,
		logFile.ServerID,
		logFile.Path,
		string(logFile.LogType),
		string(fileIdentity),
		string(meta),
		logFile.LastScannedAt,
		logFile.LastLineNumber,
		logFile.LastByteOffset,
		logFile.IsActive,
		logFile.CreatedAt,
	)
	if err != nil {
		if isForeignKeyViolation(err) {
			return serverNotFoundError(logFile.ServerID)
		}
		if isUniqueViolation(err) {
			return conflictError(fmt.Sprintf("log file path %q already exists", logFile.Path))
		}
		return fmt.Errorf("postgres: create log file %q: %w", logFile.ID, err)
	}

	return nil
}

// GetLogFileByID returns a log file by its identifier.
func (s *Storage) GetLogFileByID(ctx context.Context, id string) (*models.LogFile, error) {
	row := s.pool.QueryRow(
		ctx,
		`SELECT id, server_id, path, log_type, file_identity, meta, last_scanned_at, last_line_number, last_byte_offset, is_active, created_at
		 FROM log_files
		 WHERE id = $1`,
		id,
	)

	var logFile models.LogFile
	var logType string
	var fileIdentity []byte
	var meta []byte
	var lastScannedAt pgtype.Timestamptz
	if err := row.Scan(
		&logFile.ID,
		&logFile.ServerID,
		&logFile.Path,
		&logType,
		&fileIdentity,
		&meta,
		&lastScannedAt,
		&logFile.LastLineNumber,
		&logFile.LastByteOffset,
		&logFile.IsActive,
		&logFile.CreatedAt,
	); err != nil {
		if err == pgx.ErrNoRows {
			return nil, logFileNotFoundError(id)
		}
		return nil, fmt.Errorf("postgres: get log file %q: %w", id, err)
	}

	logFile.LogType = models.LogType(logType)
	logFile.LastScannedAt = nullableTime(lastScannedAt)
	if err := decodeJSON(fileIdentity, &logFile.FileIdentity); err != nil {
		return nil, err
	}
	if err := decodeJSON(meta, &logFile.Meta); err != nil {
		return nil, err
	}
	return cloneLogFile(&logFile), nil
}

// ListLogFilesByServer returns all log files bound to a server.
func (s *Storage) ListLogFilesByServer(ctx context.Context, serverID string) ([]*models.LogFile, error) {
	rows, err := s.pool.Query(
		ctx,
		`SELECT id, server_id, path, log_type, file_identity, meta, last_scanned_at, last_line_number, last_byte_offset, is_active, created_at
		 FROM log_files
		 WHERE server_id = $1
		 ORDER BY path ASC, id ASC`,
		serverID,
	)
	if err != nil {
		return nil, fmt.Errorf("postgres: list log files by server %q: %w", serverID, err)
	}
	defer rows.Close()

	items := make([]*models.LogFile, 0)
	for rows.Next() {
		var logFile models.LogFile
		var logType string
		var fileIdentity []byte
		var meta []byte
		var lastScannedAt pgtype.Timestamptz
		if err := rows.Scan(
			&logFile.ID,
			&logFile.ServerID,
			&logFile.Path,
			&logType,
			&fileIdentity,
			&meta,
			&lastScannedAt,
			&logFile.LastLineNumber,
			&logFile.LastByteOffset,
			&logFile.IsActive,
			&logFile.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("postgres: scan log file: %w", err)
		}

		logFile.LogType = models.LogType(logType)
		logFile.LastScannedAt = nullableTime(lastScannedAt)
		if err := decodeJSON(fileIdentity, &logFile.FileIdentity); err != nil {
			return nil, err
		}
		if err := decodeJSON(meta, &logFile.Meta); err != nil {
			return nil, err
		}
		items = append(items, cloneLogFile(&logFile))
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("postgres: iterate log files by server %q: %w", serverID, err)
	}

	return items, nil
}

// ListActiveLogFiles returns only active log files.
func (s *Storage) ListActiveLogFiles(ctx context.Context) ([]*models.LogFile, error) {
	rows, err := s.pool.Query(
		ctx,
		`SELECT id, server_id, path, log_type, file_identity, meta, last_scanned_at, last_line_number, last_byte_offset, is_active, created_at
		 FROM log_files
		 WHERE is_active = TRUE
		 ORDER BY server_id ASC, path ASC`,
	)
	if err != nil {
		return nil, fmt.Errorf("postgres: list active log files: %w", err)
	}
	defer rows.Close()

	items := make([]*models.LogFile, 0)
	for rows.Next() {
		var logFile models.LogFile
		var logType string
		var fileIdentity []byte
		var meta []byte
		var lastScannedAt pgtype.Timestamptz
		if err := rows.Scan(
			&logFile.ID,
			&logFile.ServerID,
			&logFile.Path,
			&logType,
			&fileIdentity,
			&meta,
			&lastScannedAt,
			&logFile.LastLineNumber,
			&logFile.LastByteOffset,
			&logFile.IsActive,
			&logFile.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("postgres: scan active log file: %w", err)
		}

		logFile.LogType = models.LogType(logType)
		logFile.LastScannedAt = nullableTime(lastScannedAt)
		if err := decodeJSON(fileIdentity, &logFile.FileIdentity); err != nil {
			return nil, err
		}
		if err := decodeJSON(meta, &logFile.Meta); err != nil {
			return nil, err
		}
		items = append(items, cloneLogFile(&logFile))
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("postgres: iterate active log files: %w", err)
	}

	return items, nil
}

// ListLogFilesFiltered returns a filtered and paginated log file page.
func (s *Storage) ListLogFilesFiltered(ctx context.Context, filter repository.LogFileListFilter) (repository.Page[*models.LogFile], error) {
	filter.ListOptions = normalizePage(filter.ListOptions)

	sqlFilter := sqlFilter{}
	if filter.ServerID != "" {
		sqlFilter.add("server_id = $%d", filter.ServerID)
	}
	if filter.Active != nil {
		sqlFilter.add("is_active = $%d", *filter.Active)
	}
	if filter.LogType != "" {
		sqlFilter.add("log_type = $%d", string(filter.LogType))
	}
	sqlFilter.addSearch([]string{"id", "server_id", "path", "log_type"}, filter.Q)

	var total int
	if err := s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM log_files`+sqlFilter.whereSQL(), sqlFilter.args...).Scan(&total); err != nil {
		return repository.Page[*models.LogFile]{}, fmt.Errorf("postgres: count filtered log files: %w", err)
	}

	args := append([]any{}, sqlFilter.args...)
	args = append(args, filter.Offset, filter.Limit)
	orderSQL := orderBy(filter.Sort, map[string]string{
		"path":         "path",
		"last_scanned": "last_scanned_at",
		"last_line":    "last_line_number",
		"created":      "created_at",
	}, "path", filter.Order, "ASC")
	rows, err := s.pool.Query(
		ctx,
		`SELECT id, server_id, path, log_type, file_identity, meta, last_scanned_at, last_line_number, last_byte_offset, is_active, created_at
		 FROM log_files`+sqlFilter.whereSQL()+orderSQL+fmt.Sprintf(", id ASC OFFSET $%d LIMIT $%d", len(args)-1, len(args)),
		args...,
	)
	if err != nil {
		return repository.Page[*models.LogFile]{}, fmt.Errorf("postgres: list filtered log files: %w", err)
	}
	defer rows.Close()

	items := make([]*models.LogFile, 0)
	for rows.Next() {
		var logFile models.LogFile
		var logType string
		var fileIdentity []byte
		var meta []byte
		var lastScannedAt pgtype.Timestamptz
		if err := rows.Scan(
			&logFile.ID, &logFile.ServerID, &logFile.Path, &logType, &fileIdentity, &meta,
			&lastScannedAt, &logFile.LastLineNumber, &logFile.LastByteOffset, &logFile.IsActive, &logFile.CreatedAt,
		); err != nil {
			return repository.Page[*models.LogFile]{}, fmt.Errorf("postgres: scan filtered log file: %w", err)
		}
		logFile.LogType = models.LogType(logType)
		logFile.LastScannedAt = nullableTime(lastScannedAt)
		if err := decodeJSON(fileIdentity, &logFile.FileIdentity); err != nil {
			return repository.Page[*models.LogFile]{}, err
		}
		if err := decodeJSON(meta, &logFile.Meta); err != nil {
			return repository.Page[*models.LogFile]{}, err
		}
		items = append(items, cloneLogFile(&logFile))
	}
	if err := rows.Err(); err != nil {
		return repository.Page[*models.LogFile]{}, fmt.Errorf("postgres: iterate filtered log files: %w", err)
	}

	return repository.Page[*models.LogFile]{Items: items, Total: total, Offset: filter.Offset, Limit: filter.Limit}, nil
}

// UpdateLogFile overwrites an existing log file model.
func (s *Storage) UpdateLogFile(ctx context.Context, logFile *models.LogFile) error {
	current, err := s.GetLogFileByID(ctx, logFile.ID)
	if err != nil {
		return err
	}
	if current.ServerID != logFile.ServerID {
		return fmt.Errorf("%w: log file %q server id cannot change", repository.ErrConflict, logFile.ID)
	}
	if logFile.Meta == nil {
		logFile.Meta = map[string]string{}
	}

	fileIdentity, err := encodeJSON(logFile.FileIdentity)
	if err != nil {
		return err
	}
	meta, err := encodeJSON(logFile.Meta)
	if err != nil {
		return err
	}

	result, err := s.pool.Exec(
		ctx,
		`UPDATE log_files
		 SET path = $2,
		     log_type = $3,
		     file_identity = $4,
		     meta = $5,
		     last_scanned_at = $6,
		     last_line_number = $7,
		     last_byte_offset = $8,
		     is_active = $9,
		     created_at = $10
		 WHERE id = $1`,
		logFile.ID,
		logFile.Path,
		string(logFile.LogType),
		string(fileIdentity),
		string(meta),
		logFile.LastScannedAt,
		logFile.LastLineNumber,
		logFile.LastByteOffset,
		logFile.IsActive,
		logFile.CreatedAt,
	)
	if err != nil {
		if isUniqueViolation(err) {
			return conflictError(fmt.Sprintf("log file path %q already exists", logFile.Path))
		}
		return fmt.Errorf("postgres: update log file %q: %w", logFile.ID, err)
	}

	if result.RowsAffected() == 0 {
		return logFileNotFoundError(logFile.ID)
	}

	return nil
}

// DeleteLogFile removes a log file and all data linked to it.
func (s *Storage) DeleteLogFile(ctx context.Context, id string) error {
	result, err := s.pool.Exec(ctx, `DELETE FROM log_files WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("postgres: delete log file %q: %w", id, err)
	}

	if result.RowsAffected() == 0 {
		return logFileNotFoundError(id)
	}

	return nil
}

// UpdateLastScanned stores the last scan timestamp for a log file.
func (s *Storage) UpdateLastScanned(ctx context.Context, id string) error {
	result, err := s.pool.Exec(
		ctx,
		`UPDATE log_files
		 SET last_scanned_at = $2
		 WHERE id = $1`,
		id,
		time.Now().UTC(),
	)
	if err != nil {
		return fmt.Errorf("postgres: update last scanned %q: %w", id, err)
	}

	if result.RowsAffected() == 0 {
		return logFileNotFoundError(id)
	}

	return nil
}
