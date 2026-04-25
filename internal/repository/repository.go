package repository

import "context"

// Repository combines all storage interfaces required by the application.
type Repository interface {
	ServerRepository
	LogFileRepository
	LogEntryRepository
	LogChunkRepository
	LogBatchRepository
	CheckResultRepository

	Ping(ctx context.Context) error
	Close() error
}
