package models

import "time"

// OSType — тип операционной системы удалённого сервера
type OSType string

// OS types identify supported remote operating systems.
const (
	OSLinux   OSType = "linux"
	OSWindows OSType = "windows"
	OSMacOS   OSType = "macos"
)

// AuthType — способ аутентификации по SSH
type AuthType string

// Auth types identify supported SSH authentication modes.
const (
	AuthPassword AuthType = "password"
	AuthKey      AuthType = "key"
)

// ServerStatus — статус сервера в системе
type ServerStatus string

// Server statuses describe monitoring availability.
const (
	ServerStatusActive   ServerStatus = "active"
	ServerStatusDegraded ServerStatus = "degraded"
	ServerStatusInactive ServerStatus = "inactive"
	ServerStatusError    ServerStatus = "error"
)

// ServerManagedBy identifies which source owns a server definition.

// Server представляет удалённый сервер, за журналами которого ведётся наблюдение

// ServerManagedBy identifies which source owns a server definition.
type ServerManagedBy string

// Server ownership values separate config bootstrap data from runtime API data.
const (
	ServerManagedByConfig ServerManagedBy = "config"
	ServerManagedByAPI    ServerManagedBy = "api"
)

// Server represents a remote server whose logs are monitored.
type Server struct {
	ID           string          `json:"id"`
	Name         string          `json:"name"`
	Host         string          `json:"host"`
	Port         int             `json:"port"`
	Username     string          `json:"username"`
	AuthType     AuthType        `json:"auth_type"`
	AuthValue    string          `json:"auth_value,omitempty"` // пароль или путь к приватному ключу
	OSType       OSType          `json:"os_type"`
	Status       ServerStatus    `json:"status"`
	ManagedBy    ServerManagedBy `json:"managed_by"`
	SuccessCount int64           `json:"success_count"`
	FailureCount int64           `json:"failure_count"`
	LastError    string          `json:"last_error,omitempty"`
	LastSeenAt   *time.Time      `json:"last_seen_at,omitempty"`
	BackoffUntil *time.Time      `json:"backoff_until,omitempty"`
	CreatedAt    time.Time       `json:"created_at"`
	UpdatedAt    time.Time       `json:"updated_at"`
}
