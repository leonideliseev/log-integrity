package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/lenchik/logmonitor/internal/runtimeinfo"
)

// ReadinessFunc computes the current readiness state for the running process.
type ReadinessFunc func(ctx *gin.Context) runtimeinfo.Readiness

// ProbeHandler exposes unauthenticated liveness and readiness probes.
type ProbeHandler struct {
	readiness ReadinessFunc
}

// NewProbeHandler creates a probe handler with the provided readiness callback.
func NewProbeHandler(readiness ReadinessFunc) *ProbeHandler {
	return &ProbeHandler{readiness: readiness}
}

// Health godoc
// @Summary Liveness probe
// @Tags probes
// @Produce plain
// @Success 200 {string} string "ok"
// @Router /healthz [get]
// Health returns a trivial success response when the process is alive.
func (h *ProbeHandler) Health(c *gin.Context) {
	c.String(http.StatusOK, "ok")
}

// Ready godoc
// @Summary Readiness probe
// @Tags probes
// @Produce json
// @Success 200 {object} runtimeinfo.Readiness
// @Failure 503 {object} runtimeinfo.Readiness
// @Router /readyz [get]
// Ready returns the current readiness state of the process.
func (h *ProbeHandler) Ready(c *gin.Context) {
	result := h.readiness(c)
	if !result.Ready {
		c.JSON(http.StatusServiceUnavailable, result)
		return
	}
	c.JSON(http.StatusOK, result)
}
