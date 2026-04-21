// Package models contains domain models shared across application layers.
package models

import "time"

// CheckStatus — результат проверки целостности журнала
type CheckStatus string

// Check statuses describe the result of an integrity check.
const (
	CheckStatusOK       CheckStatus = "ok"       // все записи совпадают
	CheckStatusTampered CheckStatus = "tampered" // обнаружены изменённые записи
	CheckStatusError    CheckStatus = "error"    // ошибка при проверке
)

// CheckResult представляет результат одной проверки целостности журнала
type CheckResult struct {
	ID            string      `json:"id"`
	LogFileID     string      `json:"log_file_id"`
	CheckedAt     time.Time   `json:"checked_at"`
	Status        CheckStatus `json:"status"`
	TotalLines    int64       `json:"total_lines"`
	TamperedLines int64       `json:"tampered_lines"`
	ErrorMessage  string      `json:"error_message,omitempty"`
}

// TamperedEntry описывает конкретную изменённую запись журнала
type TamperedEntry struct {
	LineNumber     int64  `json:"line_number"`
	StoredHash     string `json:"stored_hash"`
	CurrentHash    string `json:"current_hash"`
	CurrentContent string `json:"current_content"`
}
