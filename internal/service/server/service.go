// Package server contains application operations for monitored servers.
package server

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/lenchik/logmonitor/internal/repository"
	discoveryservice "github.com/lenchik/logmonitor/internal/service/discovery"
	healthservice "github.com/lenchik/logmonitor/internal/service/health"
	"github.com/lenchik/logmonitor/models"
)

// DiscoverResult contains discovery output for one server.
type DiscoverResult struct {
	LogFiles []*models.LogFile `json:"log_files,omitempty"`
	Error    string            `json:"error,omitempty"`
}

// Dashboard contains aggregated counters useful for web dashboards.
type Dashboard struct {
	Servers        Overview               `json:"servers"`
	LogFiles       LogFileOverview        `json:"log_files"`
	Problems       ProblemOverview        `json:"problems"`
	RecentProblems []models.SystemProblem `json:"recent_problems"`
}

// Overview summarizes monitored servers by lifecycle state.
type Overview struct {
	Total    int `json:"total"`
	Active   int `json:"active"`
	Degraded int `json:"degraded"`
	Inactive int `json:"inactive"`
	Error    int `json:"error"`
}

// LogFileOverview summarizes discovered log file inventory.
type LogFileOverview struct {
	Total    int `json:"total"`
	Active   int `json:"active"`
	Inactive int `json:"inactive"`
}

// ProblemOverview summarizes currently visible operator problems by severity.
type ProblemOverview struct {
	Total    int `json:"total"`
	Warning  int `json:"warning"`
	Error    int `json:"error"`
	Critical int `json:"critical"`
}

// Service provides server-related application operations for the HTTP layer.
type Service struct {
	servers   repository.ServerRepository
	discovery *discoveryservice.Service
	health    *healthservice.Service
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
	return NewServiceWithHealthAndLocker(servers, discovery, healthservice.NewService(servers, healthservice.Options{}), locker)
}

// NewServiceWithHealthAndLocker creates a server application service with health tracking and optional isolation.
func NewServiceWithHealthAndLocker(servers repository.ServerRepository, discovery *discoveryservice.Service, health *healthservice.Service, locker Locker) *Service {
	return &Service{
		servers:   servers,
		discovery: discovery,
		health:    health,
		locker:    locker,
	}
}

// List returns all registered servers.
func (s *Service) List(ctx context.Context) ([]*models.Server, error) {
	return s.servers.ListServers(ctx)
}

// ListFiltered returns a filtered server page for API list screens.
func (s *Service) ListFiltered(ctx context.Context, filter repository.ServerListFilter) (repository.Page[*models.Server], error) {
	return s.servers.ListServersFiltered(ctx, filter)
}

