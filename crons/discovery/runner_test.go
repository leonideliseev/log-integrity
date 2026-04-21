package discoverycron

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/lenchik/logmonitor/crons/locks"
	"github.com/lenchik/logmonitor/internal/repository/memory"
	discoveryservice "github.com/lenchik/logmonitor/internal/service/discovery"
	"github.com/lenchik/logmonitor/internal/testsupport"
	"github.com/lenchik/logmonitor/models"
	"github.com/ozontech/allure-go/pkg/framework/provider"
	"github.com/ozontech/allure-go/pkg/framework/runner"
)

func TestDiscoveryRunnerSynchronizesDiscoveredLogs(t *testing.T) {
	runner.Run(t, "discovery cron synchronizes discovered logs", func(t provider.T) {
		t.Epic("Crons")
		t.Feature("Discovery runner")
		t.Story("Scheduled discovery")
		t.Title("Runner discovers remote logs and syncs them into repository")

		ctx := context.Background()
		store, serverModel := prepareDiscoveryCronRepository(t)
		factory := &testsupport.SSHClientFactory{Execute: func(cmd string) (string, error) {
			if strings.Contains(cmd, "find /var/log") {
				return "/var/log/syslog\n/var/log/auth.log", nil
			}
			return "", nil
		}}
		service := discoveryservice.NewService(factory, store, nil)
		cron := NewRunnerWithOptions(testLogger(), store, service, nil, Options{MaxServerWorkers: 2})

		t.WithNewStep("Run discovery sweep", func(step provider.StepCtx) {
			cron.Run(ctx)
			logs, err := store.ListLogFilesByServer(ctx, serverModel.ID)
			step.Require().NoError(err)
			step.Require().Len(logs, 2)
			step.Require().Equal(models.LogTypeAuth, logs[0].LogType)
			step.Require().Equal(models.LogTypeSyslog, logs[1].LogType)
		})
	})
}

func TestDiscoveryRunnerSkipsLockedServer(t *testing.T) {
	runner.Run(t, "discovery cron skips locked server", func(t provider.T) {
		t.Epic("Crons")
		t.Feature("Discovery runner")
		t.Story("Per-server isolation")
		t.Title("Runner does not run discovery when server lock is already held")

		ctx := context.Background()
		store, serverModel := prepareDiscoveryCronRepository(t)
		lockManager := locks.NewManager()
		unlock, ok := lockManager.TryLock("server:" + serverModel.ID)
		t.Require().True(ok)
		defer unlock()
		service := discoveryservice.NewService(&testsupport.SSHClientFactory{}, store, nil)
		cron := NewRunnerWithOptions(testLogger(), store, service, lockManager, Options{})

		t.WithNewStep("Run discovery while lock is held", func(step provider.StepCtx) {
			cron.Run(ctx)
			logs, err := store.ListLogFilesByServer(ctx, serverModel.ID)
			step.Require().NoError(err)
			step.Require().Empty(logs)
		})
	})
}

func prepareDiscoveryCronRepository(t provider.T) (*memory.Storage, *models.Server) {
	ctx := context.Background()
	store := memory.New()
	serverModel := &models.Server{
		ID:        "srv-discovery-cron",
		Name:      "discovery-cron-host",
		Host:      "127.0.0.1",
		Username:  "demo",
		AuthType:  models.AuthPassword,
		AuthValue: "demo",
		OSType:    models.OSLinux,
		Status:    models.ServerStatusActive,
	}
	t.Require().NoError(store.CreateServer(ctx, serverModel))
	return store, serverModel
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
