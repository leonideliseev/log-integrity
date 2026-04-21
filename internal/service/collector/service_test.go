package collector_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/lenchik/logmonitor/internal/repository/memory"
	collectorservice "github.com/lenchik/logmonitor/internal/service/collector"
	"github.com/lenchik/logmonitor/internal/testsupport"
	"github.com/lenchik/logmonitor/models"
	"github.com/ozontech/allure-go/pkg/framework/provider"
	"github.com/ozontech/allure-go/pkg/framework/runner"
)

func TestCollectorServiceCollectLogFileStoresEntriesAndChunks(t *testing.T) {
	runner.Run(t, "collector stores new log entries and chunks", func(t provider.T) {
		t.Epic("Service layer")
		t.Feature("Collector")
		t.Story("Collect remote log file")
		t.Title("Stores only new lines, hashes them and creates aggregate chunks")

		ctx := context.Background()
		store, serverModel, logFile := prepareCollectorRepository(t)
		factory := &testsupport.SSHClientFactory{Execute: collectorCommandHandler("1\talpha\n2\tbeta\n3\tgamma")}
		service := collectorservice.NewServiceWithOptions(factory, store, store, store, collectorservice.Options{
			BatchSize:       2,
			ChunkSize:       2,
			StoreRawContent: true,
			ChunkHashAlgo:   "sha256",
		})

		var collected int
		t.WithNewStep("Collect log file through fake SSH client", func(step provider.StepCtx) {
			var err error
			collected, err = service.CollectLogFile(ctx, serverModel, logFile)
			step.Require().NoError(err)
		})

		t.WithNewStep("Verify stored entries and chunks", func(step provider.StepCtx) {
			step.Require().Equal(3, collected)

			entries, err := store.ListLogEntries(ctx, logFile.ID, 0, 0)
			step.Require().NoError(err)
			step.Require().Len(entries, 3)
			step.Require().EqualValues(1, entries[0].LineNumber)
			step.Require().Equal("alpha", entries[0].Content)
			step.Require().NotEmpty(entries[0].Hash)

			chunks, err := store.ListLogChunks(ctx, logFile.ID, 0, 0)
			step.Require().NoError(err)
			step.Require().Len(chunks, 2)
			step.Require().Equal(2, chunks[0].EntriesCount)
		})

		t.WithNewStep("Verify log file scan metadata was updated", func(step provider.StepCtx) {
			updated, err := store.GetLogFileByID(ctx, logFile.ID)
			step.Require().NoError(err)
			step.Require().NotNil(updated.LastScannedAt)
			step.Require().EqualValues(3, updated.LastLineNumber)
		})
	})
}

func TestCollectorServiceCollectLogFileSkipsPreviouslyStoredLines(t *testing.T) {
	runner.Run(t, "collector skips already stored lines", func(t provider.T) {
		t.Epic("Service layer")
		t.Feature("Collector")
		t.Story("Incremental collection")
		t.Title("Second collection saves only lines with numbers greater than current max")

		ctx := context.Background()
		store, serverModel, logFile := prepareCollectorRepository(t)
		factory := &testsupport.SSHClientFactory{Execute: collectorCommandHandler("1\talpha\n2\tbeta")}
		service := collectorservice.NewServiceWithOptions(factory, store, store, store, collectorservice.Options{
			BatchSize:       10,
			ChunkSize:       10,
			StoreRawContent: false,
			ChunkHashAlgo:   "sha256",
		})

		t.WithNewStep("Run initial collection", func(step provider.StepCtx) {
			collected, err := service.CollectLogFile(ctx, serverModel, logFile)
			step.Require().NoError(err)
			step.Require().Equal(2, collected)
		})

		t.WithNewStep("Run collection again with one appended line", func(step provider.StepCtx) {
			factory.Execute = collectorCommandHandler("1\talpha\n2\tbeta\n3\tgamma")
			collected, err := service.CollectLogFile(ctx, serverModel, logFile)
			step.Require().NoError(err)
			step.Require().Equal(1, collected)
		})

		t.WithNewStep("Verify raw content is not stored when disabled", func(step provider.StepCtx) {
			entries, err := store.ListLogEntries(ctx, logFile.ID, 0, 0)
			step.Require().NoError(err)
			step.Require().Len(entries, 3)
			step.Require().Empty(entries[0].Content)
			step.Require().NotEmpty(entries[0].Hash)
		})
	})
}

func TestCollectorServiceCollectLogFileResetsStateAfterRotation(t *testing.T) {
	runner.Run(t, "collector resets state after file rotation", func(t provider.T) {
		t.Epic("Service layer")
		t.Feature("Collector")
		t.Story("Log rotation")
		t.Title("Clears old entries and chunks when file identity changes")

		ctx := context.Background()
		store, serverModel, logFile := prepareCollectorRepository(t)
		lines := "1\talpha\n2\tbeta"
		identity := "device_id=dev\ninode=old\nsize_bytes=128\nmod_time_unix=10"
		factory := &testsupport.SSHClientFactory{Execute: func(cmd string) (string, error) {
			switch {
			case strings.Contains(cmd, "awk"):
				return lines, nil
			case strings.Contains(cmd, "stat -c"):
				return identity, nil
			default:
				return "", nil
			}
		}}
		service := collectorservice.NewServiceWithOptions(factory, store, store, store, collectorservice.Options{
			BatchSize:       2,
			ChunkSize:       2,
			StoreRawContent: true,
			ChunkHashAlgo:   "sha256",
		})

		t.WithNewStep("Collect initial file contents", func(step provider.StepCtx) {
			collected, err := service.CollectLogFile(ctx, serverModel, logFile)
			step.Require().NoError(err)
			step.Require().Equal(2, collected)
		})

		t.WithNewStep("Collect rotated file with a new inode", func(step provider.StepCtx) {
			lines = "1\tdelta"
			identity = "device_id=dev\ninode=new\nsize_bytes=16\nmod_time_unix=20"
			collected, err := service.CollectLogFile(ctx, serverModel, logFile)
			step.Require().NoError(err)
			step.Require().Equal(1, collected)
		})

		t.WithNewStep("Verify old entries and chunks were replaced", func(step provider.StepCtx) {
			entries, err := store.ListLogEntries(ctx, logFile.ID, 0, 0)
			step.Require().NoError(err)
			step.Require().Len(entries, 1)
			step.Require().EqualValues(1, entries[0].LineNumber)
			step.Require().Equal("delta", entries[0].Content)

			chunks, err := store.ListLogChunks(ctx, logFile.ID, 0, 0)
			step.Require().NoError(err)
			step.Require().Len(chunks, 1)
			step.Require().EqualValues(1, chunks[0].FromLineNumber)
		})
	})
}

