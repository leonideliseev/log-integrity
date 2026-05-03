package repository

import (
	"context"
	"time"

	"github.com/lenchik/logmonitor/models"
)

// ServerRepository stores monitored server definitions.
type ServerRepository interface {
	CreateServer(ctx context.Context, server *models.Server) error
	GetServerByID(ctx context.Context, id string) (*models.Server, error)
	ListServers(ctx context.Context) ([]*models.Server, error)
	ListServersFiltered(ctx context.Context, filter ServerListFilter) (Page[*models.Server], error)
	UpdateServer(ctx context.Context, server *models.Server) error
	DeleteServer(ctx context.Context, id string) error
	UpdateServerStatus(ctx context.Context, id string, status models.ServerStatus) error
	RecordServerSuccess(ctx context.Context, id string, seenAt time.Time) error
	RecordServerFailure(ctx context.Context, id string, lastError string, backoffUntil *time.Time) error
}
