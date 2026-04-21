// Package server contains application operations for monitored servers.
package server

import (
	"context"
	"fmt"
	"strings"

	"github.com/lenchik/logmonitor/internal/repository"
	discoveryservice "github.com/lenchik/logmonitor/internal/service/discovery"
	"github.com/lenchik/logmonitor/models"
)

// DiscoverResult contains discovery output for one server.
type DiscoverResult struct {
	LogFiles []*models.LogFile `json:"log_files,omitempty"`
	Error    string            `json:"error,omitempty"`
}

// Service provides server-related application operations for the HTTP layer.
type Service struct {
	servers   repository.ServerRepository
	discovery *discoveryservice.Service
	locker    Locker
}

// Locker provides optional per-server isolation for manual operations.
type Locker interface {
	TryLock(key string) (func(), bool)
}

// NewService creates a server application service.
func NewService(servers repository.ServerRepository, discovery *discoveryservice.Service) *Service {
	return NewServiceWithLocker(servers, discovery, nil)
}

// NewServiceWithLocker creates a server application service with optional server isolation.
func NewServiceWithLocker(servers repository.ServerRepository, discovery *discoveryservice.Service, locker Locker) *Service {
	return &Service{
		servers:   servers,
		discovery: discovery,
		locker:    locker,
	}
}

// List returns all registered servers.
func (s *Service) List(ctx context.Context) ([]*models.Server, error) {
	return s.servers.ListServers(ctx)
}

// Create stores a new server model.
func (s *Service) Create(ctx context.Context, serverModel *models.Server) error {
	if serverModel.Status == "" {
		serverModel.Status = models.ServerStatusActive
	}
	if serverModel.Port == 0 {
		serverModel.Port = 22
	}
	if serverModel.ManagedBy == "" {
		serverModel.ManagedBy = models.ServerManagedByAPI
	}
	if err := s.ensureUniqueIdentity(ctx, serverModel, ""); err != nil {
		return err
	}
	return s.servers.CreateServer(ctx, serverModel)
}

// Discover runs discovery for one server or for all servers.
func (s *Service) Discover(ctx context.Context, serverID string) (map[string]DiscoverResult, error) {
	items, err := s.resolveServers(ctx, serverID)
	if err != nil {
		return nil, err
	}

	result := make(map[string]DiscoverResult, len(items))
	for _, serverModel := range items {
		unlock, ok := s.tryLockServer(serverModel.ID)
		if !ok {
			result[serverModel.ID] = DiscoverResult{Error: fmt.Sprintf("server %q is busy", serverModel.Name)}
			continue
		}
		logFiles, discoverErr := s.discovery.DiscoverAndSync(ctx, serverModel)
		unlock()
		if discoverErr != nil {
			result[serverModel.ID] = DiscoverResult{Error: discoverErr.Error()}
			continue
		}
		result[serverModel.ID] = DiscoverResult{LogFiles: logFiles}
	}

	return result, nil
}

// resolveServers returns either one selected server or the whole server list.
func (s *Service) resolveServers(ctx context.Context, serverID string) ([]*models.Server, error) {
	if serverID != "" {
		serverModel, err := s.servers.GetServerByID(ctx, serverID)
		if err != nil {
			return nil, err
		}
		return []*models.Server{serverModel}, nil
	}
	return s.servers.ListServers(ctx)
}

// tryLockServer acquires optional non-blocking isolation for manual discovery.
func (s *Service) tryLockServer(serverID string) (func(), bool) {
	if s.locker == nil {
		return func() {}, true
	}
	return s.locker.TryLock("server:" + serverID)
}

// ensureUniqueIdentity prevents duplicate monitored server names and hosts.
func (s *Service) ensureUniqueIdentity(ctx context.Context, serverModel *models.Server, excludeID string) error {
	items, err := s.servers.ListServers(ctx)
	if err != nil {
		return err
	}
	for _, item := range items {
		if item.ID == excludeID {
			continue
		}
		if strings.EqualFold(item.Name, serverModel.Name) {
			return fmt.Errorf("%w: server name %q already exists", repository.ErrConflict, serverModel.Name)
		}
		if strings.EqualFold(item.Host, serverModel.Host) {
			return fmt.Errorf("%w: server host %q already exists", repository.ErrConflict, serverModel.Host)
		}
	}
	return nil
}
