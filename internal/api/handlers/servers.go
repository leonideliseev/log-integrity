package handlers

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	serverservice "github.com/lenchik/logmonitor/internal/service/server"
	"github.com/lenchik/logmonitor/models"
)

// ServerHandler handles HTTP requests related to monitored servers.
type ServerHandler struct {
	service *serverservice.Service
}

type createServerRequest struct {
	Name      string `json:"name"`
	Host      string `json:"host"`
	Port      int    `json:"port"`
	Username  string `json:"username"`
	AuthType  string `json:"auth_type"`
	AuthValue string `json:"auth_value"`
	OSType    string `json:"os_type"`
}

type serverResponse struct {
	ID        string                 `json:"id"`
	Name      string                 `json:"name"`
	Host      string                 `json:"host"`
	Port      int                    `json:"port"`
	Username  string                 `json:"username"`
	AuthType  models.AuthType        `json:"auth_type"`
	OSType    models.OSType          `json:"os_type"`
	Status    models.ServerStatus    `json:"status"`
	ManagedBy models.ServerManagedBy `json:"managed_by"`
	CreatedAt time.Time              `json:"created_at"`
	UpdatedAt time.Time              `json:"updated_at"`
}

// NewServerHandler creates a server handler with required dependencies.
func NewServerHandler(service *serverservice.Service) *ServerHandler {
	return &ServerHandler{service: service}
}

// List returns all registered servers.
func (h *ServerHandler) List(c *gin.Context) {
	items, err := h.service.List(c.Request.Context())
	if err != nil {
		writeError(c, http.StatusInternalServerError, err.Error())
		return
	}
	c.JSON(http.StatusOK, serverResponses(items))
}

// Create registers a new server from JSON payload.
func (h *ServerHandler) Create(c *gin.Context) {
	var payload createServerRequest
	if err := c.ShouldBindJSON(&payload); err != nil {
		writeError(c, http.StatusBadRequest, err.Error())
		return
	}

	serverModel, err := payload.toModel()
	if err != nil {
		writeError(c, http.StatusBadRequest, err.Error())
		return
	}
	if err := h.service.Create(c.Request.Context(), serverModel); err != nil {
		writeServiceError(c, err)
		return
	}
	c.JSON(http.StatusCreated, newServerResponse(serverModel))
}

// Discover runs log discovery for one server or for all servers.
func (h *ServerHandler) Discover(c *gin.Context) {
	type request struct {
		ServerID string `json:"server_id"`
	}

	var payload request
	if c.Request.ContentLength > 0 {
		if err := c.ShouldBindJSON(&payload); err != nil {
			writeError(c, http.StatusBadRequest, err.Error())
			return
		}
	}

	items, err := h.service.Discover(c.Request.Context(), payload.ServerID)
	if err != nil {
		writeServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, discoverResultResponses(items))
}

func (r createServerRequest) toModel() (*models.Server, error) {
	r.Name = strings.TrimSpace(r.Name)
	r.Host = strings.TrimSpace(r.Host)
	r.Username = strings.TrimSpace(r.Username)
	r.AuthType = strings.TrimSpace(r.AuthType)
	r.AuthValue = strings.TrimSpace(r.AuthValue)
	r.OSType = strings.TrimSpace(r.OSType)

	if r.Name == "" {
		return nil, fmt.Errorf("name is required")
	}
	if r.Host == "" {
		return nil, fmt.Errorf("host is required")
	}
	if r.Username == "" {
		return nil, fmt.Errorf("username is required")
	}
	if r.Port < 0 || r.Port > 65535 {
		return nil, fmt.Errorf("port must be between 1 and 65535")
	}
	if r.Port == 0 {
		r.Port = 22
	}
	if r.AuthType != string(models.AuthPassword) && r.AuthType != string(models.AuthKey) {
		return nil, fmt.Errorf("auth_type must be either password or key")
	}
	if r.AuthValue == "" {
		return nil, fmt.Errorf("auth_value is required")
	}
	if r.OSType != "" && r.OSType != string(models.OSLinux) && r.OSType != string(models.OSWindows) && r.OSType != string(models.OSMacOS) {
		return nil, fmt.Errorf("os_type must be empty, linux, windows or macos")
	}

	return &models.Server{
		Name:      r.Name,
		Host:      r.Host,
		Port:      r.Port,
		Username:  r.Username,
		AuthType:  models.AuthType(r.AuthType),
		AuthValue: r.AuthValue,
		OSType:    models.OSType(r.OSType),
		Status:    models.ServerStatusActive,
		ManagedBy: models.ServerManagedByAPI,
	}, nil
}

func serverResponses(items []*models.Server) []serverResponse {
	result := make([]serverResponse, 0, len(items))
	for _, item := range items {
		result = append(result, newServerResponse(item))
	}
	return result
}

func newServerResponse(serverModel *models.Server) serverResponse {
	return serverResponse{
		ID:        serverModel.ID,
		Name:      serverModel.Name,
		Host:      serverModel.Host,
		Port:      serverModel.Port,
		Username:  serverModel.Username,
		AuthType:  serverModel.AuthType,
		OSType:    serverModel.OSType,
		Status:    serverModel.Status,
		ManagedBy: serverModel.ManagedBy,
		CreatedAt: serverModel.CreatedAt,
		UpdatedAt: serverModel.UpdatedAt,
	}
}
