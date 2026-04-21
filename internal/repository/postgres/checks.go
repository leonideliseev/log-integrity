// Package postgres provides a pgx-backed repository implementation.
package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/lenchik/logmonitor/models"
)

// CreateCheckResult stores an integrity check result.
func (s *Storage) CreateCheckResult(ctx context.Context, result *models.CheckResult) error {
	if result.ID == "" {
		result.ID = newID("check")
	}
	if result.CheckedAt.IsZero() {
		result.CheckedAt = time.Now().UTC()
	}

	_, err := s.pool.Exec(
		ctx,
		`INSERT INTO check_results (id, log_file_id, checked_at, status, total_lines, tampered_lines, error_message)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		result.ID,
		result.LogFileID,
		result.CheckedAt,
		string(result.Status),
		result.TotalLines,
		result.TamperedLines,
		result.ErrorMessage,
	)
	if err != nil {
		if isForeignKeyViolation(err) {
			return logFileNotFoundError(result.LogFileID)
		}
		if isUniqueViolation(err) {
			return conflictError(fmt.Sprintf("check result %q already exists", result.ID))
		}
		return fmt.Errorf("postgres: create check result %q: %w", result.ID, err)
	}

	return nil
}

// GetCheckResultByID returns a check result by identifier.
func (s *Storage) GetCheckResultByID(ctx context.Context, id string) (*models.CheckResult, error) {
	row := s.pool.QueryRow(
		ctx,
		`SELECT id, log_file_id, checked_at, status, total_lines, tampered_lines, error_message
		 FROM check_results
		 WHERE id = $1`,
		id,
	)

	var result models.CheckResult
	var status string
	if err := row.Scan(
		&result.ID,
		&result.LogFileID,
		&result.CheckedAt,
		&status,
		&result.TotalLines,
		&result.TamperedLines,
		&result.ErrorMessage,
	); err != nil {
		if err == pgx.ErrNoRows {
			return nil, checkResultNotFoundError(id)
		}
		return nil, fmt.Errorf("postgres: get check result %q: %w", id, err)
	}

	result.Status = models.CheckStatus(status)
	return cloneCheckResult(&result), nil
}

// ListCheckResults returns check history for a log file.
func (s *Storage) ListCheckResults(ctx context.Context, logFileID string, offset, limit int) ([]*models.CheckResult, error) {
	offset = normalizeOffset(offset)

	var rows pgx.Rows
	var err error
	if limit <= 0 {
		rows, err = s.pool.Query(
			ctx,
			`SELECT id, log_file_id, checked_at, status, total_lines, tampered_lines, error_message
			 FROM check_results
			 WHERE log_file_id = $1
			 ORDER BY checked_at ASC, id ASC
			 OFFSET $2`,
			logFileID,
			offset,
		)
	} else {
		rows, err = s.pool.Query(
			ctx,
			`SELECT id, log_file_id, checked_at, status, total_lines, tampered_lines, error_message
			 FROM check_results
			 WHERE log_file_id = $1
			 ORDER BY checked_at ASC, id ASC
			 OFFSET $2
			 LIMIT $3`,
			logFileID,
			offset,
			limit,
		)
	}
	if err != nil {
		return nil, fmt.Errorf("postgres: list check results for %q: %w", logFileID, err)
	}
	defer rows.Close()

	items := make([]*models.CheckResult, 0)
	for rows.Next() {
		var result models.CheckResult
		var status string
		if err := rows.Scan(
			&result.ID,
			&result.LogFileID,
			&result.CheckedAt,
			&status,
			&result.TotalLines,
			&result.TamperedLines,
			&result.ErrorMessage,
		); err != nil {
			return nil, fmt.Errorf("postgres: scan check result: %w", err)
		}
		result.Status = models.CheckStatus(status)
		items = append(items, cloneCheckResult(&result))
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("postgres: iterate check results for %q: %w", logFileID, err)
	}

	return items, nil
}

// GetLatestCheckResult returns the newest saved result for a log file.
func (s *Storage) GetLatestCheckResult(ctx context.Context, logFileID string) (*models.CheckResult, error) {
	row := s.pool.QueryRow(
		ctx,
		`SELECT id, log_file_id, checked_at, status, total_lines, tampered_lines, error_message
		 FROM check_results
		 WHERE log_file_id = $1
		 ORDER BY checked_at DESC, id DESC
		 LIMIT 1`,
		logFileID,
	)

	var result models.CheckResult
	var status string
	if err := row.Scan(
		&result.ID,
		&result.LogFileID,
		&result.CheckedAt,
		&status,
		&result.TotalLines,
		&result.TamperedLines,
		&result.ErrorMessage,
	); err != nil {
		if err == pgx.ErrNoRows {
			return nil, latestCheckResultNotFoundError(logFileID)
		}
		return nil, fmt.Errorf("postgres: get latest check result for %q: %w", logFileID, err)
	}

	result.Status = models.CheckStatus(status)
	return cloneCheckResult(&result), nil
}
