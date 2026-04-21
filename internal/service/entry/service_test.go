package entry_test

import (
	"context"
	"testing"

	"github.com/lenchik/logmonitor/internal/repository/memory"
	entryservice "github.com/lenchik/logmonitor/internal/service/entry"
	_ "github.com/lenchik/logmonitor/internal/testsupport"
	"github.com/lenchik/logmonitor/models"
	"github.com/ozontech/allure-go/pkg/framework/provider"
	"github.com/ozontech/allure-go/pkg/framework/runner"
)

func TestEntryServiceListReturnsPaginatedEntries(t *testing.T) {
	runner.Run(t, "entry service returns paginated entries", func(t provider.T) {
		t.Epic("Service layer")
		t.Feature("Entry application service")
		t.Story("Read collected entries")
		t.Title("Delegates pagination to repository and returns ordered log entries")

		ctx := context.Background()
		store := memory.New()
		serverModel := &models.Server{ID: "srv-entry", Name: "entry-host", Host: "127.0.0.1", Username: "demo", AuthType: models.AuthPassword, AuthValue: "demo"}
		logFile := &models.LogFile{ID: "log-entry", ServerID: serverModel.ID, Path: "/var/log/app.log", LogType: models.LogTypeApp, IsActive: true}
		t.Require().NoError(store.CreateServer(ctx, serverModel))
		t.Require().NoError(store.CreateLogFile(ctx, logFile))
		t.Require().NoError(store.CreateLogEntries(ctx, []*models.LogEntry{
			{LogFileID: logFile.ID, LineNumber: 1, Content: "one", Hash: "hash-one"},
			{LogFileID: logFile.ID, LineNumber: 2, Content: "two", Hash: "hash-two"},
			{LogFileID: logFile.ID, LineNumber: 3, Content: "three", Hash: "hash-three"},
		}))
		service := entryservice.NewService(store)

		t.WithNewStep("List entries with offset and limit", func(step provider.StepCtx) {
			items, err := service.List(ctx, logFile.ID, 1, 1)
			step.Require().NoError(err)
			step.Require().Len(items, 1)
			step.Require().EqualValues(2, items[0].LineNumber)
		})
	})
}
