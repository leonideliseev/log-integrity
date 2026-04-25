// Package handlers contains Gin HTTP handlers for the API.
package handlers

import (
	"context"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	jobqueue "github.com/lenchik/logmonitor/internal/jobs"
	checkservice "github.com/lenchik/logmonitor/internal/service/check"
	"github.com/lenchik/logmonitor/models"
)

// CheckHandler handles integrity check history and manual check execution.
type CheckHandler struct {
	service *checkservice.Service
	jobs    *jobqueue.Manager
}

// NewCheckHandler creates a check handler with service dependency.
func NewCheckHandler(service *checkservice.Service, jobs *jobqueue.Manager) *CheckHandler {
	return &CheckHandler{service: service, jobs: jobs}
}

// List returns stored integrity check results for a log file.
func (h *CheckHandler) List(c *gin.Context) {
	logFileID := c.Query("log_file_id")
	if logFileID == "" {
		writeError(c, http.StatusBadRequest, "log_file_id is required")
		return
	}

	offset, err := parseIntQuery(c, "offset", 0)
	if err != nil {
		writeError(c, http.StatusBadRequest, err.Error())
		return
	}
	limit, err := parseIntQuery(c, "limit", 100)
	if err != nil {
		writeError(c, http.StatusBadRequest, err.Error())
		return
	}
	if err := validatePageLimit(limit); err != nil {
		writeError(c, http.StatusBadRequest, err.Error())
		return
	}

	items, err := h.service.List(c.Request.Context(), logFileID, offset, limit)
	if err != nil {
		writeServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, checkResultResponses(items))
}

// Run launches integrity checks for one log file or for all server log files.
func (h *CheckHandler) Run(c *gin.Context) {
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
		Type:           models.JobTypeIntegrity,
		IdempotencyKey: idempotencyKeyFromHeader(c),
		Fingerprint:    integrityFingerprint(payload.ServerID, payload.LogFileID),
		ServerID:       payload.ServerID,
		LogFileID:      payload.LogFileID,
		Run: func(ctx context.Context) (any, error) {
			result, err := h.service.Run(ctx, payload.ServerID, payload.LogFileID)
			if err != nil {
				return nil, err
			}
			return runResultResponses(result), nil
		},
	})
	if err != nil {
		writeJobError(c, err)
		return
	}
	c.Header("Location", "/api/jobs/"+job.ID)
	c.JSON(http.StatusAccepted, newJobResponse(job))
}

// integrityFingerprint keeps repeated manual integrity runs idempotent while identical work is still active.
func integrityFingerprint(serverID, logFileID string) string {
	if logFileID == "" {
		return "integrity:" + serverID + ":all"
	}
	return "integrity:" + serverID + ":" + logFileID
}
