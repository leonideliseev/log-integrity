package handlers

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	jobqueue "github.com/lenchik/logmonitor/internal/jobs"
	"github.com/lenchik/logmonitor/models"
)

// JobHandler exposes async queue history for API clients and a future UI.
type JobHandler struct {
	jobs *jobqueue.Manager
}

// NewJobHandler creates a job history handler with queue dependency.
func NewJobHandler(jobs *jobqueue.Manager) *JobHandler {
	return &JobHandler{jobs: jobs}
}

// List godoc
// @Summary List async jobs
// @Tags jobs
// @Produce json
// @Security ApiKeyAuth
// @Param type query string false "Optional job type filter"
// @Param status query string false "Optional job status filter"
// @Param server_id query string false "Optional server identifier"
// @Param log_file_id query string false "Optional log file identifier"
// @Param offset query int false "Pagination offset"
// @Param limit query int false "Pagination limit"
// @Success 200 {array} jobResponse
// @Failure 400 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /api/jobs [get]
// List returns queued, running and completed jobs with lightweight filtering.
func (h *JobHandler) List(c *gin.Context) {
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

	filter, err := parseJobListFilter(c, offset, limit)
	if err != nil {
		writeError(c, http.StatusBadRequest, err.Error())
		return
	}

	items, err := h.jobs.ListJobs(filter)
	if err != nil {
		writeJobError(c, err)
		return
	}
	c.JSON(http.StatusOK, jobResponses(items))
}

// Get godoc
// @Summary Get async job by ID
// @Tags jobs
// @Produce json
// @Security ApiKeyAuth
// @Param id path string true "Job identifier"
// @Success 200 {object} jobResponse
// @Failure 400 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Router /api/jobs/{id} [get]
// Get returns one queued or historical job by identifier.
func (h *JobHandler) Get(c *gin.Context) {
	jobID := strings.TrimSpace(c.Param("id"))
	if jobID == "" {
		writeError(c, http.StatusBadRequest, "job id is required")
		return
	}

	job, err := h.jobs.GetJob(jobID)
	if err != nil {
		writeJobError(c, err)
		return
	}
	c.JSON(http.StatusOK, newJobResponse(job))
}

// parseJobListFilter validates supported job history filters.
func parseJobListFilter(c *gin.Context, offset, limit int) (jobqueue.ListFilter, error) {
	filter := jobqueue.ListFilter{
		ServerID:  strings.TrimSpace(c.Query("server_id")),
		LogFileID: strings.TrimSpace(c.Query("log_file_id")),
		Offset:    offset,
		Limit:     limit,
	}

	rawType := strings.TrimSpace(c.Query("type"))
	if rawType != "" {
		jobType := models.JobType(rawType)
		switch jobType {
		case models.JobTypeDiscover, models.JobTypeCollect, models.JobTypeIntegrity:
			filter.Type = jobType
		default:
			return jobqueue.ListFilter{}, httpError("type must be one of discover, collect or integrity")
		}
	}

	rawStatus := strings.TrimSpace(c.Query("status"))
	if rawStatus != "" {
		jobStatus := models.JobStatus(rawStatus)
		switch jobStatus {
		case models.JobStatusQueued, models.JobStatusRunning, models.JobStatusSucceeded, models.JobStatusFailed, models.JobStatusCanceled:
			filter.Status = jobStatus
		default:
			return jobqueue.ListFilter{}, httpError("status must be one of queued, running, succeeded, failed or canceled")
		}
	}

	return filter, nil
}

type httpError string

// Error returns the HTTP validation message.
func (e httpError) Error() string {
	return string(e)
}
