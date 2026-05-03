// Package entry contains application operations for collected log entries.
package entry

import (
	"context"

	"github.com/lenchik/logmonitor/internal/repository"
	"github.com/lenchik/logmonitor/models"
)

// Service provides read-only entry operations for the HTTP layer.
type Service struct {
	entries repository.LogEntryRepository
}

// NewService creates an entry application service.
func NewService(entries repository.LogEntryRepository) *Service {
	return &Service{entries: entries}
}

// List returns collected log entries for one log file with pagination.
func (s *Service) List(ctx context.Context, logFileID string, offset, limit int) ([]*models.LogEntry, error) {
	return s.entries.ListLogEntries(ctx, logFileID, offset, limit)
}

// ListFiltered returns a filtered log entry page for API list screens.
func (s *Service) ListFiltered(ctx context.Context, filter repository.LogEntryListFilter) (repository.Page[*models.LogEntry], error) {
	return s.entries.ListLogEntriesFiltered(ctx, filter)
}
