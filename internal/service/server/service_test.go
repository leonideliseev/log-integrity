package server_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/lenchik/logmonitor/crons/locks"
	"github.com/lenchik/logmonitor/internal/repository/memory"
	discoveryservice "github.com/lenchik/logmonitor/internal/service/discovery"
	serverservice "github.com/lenchik/logmonitor/internal/service/server"
	"github.com/lenchik/logmonitor/internal/testsupport"
	"github.com/lenchik/logmonitor/models"
	"github.com/ozontech/allure-go/pkg/framework/provider"
	"github.com/ozontech/allure-go/pkg/framework/runner"
)

func TestServerServiceCreateListAndDiscover(t *testing.T) {
	runner.Run(t, "server service creates lists and discovers logs", func(t provider.T) {
		t.Epic("Service layer")
		t.Feature("Server application service")
		t.Story("Server lifecycle")
		t.Title("Creates server with default status and synchronizes discovered logs")

		ctx := context.Background()
		store := memory.New()
		factory := &testsupport.SSHClientFactory{Execute: func(cmd string) (string, error) {
			if strings.Contains(cmd, "find /var/log") {
				return "/var/log/syslog", nil
			}
			return "", nil
		}}
		discovery := discoveryservice.NewService(factory, store, nil)
		service := serverservice.NewService(store, discovery)
		serverModel := &models.Server{
			ID:        "srv-server",
			Name:      "server-host",
			Host:      "127.0.0.1",
			Port:      22,
			Username:  "demo",
			AuthType:  models.AuthPassword,
			AuthValue: "demo",
			OSType:    models.OSLinux,
		}

		t.WithNewStep("Create server and verify default active status", func(step provider.StepCtx) {
			step.Require().NoError(service.Create(ctx, serverModel))
			items, err := service.List(ctx)
			step.Require().NoError(err)
			step.Require().Len(items, 1)
			step.Require().Equal(models.ServerStatusActive, items[0].Status)
		})

		t.WithNewStep("Run discovery for selected server", func(step provider.StepCtx) {
			result, err := service.Discover(ctx, serverModel.ID)
			step.Require().NoError(err)
			step.Require().Len(result[serverModel.ID].LogFiles, 1)
			step.Require().Empty(result[serverModel.ID].Error)
		})
	})
}

func TestServerServiceDiscoverReportsBusyServer(t *testing.T) {
	runner.Run(t, "server discovery respects isolation lock", func(t provider.T) {
		t.Epic("Service layer")
		t.Feature("Server application service")
		t.Story("Manual operation isolation")
		t.Title("Discover returns per-server busy result when lock is already held")

		ctx := context.Background()
		store := memory.New()
		serverModel := &models.Server{
			ID:        "srv-busy",
			Name:      "busy-host",
			Host:      "127.0.0.1",
			Username:  "demo",
			AuthType:  models.AuthPassword,
			AuthValue: "demo",
			OSType:    models.OSLinux,
		}
		t.Require().NoError(store.CreateServer(ctx, serverModel))
		lockManager := locks.NewManager()
		unlock, ok := lockManager.TryLock("server:" + serverModel.ID)
		t.Require().True(ok)
		defer unlock()
		service := serverservice.NewServiceWithLocker(store, discoveryservice.NewService(&testsupport.SSHClientFactory{}, store, nil), lockManager)

		t.WithNewStep("Try discovery while server lock is held", func(step provider.StepCtx) {
			result, err := service.Discover(ctx, serverModel.ID)
			step.Require().NoError(err)
			step.Require().Contains(result[serverModel.ID].Error, "busy")
		})
	})
}

func TestServerServiceGetUpdateAndDeleteRuntimeManagedServer(t *testing.T) {
	runner.Run(t, "server service supports runtime managed crud", func(t provider.T) {
		t.Epic("Service layer")
		t.Feature("Server application service")
		t.Story("Runtime managed servers")
		t.Title("Gets updates and deletes an API-managed server")

		ctx := context.Background()
		store := memory.New()
		service := serverservice.NewService(store, discoveryservice.NewService(&testsupport.SSHClientFactory{}, store, nil))
		serverModel := &models.Server{
			ID:        "srv-runtime",
			Name:      "runtime-host",
			Host:      "10.0.0.10",
			Port:      22,
			Username:  "demo",
			AuthType:  models.AuthPassword,
			AuthValue: "secret",
			OSType:    models.OSLinux,
			ManagedBy: models.ServerManagedByAPI,
		}

		t.WithNewStep("Create server and fetch it back", func(step provider.StepCtx) {
			step.Require().NoError(service.Create(ctx, serverModel))

			storedServer, err := service.Get(ctx, serverModel.ID)
			step.Require().NoError(err)
			step.Require().Equal(serverModel.Name, storedServer.Name)
			step.Require().Equal(serverModel.AuthValue, storedServer.AuthValue)
		})

		t.WithNewStep("Update server and keep existing auth value when payload omits it", func(step provider.StepCtx) {
			updateModel := &models.Server{
				ID:       serverModel.ID,
				Name:     "runtime-host-updated",
				Host:     "10.0.0.11",
				Port:     2222,
				Username: "admin",
				AuthType: models.AuthPassword,
				OSType:   models.OSLinux,
				Status:   models.ServerStatusInactive,
			}

			step.Require().NoError(service.Update(ctx, updateModel))

			storedServer, err := service.Get(ctx, serverModel.ID)
			step.Require().NoError(err)
			step.Require().Equal("runtime-host-updated", storedServer.Name)
			step.Require().Equal("10.0.0.11", storedServer.Host)
			step.Require().Equal(2222, storedServer.Port)
			step.Require().Equal("admin", storedServer.Username)
			step.Require().Equal(models.ServerStatusInactive, storedServer.Status)
			step.Require().Equal("secret", storedServer.AuthValue)
			step.Require().Equal(models.ServerManagedByAPI, storedServer.ManagedBy)
		})

		t.WithNewStep("Delete server and verify it is gone", func(step provider.StepCtx) {
			step.Require().NoError(service.Delete(ctx, serverModel.ID))

			_, err := service.Get(ctx, serverModel.ID)
			step.Require().Error(err)
		})
	})
}

