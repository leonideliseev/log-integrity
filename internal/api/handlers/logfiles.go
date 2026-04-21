package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	logfileservice "github.com/lenchik/logmonitor/internal/service/logfile"
)

// LogFileHandler handles HTTP requests related to remote log files.
type LogFileHandler struct {
	service *logfileservice.Service
}

// NewLogFileHandler creates a log file handler with required dependencies.
func NewLogFileHandler(service *logfileservice.Service) *LogFileHandler {
	return &LogFileHandler{service: service}
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
	if payload.ServerID == "" {
		writeError(c, http.StatusBadRequest, "server_id is required")
		return
	}

	result, err := h.service.Collect(c.Request.Context(), payload.ServerID, payload.LogFileID)
	if err != nil {
		writeServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, collectResultResponses(result))
}
