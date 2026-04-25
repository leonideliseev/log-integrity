package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/lenchik/logmonitor/internal/runtimeinfo"
)

// RuntimeHandler exposes runtime validation data for operators.
type RuntimeHandler struct {
	state *runtimeinfo.State
}

// NewRuntimeHandler creates a runtime handler with shared runtime state.
func NewRuntimeHandler(state *runtimeinfo.State) *RuntimeHandler {
	return &RuntimeHandler{state: state}
}

// Validation returns runtime startup warnings and environment resolution results.
func (h *RuntimeHandler) Validation(c *gin.Context) {
	c.JSON(http.StatusOK, h.state.Snapshot())
}
