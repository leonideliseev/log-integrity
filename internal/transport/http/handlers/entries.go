package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/lenchik/logmonitor/internal/repository"
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

// List godoc
// @Summary List stored log entries
// @Tags entries
// @Produce json
// @Security ApiKeyAuth
// @Param log_file_id query string true "Log file identifier"
// @Param offset query int false "Pagination offset"
// @Param limit query int false "Pagination limit"
// @Success 200 {array} logEntryResponse
// @Failure 400 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /api/entries [get]
// List returns stored log entries for one log file with pagination.
func (h *EntryHandler) List(c *gin.Context) {
	if isPagedListRequest(c, "from_line", "to_line") {
		h.listPaged(c)
		return
	}

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

func (h *EntryHandler) listPaged(c *gin.Context) {
	logFileID := trimQuery(c, "log_file_id")
	if logFileID == "" {
		writeError(c, http.StatusBadRequest, "log_file_id is required")
		return
	}
	offset, limit, err := parsePageQuery(c)
	if err != nil {
		writeError(c, http.StatusBadRequest, err.Error())
		return
	}
	fromLine, err := parseIntQuery(c, "from_line", 0)
	if err != nil {
		writeError(c, http.StatusBadRequest, err.Error())
		return
	}
	toLine, err := parseIntQuery(c, "to_line", 0)
	if err != nil {
		writeError(c, http.StatusBadRequest, err.Error())
		return
	}
	sortBy := trimQuery(c, "sort")
	if err := validateEnum(sortBy, "sort", "line_number", "collected_at"); err != nil {
		writeError(c, http.StatusBadRequest, err.Error())
		return
	}
	order := trimQuery(c, "order")
	if err := validateEnum(order, "order", "asc", "desc"); err != nil {
		writeError(c, http.StatusBadRequest, err.Error())
		return
	}

	page, err := h.service.ListFiltered(c.Request.Context(), repository.LogEntryListFilter{
		ListOptions: repository.ListOptions{
			Q:      trimQuery(c, "q"),
			Offset: offset,
			Limit:  limit,
			Sort:   sortBy,
			Order:  order,
		},
		LogFileID: logFileID,
		FromLine:  int64(fromLine),
		ToLine:    int64(toLine),
	})
	if err != nil {
		writeServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, pageResponse[logEntryResponse]{
		Items:  logEntryResponses(page.Items),
		Total:  page.Total,
		Offset: page.Offset,
		Limit:  page.Limit,
	})
}
