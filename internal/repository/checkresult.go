// Package repository defines storage interfaces and shared repository errors.
package repository

import (
	"context"

	"github.com/lenchik/logmonitor/models"
)

// CheckResultRepository stores integrity check results.
type CheckResultRepository interface {
	CreateCheckResult(ctx context.Context, result *models.CheckResult) error
	GetCheckResultByID(ctx context.Context, id string) (*models.CheckResult, error)
	ListCheckResults(ctx context.Context, logFileID string, offset, limit int) ([]*models.CheckResult, error)
	GetLatestCheckResult(ctx context.Context, logFileID string) (*models.CheckResult, error)
}
