package repository

import (
	"context"

	"github.com/lenchik/logmonitor/models"
)

// LogChunkRepository stores aggregate chunk hashes for log files.
type LogChunkRepository interface {
	CreateLogChunk(ctx context.Context, chunk *models.LogChunk) error
	CreateLogChunks(ctx context.Context, chunks []*models.LogChunk) error
	ListLogChunks(ctx context.Context, logFileID string, offset, limit int) ([]*models.LogChunk, error)
	GetLatestLogChunk(ctx context.Context, logFileID string) (*models.LogChunk, error)
	DeleteLogChunksByLogFile(ctx context.Context, logFileID string) error
}
