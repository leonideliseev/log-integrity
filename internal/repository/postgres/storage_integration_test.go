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

func TestPostgresRepositoryIntegrationLogFileMutationPaginationAndCleanup(t *testing.T) {
	ctx := context.Background()
	store := newPostgresRepository(t, ctx)
	serverModel := createIntegrationServer(t, ctx, store, "srv-it-logfile-flow", "logfile-flow", "192.0.2.30")

	activeLog := &models.LogFile{
		ID:       "log-it-active",
		ServerID: serverModel.ID,
		Path:     "/var/log/app.log",
		LogType:  models.LogTypeApp,
		IsActive: true,
		Meta:     map[string]string{"service": "app"},
	}
	inactiveLog := &models.LogFile{
		ID:       "log-it-inactive",
		ServerID: serverModel.ID,
		Path:     "/var/log/old.log",
		LogType:  models.LogTypeApp,
		IsActive: false,
	}
	mustNoError(t, store.CreateLogFile(ctx, activeLog))
	mustNoError(t, store.CreateLogFile(ctx, inactiveLog))

	activeOnly, err := store.ListActiveLogFiles(ctx)
	mustNoError(t, err)
	if len(activeOnly) != 1 || activeOnly[0].ID != activeLog.ID {
		t.Fatalf("expected only active log file, got %#v", activeOnly)
	}

	activeLog.FileIdentity = models.FileIdentity{DeviceID: "rotated-device", Inode: "rotated-inode", SizeBytes: 10}
	activeLog.Meta["rotation"] = "detected"
	activeLog.LastLineNumber = 0
	activeLog.LastByteOffset = 0
	mustNoError(t, store.UpdateLogFile(ctx, activeLog))
	mustNoError(t, store.UpdateLastScanned(ctx, activeLog.ID))

	updatedLog, err := store.GetLogFileByID(ctx, activeLog.ID)
	mustNoError(t, err)
	if updatedLog.FileIdentity.Inode != "rotated-inode" || updatedLog.Meta["rotation"] != "detected" || updatedLog.LastScannedAt == nil {
		t.Fatalf("expected updated log file identity, meta and scan time, got %#v", updatedLog)
	}

	entries := make([]*models.LogEntry, 0, 5)
	for line := int64(1); line <= 5; line++ {
		entries = append(entries, &models.LogEntry{
			ID:         "entry-it-page-" + string(rune('0'+line)),
			LogFileID:  activeLog.ID,
			LineNumber: line,
			Content:    "line",
			Hash:       "hash",
		})
	}
	mustNoError(t, store.CreateLogEntries(ctx, entries))

	page, err := store.ListLogEntries(ctx, activeLog.ID, 2, 2)
	mustNoError(t, err)
	if len(page) != 2 || page[0].LineNumber != 3 || page[1].LineNumber != 4 {
		t.Fatalf("expected paginated lines 3 and 4, got %#v", page)
	}

	rangeEntries, err := store.ListLogEntriesByLineRange(ctx, activeLog.ID, 2, 4)
	mustNoError(t, err)
	if len(rangeEntries) != 3 || rangeEntries[0].LineNumber != 2 || rangeEntries[2].LineNumber != 4 {
		t.Fatalf("expected inclusive range 2..4, got %#v", rangeEntries)
	}

	chunks := []*models.LogChunk{
		newIntegrationChunk("chunk-it-page-1", activeLog.ID, 1, 1, 2),
		newIntegrationChunk("chunk-it-page-2", activeLog.ID, 2, 3, 4),
		newIntegrationChunk("chunk-it-page-3", activeLog.ID, 3, 5, 5),
	}
	mustNoError(t, store.CreateLogChunks(ctx, chunks))

	chunkPage, err := store.ListLogChunks(ctx, activeLog.ID, 1, 1)
	mustNoError(t, err)
	if len(chunkPage) != 1 || chunkPage[0].ChunkNumber != 2 {
		t.Fatalf("expected second chunk page, got %#v", chunkPage)
	}

	latestChunk, err := store.GetLatestLogChunk(ctx, activeLog.ID)
	mustNoError(t, err)
	if latestChunk.ChunkNumber != 3 {
		t.Fatalf("expected latest chunk number 3, got %d", latestChunk.ChunkNumber)
	}

	mustNoError(t, store.DeleteLogEntriesByLogFile(ctx, activeLog.ID))
	count, err := store.CountLogEntries(ctx, activeLog.ID)
	mustNoError(t, err)
	if count != 0 {
		t.Fatalf("expected log entries cleanup, got %d entries", count)
	}

	mustNoError(t, store.DeleteLogChunksByLogFile(ctx, activeLog.ID))
	_, err = store.GetLatestLogChunk(ctx, activeLog.ID)
	if !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("expected latest chunk not found after cleanup, got %v", err)
	}
}

