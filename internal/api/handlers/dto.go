package handlers

import (
	"encoding/json"
	"time"

	checkservice "github.com/lenchik/logmonitor/internal/service/check"
	logfileservice "github.com/lenchik/logmonitor/internal/service/logfile"
	serverservice "github.com/lenchik/logmonitor/internal/service/server"
	"github.com/lenchik/logmonitor/models"
)

// discoverRequest describes the payload for an async discovery submission.
type discoverRequest struct {
	ServerID string `json:"server_id"`
}

// collectRequest describes the payload for an async collection submission.
type collectRequest struct {
	ServerID  string `json:"server_id"`
	LogFileID string `json:"log_file_id"`
}

// checkRunRequest describes the payload for an async integrity submission.
type checkRunRequest struct {
	ServerID  string `json:"server_id"`
	LogFileID string `json:"log_file_id"`
}

type logFileResponse struct {
	ID             string              `json:"id"`
	ServerID       string              `json:"server_id"`
	Path           string              `json:"path"`
	LogType        models.LogType      `json:"log_type"`
	FileIdentity   models.FileIdentity `json:"file_identity"`
	Meta           map[string]string   `json:"meta,omitempty"`
	LastScannedAt  *time.Time          `json:"last_scanned_at,omitempty"`
	LastLineNumber int64               `json:"last_line_number"`
	LastByteOffset int64               `json:"last_byte_offset"`
	IsActive       bool                `json:"is_active"`
	CreatedAt      time.Time           `json:"created_at"`
}

type logEntryResponse struct {
	ID          string    `json:"id"`
	LogFileID   string    `json:"log_file_id"`
	LineNumber  int64     `json:"line_number"`
	Content     string    `json:"content"`
	Hash        string    `json:"hash"`
	CollectedAt time.Time `json:"collected_at"`
}

type checkResultResponse struct {
	ID            string                 `json:"id"`
	LogFileID     string                 `json:"log_file_id"`
	CheckedAt     time.Time              `json:"checked_at"`
	Status        models.CheckStatus     `json:"status"`
	Severity      models.ProblemSeverity `json:"severity,omitempty"`
	ProblemType   models.ProblemType     `json:"problem_type,omitempty"`
	TotalLines    int64                  `json:"total_lines"`
	TamperedLines int64                  `json:"tampered_lines"`
	ErrorMessage  string                 `json:"error_message,omitempty"`
}

type tamperedEntryResponse struct {
	LineNumber     int64  `json:"line_number"`
	StoredHash     string `json:"stored_hash"`
	CurrentHash    string `json:"current_hash"`
	CurrentContent string `json:"current_content"`
}

type discoverResultResponse struct {
	LogFiles []logFileResponse `json:"log_files,omitempty"`
	Error    string            `json:"error,omitempty"`
}

type collectResultResponse struct {
	CollectedEntries int    `json:"collected_entries,omitempty"`
	Error            string `json:"error,omitempty"`
}

type runResultResponse struct {
	Result          *checkResultResponse    `json:"result,omitempty"`
	TamperedEntries []tamperedEntryResponse `json:"tampered_entries,omitempty"`
	Error           string                  `json:"error,omitempty"`
}

type systemProblemResponse struct {
	Severity   models.ProblemSeverity `json:"severity"`
	Type       models.ProblemType     `json:"type"`
	ServerID   string                 `json:"server_id,omitempty"`
	ServerName string                 `json:"server_name,omitempty"`
	LogFileID  string                 `json:"log_file_id,omitempty"`
	LogPath    string                 `json:"log_path,omitempty"`
	Message    string                 `json:"message"`
	DetectedAt time.Time              `json:"detected_at"`
}

type dashboardResponse struct {
	Servers        dashboardServerCounters  `json:"servers"`
	LogFiles       dashboardLogFileCounters `json:"log_files"`
	Problems       dashboardProblemCounters `json:"problems"`
	RecentProblems []systemProblemResponse  `json:"recent_problems"`
}

type dashboardServerCounters struct {
	Total    int `json:"total"`
	Active   int `json:"active"`
	Degraded int `json:"degraded"`
	Inactive int `json:"inactive"`
	Error    int `json:"error"`
}

type dashboardLogFileCounters struct {
	Total    int `json:"total"`
	Active   int `json:"active"`
	Inactive int `json:"inactive"`
}

