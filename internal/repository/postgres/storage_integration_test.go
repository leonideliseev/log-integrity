//go:build integration

package postgres_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lenchik/logmonitor/internal/repository"
	postgresrepo "github.com/lenchik/logmonitor/internal/repository/postgres"
	"github.com/lenchik/logmonitor/internal/security"
	"github.com/lenchik/logmonitor/models"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
)

func TestPostgresRepositoryIntegrationLifecycle(t *testing.T) {
	ctx := context.Background()
	store := newPostgresRepository(t, ctx)

	serverModel := &models.Server{
		ID:        "srv-it-lifecycle",
		Name:      "integration-main",
		Host:      "192.0.2.10",
		Port:      22,
		Username:  "demo",
		AuthType:  models.AuthPassword,
		AuthValue: "plain-secret",
		OSType:    models.OSLinux,
		Status:    models.ServerStatusActive,
		ManagedBy: models.ServerManagedByAPI,
	}
	mustNoError(t, store.CreateServer(ctx, serverModel))

	storedServer, err := store.GetServerByID(ctx, serverModel.ID)
	mustNoError(t, err)
	if storedServer.AuthValue != serverModel.AuthValue {
		t.Fatalf("expected decrypted auth value %q, got %q", serverModel.AuthValue, storedServer.AuthValue)
	}
	if storedServer.ManagedBy != models.ServerManagedByAPI {
		t.Fatalf("expected managed_by api, got %q", storedServer.ManagedBy)
	}

	mustNoError(t, store.UpdateServerStatus(ctx, serverModel.ID, models.ServerStatusError))
	storedServer, err = store.GetServerByID(ctx, serverModel.ID)
	mustNoError(t, err)
	if storedServer.Status != models.ServerStatusError {
		t.Fatalf("expected server status error, got %q", storedServer.Status)
	}

	backoffUntil := time.Now().UTC().Add(time.Minute)
	mustNoError(t, store.RecordServerFailure(ctx, serverModel.ID, "ssh: connection refused", &backoffUntil))
	storedServer, err = store.GetServerByID(ctx, serverModel.ID)
	mustNoError(t, err)
	if storedServer.FailureCount != 1 || storedServer.LastError == "" || storedServer.BackoffUntil == nil {
		t.Fatalf("expected failed health state, got failure_count=%d last_error=%q backoff_until=%v", storedServer.FailureCount, storedServer.LastError, storedServer.BackoffUntil)
	}

	mustNoError(t, store.RecordServerSuccess(ctx, serverModel.ID, time.Now().UTC()))
	storedServer, err = store.GetServerByID(ctx, serverModel.ID)
	mustNoError(t, err)
	if storedServer.SuccessCount != 1 || storedServer.FailureCount != 0 || storedServer.LastSeenAt == nil || storedServer.LastError != "" || storedServer.BackoffUntil != nil {
		t.Fatalf("expected successful health state, got success_count=%d failure_count=%d last_seen_at=%v last_error=%q backoff_until=%v", storedServer.SuccessCount, storedServer.FailureCount, storedServer.LastSeenAt, storedServer.LastError, storedServer.BackoffUntil)
	}

	logFile := &models.LogFile{
		ID:       "log-it-lifecycle",
		ServerID: serverModel.ID,
		Path:     "/var/log/auth.log",
		LogType:  models.LogTypeAuth,
		FileIdentity: models.FileIdentity{
			DeviceID:  "device-a",
			Inode:     "inode-a",
			SizeBytes: 2048,
		},
		Meta: map[string]string{
			"source": "integration",
		},
		LastLineNumber: 2,
		LastByteOffset: 42,
		IsActive:       true,
	}
	mustNoError(t, store.CreateLogFile(ctx, logFile))

	storedLogFile, err := store.GetLogFileByID(ctx, logFile.ID)
	mustNoError(t, err)
	if storedLogFile.FileIdentity.Inode != logFile.FileIdentity.Inode {
		t.Fatalf("expected inode %q, got %q", logFile.FileIdentity.Inode, storedLogFile.FileIdentity.Inode)
	}
	if storedLogFile.Meta["source"] != "integration" {
		t.Fatalf("expected log file meta to round-trip, got %#v", storedLogFile.Meta)
	}

	entries := []*models.LogEntry{
		{ID: "entry-it-1", LogFileID: logFile.ID, LineNumber: 1, Content: "accepted password", Hash: "hash-1"},
		{ID: "entry-it-2", LogFileID: logFile.ID, LineNumber: 2, Content: "session opened", Hash: "hash-2"},
	}
	chunks := []*models.LogChunk{
		{
			ID:             "chunk-it-1",
			LogFileID:      logFile.ID,
			ChunkNumber:    1,
			FromLineNumber: 1,
			ToLineNumber:   2,
			FromByteOffset: 0,
			ToByteOffset:   42,
			EntriesCount:   len(entries),
			Hash:           "chunk-hash-1",
			HashAlgorithm:  "hmac-sha256",
		},
	}
	mustNoError(t, store.CreateLogEntriesWithChunks(ctx, entries, chunks))

	count, err := store.CountLogEntries(ctx, logFile.ID)
	mustNoError(t, err)
	if count != int64(len(entries)) {
		t.Fatalf("expected %d entries, got %d", len(entries), count)
	}

	maxLine, err := store.GetMaxLineNumber(ctx, logFile.ID)
	mustNoError(t, err)
	if maxLine != 2 {
		t.Fatalf("expected max line 2, got %d", maxLine)
	}

	rangeEntries, err := store.ListLogEntriesByLineRange(ctx, logFile.ID, 1, 2)
	mustNoError(t, err)
	if len(rangeEntries) != len(entries) {
		t.Fatalf("expected %d range entries, got %d", len(entries), len(rangeEntries))
	}

	latestChunk, err := store.GetLatestLogChunk(ctx, logFile.ID)
	mustNoError(t, err)
	if latestChunk.Hash != chunks[0].Hash {
		t.Fatalf("expected latest chunk hash %q, got %q", chunks[0].Hash, latestChunk.Hash)
	}

	checkResult := &models.CheckResult{
		ID:            "check-it-1",
		LogFileID:     logFile.ID,
		Status:        models.CheckStatusOK,
		TotalLines:    2,
		TamperedLines: 0,
	}
	mustNoError(t, store.CreateCheckResult(ctx, checkResult))

	latestCheck, err := store.GetLatestCheckResult(ctx, logFile.ID)
	mustNoError(t, err)
	if latestCheck.Status != models.CheckStatusOK {
		t.Fatalf("expected latest check status ok, got %q", latestCheck.Status)
	}

	mustNoError(t, store.DeleteServer(ctx, serverModel.ID))
	_, err = store.GetLogFileByID(ctx, logFile.ID)
	if !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("expected log file cascade delete, got %v", err)
	}
}

