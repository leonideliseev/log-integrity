package check_test

import (
	"context"
	"strings"
	"testing"

	"github.com/lenchik/logmonitor/crons/locks"
	"github.com/lenchik/logmonitor/internal/repository/memory"
	checkservice "github.com/lenchik/logmonitor/internal/service/check"
	integrityservice "github.com/lenchik/logmonitor/internal/service/integrity"
	"github.com/lenchik/logmonitor/internal/testsupport"
	"github.com/lenchik/logmonitor/models"
	"github.com/lenchik/logmonitor/pkg/hasher"
	"github.com/ozontech/allure-go/pkg/framework/provider"
	"github.com/ozontech/allure-go/pkg/framework/runner"
)

func TestCheckServiceListAndRun(t *testing.T) {
	runner.Run(t, "check service lists and runs integrity checks", func(t provider.T) {
		t.Epic("Service layer")
		t.Feature("Check application service")
		t.Story("Manual integrity check")
		t.Title("Runs selected log check and exposes stored history")

		ctx := context.Background()
		store, serverModel, logFile := prepareCheckRepository(t)
		factory := &testsupport.SSHClientFactory{Execute: func(cmd string) (string, error) {
			if strings.Contains(cmd, "awk") {
				return "1\talpha", nil
			}
			return "", nil
		}}
		integrity := integrityservice.NewService(factory, store, store)
		service := checkservice.NewService(store, store, store, integrity)

		t.WithNewStep("Run selected log file check", func(step provider.StepCtx) {
			result, err := service.Run(ctx, serverModel.ID, logFile.ID)
			step.Require().NoError(err)
			step.Require().Equal(models.CheckStatusOK, result[logFile.ID].Result.Status)
			step.Require().Empty(result[logFile.ID].TamperedEntries)
		})

		t.WithNewStep("Read saved check history", func(step provider.StepCtx) {
			history, err := service.List(ctx, logFile.ID, 0, 10)
			step.Require().NoError(err)
			step.Require().Len(history, 1)
			step.Require().Equal(models.CheckStatusOK, history[0].Status)
		})
	})
}

func TestCheckServiceRejectsForeignLogFile(t *testing.T) {
	runner.Run(t, "check service rejects foreign log file", func(t provider.T) {
		t.Epic("Service layer")
		t.Feature("Check application service")
		t.Story("Input validation")
		t.Title("Manual check fails when log_file_id belongs to another server")

		ctx := context.Background()
		store, serverModel, _ := prepareCheckRepository(t)
		foreignServer := &models.Server{ID: "srv-check-foreign", Name: "foreign", Host: "127.0.0.2", Username: "demo", AuthType: models.AuthPassword, AuthValue: "demo"}
		t.Require().NoError(store.CreateServer(ctx, foreignServer))
		foreignLog := &models.LogFile{ID: "log-check-foreign", ServerID: foreignServer.ID, Path: "/var/log/foreign.log", LogType: models.LogTypeApp, IsActive: true}
		t.Require().NoError(store.CreateLogFile(ctx, foreignLog))
		service := checkservice.NewService(store, store, store, integrityservice.NewService(&testsupport.SSHClientFactory{}, store, store))

		t.WithNewStep("Run check for another server's log file", func(step provider.StepCtx) {
			result, err := service.Run(ctx, serverModel.ID, foreignLog.ID)
			step.Require().Error(err)
			step.Require().Nil(result)
			step.Require().Contains(err.Error(), "does not belong")
		})
	})
}

func TestCheckServiceRunReportsBusyServer(t *testing.T) {
	runner.Run(t, "check service respects isolation lock", func(t provider.T) {
		t.Epic("Service layer")
		t.Feature("Check application service")
		t.Story("Manual operation isolation")
		t.Title("Run fails fast when server lock is already held")

		ctx := context.Background()
		store, serverModel, logFile := prepareCheckRepository(t)
		lockManager := locks.NewManager()
		unlock, ok := lockManager.TryLock("server:" + serverModel.ID)
		t.Require().True(ok)
		defer unlock()
		service := checkservice.NewServiceWithLocker(store, store, store, integrityservice.NewService(&testsupport.SSHClientFactory{}, store, store), lockManager)

		t.WithNewStep("Run check while server is locked", func(step provider.StepCtx) {
			result, err := service.Run(ctx, serverModel.ID, logFile.ID)
			step.Require().Error(err)
			step.Require().Nil(result)
			step.Require().Contains(err.Error(), "busy")
		})
	})
}

func prepareCheckRepository(t provider.T) (*memory.Storage, *models.Server, *models.LogFile) {
	ctx := context.Background()
	store := memory.New()
	serverModel := &models.Server{
		ID:        "srv-check",
		Name:      "check-host",
		Host:      "127.0.0.1",
		Port:      22,
		Username:  "demo",
		AuthType:  models.AuthPassword,
		AuthValue: "demo",
		OSType:    models.OSLinux,
		Status:    models.ServerStatusActive,
	}
	t.Require().NoError(store.CreateServer(ctx, serverModel))
	logFile := &models.LogFile{ID: "log-check", ServerID: serverModel.ID, Path: "/var/log/app.log", LogType: models.LogTypeApp, IsActive: true}
	t.Require().NoError(store.CreateLogFile(ctx, logFile))
	t.Require().NoError(store.CreateLogEntry(ctx, &models.LogEntry{LogFileID: logFile.ID, LineNumber: 1, Content: "alpha", Hash: hasher.SHA256String("alpha")}))
	return store, serverModel, logFile
}
