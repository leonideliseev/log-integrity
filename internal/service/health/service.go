// Package health tracks remote server availability lifecycle.
package health

import (
	"context"
	"errors"
	"time"

	"github.com/lenchik/logmonitor/internal/repository"
	"github.com/lenchik/logmonitor/models"
)

const defaultLastErrorMaxLength = 2048

// Options controls server health counters and backoff behavior.
type Options struct {
	FailureThreshold   int
	BackoffBase        time.Duration
	BackoffMax         time.Duration
	LastErrorMaxLength int
}

// Service records server health transitions after remote operations.
type Service struct {
	servers repository.ServerRepository
	options Options
	now     func() time.Time
}

// NewService creates a server health service.
func NewService(servers repository.ServerRepository, options Options) *Service {
	return &Service{
		servers: servers,
		options: normalizeOptions(options),
		now:     func() time.Time { return time.Now().UTC() },
	}
}

// ShouldSkip reports whether scheduled work should wait because a server is in backoff.
func (s *Service) ShouldSkip(serverModel *models.Server) bool {
	if s == nil || serverModel == nil || serverModel.BackoffUntil == nil {
		return false
	}
	return s.now().Before(*serverModel.BackoffUntil)
}

// RecordSuccess marks a server as reachable and clears failure state.
func (s *Service) RecordSuccess(ctx context.Context, serverID string) error {
	if s == nil {
		return nil
	}
	return s.servers.RecordServerSuccess(ctx, serverID, s.now())
}

// RecordFailure marks a server as failed and calculates the next retry backoff.
func (s *Service) RecordFailure(ctx context.Context, serverModel *models.Server, err error) error {
	if s == nil || serverModel == nil {
		return nil
	}

	nextFailureCount := serverModel.FailureCount + 1
	backoffUntil := s.nextBackoffUntil(nextFailureCount)
	return s.servers.RecordServerFailure(ctx, serverModel.ID, s.lastError(err), backoffUntil)
}

// RecordDegraded marks a server as reachable but partially unhealthy.
func (s *Service) RecordDegraded(ctx context.Context, serverID, detail string) error {
	if s == nil {
		return nil
	}

	return s.mutateServer(ctx, serverID, func(serverModel *models.Server) {
		now := s.now()
		serverModel.Status = models.ServerStatusDegraded
		serverModel.FailureCount = 0
		serverModel.LastError = s.lastErrorFromMessage(detail)
		serverModel.LastSeenAt = &now
		serverModel.BackoffUntil = nil
	})
}

// ClearBackoff resets the temporary failure state so operators can retry a server immediately.
func (s *Service) ClearBackoff(ctx context.Context, serverID string) error {
	if s == nil {
		return nil
	}

	return s.mutateServer(ctx, serverID, func(serverModel *models.Server) {
		serverModel.FailureCount = 0
		serverModel.LastError = ""
		serverModel.BackoffUntil = nil
		if serverModel.Status == models.ServerStatusError {
			serverModel.Status = models.ServerStatusActive
		}
	})
}

func (s *Service) nextBackoffUntil(nextFailureCount int64) *time.Time {
	if s.options.BackoffBase <= 0 || s.options.BackoffMax <= 0 {
		return nil
	}
	if nextFailureCount < int64(s.options.FailureThreshold) {
		return nil
	}

	duration := s.options.BackoffBase
	for i := int64(0); i < nextFailureCount-int64(s.options.FailureThreshold); i++ {
		if duration >= s.options.BackoffMax/2 {
			duration = s.options.BackoffMax
			break
		}
		duration *= 2
	}
	if duration > s.options.BackoffMax {
		duration = s.options.BackoffMax
	}

	backoffUntil := s.now().Add(duration)
	return &backoffUntil
}

func (s *Service) lastError(err error) string {
	if err == nil {
		err = errors.New("unknown server health error")
	}

	return s.lastErrorFromMessage(err.Error())
}

func normalizeOptions(options Options) Options {
	if options.FailureThreshold <= 0 {
		options.FailureThreshold = 1
	}
	if options.LastErrorMaxLength == 0 {
		options.LastErrorMaxLength = defaultLastErrorMaxLength
	}
	return options
}

// mutateServer loads the current server state, applies changes and persists the full model.
func (s *Service) mutateServer(ctx context.Context, serverID string, mutate func(serverModel *models.Server)) error {
	serverModel, err := s.servers.GetServerByID(ctx, serverID)
	if err != nil {
		return err
	}

	mutate(serverModel)
	return s.servers.UpdateServer(ctx, serverModel)
}

// lastErrorFromMessage normalizes stored server error details to the configured length.
func (s *Service) lastErrorFromMessage(value string) string {
	if value == "" {
		value = "unknown server health error"
	}
	if s.options.LastErrorMaxLength <= 0 || len(value) <= s.options.LastErrorMaxLength {
		return value
	}
	return value[:s.options.LastErrorMaxLength]
}
