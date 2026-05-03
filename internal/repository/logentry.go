package repository

import (
	"context"

	"github.com/lenchik/logmonitor/models"
)

// LogEntryRepository stores hashed log entries.
type LogEntryRepository interface {
	CreateLogEntry(ctx context.Context, entry *models.LogEntry) error
	CreateLogEntries(ctx context.Context, entries []*models.LogEntry) error
	GetLogEntryByLine(ctx context.Context, logFileID string, lineNumber int64) (*models.LogEntry, error)
	ListLogEntries(ctx context.Context, logFileID string, offset, limit int) ([]*models.LogEntry, error)
	ListLogEntriesFiltered(ctx context.Context, filter LogEntryListFilter) (Page[*models.LogEntry], error)
	ListLogEntriesByLineRange(ctx context.Context, logFileID string, fromLine, toLine int64) ([]*models.LogEntry, error)
	GetMaxLineNumber(ctx context.Context, logFileID string) (int64, error)
	CountLogEntries(ctx context.Context, logFileID string) (int64, error)
	DeleteLogEntriesByLogFile(ctx context.Context, logFileID string) error
}
