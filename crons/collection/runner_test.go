package collectioncron

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/lenchik/logmonitor/crons/locks"
	"github.com/lenchik/logmonitor/internal/repository/memory"
	collectorservice "github.com/lenchik/logmonitor/internal/service/collector"
	"github.com/lenchik/logmonitor/internal/testsupport"
	"github.com/lenchik/logmonitor/models"
	"github.com/ozontech/allure-go/pkg/framework/provider"
	"github.com/ozontech/allure-go/pkg/framework/runner"
)

func TestCollectionRunnerCollectsOnlyActiveLogFiles(t *testing.T) {
	runner.Run(t, "collection cron collects only active log files", func(t provider.T) {
		t.Epic("Crons")
		t.Feature("Collection runner")
		t.Story("Scheduled log collection")
		t.Title("Runner filters inactive log files before invoking collector")

		ctx := context.Background()
		store, serverModel, activeLog, inactiveLog := prepareCollectionCronRepository(t)
		factory := &testsupport.SSHClientFactory{Execute: func(cmd string) (string, error) {
			switch {
			case strings.Contains(cmd, "active.log") && strings.Contains(cmd, "awk"):
				return "1\tactive", nil
			case strings.Contains(cmd, "inactive.log") && strings.Contains(cmd, "awk"):
				return "1\tinactive", nil
			case strings.Contains(cmd, "stat -c"):
				return "device_id=dev\ninode=ino\nsize_bytes=1\nmod_time_unix=1", nil
			default:
				return "", nil
			}
		}}
		collector := collectorservice.NewServiceWithOptions(factory, store, store, store, collectorservice.Options{})
		cron := NewRunnerWithOptions(testLogger(), store, store, collector, nil, Options{MaxServerWorkers: 2, MaxLogFileWorkersPerHost: 2})

		t.WithNewStep("Run collection sweep", func(step provider.StepCtx) {
			cron.Run(ctx)
			activeCount, err := store.CountLogEntries(ctx, activeLog.ID)
			step.Require().NoError(err)
			step.Require().EqualValues(1, activeCount)

			inactiveCount, err := store.CountLogEntries(ctx, inactiveLog.ID)
			step.Require().NoError(err)
			step.Require().EqualValues(0, inactiveCount)
			step.Require().Equal("srv-collection", serverModel.ID)
		})
	})
}

func TestCollectionRunnerSkipsLockedServer(t *testing.T) {
	runner.Run(t, "collection cron skips locked server", func(t provider.T) {
		t.Epic("Crons")
		t.Feature("Collection runner")
		t.Story("Per-server isolation")
		t.Title("Runner does not collect logs when server lock is already held")

		ctx := context.Background()
		store, serverModel, activeLog, _ := prepareCollectionCronRepository(t)
		lockManager := locks.NewManager()
		unlock, ok := lockManager.TryLock("server:" + serverModel.ID)
		t.Require().True(ok)
		defer unlock()
		collector := collectorservice.NewServiceWithOptions(&testsupport.SSHClientFactory{}, store, store, store, collectorservice.Options{})
		cron := NewRunnerWithOptions(testLogger(), store, store, collector, lockManager, Options{})

		t.WithNewStep("Run collection while lock is held", func(step provider.StepCtx) {
			cron.Run(ctx)
			count, err := store.CountLogEntries(ctx, activeLog.ID)
			step.Require().NoError(err)
			step.Require().EqualValues(0, count)
		})
	})
}

func prepareCollectionCronRepository(t provider.T) (*memory.Storage, *models.Server, *models.LogFile, *models.LogFile) {
	ctx := context.Background()
	store := memory.New()
	serverModel := &models.Server{
		ID:        "srv-collection",
		Name:      "collection-host",
		Host:      "127.0.0.1",
		Username:  "demo",
		AuthType:  models.AuthPassword,
		AuthValue: "demo",
		OSType:    models.OSLinux,
		Status:    models.ServerStatusActive,
	}
	t.Require().NoError(store.CreateServer(ctx, serverModel))
	activeLog := &models.LogFile{ID: "log-active", ServerID: serverModel.ID, Path: "/var/log/active.log", LogType: models.LogTypeApp, IsActive: true}
	inactiveLog := &models.LogFile{ID: "log-inactive", ServerID: serverModel.ID, Path: "/var/log/inactive.log", LogType: models.LogTypeApp, IsActive: false}
	t.Require().NoError(store.CreateLogFile(ctx, activeLog))
	t.Require().NoError(store.CreateLogFile(ctx, inactiveLog))
	return store, serverModel, activeLog, inactiveLog
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