// Get returns one registered server by identifier.
func (s *Service) Get(ctx context.Context, serverID string) (*models.Server, error) {
	return s.servers.GetServerByID(ctx, serverID)
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

// Update overwrites an API-managed server while preserving internal lifecycle fields.
func (s *Service) Update(ctx context.Context, serverModel *models.Server) error {
	current, err := s.servers.GetServerByID(ctx, serverModel.ID)
	if err != nil {
		return err
	}
	if err := ensureAPIServerMutable(current); err != nil {
		return err
	}

	if serverModel.Port == 0 {
		serverModel.Port = 22
	}
	if serverModel.AuthValue == "" {
		serverModel.AuthValue = current.AuthValue
	}
	if serverModel.Status == "" {
		serverModel.Status = current.Status
	}

	serverModel.ManagedBy = current.ManagedBy
	serverModel.CreatedAt = current.CreatedAt
	serverModel.SuccessCount = current.SuccessCount
	serverModel.FailureCount = current.FailureCount
	serverModel.LastError = current.LastError
	serverModel.LastSeenAt = current.LastSeenAt
	serverModel.BackoffUntil = current.BackoffUntil

	if err := s.ensureUniqueIdentity(ctx, serverModel, current.ID); err != nil {
		return err
	}
	return s.servers.UpdateServer(ctx, serverModel)
}

// Delete removes an API-managed server and all dependent monitored data.
func (s *Service) Delete(ctx context.Context, serverID string) error {
	serverModel, err := s.servers.GetServerByID(ctx, serverID)
	if err != nil {
		return err
	}
	if err := ensureAPIServerMutable(serverModel); err != nil {
		return err
	}
	return s.servers.DeleteServer(ctx, serverID)
}

// Retry clears temporary failure state so scheduled or manual operations can run immediately.
func (s *Service) Retry(ctx context.Context, serverID string) (*models.Server, error) {
	if err := s.health.ClearBackoff(ctx, serverID); err != nil {
		return nil, err
	}
	return s.servers.GetServerByID(ctx, serverID)
}

// Dashboard returns aggregated data that a future UI can render with few API calls.
func (s *Service) Dashboard(ctx context.Context) (*Dashboard, error) {
	logFiles, _, err := s.problemRepositories()
	if err != nil {
		return nil, err
	}

	servers, err := s.servers.ListServers(ctx)
	if err != nil {
		return nil, err
	}

	problems, err := s.ListProblems(ctx)
	if err != nil {
		return nil, err
	}

	dashboard := &Dashboard{}
	for _, serverModel := range servers {
		dashboard.Servers.Total++
		switch serverModel.Status {
		case models.ServerStatusActive:
			dashboard.Servers.Active++
		case models.ServerStatusDegraded:
			dashboard.Servers.Degraded++
		case models.ServerStatusInactive:
			dashboard.Servers.Inactive++
		case models.ServerStatusError:
			dashboard.Servers.Error++
		}

		items, err := logFiles.ListLogFilesByServer(ctx, serverModel.ID)
		if err != nil {
			return nil, err
		}
		for _, logFile := range items {
			dashboard.LogFiles.Total++
			if logFile.IsActive {
				dashboard.LogFiles.Active++
			} else {
				dashboard.LogFiles.Inactive++
			}
		}
	}

	for _, problem := range problems {
		dashboard.Problems.Total++
		switch problem.Severity {
		case models.ProblemSeverityWarning:
			dashboard.Problems.Warning++
		case models.ProblemSeverityError:
			dashboard.Problems.Error++
		case models.ProblemSeverityCritical:
			dashboard.Problems.Critical++
		}
	}

	if len(problems) > 10 {
		dashboard.RecentProblems = append(dashboard.RecentProblems, problems[:10]...)
	} else {
		dashboard.RecentProblems = append(dashboard.RecentProblems, problems...)
	}

	return dashboard, nil
}

// ListProblems returns aggregated operational issues for servers and their latest integrity checks.
func (s *Service) ListProblems(ctx context.Context) ([]models.SystemProblem, error) {
	logFiles, checks, err := s.problemRepositories()
	if err != nil {
		return nil, err
	}

	servers, err := s.servers.ListServers(ctx)
	if err != nil {
		return nil, err
	}

	problems := make([]models.SystemProblem, 0)
	for _, serverModel := range servers {
		problems = append(problems, serverHealthProblems(serverModel)...)

		serverLogFiles, err := logFiles.ListLogFilesByServer(ctx, serverModel.ID)
		if err != nil {
			return nil, err
		}
		for _, logFile := range serverLogFiles {
			if !logFile.IsActive {
				continue
			}

			latestCheck, err := checks.GetLatestCheckResult(ctx, logFile.ID)
			if err != nil {
				if errors.Is(err, repository.ErrNotFound) {
					continue
				}
				return nil, err
			}

			problemType := models.ProblemTypeForCheckStatus(latestCheck.Status)
			severity := models.SeverityForCheckStatus(latestCheck.Status)
			if problemType == "" || severity == "" {
				continue
			}

			problems = append(problems, models.SystemProblem{
				Severity:   severity,
				Type:       problemType,
				ServerID:   serverModel.ID,
				ServerName: serverModel.Name,
				LogFileID:  logFile.ID,
				LogPath:    logFile.Path,
				Message:    checkProblemMessage(latestCheck, logFile.Path),
				DetectedAt: latestCheck.CheckedAt,
			})
		}
	}

	sort.SliceStable(problems, func(i, j int) bool {
		left := severityRank(problems[i].Severity)
		right := severityRank(problems[j].Severity)
		if left != right {
			return left > right
		}
		return problems[i].DetectedAt.After(problems[j].DetectedAt)
	})

	return problems, nil
}

// ListProblemsFiltered returns a filtered problem page for API list screens.
func (s *Service) ListProblemsFiltered(ctx context.Context, filter ProblemListFilter) (repository.Page[models.SystemProblem], error) {
	problems, err := s.ListProblems(ctx)
	if err != nil {
		return repository.Page[models.SystemProblem]{}, err
	}

	filtered := make([]models.SystemProblem, 0, len(problems))
	for _, problem := range problems {
		if filter.Severity != "" && problem.Severity != filter.Severity {
			continue
		}
		if filter.Type != "" && problem.Type != filter.Type {
			continue
		}
		if filter.ServerID != "" && problem.ServerID != filter.ServerID {
			continue
		}
		if !problemMatchesQuery(problem, filter.Q) {
			continue
		}
		filtered = append(filtered, problem)
	}

	sort.SliceStable(filtered, func(i, j int) bool {
		less := compareProblems(filtered[i], filtered[j], filter.Sort)
		if strings.EqualFold(filter.Order, "asc") {
			return less
		}
		return compareProblems(filtered[j], filtered[i], filter.Sort)
	})

	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}
	limit := filter.Limit
	if limit <= 0 {
		limit = 50
	}
	end := len(filtered)
	if offset >= len(filtered) {
		return repository.Page[models.SystemProblem]{Items: []models.SystemProblem{}, Total: len(filtered), Offset: offset, Limit: limit}, nil
	}
	if offset+limit < end {
		end = offset + limit
	}
	return repository.Page[models.SystemProblem]{
		Items:  append([]models.SystemProblem{}, filtered[offset:end]...),
		Total:  len(filtered),
		Offset: offset,
		Limit:  limit,
	}, nil
}

