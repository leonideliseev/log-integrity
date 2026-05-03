// Package memory provides an in-memory repository implementation.
package memory

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/lenchik/logmonitor/internal/repository"
	"github.com/lenchik/logmonitor/models"
)

// CreateCheckResult stores an integrity check result.
func (s *Storage) CreateCheckResult(_ context.Context, result *models.CheckResult) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.logFiles[result.LogFileID]; !ok {
		return fmt.Errorf("%w: log file %q", repository.ErrNotFound, result.LogFileID)
	}
	if result.ID == "" {
		result.ID = newID("check")
	}
	if _, exists := s.checks[result.ID]; exists {
		return fmt.Errorf("%w: check result %q already exists", repository.ErrConflict, result.ID)
	}
	if result.CheckedAt.IsZero() {
		result.CheckedAt = time.Now().UTC()
	}

	s.checks[result.ID] = cloneCheckResult(result)
	s.checksByLogFile[result.LogFileID] = append(s.checksByLogFile[result.LogFileID], result.ID)
	return nil
}

// GetCheckResultByID returns a check result by identifier.
func (s *Storage) GetCheckResultByID(_ context.Context, id string) (*models.CheckResult, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result, ok := s.checks[id]
	if !ok {
		return nil, fmt.Errorf("%w: check result %q", repository.ErrNotFound, id)
	}
	return cloneCheckResult(result), nil
}

// ListCheckResults returns check history for a log file.
func (s *Storage) ListCheckResults(_ context.Context, logFileID string, offset, limit int) ([]*models.CheckResult, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	items := s.listCheckResultsLocked(logFileID)
	return paginate(items, offset, limit), nil
}

// ListCheckResultsFiltered returns a filtered and paginated check result page.
func (s *Storage) ListCheckResultsFiltered(_ context.Context, filter repository.CheckResultListFilter) (repository.Page[*models.CheckResult], error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	items := make([]*models.CheckResult, 0)
	for _, item := range s.checks {
		if filter.LogFileID != "" && item.LogFileID != filter.LogFileID {
			continue
		}
		if filter.Status != "" && item.Status != filter.Status {
			continue
		}
		if filter.Severity != "" && models.SeverityForCheckStatus(item.Status) != filter.Severity {
			continue
		}
		if filter.ProblemType != "" && models.ProblemTypeForCheckStatus(item.Status) != filter.ProblemType {
			continue
		}
		if !matchesSearch(filter.Q, item.ID, item.LogFileID, item.ErrorMessage, string(item.Status)) {
			continue
		}
		items = append(items, cloneCheckResult(item))
	}

	sort.Slice(items, func(i, j int) bool {
		less := compareCheckResults(items[i], items[j], filter.Sort)
		if ascending(filter.Order) {
			return less
		}
		return compareCheckResults(items[j], items[i], filter.Sort)
	})

	return paged(items, filter.Offset, filter.Limit), nil
}

func compareCheckResults(left, right *models.CheckResult, sortBy string) bool {
	switch sortBy {
	case "status":
		if left.Status == right.Status {
			return left.CheckedAt.Before(right.CheckedAt)
		}
		return left.Status < right.Status
	case "tampered_lines":
		if left.TamperedLines == right.TamperedLines {
			return left.CheckedAt.Before(right.CheckedAt)
		}
		return left.TamperedLines < right.TamperedLines
	default:
		return left.CheckedAt.Before(right.CheckedAt)
	}
}

// GetLatestCheckResult returns the newest saved result for a log file.
func (s *Storage) GetLatestCheckResult(_ context.Context, logFileID string) (*models.CheckResult, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	items := s.listCheckResultsLocked(logFileID)
	if len(items) == 0 {
		return nil, fmt.Errorf("%w: latest check result for log file %q", repository.ErrNotFound, logFileID)
	}
	return items[len(items)-1], nil
}

// listCheckResultsLocked returns results ordered by check time.
func (s *Storage) listCheckResultsLocked(logFileID string) []*models.CheckResult {
	items := make([]*models.CheckResult, 0, len(s.checksByLogFile[logFileID]))
	for _, checkID := range s.checksByLogFile[logFileID] {
		items = append(items, cloneCheckResult(s.checks[checkID]))
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].CheckedAt.Before(items[j].CheckedAt)
	})
	return items
}
