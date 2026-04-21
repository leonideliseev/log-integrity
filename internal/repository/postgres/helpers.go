package postgres

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/lenchik/logmonitor/internal/repository"
	"github.com/lenchik/logmonitor/models"
)

// newID generates a repository identifier compatible with the in-memory storage.
func newID(prefix string) string {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("%s_%d", prefix, time.Now().UTC().UnixNano())
	}
	return prefix + "_" + hex.EncodeToString(buf)
}

// isUniqueViolation reports whether an error comes from a PostgreSQL unique constraint.
func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

// isForeignKeyViolation reports whether an error comes from a PostgreSQL foreign key constraint.
func isForeignKeyViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23503"
}

// serverNotFoundError returns a consistent repository error for missing servers.
func serverNotFoundError(id string) error {
	return fmt.Errorf("%w: server %q", repository.ErrNotFound, id)
}

// logFileNotFoundError returns a consistent repository error for missing log files.
func logFileNotFoundError(id string) error {
	return fmt.Errorf("%w: log file %q", repository.ErrNotFound, id)
}

// logEntryNotFoundError returns a consistent repository error for missing log entries.
func logEntryNotFoundError(logFileID string, lineNumber int64) error {
	return fmt.Errorf("%w: log entry %q line %d", repository.ErrNotFound, logFileID, lineNumber)
}

// checkResultNotFoundError returns a consistent repository error for missing check results.
func checkResultNotFoundError(id string) error {
	return fmt.Errorf("%w: check result %q", repository.ErrNotFound, id)
}

// latestCheckResultNotFoundError returns a consistent repository error for missing latest check result.
func latestCheckResultNotFoundError(logFileID string) error {
	return fmt.Errorf("%w: latest check result for log file %q", repository.ErrNotFound, logFileID)
}

// conflictError wraps a repository-level conflict with a detailed message.
func conflictError(message string) error {
	return fmt.Errorf("%w: %s", repository.ErrConflict, message)
}

// cloneServer returns a detached copy of the server model.
func cloneServer(in *models.Server) *models.Server {
	copyModel := *in
	if in.LastSeenAt != nil {
		lastSeenAt := *in.LastSeenAt
		copyModel.LastSeenAt = &lastSeenAt
	}
	if in.BackoffUntil != nil {
		backoffUntil := *in.BackoffUntil
		copyModel.BackoffUntil = &backoffUntil
	}
	return &copyModel
}

// cloneLogFile returns a detached copy of the log file model.
func cloneLogFile(in *models.LogFile) *models.LogFile {
	copyModel := *in
	if in.LastScannedAt != nil {
		ts := *in.LastScannedAt
		copyModel.LastScannedAt = &ts
	}
	return &copyModel
}

// cloneLogEntry returns a detached copy of the entry model.
func cloneLogEntry(in *models.LogEntry) *models.LogEntry {
	copyModel := *in
	return &copyModel
}

// cloneLogChunk returns a detached copy of the log chunk model.
func cloneLogChunk(in *models.LogChunk) *models.LogChunk {
	copyModel := *in
	return &copyModel
}

// cloneCheckResult returns a detached copy of the check result model.
func cloneCheckResult(in *models.CheckResult) *models.CheckResult {
	copyModel := *in
	return &copyModel
}

// normalizeOffset ensures pagination offset stays non-negative.
func normalizeOffset(offset int) int {
	if offset < 0 {
		return 0
	}
	return offset
}

// nullableTime converts pgx nullable timestamptz to *time.Time.
func nullableTime(value pgtype.Timestamptz) *time.Time {
	if !value.Valid {
		return nil
	}
	ts := value.Time
	return &ts
}

// encodeJSON serializes repository JSON fields for jsonb columns.
func encodeJSON(value any) ([]byte, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("postgres: encode json: %w", err)
	}
	return data, nil
}

// decodeJSON deserializes repository JSON fields from jsonb columns.
func decodeJSON(data []byte, target any) error {
	if len(data) == 0 {
		data = []byte("{}")
	}
	if err := json.Unmarshal(data, target); err != nil {
		return fmt.Errorf("postgres: decode json: %w", err)
	}
	return nil
}