// ProblemListFilter narrows aggregated problem list reads.
type ProblemListFilter struct {
	repository.ListOptions
	Severity models.ProblemSeverity
	Type     models.ProblemType
	ServerID string
}

func problemMatchesQuery(problem models.SystemProblem, query string) bool {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return true
	}
	values := []string{string(problem.Severity), string(problem.Type), problem.ServerID, problem.ServerName, problem.LogFileID, problem.LogPath, problem.Message}
	for _, value := range values {
		if strings.Contains(strings.ToLower(value), query) {
			return true
		}
	}
	return false
}

func compareProblems(left, right models.SystemProblem, sortBy string) bool {
	switch sortBy {
	case "detected_at":
		return left.DetectedAt.Before(right.DetectedAt)
	case "server":
		leftServer := left.ServerName
		if leftServer == "" {
			leftServer = left.ServerID
		}
		rightServer := right.ServerName
		if rightServer == "" {
			rightServer = right.ServerID
		}
		return leftServer < rightServer
	default:
		leftRank := severityRank(left.Severity)
		rightRank := severityRank(right.Severity)
		if leftRank == rightRank {
			return left.DetectedAt.Before(right.DetectedAt)
		}
		return leftRank < rightRank
	}
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
			_ = s.health.RecordFailure(ctx, serverModel, discoverErr)
			result[serverModel.ID] = DiscoverResult{Error: discoverErr.Error()}
			continue
		}
		_ = s.health.RecordSuccess(ctx, serverModel.ID)
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