type dashboardProblemCounters struct {
	Total    int `json:"total"`
	Warning  int `json:"warning"`
	Error    int `json:"error"`
	Critical int `json:"critical"`
}

type jobResponse struct {
	ID             string           `json:"id"`
	Type           models.JobType   `json:"type"`
	Status         models.JobStatus `json:"status"`
	IdempotencyKey string           `json:"idempotency_key,omitempty"`
	Fingerprint    string           `json:"fingerprint,omitempty"`
	ServerID       string           `json:"server_id,omitempty"`
	LogFileID      string           `json:"log_file_id,omitempty"`
	Error          string           `json:"error,omitempty"`
	Result         json.RawMessage  `json:"result,omitempty"`
	CreatedAt      time.Time        `json:"created_at"`
	StartedAt      *time.Time       `json:"started_at,omitempty"`
	FinishedAt     *time.Time       `json:"finished_at,omitempty"`
}

// logFileResponses converts log file domain models to API responses.
func logFileResponses(items []*models.LogFile) []logFileResponse {
	result := make([]logFileResponse, 0, len(items))
	for _, item := range items {
		result = append(result, newLogFileResponse(item))
	}
	return result
}

// newLogFileResponse converts one log file domain model to an API response.
func newLogFileResponse(logFile *models.LogFile) logFileResponse {
	return logFileResponse{
		ID:             logFile.ID,
		ServerID:       logFile.ServerID,
		Path:           logFile.Path,
		LogType:        logFile.LogType,
		FileIdentity:   logFile.FileIdentity,
		Meta:           cloneStringMap(logFile.Meta),
		LastScannedAt:  logFile.LastScannedAt,
		LastLineNumber: logFile.LastLineNumber,
		LastByteOffset: logFile.LastByteOffset,
		IsActive:       logFile.IsActive,
		CreatedAt:      logFile.CreatedAt,
	}
}

// logEntryResponses converts log entry domain models to API responses.
func logEntryResponses(items []*models.LogEntry) []logEntryResponse {
	result := make([]logEntryResponse, 0, len(items))
	for _, item := range items {
		result = append(result, newLogEntryResponse(item))
	}
	return result
}

// newLogEntryResponse converts one collected log entry to an API response.
func newLogEntryResponse(entry *models.LogEntry) logEntryResponse {
	return logEntryResponse{
		ID:          entry.ID,
		LogFileID:   entry.LogFileID,
		LineNumber:  entry.LineNumber,
		Content:     entry.Content,
		Hash:        entry.Hash,
		CollectedAt: entry.CollectedAt,
	}
}

// checkResultResponses converts integrity check domain models to API responses.
func checkResultResponses(items []*models.CheckResult) []checkResultResponse {
	result := make([]checkResultResponse, 0, len(items))
	for _, item := range items {
		result = append(result, newCheckResultResponse(item))
	}
	return result
}

// newCheckResultResponse converts one integrity check result to an API response.
func newCheckResultResponse(checkResult *models.CheckResult) checkResultResponse {
	return checkResultResponse{
		ID:            checkResult.ID,
		LogFileID:     checkResult.LogFileID,
		CheckedAt:     checkResult.CheckedAt,
		Status:        checkResult.Status,
		Severity:      models.SeverityForCheckStatus(checkResult.Status),
		ProblemType:   models.ProblemTypeForCheckStatus(checkResult.Status),
		TotalLines:    checkResult.TotalLines,
		TamperedLines: checkResult.TamperedLines,
		ErrorMessage:  checkResult.ErrorMessage,
	}
}

// tamperedEntryResponses converts tampering details to API responses.
func tamperedEntryResponses(items []models.TamperedEntry) []tamperedEntryResponse {
	result := make([]tamperedEntryResponse, 0, len(items))
	for _, item := range items {
		result = append(result, tamperedEntryResponse{
			LineNumber:     item.LineNumber,
			StoredHash:     item.StoredHash,
			CurrentHash:    item.CurrentHash,
			CurrentContent: item.CurrentContent,
		})
	}
	return result
}

// discoverResultResponses converts server discovery service results to API responses.
func discoverResultResponses(items map[string]serverservice.DiscoverResult) map[string]discoverResultResponse {
	result := make(map[string]discoverResultResponse, len(items))
	for serverID, item := range items {
		result[serverID] = discoverResultResponse{
			LogFiles: logFileResponses(item.LogFiles),
			Error:    item.Error,
		}
	}
	return result
}

