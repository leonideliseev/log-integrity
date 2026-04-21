package server_test

import (
	"context"
	"strings"
	"testing"

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
