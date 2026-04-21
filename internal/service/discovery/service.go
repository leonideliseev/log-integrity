package discovery

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/lenchik/logmonitor/internal/repository"
	"github.com/lenchik/logmonitor/internal/ssh"
	"github.com/lenchik/logmonitor/models"
)

// Service detects remote operating systems and synchronizes discovered log files.
type Service struct {
	clientFactory ssh.ClientFactory
	registry      *Registry
	logFiles      repository.LogFileRepository
	servers       repository.ServerRepository
}

// NewDefaultRegistry creates a registry with built-in discoverers for supported OSes.
func NewDefaultRegistry() *Registry {
	registry := NewRegistry()
	registry.Register(NewLinuxDiscoverer())
	registry.Register(NewWindowsDiscoverer())
	registry.Register(NewMacOSDiscoverer())
	return registry
}

// NewService creates a discovery service with SSH, repository and registry dependencies.
func NewService(clientFactory ssh.ClientFactory, logFiles repository.LogFileRepository, registry *Registry) *Service {
	return NewServiceWithServerRepository(clientFactory, logFiles, nil, registry)
}

// NewServiceWithServerRepository creates a discovery service that can persist detected OS types.
func NewServiceWithServerRepository(clientFactory ssh.ClientFactory, logFiles repository.LogFileRepository, servers repository.ServerRepository, registry *Registry) *Service {
	if registry == nil {
		registry = NewDefaultRegistry()
	}

	return &Service{
		clientFactory: clientFactory,
		registry:      registry,
		logFiles:      logFiles,
		servers:       servers,
	}
}

// Discover detects remote log files for a concrete server.
func (s *Service) Discover(ctx context.Context, serverModel *models.Server) ([]DiscoveredLog, error) {
	client := s.clientFactory.NewClient()
	if err := client.Connect(serverModel); err != nil {
		return nil, fmt.Errorf("discovery: connect to server %q: %w", serverModel.Name, err)
	}
	defer func() {
		_ = client.Close()
	}()

	osType := serverModel.OSType
	if osType == "" {
		detected, err := DetectOSType(ctx, client)
		if err != nil {
			return nil, err
		}
		serverModel.OSType = detected
		osType = detected
		if s.servers != nil {
			if err := s.servers.UpdateServer(ctx, serverModel); err != nil {
				return nil, fmt.Errorf("discovery: persist detected os for server %q: %w", serverModel.Name, err)
			}
		}
	}

	discoverer, ok := s.registry.Get(osType)
	if !ok {
		return nil, fmt.Errorf("discovery: unsupported os %q", osType)
	}

	return discoverer.Discover(client)
}

// DiscoverAndSync performs discovery and then synchronizes results into the repository.
func (s *Service) DiscoverAndSync(ctx context.Context, serverModel *models.Server) ([]*models.LogFile, error) {
	discovered, err := s.Discover(ctx, serverModel)
	if err != nil {
		return nil, err
	}
	return s.Sync(ctx, serverModel.ID, discovered)
}

// Sync reconciles discovered log files with the currently stored repository state.
func (s *Service) Sync(ctx context.Context, serverID string, discovered []DiscoveredLog) ([]*models.LogFile, error) {
	if s.logFiles == nil {
		return nil, errors.New("discovery: log file repository is not configured")
	}

	existing, err := s.logFiles.ListLogFilesByServer(ctx, serverID)
	if err != nil {
		return nil, fmt.Errorf("discovery: list existing log files: %w", err)
	}

	existingByPath := make(map[string]*models.LogFile, len(existing))
	for _, item := range existing {
		existingByPath[item.Path] = item
	}

	discoveredPaths := make(map[string]DiscoveredLog, len(discovered))
	// First update or create everything that is currently visible on the remote host.
	for _, item := range discovered {
		discoveredPaths[item.Path] = item
	}

	for _, item := range discovered {
		if current, ok := existingByPath[item.Path]; ok {
			if current.LogType != item.LogType || !current.IsActive {
				current.LogType = item.LogType
				current.IsActive = true
				if err := s.logFiles.UpdateLogFile(ctx, current); err != nil {
					return nil, fmt.Errorf("discovery: update log file %q: %w", item.Path, err)
				}
			}
			continue
		}

		logFile := &models.LogFile{
			ServerID: serverID,
			Path:     item.Path,
			LogType:  item.LogType,
			IsActive: true,
		}
		if err := s.logFiles.CreateLogFile(ctx, logFile); err != nil {
			return nil, fmt.Errorf("discovery: create log file %q: %w", item.Path, err)
		}
	}

	// Then deactivate log files that disappeared from the latest discovery result.
	for _, item := range existing {
		if _, ok := discoveredPaths[item.Path]; ok {
			continue
		}
		if !item.IsActive {
			continue
		}
		item.IsActive = false
		if err := s.logFiles.UpdateLogFile(ctx, item); err != nil {
			return nil, fmt.Errorf("discovery: deactivate log file %q: %w", item.Path, err)
		}
	}

	return s.logFiles.ListLogFilesByServer(ctx, serverID)
}

// DetectOSType probes the remote server and infers its operating system.
func DetectOSType(ctx context.Context, client ssh.Client) (models.OSType, error) {
	if output, err := client.ExecuteContext(ctx, "uname -s"); err == nil {
		normalized := strings.ToLower(strings.TrimSpace(output))
		switch {
		case strings.Contains(normalized, "linux"):
			return models.OSLinux, nil
		case strings.Contains(normalized, "darwin"):
			return models.OSMacOS, nil
		}
	}

	if output, err := client.ExecuteContext(ctx, `powershell -NoProfile -Command "$env:OS"`); err == nil {
		if strings.Contains(strings.ToLower(output), "windows") {
			return models.OSWindows, nil
		}
	}

	if output, err := client.ExecuteContext(ctx, "cmd /c ver"); err == nil {
		if strings.Contains(strings.ToLower(output), "windows") {
			return models.OSWindows, nil
		}
	}

	return "", errors.New("discovery: could not detect remote operating system")
}
