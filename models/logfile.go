package models

import "time"

// LogType — тип журнала событий
type LogType string

// Log types classify discovered log sources.
const (
	LogTypeSyslog   LogType = "syslog"
	LogTypeAuth     LogType = "auth"
	LogTypeNginx    LogType = "nginx"
	LogTypeApache   LogType = "apache"
	LogTypeEventLog LogType = "eventlog" // Windows Event Log
	LogTypeKernel   LogType = "kernel"
	LogTypeApp      LogType = "app"
	LogTypeUnknown  LogType = "unknown"
)

// LogFile представляет журнал событий, обнаруженный на удалённом сервере
type LogFile struct {
	ID             string            `json:"id"`
	ServerID       string            `json:"server_id"`
	Path           string            `json:"path"` // путь к файлу журнала на удалённом сервере
	LogType        LogType           `json:"log_type"`
	FileIdentity   FileIdentity      `json:"file_identity"`
	Meta           map[string]string `json:"meta,omitempty"`
	LastScannedAt  *time.Time        `json:"last_scanned_at,omitempty"`
	LastLineNumber int64             `json:"last_line_number"`
	LastByteOffset int64             `json:"last_byte_offset"`
	IsActive       bool              `json:"is_active"`
	CreatedAt      time.Time         `json:"created_at"`
}

// FileIdentity describes stable file identifiers that differ across operating systems.
type FileIdentity struct {
	DeviceID    string `json:"device_id,omitempty"`
	Inode       string `json:"inode,omitempty"`
	VolumeID    string `json:"volume_id,omitempty"`
	FileID      string `json:"file_id,omitempty"`
	EventLog    string `json:"event_log,omitempty"`
	SizeBytes   int64  `json:"size_bytes,omitempty"`
	ModTimeUnix int64  `json:"mod_time_unix,omitempty"`
}