// ensureAPIServerMutable blocks API updates for servers seeded from configuration.
func ensureAPIServerMutable(serverModel *models.Server) error {
	if serverModel.ManagedBy == "" || serverModel.ManagedBy == models.ServerManagedByConfig {
		return fmt.Errorf(
			"%w: server %q is managed by config and cannot be changed via api",
			repository.ErrConflict,
			serverModel.ID,
		)
	}
	return nil
}

// problemRepositories resolves optional storage dependencies required for the operational problems endpoint.
func (s *Service) problemRepositories() (repository.LogFileRepository, repository.CheckResultRepository, error) {
	logFiles, ok := s.servers.(repository.LogFileRepository)
	if !ok {
		return nil, nil, fmt.Errorf("server: log file repository is not configured")
	}
	checks, ok := s.servers.(repository.CheckResultRepository)
	if !ok {
		return nil, nil, fmt.Errorf("server: check result repository is not configured")
	}
	return logFiles, checks, nil
}

// serverHealthProblems converts one server lifecycle state into operator-facing problem items.
func serverHealthProblems(serverModel *models.Server) []models.SystemProblem {
	problems := make([]models.SystemProblem, 0, 2)

	switch serverModel.Status {
	case models.ServerStatusError:
		problems = append(problems, models.SystemProblem{
			Severity:   models.ProblemSeverityError,
			Type:       models.ProblemTypeServerUnreachable,
			ServerID:   serverModel.ID,
			ServerName: serverModel.Name,
			Message:    serverProblemMessage(serverModel, "server is unavailable"),
			DetectedAt: problemDetectedAt(serverModel),
		})
	case models.ServerStatusDegraded:
		problems = append(problems, models.SystemProblem{
			Severity:   models.ProblemSeverityWarning,
			Type:       models.ProblemTypeServerDegraded,
			ServerID:   serverModel.ID,
			ServerName: serverModel.Name,
			Message:    serverProblemMessage(serverModel, "server is reachable but partially unhealthy"),
			DetectedAt: problemDetectedAt(serverModel),
		})
	}

	if serverModel.BackoffUntil != nil {
		problems = append(problems, models.SystemProblem{
			Severity:   models.ProblemSeverityWarning,
			Type:       models.ProblemTypeServerBackoff,
			ServerID:   serverModel.ID,
			ServerName: serverModel.Name,
			Message:    fmt.Sprintf("server is temporarily paused until %s", serverModel.BackoffUntil.UTC().Format(time.RFC3339)),
			DetectedAt: problemDetectedAt(serverModel),
		})
	}

	return problems
}

// serverProblemMessage prefers the latest stored server error details when they are available.
func serverProblemMessage(serverModel *models.Server, fallback string) string {
	if strings.TrimSpace(serverModel.LastError) != "" {
		return serverModel.LastError
	}
	return fallback
}

// checkProblemMessage formats the latest integrity issue for one log file.
func checkProblemMessage(result *models.CheckResult, path string) string {
	switch result.Status {
	case models.CheckStatusTampered:
		return fmt.Sprintf("integrity violation detected in %q: %d tampered lines", path, result.TamperedLines)
	case models.CheckStatusError:
		if result.ErrorMessage != "" {
			return result.ErrorMessage
		}
		return fmt.Sprintf("integrity check failed for %q", path)
	default:
		return ""
	}
}

// problemDetectedAt picks the most recent server lifecycle timestamp available for the problem list.
func problemDetectedAt(serverModel *models.Server) time.Time {
	if !serverModel.UpdatedAt.IsZero() {
		return serverModel.UpdatedAt
	}
	if serverModel.LastSeenAt != nil {
		return *serverModel.LastSeenAt
	}
	return serverModel.CreatedAt
}

// severityRank sorts critical issues above errors and warnings in the aggregated problem response.
func severityRank(severity models.ProblemSeverity) int {
	switch severity {
	case models.ProblemSeverityCritical:
		return 3
	case models.ProblemSeverityError:
		return 2
	case models.ProblemSeverityWarning:
		return 1
	default:
		return 0
	}
}
