package repository

import (
	"context"

	"github.com/lenchik/logmonitor/models"
)

// LogBatchRepository atomically stores log entries and their aggregate chunks.
type LogBatchRepository interface {
	CreateLogEntriesWithChunks(ctx context.Context, entries []*models.LogEntry, chunks []*models.LogChunk) error
}
