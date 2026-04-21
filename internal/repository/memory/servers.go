package memory

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/lenchik/logmonitor/internal/repository"
	"github.com/lenchik/logmonitor/models"
)

// CreateServer saves a new server model in memory.
func (s *Storage) CreateServer(_ context.Context, serverModel *models.Server) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if serverModel.ID == "" {
		serverModel.ID = newID("srv")
	}
	if _, exists := s.servers[serverModel.ID]; exists {
		return fmt.Errorf("%w: server %q already exists", repository.ErrConflict, serverModel.ID)
	}
	if existing := s.findServerByNameOrHostLocked(serverModel.Name, serverModel.Host, ""); existing != nil {
		return fmt.Errorf("%w: server name or host already exists", repository.ErrConflict)
	}

	now := time.Now().UTC()
	if serverModel.CreatedAt.IsZero() {
		serverModel.CreatedAt = now
	}
	if serverModel.UpdatedAt.IsZero() {
		serverModel.UpdatedAt = serverModel.CreatedAt
	}
	if serverModel.Status == "" {
		serverModel.Status = models.ServerStatusInactive
	}
	if serverModel.ManagedBy == "" {
		serverModel.ManagedBy = models.ServerManagedByAPI
	}

	s.servers[serverModel.ID] = cloneServer(serverModel)
	return nil
}

// GetServerByID returns a server by its identifier.
func (s *Storage) GetServerByID(_ context.Context, id string) (*models.Server, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	serverModel, ok := s.servers[id]
	if !ok {
		return nil, fmt.Errorf("%w: server %q", repository.ErrNotFound, id)
	}

	return cloneServer(serverModel), nil
}

// ListServers returns all stored servers ordered by name.
func (s *Storage) ListServers(_ context.Context) ([]*models.Server, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	items := make([]*models.Server, 0, len(s.servers))
	for _, item := range s.servers {
		items = append(items, cloneServer(item))
	}

	sort.Slice(items, func(i, j int) bool {
		if items[i].Name == items[j].Name {
			return items[i].ID < items[j].ID
		}
		return items[i].Name < items[j].Name
	})

	return items, nil
}

// UpdateServer overwrites an existing server model.
func (s *Storage) UpdateServer(_ context.Context, serverModel *models.Server) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.servers[serverModel.ID]; !ok {
		return fmt.Errorf("%w: server %q", repository.ErrNotFound, serverModel.ID)
	}
	if existing := s.findServerByNameOrHostLocked(serverModel.Name, serverModel.Host, serverModel.ID); existing != nil {
		return fmt.Errorf("%w: server name or host already exists", repository.ErrConflict)
	}

	copyModel := cloneServer(serverModel)
	if copyModel.ManagedBy == "" {
		copyModel.ManagedBy = models.ServerManagedByAPI
	}
	copyModel.UpdatedAt = time.Now().UTC()
	s.servers[serverModel.ID] = copyModel
	return nil
}

func (s *Storage) findServerByNameOrHostLocked(name, host, excludeID string) *models.Server {
	for _, item := range s.servers {
		if item.ID == excludeID {
			continue
		}
		if strings.EqualFold(item.Name, name) || strings.EqualFold(item.Host, host) {
			return item
		}
	}
	return nil
}

// DeleteServer removes a server and every dependent entity linked to it.
func (s *Storage) DeleteServer(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.servers[id]; !ok {
		return fmt.Errorf("%w: server %q", repository.ErrNotFound, id)
	}

	// Remove dependent log files first so entries and checks disappear together.
	for _, logFileID := range s.logFilesByServerIDs(id) {
		s.deleteLogFileLocked(logFileID)
	}

	delete(s.servers, id)
	delete(s.logFilesByServer, id)
	return nil
}

// UpdateServerStatus updates only the server status field.
func (s *Storage) UpdateServerStatus(_ context.Context, id string, status models.ServerStatus) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	serverModel, ok := s.servers[id]
	if !ok {
		return fmt.Errorf("%w: server %q", repository.ErrNotFound, id)
	}

	serverModel.Status = status
	serverModel.UpdatedAt = time.Now().UTC()
	return nil
}

// RecordServerSuccess records a successful remote server operation.
func (s *Storage) RecordServerSuccess(_ context.Context, id string, seenAt time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	serverModel, ok := s.servers[id]
	if !ok {
		return fmt.Errorf("%w: server %q", repository.ErrNotFound, id)
	}

	serverModel.Status = models.ServerStatusActive
	serverModel.SuccessCount++
	serverModel.FailureCount = 0
	serverModel.LastError = ""
	serverModel.LastSeenAt = &seenAt
	serverModel.BackoffUntil = nil
	serverModel.UpdatedAt = time.Now().UTC()
	return nil
}

// RecordServerFailure records a failed remote server operation.
func (s *Storage) RecordServerFailure(_ context.Context, id string, lastError string, backoffUntil *time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	serverModel, ok := s.servers[id]
	if !ok {
		return fmt.Errorf("%w: server %q", repository.ErrNotFound, id)
	}

	serverModel.Status = models.ServerStatusError
	serverModel.FailureCount++
	serverModel.LastError = lastError
	if backoffUntil != nil {
		backoffCopy := *backoffUntil
		serverModel.BackoffUntil = &backoffCopy
	} else {
		serverModel.BackoffUntil = nil
	}
	serverModel.UpdatedAt = time.Now().UTC()
	return nil
}
