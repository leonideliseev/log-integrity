package discovery_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/lenchik/logmonitor/internal/repository/memory"
	discoveryservice "github.com/lenchik/logmonitor/internal/service/discovery"
	"github.com/lenchik/logmonitor/internal/testsupport"
	"github.com/lenchik/logmonitor/models"
	"github.com/ozontech/allure-go/pkg/framework/provider"
	"github.com/ozontech/allure-go/pkg/framework/runner"
)

func TestDiscoveryServiceSyncCreatesUpdatesAndDeactivatesLogFiles(t *testing.T) {
	runner.Run(t, "discovery sync reconciles log files", func(t provider.T) {
		t.Epic("Service layer")
		t.Feature("Discovery")
		t.Story("Repository synchronization")
		t.Title("Creates new log files, updates existing ones and deactivates disappeared paths")

		ctx := context.Background()
		store, serverModel := prepareDiscoveryRepository(t)
		existing := &models.LogFile{
			ID:       "log-existing",
			ServerID: serverModel.ID,
			Path:     "/var/log/old.log",
			LogType:  models.LogTypeUnknown,
			IsActive: true,
		}
		stale := &models.LogFile{
			ID:       "log-stale",
			ServerID: serverModel.ID,
			Path:     "/var/log/stale.log",
			LogType:  models.LogTypeApp,
			IsActive: true,
		}
		t.Require().NoError(store.CreateLogFile(ctx, existing))
		t.Require().NoError(store.CreateLogFile(ctx, stale))
		service := discoveryservice.NewService(&testsupport.SSHClientFactory{}, store, nil)

		var synced []*models.LogFile
		t.WithNewStep("Sync discovered paths", func(step provider.StepCtx) {
			var err error
			synced, err = service.Sync(ctx, serverModel.ID, []discoveryservice.DiscoveredLog{
				{Path: "/var/log/old.log", LogType: models.LogTypeSyslog},
				{Path: "/var/log/new.log", LogType: models.LogTypeApp},
			})
			step.Require().NoError(err)
			step.Require().Len(synced, 3)
		})

		t.WithNewStep("Verify existing, new and stale log states", func(step provider.StepCtx) {
			updatedOld, err := store.GetLogFileByID(ctx, "log-existing")
			step.Require().NoError(err)
			step.Require().Equal(models.LogTypeSyslog, updatedOld.LogType)
			step.Require().True(updatedOld.IsActive)

			updatedStale, err := store.GetLogFileByID(ctx, "log-stale")
			step.Require().NoError(err)
			step.Require().False(updatedStale.IsActive)

			active, err := store.ListActiveLogFiles(ctx)
			step.Require().NoError(err)
			step.Require().Len(active, 2)
		})
	})
}

func TestDiscoveryServiceDiscoverDetectsLinuxAndParsesLogs(t *testing.T) {
	runner.Run(t, "discovery detects OS and parses Linux logs", func(t provider.T) {
		t.Epic("Service layer")
		t.Feature("Discovery")
		t.Story("Remote discovery")
		t.Title("Detects Linux over SSH and parses discovered log paths")

		ctx := context.Background()
		store, serverModel := prepareDiscoveryRepository(t)
		serverModel.OSType = ""
		factory := &testsupport.SSHClientFactory{Execute: func(cmd string) (string, error) {
			switch {
			case cmd == "uname -s":
				return "Linux", nil
			case strings.Contains(cmd, "find /var/log"):
				return "/var/log/syslog\n/var/log/nginx/access.log\n/var/log/syslog", nil
			default:
				return "", nil
			}
		}}
		service := discoveryservice.NewService(factory, store, nil)

		t.WithNewStep("Run discovery for server without preset OS", func(step provider.StepCtx) {
			discovered, err := service.Discover(ctx, serverModel)
			step.Require().NoError(err)
			step.Require().Len(discovered, 2)
			step.Require().Equal(models.OSLinux, serverModel.OSType)
			step.Require().Equal(models.LogTypeSyslog, discovered[0].LogType)
			step.Require().Equal(models.LogTypeNginx, discovered[1].LogType)
		})
	})
}

func TestDiscoveryServiceDiscoverReturnsConnectionError(t *testing.T) {
	runner.Run(t, "discovery returns connection errors", func(t provider.T) {
		t.Epic("Service layer")
		t.Feature("Discovery")
		t.Story("Remote failure")
		t.Title("SSH connection failure is propagated to caller")

		ctx := context.Background()
		store, serverModel := prepareDiscoveryRepository(t)
		factory := &testsupport.SSHClientFactory{ConnectErr: errors.New("ssh refused")}
		service := discoveryservice.NewService(factory, store, nil)

		t.WithNewStep("Run discovery with failing SSH connect", func(step provider.StepCtx) {
			discovered, err := service.Discover(ctx, serverModel)
			step.Require().Error(err)
			step.Require().Nil(discovered)
			step.Require().Contains(err.Error(), "ssh refused")
		})
	})
}

func TestDetectOSTypeSupportsLinuxMacOSWindowsAndUnknown(t *testing.T) {
	runner.Run(t, "discovery detects OS type", func(t provider.T) {
		t.Epic("Service layer")
		t.Feature("Discovery")
		t.Story("OS detection")
		t.Title("Detects supported operating systems from probe command outputs")

		cases := []struct {
			name    string
			outputs map[string]string
			want    models.OSType
			wantErr bool
		}{
			{name: "linux", outputs: map[string]string{"uname -s": "Linux"}, want: models.OSLinux},
			{name: "macos", outputs: map[string]string{"uname -s": "Darwin"}, want: models.OSMacOS},
			{name: "windows powershell", outputs: map[string]string{`powershell -NoProfile -Command "$env:OS"`: "Windows_NT"}, want: models.OSWindows},
			{name: "unknown", outputs: map[string]string{}, wantErr: true},
		}

		for _, tc := range cases {
			tc := tc
			t.WithNewStep("Detect "+tc.name, func(step provider.StepCtx) {
				client := (&testsupport.SSHClientFactory{Execute: func(cmd string) (string, error) {
					if output, ok := tc.outputs[cmd]; ok {
						return output, nil
					}
					return "", errors.New("not supported")
				}}).NewClient()
				got, err := discoveryservice.DetectOSType(context.Background(), client)
				if tc.wantErr {
					step.Require().Error(err)
					return
				}
				step.Require().NoError(err)
				step.Require().Equal(tc.want, got)
			})
		}
	})
}

func prepareDiscoveryRepository(t provider.T) (*memory.Storage, *models.Server) {
	ctx := context.Background()
	store := memory.New()
	serverModel := &models.Server{
		ID:        "srv-discovery",
		Name:      "discovery-host",
		Host:      "127.0.0.1",
		Port:      22,
		Username:  "demo",
		AuthType:  models.AuthPassword,
		AuthValue: "demo",
		OSType:    models.OSLinux,
		Status:    models.ServerStatusActive,
	}
	t.Require().NoError(store.CreateServer(ctx, serverModel))
	return store, serverModel
}