func TestServerServiceRejectsConfigManagedMutation(t *testing.T) {
	runner.Run(t, "server service protects config managed servers from api mutation", func(t provider.T) {
		t.Epic("Service layer")
		t.Feature("Server application service")
		t.Story("Config bootstrap ownership")
		t.Title("Rejects update and delete for config-managed servers")

		ctx := context.Background()
		store := memory.New()
		service := serverservice.NewService(store, discoveryservice.NewService(&testsupport.SSHClientFactory{}, store, nil))
		serverModel := &models.Server{
			ID:        "srv-config",
			Name:      "config-host",
			Host:      "10.0.0.20",
			Port:      22,
			Username:  "demo",
			AuthType:  models.AuthPassword,
			AuthValue: "secret",
			OSType:    models.OSLinux,
			ManagedBy: models.ServerManagedByConfig,
		}
		t.Require().NoError(store.CreateServer(ctx, serverModel))

		t.WithNewStep("Reject update for config-managed server", func(step provider.StepCtx) {
			err := service.Update(ctx, &models.Server{
				ID:        serverModel.ID,
				Name:      "config-host-updated",
				Host:      "10.0.0.21",
				Port:      22,
				Username:  "admin",
				AuthType:  models.AuthPassword,
				AuthValue: "new-secret",
				OSType:    models.OSLinux,
			})
			step.Require().Error(err)
			step.Require().Contains(err.Error(), "managed by config")
		})

		t.WithNewStep("Reject delete for config-managed server", func(step provider.StepCtx) {
			err := service.Delete(ctx, serverModel.ID)
			step.Require().Error(err)
			step.Require().Contains(err.Error(), "managed by config")
		})
	})
}

func TestServerServiceRetryAndListProblems(t *testing.T) {
	runner.Run(t, "server service exposes retry and operational problems", func(t provider.T) {
		t.Epic("Service layer")
		t.Feature("Server application service")
		t.Story("Operational visibility")
		t.Title("Lists aggregated problems and clears retry state")

		ctx := context.Background()
		store := memory.New()
		service := serverservice.NewService(store, discoveryservice.NewService(&testsupport.SSHClientFactory{}, store, nil))

		serverModel := &models.Server{
			ID:           "srv-problem",
			Name:         "problem-host",
			Host:         "10.0.0.30",
			Port:         22,
			Username:     "demo",
			AuthType:     models.AuthPassword,
			AuthValue:    "secret",
			Status:       models.ServerStatusError,
			ManagedBy:    models.ServerManagedByAPI,
			LastError:    "ssh connection refused",
			FailureCount: 2,
		}
		backoffUntil := time.Date(2026, 4, 23, 14, 0, 0, 0, time.UTC)
		serverModel.BackoffUntil = &backoffUntil
		t.Require().NoError(store.CreateServer(ctx, serverModel))

		logFile := &models.LogFile{
			ID:       "log-problem",
			ServerID: serverModel.ID,
			Path:     "/var/log/syslog",
			LogType:  models.LogTypeSyslog,
			IsActive: true,
		}
		t.Require().NoError(store.CreateLogFile(ctx, logFile))
		t.Require().NoError(store.CreateCheckResult(ctx, &models.CheckResult{
			ID:            "check-problem",
			LogFileID:     logFile.ID,
			CheckedAt:     time.Date(2026, 4, 23, 13, 0, 0, 0, time.UTC),
			Status:        models.CheckStatusTampered,
			TotalLines:    10,
			TamperedLines: 2,
		}))

		t.WithNewStep("List aggregated server and integrity problems", func(step provider.StepCtx) {
			problems, err := service.ListProblems(ctx)
			step.Require().NoError(err)
			step.Require().Len(problems, 3)
			step.Require().Equal(models.ProblemSeverityCritical, problems[0].Severity)
			step.Require().Equal(models.ProblemTypeIntegrityTampered, problems[0].Type)
		})

		t.WithNewStep("Clear retry state for the failed server", func(step provider.StepCtx) {
			updatedServer, err := service.Retry(ctx, serverModel.ID)
			step.Require().NoError(err)
			step.Require().Equal(models.ServerStatusActive, updatedServer.Status)
			step.Require().Nil(updatedServer.BackoffUntil)
			step.Require().Zero(updatedServer.FailureCount)
			step.Require().Empty(updatedServer.LastError)
		})
	})
}
