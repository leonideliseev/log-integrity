package logfile_test

import (
	"context"
	"strings"
	"testing"

	"github.com/lenchik/logmonitor/crons/locks"
	"github.com/lenchik/logmonitor/internal/repository/memory"
	collectorservice "github.com/lenchik/logmonitor/internal/service/collector"
	logfileservice "github.com/lenchik/logmonitor/internal/service/logfile"
	"github.com/lenchik/logmonitor/internal/testsupport"
	"github.com/lenchik/logmonitor/models"
	"github.com/ozontech/allure-go/pkg/framework/provider"
	"github.com/ozontech/allure-go/pkg/framework/runner"
)

func TestLogFileServiceListAndCollect(t *testing.T) {
	runner.Run(t, "logfile service lists and collects logs", func(t provider.T) {
		t.Epic("Service layer")
		t.Feature("Log file application service")
		t.Story("Manual collection")
		t.Title("Lists server log files and collects selected log through collector")

		ctx := context.Background()
		store, serverModel, logFile := prepareLogFileRepository(t)
		factory := &testsupport.SSHClientFactory{Execute: func(cmd string) (string, error) {
			if strings.Contains(cmd, "awk") {
				return "1\talpha\n2\tbeta", nil
			}
			if strings.Contains(cmd, "stat -c") {
				return "device_id=dev\ninode=ino\nsize_bytes=12\nmod_time_unix=1", nil
			}
			return "", nil
		}}
		collector := collectorservice.NewServiceWithOptions(factory, store, store, store, collectorservice.Options{})
		service := logfileservice.NewService(store, store, collector)

		t.WithNewStep("List log files by server", func(step provider.StepCtx) {
			items, err := service.List(ctx, serverModel.ID)
			step.Require().NoError(err)
			step.Require().Len(items, 1)
		})

		t.WithNewStep("Collect selected log file", func(step provider.StepCtx) {
			result, err := service.Collect(ctx, serverModel.ID, logFile.ID)
			step.Require().NoError(err)
			step.Require().Equal(2, result[logFile.ID].CollectedEntries)
			step.Require().Empty(result[logFile.ID].Error)
		})
	})
}

func TestLogFileServiceRejectsForeignLogFile(t *testing.T) {
	runner.Run(t, "logfile service rejects foreign log file", func(t provider.T) {
		t.Epic("Service layer")
		t.Feature("Log file application service")
		t.Story("Input validation")
		t.Title("Manual collection fails when log_file_id belongs to another server")

		ctx := context.Background()
		store, serverModel, _ := prepareLogFileRepository(t)
		foreignServer := &models.Server{ID: "srv-foreign", Name: "foreign", Host: "127.0.0.2", Username: "demo", AuthType: models.AuthPassword, AuthValue: "demo"}
		t.Require().NoError(store.CreateServer(ctx, foreignServer))
		foreignLog := &models.LogFile{ID: "log-foreign", ServerID: foreignServer.ID, Path: "/var/log/foreign.log", LogType: models.LogTypeApp, IsActive: true}
		t.Require().NoError(store.CreateLogFile(ctx, foreignLog))
		collector := collectorservice.NewServiceWithOptions(&testsupport.SSHClientFactory{}, store, store, store, collectorservice.Options{})
		service := logfileservice.NewService(store, store, collector)

		t.WithNewStep("Collect log file from another server", func(step provider.StepCtx) {
			result, err := service.Collect(ctx, serverModel.ID, foreignLog.ID)
			step.Require().Error(err)
			step.Require().Nil(result)
			step.Require().Contains(err.Error(), "does not belong")
		})
	})
}

func TestLogFileServiceCollectReportsBusyServer(t *testing.T) {
	runner.Run(t, "logfile service respects isolation lock", func(t provider.T) {
		t.Epic("Service layer")
		t.Feature("Log file application service")
		t.Story("Manual operation isolation")
		t.Title("Collect fails fast when server lock is already held")

		ctx := context.Background()
		store, serverModel, logFile := prepareLogFileRepository(t)
		lockManager := locks.NewManager()
		unlock, ok := lockManager.TryLock("server:" + serverModel.ID)
		t.Require().True(ok)
		defer unlock()
		collector := collectorservice.NewServiceWithOptions(&testsupport.SSHClientFactory{}, store, store, store, collectorservice.Options{})
		service := logfileservice.NewServiceWithLocker(store, store, collector, lockManager)

		t.WithNewStep("Collect while server is locked", func(step provider.StepCtx) {
			result, err := service.Collect(ctx, serverModel.ID, logFile.ID)
			step.Require().Error(err)
			step.Require().Nil(result)
			step.Require().Contains(err.Error(), "busy")
		})
	})
}

func prepareLogFileRepository(t provider.T) (*memory.Storage, *models.Server, *models.LogFile) {
	ctx := context.Background()
	store := memory.New()
	serverModel := &models.Server{
		ID:        "srv-logfile",
		Name:      "logfile-host",
		Host:      "127.0.0.1",
		Port:      22,
		Username:  "demo",
		AuthType:  models.AuthPassword,
		AuthValue: "demo",
		OSType:    models.OSLinux,
		Status:    models.ServerStatusActive,
	}
	t.Require().NoError(store.CreateServer(ctx, serverModel))
	logFile := &models.LogFile{ID: "log-logfile", ServerID: serverModel.ID, Path: "/var/log/app.log", LogType: models.LogTypeApp, IsActive: true}
	t.Require().NoError(store.CreateLogFile(ctx, logFile))
	return store, serverModel, logFile
}
