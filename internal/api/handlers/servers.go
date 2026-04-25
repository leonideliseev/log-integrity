package handlers

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	jobqueue "github.com/lenchik/logmonitor/internal/jobs"
	"github.com/lenchik/logmonitor/internal/repository"
	serverservice "github.com/lenchik/logmonitor/internal/service/server"
	"github.com/lenchik/logmonitor/models"
)

// ServerHandler handles HTTP requests related to monitored servers.
type ServerHandler struct {
	service *serverservice.Service
	jobs    *jobqueue.Manager
}

// createServerRequest describes the payload used to register a new monitored server.
type createServerRequest struct {
	Name      string `json:"name"`
	Host      string `json:"host"`
	Port      int    `json:"port"`
	Username  string `json:"username"`
	AuthType  string `json:"auth_type"`
	AuthValue string `json:"auth_value"`
	OSType    string `json:"os_type"`
}

// updateServerRequest describes the payload used to overwrite an API-managed server.
type updateServerRequest struct {
	Name      string `json:"name"`
	Host      string `json:"host"`
	Port      int    `json:"port"`
	Username  string `json:"username"`
	AuthType  string `json:"auth_type"`
	AuthValue string `json:"auth_value"`
	OSType    string `json:"os_type"`
	Status    string `json:"status"`
}

// serverResponse describes the public representation of a monitored server.
type serverResponse struct {
	ID           string                 `json:"id"`
	Name         string                 `json:"name"`
	Host         string                 `json:"host"`
	Port         int                    `json:"port"`
	Username     string                 `json:"username"`
	AuthType     models.AuthType        `json:"auth_type"`
	OSType       models.OSType          `json:"os_type"`
	Status       models.ServerStatus    `json:"status"`
	ManagedBy    models.ServerManagedBy `json:"managed_by"`
	SuccessCount int64                  `json:"success_count"`
	FailureCount int64                  `json:"failure_count"`
	LastError    string                 `json:"last_error,omitempty"`
	LastSeenAt   *time.Time             `json:"last_seen_at,omitempty"`
	BackoffUntil *time.Time             `json:"backoff_until,omitempty"`
	CreatedAt    time.Time              `json:"created_at"`
	UpdatedAt    time.Time              `json:"updated_at"`
}

// serverPayload groups server fields so validation is shared between create and update.
type serverPayload struct {
	Name                string
	Host                string
	Port                int
	Username            string
	AuthType            string
	AuthValue           string
	OSType              string
	Status              string
	DefaultStatus       models.ServerStatus
	DefaultManagedBy    models.ServerManagedBy
	RequireAuthValue    bool
	AllowEmptyAuthValue bool
	AllowEmptyStatus    bool
}

// NewServerHandler creates a server handler with required dependencies.
func NewServerHandler(service *serverservice.Service, jobs *jobqueue.Manager) *ServerHandler {
	return &ServerHandler{service: service, jobs: jobs}
}

// List returns all registered servers.
func (h *ServerHandler) List(c *gin.Context) {
	items, err := h.service.List(c.Request.Context())
	if err != nil {
		writeError(c, http.StatusInternalServerError, err.Error())
		return
	}
	c.JSON(http.StatusOK, serverResponses(items))
}

// Dashboard returns aggregated counters for future UI screens.
func (h *ServerHandler) Dashboard(c *gin.Context) {
	dashboard, err := h.service.Dashboard(c.Request.Context())
	if err != nil {
		writeServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, newDashboardResponse(dashboard))
}

// ListProblems returns aggregated operational issues across servers and log files.
func (h *ServerHandler) ListProblems(c *gin.Context) {
	items, err := h.service.ListProblems(c.Request.Context())
	if err != nil {
		writeServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, systemProblemResponses(items))
}

// Get returns one registered server by identifier.
func (h *ServerHandler) Get(c *gin.Context) {
	serverID, ok := serverIDFromPath(c)
	if !ok {
		return
	}

	serverModel, err := h.service.Get(c.Request.Context(), serverID)
	if err != nil {
		writeServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, newServerResponse(serverModel))
}

