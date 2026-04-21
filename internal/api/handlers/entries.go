package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	entryservice "github.com/lenchik/logmonitor/internal/service/entry"
)

// EntryHandler handles read-only operations for collected log entries.
type EntryHandler struct {
	service *entryservice.Service
}

// NewEntryHandler creates an entry handler with service dependency.
func NewEntryHandler(service *entryservice.Service) *EntryHandler {
	return &EntryHandler{service: service}
}

// List returns stored log entries for one log file with pagination.
func (h *EntryHandler) List(c *gin.Context) {
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
	c.JSON(http.StatusOK, logEntryResponses(items))
}
