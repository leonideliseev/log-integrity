package models

import "time"

// LogEntry представляет одну строку журнала с её хэшем
type LogEntry struct {
	ID          string    `json:"id"`
	LogFileID   string    `json:"log_file_id"`
	LineNumber  int64     `json:"line_number"`
	Content     string    `json:"content"`
	Hash        string    `json:"hash"` // SHA-256 hex от Content
	CollectedAt time.Time `json:"collected_at"`
}
