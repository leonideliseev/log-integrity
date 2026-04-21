package repository

// Repository combines all storage interfaces required by the application.
type Repository interface {
	ServerRepository
	LogFileRepository
	LogEntryRepository
	LogChunkRepository
	LogBatchRepository
	CheckResultRepository

	Close() error
}