func TestPostgresRepositoryIntegrationCheckResultOrdering(t *testing.T) {
	ctx := context.Background()
	store := newPostgresRepository(t, ctx)
	serverModel := createIntegrationServer(t, ctx, store, "srv-it-checks", "checks-flow", "192.0.2.40")
	logFile := createIntegrationLogFile(t, ctx, store, "log-it-checks", serverModel.ID, "/var/log/checks.log")

	_, err := store.GetLatestCheckResult(ctx, logFile.ID)
	if !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("expected latest check result not found before checks exist, got %v", err)
	}

	baseTime := time.Date(2026, 4, 22, 12, 0, 0, 0, time.UTC)
	results := []*models.CheckResult{
		{ID: "check-it-order-2", LogFileID: logFile.ID, CheckedAt: baseTime.Add(2 * time.Minute), Status: models.CheckStatusTampered, TotalLines: 10, TamperedLines: 1},
		{ID: "check-it-order-1", LogFileID: logFile.ID, CheckedAt: baseTime.Add(time.Minute), Status: models.CheckStatusOK, TotalLines: 10},
		{ID: "check-it-order-3", LogFileID: logFile.ID, CheckedAt: baseTime.Add(3 * time.Minute), Status: models.CheckStatusError, TotalLines: 0, ErrorMessage: "read failed"},
	}
	for _, result := range results {
		mustNoError(t, store.CreateCheckResult(ctx, result))
	}

	historyPage, err := store.ListCheckResults(ctx, logFile.ID, 1, 2)
	mustNoError(t, err)
	if len(historyPage) != 2 || historyPage[0].ID != "check-it-order-2" || historyPage[1].ID != "check-it-order-3" {
		t.Fatalf("expected chronological check history page, got %#v", historyPage)
	}

	latest, err := store.GetLatestCheckResult(ctx, logFile.ID)
	mustNoError(t, err)
	if latest.ID != "check-it-order-3" || latest.Status != models.CheckStatusError || latest.ErrorMessage != "read failed" {
		t.Fatalf("expected latest error check result, got %#v", latest)
	}
}

func TestPostgresRepositoryIntegrationBatchRollbackOnDuplicateLine(t *testing.T) {
	ctx := context.Background()
	store := newPostgresRepository(t, ctx)
	serverModel := createIntegrationServer(t, ctx, store, "srv-it-batch-rollback", "batch-rollback", "192.0.2.50")
	logFile := createIntegrationLogFile(t, ctx, store, "log-it-batch-rollback", serverModel.ID, "/var/log/batch.log")

	entries := []*models.LogEntry{
		{ID: "entry-it-rollback-1", LogFileID: logFile.ID, LineNumber: 1, Content: "first", Hash: "hash-1"},
		{ID: "entry-it-rollback-2", LogFileID: logFile.ID, LineNumber: 1, Content: "duplicate", Hash: "hash-2"},
	}
	chunks := []*models.LogChunk{
		newIntegrationChunk("chunk-it-rollback-1", logFile.ID, 1, 1, 1),
	}

	if err := store.CreateLogEntriesWithChunks(ctx, entries, chunks); !errors.Is(err, repository.ErrConflict) {
		t.Fatalf("expected duplicate line conflict, got %v", err)
	}

	count, err := store.CountLogEntries(ctx, logFile.ID)
	mustNoError(t, err)
	if count != 0 {
		t.Fatalf("expected failed batch to rollback entries, got %d entries", count)
	}

	chunkList, err := store.ListLogChunks(ctx, logFile.ID, 0, 0)
	mustNoError(t, err)
	if len(chunkList) != 0 {
		t.Fatalf("expected failed batch to rollback chunks, got %#v", chunkList)
	}
}

func createIntegrationServer(t *testing.T, ctx context.Context, store repository.Repository, id, name, host string) *models.Server {
	t.Helper()

	serverModel := &models.Server{
		ID:        id,
		Name:      name,
		Host:      host,
		Port:      22,
		Username:  "demo",
		AuthType:  models.AuthPassword,
		AuthValue: "plain-secret",
		Status:    models.ServerStatusActive,
	}
	mustNoError(t, store.CreateServer(ctx, serverModel))
	return serverModel
}

func createIntegrationLogFile(t *testing.T, ctx context.Context, store repository.Repository, id, serverID, path string) *models.LogFile {
	t.Helper()

	logFile := &models.LogFile{
		ID:       id,
		ServerID: serverID,
		Path:     path,
		LogType:  models.LogTypeApp,
		IsActive: true,
	}
	mustNoError(t, store.CreateLogFile(ctx, logFile))
	return logFile
}

func newIntegrationChunk(id, logFileID string, chunkNumber, fromLine, toLine int64) *models.LogChunk {
	return &models.LogChunk{
		ID:             id,
		LogFileID:      logFileID,
		ChunkNumber:    chunkNumber,
		FromLineNumber: fromLine,
		ToLineNumber:   toLine,
		FromByteOffset: fromLine * 10,
		ToByteOffset:   toLine * 10,
		EntriesCount:   int(toLine - fromLine + 1),
		Hash:           "chunk-hash",
		HashAlgorithm:  "hmac-sha256",
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
