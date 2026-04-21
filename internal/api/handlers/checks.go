// Package handlers contains Gin HTTP handlers for the API.
package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	checkservice "github.com/lenchik/logmonitor/internal/service/check"
)

// CheckHandler handles integrity check history and manual check execution.
type CheckHandler struct {
	service *checkservice.Service
}

// NewCheckHandler creates a check handler with service dependency.
func NewCheckHandler(service *checkservice.Service) *CheckHandler {
	return &CheckHandler{service: service}
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
	if payload.ServerID == "" {
		writeError(c, http.StatusBadRequest, "server_id is required")
		return
	}

	result, err := h.service.Run(c.Request.Context(), payload.ServerID, payload.LogFileID)
	if err != nil {
		writeServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, runResultResponses(result))
}
