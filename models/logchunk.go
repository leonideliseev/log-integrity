package models

import "time"

// LogChunk stores an aggregate hash for a contiguous group of collected log entries.
type LogChunk struct {
	ID             string    `json:"id"`
	LogFileID      string    `json:"log_file_id"`
	ChunkNumber    int64     `json:"chunk_number"`
	FromLineNumber int64     `json:"from_line_number"`
	ToLineNumber   int64     `json:"to_line_number"`
	FromByteOffset int64     `json:"from_byte_offset"`
	ToByteOffset   int64     `json:"to_byte_offset"`
	EntriesCount   int       `json:"entries_count"`
	Hash           string    `json:"hash"`
	HashAlgorithm  string    `json:"hash_algorithm"`
	CreatedAt      time.Time `json:"created_at"`
}
