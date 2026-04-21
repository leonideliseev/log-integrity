package integritycron

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/lenchik/logmonitor/crons/locks"
	"github.com/lenchik/logmonitor/internal/repository/memory"
	integrityservice "github.com/lenchik/logmonitor/internal/service/integrity"
	"github.com/lenchik/logmonitor/internal/testsupport"
	"github.com/lenchik/logmonitor/models"
	"github.com/lenchik/logmonitor/pkg/hasher"
	"github.com/ozontech/allure-go/pkg/framework/provider"
	"github.com/ozontech/allure-go/pkg/framework/runner"
)

func TestIntegrityRunnerChecksOnlyActiveLogFiles(t *testing.T) {
	runner.Run(t, "integrity cron checks only active log files", func(t provider.T) {
		t.Epic("Crons")
		t.Feature("Integrity runner")
		t.Story("Scheduled integrity check")
		t.Title("Runner filters inactive log files before invoking integrity service")

		ctx := context.Background()
		store, serverModel, activeLog, inactiveLog := prepareIntegrityCronRepository(t)
		factory := &testsupport.SSHClientFactory{Execute: func(cmd string) (string, error) {
			if strings.Contains(cmd, "awk") {
				return "1\talpha", nil
			}
			return "", nil
		}}
		service := integrityservice.NewService(factory, store, store)
		cron := NewRunnerWithOptions(testLogger(), store, store, service, nil, Options{MaxServerWorkers: 2, MaxLogFileWorkersPerHost: 2})

		t.WithNewStep("Run integrity sweep", func(step provider.StepCtx) {
			cron.Run(ctx)
			activeChecks, err := store.ListCheckResults(ctx, activeLog.ID, 0, 0)
			step.Require().NoError(err)
			step.Require().Len(activeChecks, 1)
			step.Require().Equal(models.CheckStatusOK, activeChecks[0].Status)

			inactiveChecks, err := store.ListCheckResults(ctx, inactiveLog.ID, 0, 0)
			step.Require().NoError(err)
			step.Require().Empty(inactiveChecks)
			step.Require().Equal("srv-integrity-cron", serverModel.ID)
		})
	})
}

func TestIntegrityRunnerSkipsLockedServer(t *testing.T) {
	runner.Run(t, "integrity cron skips locked server", func(t provider.T) {
		t.Epic("Crons")
		t.Feature("Integrity runner")
		t.Story("Per-server isolation")
		t.Title("Runner does not run checks when server lock is already held")

		ctx := context.Background()
		store, serverModel, activeLog, _ := prepareIntegrityCronRepository(t)
		lockManager := locks.NewManager()
		unlock, ok := lockManager.TryLock("server:" + serverModel.ID)
		t.Require().True(ok)
		defer unlock()
		service := integrityservice.NewService(&testsupport.SSHClientFactory{}, store, store)
		cron := NewRunnerWithOptions(testLogger(), store, store, service, lockManager, Options{})

		t.WithNewStep("Run integrity while lock is held", func(step provider.StepCtx) {
			cron.Run(ctx)
			checks, err := store.ListCheckResults(ctx, activeLog.ID, 0, 0)
			step.Require().NoError(err)
			step.Require().Empty(checks)
		})
	})
}

func prepareIntegrityCronRepository(t provider.T) (*memory.Storage, *models.Server, *models.LogFile, *models.LogFile) {
	ctx := context.Background()
	store := memory.New()
	serverModel := &models.Server{
		ID:        "srv-integrity-cron",
		Name:      "integrity-cron-host",
		Host:      "127.0.0.1",
		Username:  "demo",
		AuthType:  models.AuthPassword,
		AuthValue: "demo",
		OSType:    models.OSLinux,
		Status:    models.ServerStatusActive,
	}
	t.Require().NoError(store.CreateServer(ctx, serverModel))
	activeLog := &models.LogFile{ID: "log-integrity-active", ServerID: serverModel.ID, Path: "/var/log/active.log", LogType: models.LogTypeApp, IsActive: true}
	inactiveLog := &models.LogFile{ID: "log-integrity-inactive", ServerID: serverModel.ID, Path: "/var/log/inactive.log", LogType: models.LogTypeApp, IsActive: false}
	t.Require().NoError(store.CreateLogFile(ctx, activeLog))
	t.Require().NoError(store.CreateLogFile(ctx, inactiveLog))
	t.Require().NoError(store.CreateLogEntry(ctx, &models.LogEntry{LogFileID: activeLog.ID, LineNumber: 1, Content: "alpha", Hash: hasher.SHA256String("alpha")}))
	t.Require().NoError(store.CreateLogEntry(ctx, &models.LogEntry{LogFileID: inactiveLog.ID, LineNumber: 1, Content: "alpha", Hash: hasher.SHA256String("alpha")}))
	return store, serverModel, activeLog, inactiveLog
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