func TestPostgresRepositoryIntegrationConstraints(t *testing.T) {
	ctx := context.Background()
	store := newPostgresRepository(t, ctx)

	serverModel := &models.Server{
		ID:        "srv-it-constraints",
		Name:      "unique-server",
		Host:      "192.0.2.20",
		Port:      22,
		Username:  "demo",
		AuthType:  models.AuthPassword,
		AuthValue: "plain-secret",
		Status:    models.ServerStatusActive,
		ManagedBy: models.ServerManagedByConfig,
	}
	mustNoError(t, store.CreateServer(ctx, serverModel))

	duplicateServer := &models.Server{
		ID:        "srv-it-constraints-duplicate",
		Name:      "UNIQUE-SERVER",
		Host:      "192.0.2.21",
		Port:      22,
		Username:  "demo",
		AuthType:  models.AuthPassword,
		AuthValue: "plain-secret",
		Status:    models.ServerStatusActive,
	}
	if err := store.CreateServer(ctx, duplicateServer); !errors.Is(err, repository.ErrConflict) {
		t.Fatalf("expected duplicate server conflict, got %v", err)
	}

	logFile := &models.LogFile{
		ID:       "log-it-constraints",
		ServerID: serverModel.ID,
		Path:     "/var/log/syslog",
		LogType:  models.LogTypeSyslog,
		IsActive: true,
	}
	mustNoError(t, store.CreateLogFile(ctx, logFile))

	duplicateLogFile := &models.LogFile{
		ID:       "log-it-constraints-duplicate",
		ServerID: serverModel.ID,
		Path:     logFile.Path,
		LogType:  models.LogTypeSyslog,
		IsActive: true,
	}
	if err := store.CreateLogFile(ctx, duplicateLogFile); !errors.Is(err, repository.ErrConflict) {
		t.Fatalf("expected duplicate log file conflict, got %v", err)
	}

	missingServerLogFile := &models.LogFile{
		ID:       "log-it-missing-server",
		ServerID: "srv-missing",
		Path:     "/var/log/missing.log",
		LogType:  models.LogTypeUnknown,
		IsActive: true,
	}
	if err := store.CreateLogFile(ctx, missingServerLogFile); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("expected missing server error, got %v", err)
	}
}

func newPostgresRepository(t *testing.T, ctx context.Context) repository.Repository {
	t.Helper()

	container, err := tcpostgres.Run(
		ctx,
		"postgres:16-alpine",
		tcpostgres.WithDatabase("logmonitor_test"),
		tcpostgres.WithUsername("logmonitor"),
		tcpostgres.WithPassword("logmonitor"),
		tcpostgres.BasicWaitStrategies(),
	)
	if err != nil {
		if shouldSkipDockerIntegrationTest(err) {
			t.Skipf("skip PostgreSQL integration test: %v", err)
		}
		t.Fatalf("start PostgreSQL testcontainer: %v", err)
	}
	testcontainers.CleanupContainer(t, container)

	dsn, err := container.ConnectionString(ctx, "sslmode=disable")
	mustNoError(t, err)

	authCipher, err := security.NewStringCipher("integration-auth-secret")
	mustNoError(t, err)

	store, err := postgresrepo.OpenWithOptions(dsn, postgresrepo.Options{
		MaxConns:      4,
		MinConns:      1,
		MigrationsDir: filepath.Join("..", "..", "..", "migrations"),
		AuthCipher:    authCipher,
	})
	mustNoError(t, err)

	t.Cleanup(func() {
		mustNoError(t, store.Close())
	})

	return store
}

func shouldSkipDockerIntegrationTest(err error) bool {
	if os.Getenv("CI") != "" {
		return false
	}

	message := err.Error()
	return strings.Contains(message, "rootless Docker is not supported on Windows") ||
		strings.Contains(message, "failed to create Docker provider") ||
		strings.Contains(message, "Cannot connect to the Docker daemon")
}

func mustNoError(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
