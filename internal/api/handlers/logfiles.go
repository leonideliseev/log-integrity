package handlers

import (
	"context"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	jobqueue "github.com/lenchik/logmonitor/internal/jobs"
	logfileservice "github.com/lenchik/logmonitor/internal/service/logfile"
	"github.com/lenchik/logmonitor/models"
)

// LogFileHandler handles HTTP requests related to remote log files.
type LogFileHandler struct {
	service *logfileservice.Service
	jobs    *jobqueue.Manager
}

// NewLogFileHandler creates a log file handler with required dependencies.
func NewLogFileHandler(service *logfileservice.Service, jobs *jobqueue.Manager) *LogFileHandler {
	return &LogFileHandler{service: service, jobs: jobs}
}

// List returns active log files or log files of a concrete server.
func (h *LogFileHandler) List(c *gin.Context) {
	serverID := c.Query("server_id")
	items, err := h.service.List(c.Request.Context(), serverID)
	if err != nil {
		writeServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, logFileResponses(items))
}

// Collect reads remote log files and persists newly discovered entries.
func (h *LogFileHandler) Collect(c *gin.Context) {
	type request struct {
		ServerID  string `json:"server_id"`
		LogFileID string `json:"log_file_id"`
	}

	var payload request
	if err := c.ShouldBindJSON(&payload); err != nil {
		writeError(c, http.StatusBadRequest, err.Error())
		return
	}
	payload.ServerID = strings.TrimSpace(payload.ServerID)
	payload.LogFileID = strings.TrimSpace(payload.LogFileID)
	if payload.ServerID == "" {
		writeError(c, http.StatusBadRequest, "server_id is required")
		return
	}

	job, _, err := h.jobs.Submit(jobqueue.TaskSpec{
		Type:           models.JobTypeCollect,
		IdempotencyKey: idempotencyKeyFromHeader(c),
		Fingerprint:    collectFingerprint(payload.ServerID, payload.LogFileID),
		ServerID:       payload.ServerID,
		LogFileID:      payload.LogFileID,
		Run: func(ctx context.Context) (any, error) {
			result, err := h.service.Collect(ctx, payload.ServerID, payload.LogFileID)
			if err != nil {
				return nil, err
			}
			return collectResultResponses(result), nil
		},
	})
	if err != nil {
		writeJobError(c, err)
		return
	}
	c.Header("Location", "/api/jobs/"+job.ID)
	c.JSON(http.StatusAccepted, newJobResponse(job))
}

// collectFingerprint keeps repeated manual collection requests idempotent while the same work is still active.
func collectFingerprint(serverID, logFileID string) string {
	if logFileID == "" {
		return "collect:" + serverID + ":all"
	}
	return "collect:" + serverID + ":" + logFileID
}