func TestCollectorServiceCollectLogFileReturnsReadError(t *testing.T) {
	runner.Run(t, "collector returns read errors", func(t provider.T) {
		t.Epic("Service layer")
		t.Feature("Collector")
		t.Story("Remote read failure")
		t.Title("Does not update repository when SSH read command fails")

		ctx := context.Background()
		store, serverModel, logFile := prepareCollectorRepository(t)
		factory := &testsupport.SSHClientFactory{Execute: func(cmd string) (string, error) {
			if strings.Contains(cmd, "awk") {
				return "", errors.New("remote file is unavailable")
			}
			return "device_id=dev\ninode=ino\nsize_bytes=10\nmod_time_unix=1", nil
		}}
		service := collectorservice.NewServiceWithOptions(factory, store, store, store, collectorservice.Options{})

		t.WithNewStep("Collect a log file with failing read command", func(step provider.StepCtx) {
			collected, err := service.CollectLogFile(ctx, serverModel, logFile)
			step.Require().Error(err)
			step.Require().Equal(0, collected)
		})

		t.WithNewStep("Verify no entries were persisted", func(step provider.StepCtx) {
			count, err := store.CountLogEntries(ctx, logFile.ID)
			step.Require().NoError(err)
			step.Require().EqualValues(0, count)
		})
	})
}

func TestReadLogLinesAfterUsesWindowsEventRecordID(t *testing.T) {
	runner.Run(t, "collector reads Windows events after stored record id", func(t provider.T) {
		t.Epic("Service layer")
		t.Feature("Collector")
		t.Story("Windows Event Log")
		t.Title("Uses RecordId as a stable line number for incremental reads")

		factory := &testsupport.SSHClientFactory{Execute: func(cmd string) (string, error) {
			return "101\tcreated", nil
		}}
		client := factory.NewClient()

		t.WithNewStep("Read event log after previous RecordId", func(step provider.StepCtx) {
			lines, err := collectorservice.ReadLogLinesAfter(context.Background(), client, &models.LogFile{Path: "eventlog://Application"}, 100)
			step.Require().NoError(err)
			step.Require().Len(lines, 1)
			step.Require().EqualValues(101, lines[0].Number)
		})

		t.WithNewStep("Verify generated command filters by RecordId", func(step provider.StepCtx) {
			commands := factory.Commands()
			step.Require().Len(commands, 1)
			step.Require().Contains(commands[0], "RecordId -gt 100")
		})
	})
}

func TestReadLogLinesParsesAndSortsOutput(t *testing.T) {
	runner.Run(t, "collector parses numbered line output", func(t provider.T) {
		t.Epic("Service layer")
		t.Feature("Collector")
		t.Story("Line parsing")
		t.Title("Parses tab and whitespace numbered lines in ascending order")

		client := (&testsupport.SSHClientFactory{Execute: func(string) (string, error) {
			return "3\tthird\n1 first\n2\tsecond", nil
		}}).NewClient()

		t.WithNewStep("Read and parse remote command output", func(step provider.StepCtx) {
			lines, err := collectorservice.ReadLogLines(context.Background(), client, &models.LogFile{Path: "/var/log/app.log"})
			step.Require().NoError(err)
			step.Require().Len(lines, 3)
			step.Require().EqualValues(1, lines[0].Number)
			step.Require().Equal("first", lines[0].Content)
			step.Require().EqualValues(3, lines[2].Number)
		})
	})
}

func prepareCollectorRepository(t provider.T) (*memory.Storage, *models.Server, *models.LogFile) {
	ctx := context.Background()
	store := memory.New()
	serverModel := &models.Server{
		ID:        "srv-collector",
		Name:      "collector-host",
		Host:      "127.0.0.1",
		Port:      22,
		Username:  "demo",
		AuthType:  models.AuthPassword,
		AuthValue: "demo",
		OSType:    models.OSLinux,
		Status:    models.ServerStatusActive,
	}
	t.Require().NoError(store.CreateServer(ctx, serverModel))

	logFile := &models.LogFile{
		ID:       "log-collector",
		ServerID: serverModel.ID,
		Path:     "/var/log/app.log",
		LogType:  models.LogTypeApp,
		IsActive: true,
	}
	t.Require().NoError(store.CreateLogFile(ctx, logFile))
	return store, serverModel, logFile
}

func collectorCommandHandler(lines string) testsupport.SSHCommandHandler {
	return func(cmd string) (string, error) {
		switch {
		case strings.Contains(cmd, "awk"):
			return lines, nil
		case strings.Contains(cmd, "stat -c"):
			return "device_id=dev\ninode=ino\nsize_bytes=128\nmod_time_unix=10", nil
		default:
			return "", nil
		}
	}
}
