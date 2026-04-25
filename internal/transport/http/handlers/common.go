package handlers

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	jobqueue "github.com/lenchik/logmonitor/internal/jobs"
	"github.com/lenchik/logmonitor/internal/repository"
)

const maxPageLimit = 1000

// writeError sends a uniform JSON error payload to the client.
func writeError(c *gin.Context, status int, message string) {
	c.JSON(status, gin.H{"error": message})
}

// writeServiceError maps domain and repository errors to HTTP status codes.
func writeServiceError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, repository.ErrNotFound):
		writeError(c, http.StatusNotFound, err.Error())
	case errors.Is(err, repository.ErrConflict):
		writeError(c, http.StatusConflict, err.Error())
	default:
		writeError(c, http.StatusInternalServerError, err.Error())
	}
}

// writeJobError maps async queue errors to HTTP status codes.
func writeJobError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, jobqueue.ErrNotFound):
		writeError(c, http.StatusNotFound, err.Error())
	case errors.Is(err, jobqueue.ErrQueueFull), errors.Is(err, jobqueue.ErrShuttingDown):
		writeError(c, http.StatusServiceUnavailable, err.Error())
	default:
		writeError(c, http.StatusInternalServerError, err.Error())
	}
}

// parseIntQuery reads an integer query parameter with a default fallback.
func parseIntQuery(c *gin.Context, key string, defaultValue int) (int, error) {
	raw := c.Query(key)
	if raw == "" {
		return defaultValue, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("invalid %s: %w", key, err)
	}
	if value < 0 {
		return 0, fmt.Errorf("%s must be greater than or equal to zero", key)
	}
	return value, nil
}

// validatePageLimit prevents accidental unbounded API reads.
func validatePageLimit(limit int) error {
	if limit <= 0 {
		return fmt.Errorf("limit must be greater than zero")
	}
	if limit > maxPageLimit {
		return fmt.Errorf("limit must be less than or equal to %d", maxPageLimit)
	}
	return nil
}

// idempotencyKeyFromHeader extracts an optional client-supplied deduplication key.
func idempotencyKeyFromHeader(c *gin.Context) string {
	return strings.TrimSpace(c.GetHeader("X-Idempotency-Key"))
}