// Create registers a new server from JSON payload.
func (h *ServerHandler) Create(c *gin.Context) {
	var payload createServerRequest
	if err := c.ShouldBindJSON(&payload); err != nil {
		writeError(c, http.StatusBadRequest, err.Error())
		return
	}

	serverModel, err := payload.toModel()
	if err != nil {
		writeError(c, http.StatusBadRequest, err.Error())
		return
	}
	if err := h.service.Create(c.Request.Context(), serverModel); err != nil {
		writeServiceError(c, err)
		return
	}
	c.JSON(http.StatusCreated, newServerResponse(serverModel))
}

// Retry clears temporary failure state so operators can retry a server immediately.
func (h *ServerHandler) Retry(c *gin.Context) {
	serverID, ok := serverIDFromPath(c)
	if !ok {
		return
	}

	serverModel, err := h.service.Retry(c.Request.Context(), serverID)
	if err != nil {
		writeServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, newServerResponse(serverModel))
}

// Update overwrites one API-managed server from JSON payload.
func (h *ServerHandler) Update(c *gin.Context) {
	serverID, ok := serverIDFromPath(c)
	if !ok {
		return
	}

	var payload updateServerRequest
	if err := c.ShouldBindJSON(&payload); err != nil {
		writeError(c, http.StatusBadRequest, err.Error())
		return
	}

	serverModel, err := payload.toModel(serverID)
	if err != nil {
		writeError(c, http.StatusBadRequest, err.Error())
		return
	}
	if err := h.service.Update(c.Request.Context(), serverModel); err != nil {
		writeServiceError(c, err)
		return
	}

	updatedServer, err := h.service.Get(c.Request.Context(), serverID)
	if err != nil {
		writeServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, newServerResponse(updatedServer))
}

// Delete removes one API-managed server and all related monitoring data.
func (h *ServerHandler) Delete(c *gin.Context) {
	serverID, ok := serverIDFromPath(c)
	if !ok {
		return
	}

	if err := h.service.Delete(c.Request.Context(), serverID); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			c.Status(http.StatusNoContent)
			return
		}
		writeServiceError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// Discover runs log discovery for one server or for all servers.
func (h *ServerHandler) Discover(c *gin.Context) {
	type request struct {
		ServerID string `json:"server_id"`
	}

	var payload request
	if c.Request.ContentLength > 0 {
		if err := c.ShouldBindJSON(&payload); err != nil {
			writeError(c, http.StatusBadRequest, err.Error())
			return
		}
	}
	payload.ServerID = strings.TrimSpace(payload.ServerID)

	job, _, err := h.jobs.Submit(jobqueue.TaskSpec{
		Type:           models.JobTypeDiscover,
		IdempotencyKey: idempotencyKeyFromHeader(c),
		Fingerprint:    discoverFingerprint(payload.ServerID),
		ServerID:       payload.ServerID,
		Run: func(ctx context.Context) (any, error) {
			items, err := h.service.Discover(ctx, payload.ServerID)
			if err != nil {
				return nil, err
			}
			return discoverResultResponses(items), nil
		},
	})
	if err != nil {
		writeJobError(c, err)
		return
	}

	c.Header("Location", "/api/jobs/"+job.ID)
	c.JSON(http.StatusAccepted, newJobResponse(job))
}

// discoverFingerprint keeps repeated manual discovery requests idempotent while a job is queued or running.
func discoverFingerprint(serverID string) string {
	if serverID == "" {
		return "discover:all"
	}
	return "discover:" + serverID
}

// toModel validates and converts a create payload to a domain model.
func (r createServerRequest) toModel() (*models.Server, error) {
	return validateServerPayload(serverPayload{
		Name:             r.Name,
		Host:             r.Host,
		Port:             r.Port,
		Username:         r.Username,
		AuthType:         r.AuthType,
		AuthValue:        r.AuthValue,
		OSType:           r.OSType,
		DefaultStatus:    models.ServerStatusActive,
		DefaultManagedBy: models.ServerManagedByAPI,
		RequireAuthValue: true,
	})
}

// toModel validates and converts an update payload to a domain model.
func (r updateServerRequest) toModel(serverID string) (*models.Server, error) {
	serverModel, err := validateServerPayload(serverPayload{
		Name:                r.Name,
		Host:                r.Host,
		Port:                r.Port,
		Username:            r.Username,
		AuthType:            r.AuthType,
		AuthValue:           r.AuthValue,
		OSType:              r.OSType,
		Status:              r.Status,
		AllowEmptyAuthValue: true,
		AllowEmptyStatus:    true,
	})
	if err != nil {
		return nil, err
	}
	serverModel.ID = serverID
	return serverModel, nil
}