// collectResultResponses converts log collection service results to API responses.
func collectResultResponses(items map[string]logfileservice.CollectResult) map[string]collectResultResponse {
	result := make(map[string]collectResultResponse, len(items))
	for logFileID, item := range items {
		result[logFileID] = collectResultResponse{
			CollectedEntries: item.CollectedEntries,
			Error:            item.Error,
		}
	}
	return result
}

// runResultResponses converts integrity run service results to API responses.
func runResultResponses(items map[string]checkservice.RunResult) map[string]runResultResponse {
	result := make(map[string]runResultResponse, len(items))
	for logFileID, item := range items {
		var checkResult *checkResultResponse
		if item.Result != nil {
			response := newCheckResultResponse(item.Result)
			checkResult = &response
		}

		result[logFileID] = runResultResponse{
			Result:          checkResult,
			TamperedEntries: tamperedEntryResponses(item.TamperedEntries),
			Error:           item.Error,
		}
	}
	return result
}

// systemProblemResponses converts service-level operational problems to transport DTOs.
func systemProblemResponses(items []models.SystemProblem) []systemProblemResponse {
	result := make([]systemProblemResponse, 0, len(items))
	for _, item := range items {
		result = append(result, systemProblemResponse{
			Severity:   item.Severity,
			Type:       item.Type,
			ServerID:   item.ServerID,
			ServerName: item.ServerName,
			LogFileID:  item.LogFileID,
			LogPath:    item.LogPath,
			Message:    item.Message,
			DetectedAt: item.DetectedAt,
		})
	}
	return result
}

// newDashboardResponse converts aggregated dashboard data to a transport DTO.
func newDashboardResponse(dashboard *serverservice.Dashboard) dashboardResponse {
	return dashboardResponse{
		Servers: dashboardServerCounters{
			Total:    dashboard.Servers.Total,
			Active:   dashboard.Servers.Active,
			Degraded: dashboard.Servers.Degraded,
			Inactive: dashboard.Servers.Inactive,
			Error:    dashboard.Servers.Error,
		},
		LogFiles: dashboardLogFileCounters{
			Total:    dashboard.LogFiles.Total,
			Active:   dashboard.LogFiles.Active,
			Inactive: dashboard.LogFiles.Inactive,
		},
		Problems: dashboardProblemCounters{
			Total:    dashboard.Problems.Total,
			Warning:  dashboard.Problems.Warning,
			Error:    dashboard.Problems.Error,
			Critical: dashboard.Problems.Critical,
		},
		RecentProblems: systemProblemResponses(dashboard.RecentProblems),
	}
}

// jobResponses converts async job models to transport DTOs.
func jobResponses(items []*models.Job) []jobResponse {
	result := make([]jobResponse, 0, len(items))
	for _, item := range items {
		result = append(result, newJobResponse(item))
	}
	return result
}

// newJobResponse converts one async job model to a transport DTO.
func newJobResponse(job *models.Job) jobResponse {
	return jobResponse{
		ID:             job.ID,
		Type:           job.Type,
		Status:         job.Status,
		IdempotencyKey: job.IdempotencyKey,
		Fingerprint:    job.Fingerprint,
		ServerID:       job.ServerID,
		LogFileID:      job.LogFileID,
		Error:          job.Error,
		Result:         cloneRawJSON(job.Result),
		CreatedAt:      job.CreatedAt,
		StartedAt:      cloneTimePointer(job.StartedAt),
		FinishedAt:     cloneTimePointer(job.FinishedAt),
	}
}

// cloneStringMap protects response DTOs from sharing mutable maps with domain models.
func cloneStringMap(input map[string]string) map[string]string {
	if input == nil {
		return nil
	}
	result := make(map[string]string, len(input))
	for key, value := range input {
		result[key] = value
	}
	return result
}

// cloneRawJSON protects response payloads from sharing mutable JSON slices with job history.
func cloneRawJSON(input json.RawMessage) json.RawMessage {
	if input == nil {
		return nil
	}
	result := make(json.RawMessage, len(input))
	copy(result, input)
	return result
}

// cloneTimePointer allocates an independent copy of an optional timestamp pointer.
func cloneTimePointer(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	result := *value
	return &result
}
