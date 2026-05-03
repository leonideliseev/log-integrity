// Package postgres provides a pgx-backed repository implementation.
package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/lenchik/logmonitor/internal/repository"
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

// ListCheckResultsFiltered returns a filtered and paginated check result page.
func (s *Storage) ListCheckResultsFiltered(ctx context.Context, filter repository.CheckResultListFilter) (repository.Page[*models.CheckResult], error) {
	filter.ListOptions = normalizePage(filter.ListOptions)

	sqlFilter := sqlFilter{}
	if filter.LogFileID != "" {
		sqlFilter.add("log_file_id = $%d", filter.LogFileID)
	}
	if filter.Status != "" {
		sqlFilter.add("status = $%d", string(filter.Status))
	}
	if filter.Severity != "" {
		sqlFilter.add(checkSeverityCondition(filter.Severity, "%d"), string(filter.Severity))
	}
	if filter.ProblemType != "" {
		sqlFilter.add(checkProblemTypeCondition(filter.ProblemType, "%d"), string(filter.ProblemType))
	}
	sqlFilter.addSearch([]string{"id", "log_file_id", "status", "error_message"}, filter.Q)

	var total int
	if err := s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM check_results`+sqlFilter.whereSQL(), sqlFilter.args...).Scan(&total); err != nil {
		return repository.Page[*models.CheckResult]{}, fmt.Errorf("postgres: count filtered check results: %w", err)
	}

	args := append([]any{}, sqlFilter.args...)
	args = append(args, filter.Offset, filter.Limit)
	orderSQL := orderBy(filter.Sort, map[string]string{
		"checked_at":     "checked_at",
		"status":         "status",
		"tampered_lines": "tampered_lines",
	}, "checked_at", filter.Order, "DESC")
	rows, err := s.pool.Query(
		ctx,
		`SELECT id, log_file_id, checked_at, status, total_lines, tampered_lines, error_message
		 FROM check_results`+sqlFilter.whereSQL()+orderSQL+fmt.Sprintf(", id ASC OFFSET $%d LIMIT $%d", len(args)-1, len(args)),
		args...,
	)
	if err != nil {
		return repository.Page[*models.CheckResult]{}, fmt.Errorf("postgres: list filtered check results: %w", err)
	}
	defer rows.Close()

	items := make([]*models.CheckResult, 0)
	for rows.Next() {
		var result models.CheckResult
		var status string
		if err := rows.Scan(&result.ID, &result.LogFileID, &result.CheckedAt, &status, &result.TotalLines, &result.TamperedLines, &result.ErrorMessage); err != nil {
			return repository.Page[*models.CheckResult]{}, fmt.Errorf("postgres: scan filtered check result: %w", err)
		}
		result.Status = models.CheckStatus(status)
		items = append(items, cloneCheckResult(&result))
	}
	if err := rows.Err(); err != nil {
		return repository.Page[*models.CheckResult]{}, fmt.Errorf("postgres: iterate filtered check results: %w", err)
	}

	return repository.Page[*models.CheckResult]{Items: items, Total: total, Offset: filter.Offset, Limit: filter.Limit}, nil
}

func checkSeverityCondition(_ models.ProblemSeverity, placeholderPattern string) string {
	return fmt.Sprintf(`CASE status
		WHEN 'tampered' THEN 'critical'
		WHEN 'error' THEN 'error'
		ELSE ''
	END = $%s`, placeholderPattern)
}

func checkProblemTypeCondition(_ models.ProblemType, placeholderPattern string) string {
	return fmt.Sprintf(`CASE status
		WHEN 'tampered' THEN 'integrity_tampered'
		WHEN 'error' THEN 'integrity_check_error'
		ELSE ''
	END = $%s`, placeholderPattern)
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
