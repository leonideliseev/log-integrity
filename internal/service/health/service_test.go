package health

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/lenchik/logmonitor/internal/repository/memory"
	"github.com/lenchik/logmonitor/models"
)

func TestServiceRecordsFailureBackoffAndSuccess(t *testing.T) {
	ctx := context.Background()
	store := memory.New()
	serverModel := &models.Server{
		ID:        "srv-health",
		Name:      "health-host",
		Host:      "127.0.0.1",
		Username:  "demo",
		AuthType:  models.AuthPassword,
		AuthValue: "demo",
		Status:    models.ServerStatusActive,
	}
	if err := store.CreateServer(ctx, serverModel); err != nil {
		t.Fatalf("create server: %v", err)
	}

	now := time.Date(2026, 4, 22, 12, 0, 0, 0, time.UTC)
	service := NewService(store, Options{
		FailureThreshold:   2,
		BackoffBase:        time.Minute,
		BackoffMax:         5 * time.Minute,
		LastErrorMaxLength: 12,
	})
	service.now = func() time.Time { return now }

	if err := service.RecordFailure(ctx, serverModel, errors.New("ssh connection refused")); err != nil {
		t.Fatalf("record first failure: %v", err)
	}
	serverModel, err := store.GetServerByID(ctx, serverModel.ID)
	if err != nil {
		t.Fatalf("get server after first failure: %v", err)
	}
	if serverModel.FailureCount != 1 || serverModel.BackoffUntil != nil {
		t.Fatalf("expected first failure without backoff, got failure_count=%d backoff_until=%v", serverModel.FailureCount, serverModel.BackoffUntil)
	}
	if serverModel.LastError != "ssh connecti" {
		t.Fatalf("expected truncated last_error, got %q", serverModel.LastError)
	}

	if err := service.RecordFailure(ctx, serverModel, errors.New("ssh connection refused")); err != nil {
		t.Fatalf("record second failure: %v", err)
	}
	serverModel, err = store.GetServerByID(ctx, serverModel.ID)
	if err != nil {
		t.Fatalf("get server after second failure: %v", err)
	}
	if serverModel.BackoffUntil == nil || !serverModel.BackoffUntil.Equal(now.Add(time.Minute)) {
		t.Fatalf("expected one minute backoff, got %v", serverModel.BackoffUntil)
	}
	if !service.ShouldSkip(serverModel) {
		t.Fatalf("expected server to be skipped while backoff is active")
	}

	if err := service.RecordSuccess(ctx, serverModel.ID); err != nil {
		t.Fatalf("record success: %v", err)
	}
	serverModel, err = store.GetServerByID(ctx, serverModel.ID)
	if err != nil {
		t.Fatalf("get server after success: %v", err)
	}
	if serverModel.SuccessCount != 1 || serverModel.FailureCount != 0 || serverModel.LastError != "" || serverModel.BackoffUntil != nil || serverModel.LastSeenAt == nil {
		t.Fatalf("expected cleared health state after success, got %#v", serverModel)
	}
}
