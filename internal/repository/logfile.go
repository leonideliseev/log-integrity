package repository

import (
	"context"

	"github.com/lenchik/logmonitor/models"
)

// LogFileRepository stores discovered remote log files.
type LogFileRepository interface {
	CreateLogFile(ctx context.Context, logFile *models.LogFile) error
	GetLogFileByID(ctx context.Context, id string) (*models.LogFile, error)
	ListLogFilesByServer(ctx context.Context, serverID string) ([]*models.LogFile, error)
	ListActiveLogFiles(ctx context.Context) ([]*models.LogFile, error)
	UpdateLogFile(ctx context.Context, logFile *models.LogFile) error
	DeleteLogFile(ctx context.Context, id string) error
	UpdateLastScanned(ctx context.Context, id string) error
}