// validateServerPayload validates handler input and converts it into a domain server model.
func validateServerPayload(payload serverPayload) (*models.Server, error) {
	payload.Name = strings.TrimSpace(payload.Name)
	payload.Host = strings.TrimSpace(payload.Host)
	payload.Username = strings.TrimSpace(payload.Username)
	payload.AuthType = strings.TrimSpace(payload.AuthType)
	payload.AuthValue = strings.TrimSpace(payload.AuthValue)
	payload.OSType = strings.TrimSpace(payload.OSType)
	payload.Status = strings.TrimSpace(payload.Status)

	if payload.Name == "" {
		return nil, fmt.Errorf("name is required")
	}
	if payload.Host == "" {
		return nil, fmt.Errorf("host is required")
	}
	if payload.Username == "" {
		return nil, fmt.Errorf("username is required")
	}
	if payload.Port < 0 || payload.Port > 65535 {
		return nil, fmt.Errorf("port must be between 1 and 65535")
	}
	if payload.Port == 0 {
		payload.Port = 22
	}
	if payload.AuthType != string(models.AuthPassword) && payload.AuthType != string(models.AuthKey) {
		return nil, fmt.Errorf("auth_type must be either password or key")
	}
	if payload.RequireAuthValue && payload.AuthValue == "" {
		return nil, fmt.Errorf("auth_value is required")
	}
	if payload.OSType != "" && payload.OSType != string(models.OSLinux) && payload.OSType != string(models.OSWindows) && payload.OSType != string(models.OSMacOS) {
		return nil, fmt.Errorf("os_type must be empty, linux, windows or macos")
	}
	if payload.Status != "" && payload.Status != string(models.ServerStatusActive) && payload.Status != string(models.ServerStatusInactive) {
		return nil, fmt.Errorf("status must be empty, active or inactive")
	}
	if payload.Status == "" && !payload.AllowEmptyStatus {
		payload.Status = string(payload.DefaultStatus)
	}
	if payload.AuthValue == "" && !payload.AllowEmptyAuthValue && !payload.RequireAuthValue {
		return nil, fmt.Errorf("auth_value is required")
	}

	return &models.Server{
		Name:      payload.Name,
		Host:      payload.Host,
		Port:      payload.Port,
		Username:  payload.Username,
		AuthType:  models.AuthType(payload.AuthType),
		AuthValue: payload.AuthValue,
		OSType:    models.OSType(payload.OSType),
		Status:    models.ServerStatus(payload.Status),
		ManagedBy: payload.DefaultManagedBy,
	}, nil
}

// serverIDFromPath extracts and validates the path identifier for one server.
func serverIDFromPath(c *gin.Context) (string, bool) {
	serverID := strings.TrimSpace(c.Param("id"))
	if serverID == "" {
		writeError(c, http.StatusBadRequest, "server id is required")
		return "", false
	}
	return serverID, true
}

// serverResponses converts domain servers to transport DTOs.
func serverResponses(items []*models.Server) []serverResponse {
	result := make([]serverResponse, 0, len(items))
	for _, item := range items {
		result = append(result, newServerResponse(item))
	}
	return result
}

// newServerResponse converts one domain server to an API response payload.
func newServerResponse(serverModel *models.Server) serverResponse {
	return serverResponse{
		ID:           serverModel.ID,
		Name:         serverModel.Name,
		Host:         serverModel.Host,
		Port:         serverModel.Port,
		Username:     serverModel.Username,
		AuthType:     serverModel.AuthType,
		OSType:       serverModel.OSType,
		Status:       serverModel.Status,
		ManagedBy:    serverModel.ManagedBy,
		SuccessCount: serverModel.SuccessCount,
		FailureCount: serverModel.FailureCount,
		LastError:    serverModel.LastError,
		LastSeenAt:   serverModel.LastSeenAt,
		BackoffUntil: serverModel.BackoffUntil,
		CreatedAt:    serverModel.CreatedAt,
		UpdatedAt:    serverModel.UpdatedAt,
	}
}
