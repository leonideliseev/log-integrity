package integrity_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/lenchik/logmonitor/internal/repository/memory"
	integrityservice "github.com/lenchik/logmonitor/internal/service/integrity"
	"github.com/lenchik/logmonitor/internal/testsupport"
	"github.com/lenchik/logmonitor/models"
	"github.com/lenchik/logmonitor/pkg/hasher"
	"github.com/ozontech/allure-go/pkg/framework/provider"
	"github.com/ozontech/allure-go/pkg/framework/runner"
)

func TestIntegrityServiceCheckLogFileOK(t *testing.T) {
	runner.Run(t, "integrity check returns ok", func(t provider.T) {
		t.Epic("Service layer")
		t.Feature("Integrity")
		t.Story("Hash comparison")
		t.Title("Stored hashes match current remote log contents")

		ctx := context.Background()
		store, serverModel, logFile := prepareIntegrityRepository(t)
		factory := &testsupport.SSHClientFactory{Execute: integrityReadHandler("1\talpha\n2\tbeta")}
		service := integrityservice.NewService(factory, store, store)

		t.WithNewStep("Run integrity check", func(step provider.StepCtx) {
			result, tampered, err := service.CheckLogFile(ctx, serverModel, logFile)
			step.Require().NoError(err)
			step.Require().Equal(models.CheckStatusOK, result.Status)
			step.Require().EqualValues(2, result.TotalLines)
			step.Require().Empty(tampered)
		})

		t.WithNewStep("Verify check result was persisted", func(step provider.StepCtx) {
			results, err := store.ListCheckResults(ctx, logFile.ID, 0, 0)
			step.Require().NoError(err)
			step.Require().Len(results, 1)
			step.Require().Equal(models.CheckStatusOK, results[0].Status)
		})
	})
}

func TestIntegrityServiceCheckLogFileDetectsTampering(t *testing.T) {
	runner.Run(t, "integrity check detects tampering", func(t provider.T) {
		t.Epic("Service layer")
		t.Feature("Integrity")
		t.Story("Tamper detection")
		t.Title("Changed and missing lines are returned as tampered entries")

		ctx := context.Background()
		store, serverModel, logFile := prepareIntegrityRepository(t)
		factory := &testsupport.SSHClientFactory{Execute: integrityReadHandler("1\talpha changed")}
		service := integrityservice.NewService(factory, store, store)

		t.WithNewStep("Run integrity check against modified remote contents", func(step provider.StepCtx) {
			result, tampered, err := service.CheckLogFile(ctx, serverModel, logFile)
			step.Require().NoError(err)
			step.Require().Equal(models.CheckStatusTampered, result.Status)
			step.Require().EqualValues(2, result.TotalLines)
			step.Require().EqualValues(2, result.TamperedLines)
			step.Require().Len(tampered, 2)
			step.Require().EqualValues(1, tampered[0].LineNumber)
			step.Require().EqualValues(2, tampered[1].LineNumber)
		})
	})
}

func TestIntegrityServiceCheckLogFileUsesChunksToNarrowTampering(t *testing.T) {
	runner.Run(t, "integrity check narrows tampering through chunks", func(t provider.T) {
		t.Epic("Service layer")
		t.Feature("Integrity")
		t.Story("Chunk comparison")
		t.Title("Checks entry details only for chunks whose aggregate hash changed")

		ctx := context.Background()
		store, serverModel, logFile := prepareIntegrityRepository(t)
		t.Require().NoError(store.CreateLogEntry(ctx, &models.LogEntry{LogFileID: logFile.ID, LineNumber: 3, Content: "gamma", Hash: hasher.SHA256String("gamma")}))
		t.Require().NoError(store.CreateLogChunks(ctx, []*models.LogChunk{
			{LogFileID: logFile.ID, ChunkNumber: 0, FromLineNumber: 1, ToLineNumber: 2, EntriesCount: 2, Hash: chunkHash("alpha", "beta"), HashAlgorithm: "sha256"},
			{LogFileID: logFile.ID, ChunkNumber: 1, FromLineNumber: 3, ToLineNumber: 3, EntriesCount: 1, Hash: chunkHash("gamma"), HashAlgorithm: "sha256"},
		}))

		factory := &testsupport.SSHClientFactory{Execute: integrityReadHandler("1\talpha\n2\tbeta changed\n3\tgamma")}
		service := integrityservice.NewServiceWithChunks(factory, store, store, store)

		t.WithNewStep("Run integrity check against one changed chunk", func(step provider.StepCtx) {
			result, tampered, err := service.CheckLogFile(ctx, serverModel, logFile)
			step.Require().NoError(err)
			step.Require().Equal(models.CheckStatusTampered, result.Status)
			step.Require().EqualValues(3, result.TotalLines)
			step.Require().EqualValues(1, result.TamperedLines)
			step.Require().Len(tampered, 1)
			step.Require().EqualValues(2, tampered[0].LineNumber)
		})
	})
}

func TestIntegrityServiceCheckLogFileChecksEntriesOutsideChunkCoverage(t *testing.T) {
	runner.Run(t, "integrity check verifies entries outside chunk coverage", func(t provider.T) {
		t.Epic("Service layer")
		t.Feature("Integrity")
		t.Story("Chunk coverage")
		t.Title("Falls back to stored entries that are not covered by aggregate chunks")

		ctx := context.Background()
		store, serverModel, logFile := prepareIntegrityRepository(t)
		t.Require().NoError(store.CreateLogChunks(ctx, []*models.LogChunk{
			{LogFileID: logFile.ID, ChunkNumber: 0, FromLineNumber: 1, ToLineNumber: 1, EntriesCount: 1, Hash: chunkHash("alpha"), HashAlgorithm: "sha256"},
		}))

		factory := &testsupport.SSHClientFactory{Execute: integrityReadHandler("1\talpha\n2\tbeta changed")}
		service := integrityservice.NewServiceWithChunks(factory, store, store, store)

		t.WithNewStep("Run integrity check with a missing chunk for line two", func(step provider.StepCtx) {
			result, tampered, err := service.CheckLogFile(ctx, serverModel, logFile)
			step.Require().NoError(err)
			step.Require().Equal(models.CheckStatusTampered, result.Status)
			step.Require().EqualValues(1, result.TamperedLines)
			step.Require().Len(tampered, 1)
			step.Require().EqualValues(2, tampered[0].LineNumber)
		})
	})
}

func TestIntegrityServiceCheckLogFileHandlesSparseChunkLineNumbers(t *testing.T) {
	runner.Run(t, "integrity check handles sparse chunk line numbers", func(t provider.T) {
		t.Epic("Service layer")
		t.Feature("Integrity")
		t.Story("Chunk comparison")
		t.Title("Hashes chunk entries by stored line numbers instead of assuming contiguous ranges")

		ctx := context.Background()
		store, serverModel, logFile := prepareIntegrityRepository(t)
		t.Require().NoError(store.CreateLogEntry(ctx, &models.LogEntry{LogFileID: logFile.ID, LineNumber: 10, Content: "omega", Hash: hasher.SHA256String("omega")}))
		t.Require().NoError(store.CreateLogChunks(ctx, []*models.LogChunk{
			{LogFileID: logFile.ID, ChunkNumber: 0, FromLineNumber: 1, ToLineNumber: 10, EntriesCount: 3, Hash: chunkHash("alpha", "beta", "omega"), HashAlgorithm: "sha256"},
		}))

		factory := &testsupport.SSHClientFactory{Execute: integrityReadHandler("1\talpha\n2\tbeta\n10\tomega")}
		service := integrityservice.NewServiceWithChunks(factory, store, store, store)

		t.WithNewStep("Run integrity check against sparse but unchanged line numbers", func(step provider.StepCtx) {
			result, tampered, err := service.CheckLogFile(ctx, serverModel, logFile)
			step.Require().NoError(err)
			step.Require().Equal(models.CheckStatusOK, result.Status)
			step.Require().EqualValues(3, result.TotalLines)
			step.Require().Empty(tampered)
		})
	})
}

func TestIntegrityServiceCheckLogFileReturnsErrorWhenSourceIdentityChanged(t *testing.T) {
	runner.Run(t, "integrity check stops on source identity change", func(t provider.T) {
		t.Epic("Service layer")
		t.Feature("Integrity")
		t.Story("Log rotation")
		t.Title("Does not report tampering when the log source identity no longer matches the collected baseline")

		ctx := context.Background()
		store, serverModel, logFile := prepareIntegrityRepository(t)
		logFile.FileIdentity = models.FileIdentity{
			DeviceID:    "dev",
			Inode:       "old",
			SizeBytes:   128,
			ModTimeUnix: 10,
		}
		t.Require().NoError(store.UpdateLogFile(ctx, logFile))

		factory := &testsupport.SSHClientFactory{Execute: func(cmd string) (string, error) {
			switch {
			case strings.Contains(cmd, "stat -c"):
				return "device_id=dev\ninode=new\nsize_bytes=32\nmod_time_unix=20", nil
			case strings.Contains(cmd, "awk"):
				return "1\talpha changed\n2\tbeta changed", nil
			default:
				return "", nil
			}
		}}
		service := integrityservice.NewService(factory, store, store)

		t.WithNewStep("Run integrity check after the underlying file rotated", func(step provider.StepCtx) {
			result, tampered, err := service.CheckLogFile(ctx, serverModel, logFile)
			step.Require().NoError(err)
			step.Require().Nil(tampered)
			step.Require().Equal(models.CheckStatusError, result.Status)
			step.Require().Contains(result.ErrorMessage, "run collection again before integrity check")
		})

		t.WithNewStep("Verify the latest persisted check result reflects baseline mismatch instead of tampering", func(step provider.StepCtx) {
			results, err := store.ListCheckResults(ctx, logFile.ID, 0, 0)
			step.Require().NoError(err)
			step.Require().Len(results, 1)
			step.Require().Equal(models.CheckStatusError, results[0].Status)
			step.Require().Contains(results[0].ErrorMessage, "run collection again before integrity check")
		})
	})
}

func TestIntegrityServiceCheckLogFileRecordsConnectionError(t *testing.T) {
	runner.Run(t, "integrity check records connection errors", func(t provider.T) {
		t.Epic("Service layer")
		t.Feature("Integrity")
		t.Story("Remote failure")
		t.Title("Connection failure is saved as an error check result")

		ctx := context.Background()
		store, serverModel, logFile := prepareIntegrityRepository(t)
		factory := &testsupport.SSHClientFactory{ConnectErr: errors.New("ssh refused")}
		service := integrityservice.NewService(factory, store, store)

		t.WithNewStep("Run integrity check with failing SSH connect", func(step provider.StepCtx) {
			result, tampered, err := service.CheckLogFile(ctx, serverModel, logFile)
			step.Require().Error(err)
			step.Require().Nil(tampered)
			step.Require().Equal(models.CheckStatusError, result.Status)
			step.Require().Contains(result.ErrorMessage, "ssh refused")
		})

		t.WithNewStep("Verify error result was persisted", func(step provider.StepCtx) {
			results, err := store.ListCheckResults(ctx, logFile.ID, 0, 0)
			step.Require().NoError(err)
			step.Require().Len(results, 1)
			step.Require().Equal(models.CheckStatusError, results[0].Status)
		})
	})
}

func TestIntegrityServiceCheckServerStopsOnFirstError(t *testing.T) {
	runner.Run(t, "integrity server check returns partial results", func(t provider.T) {
		t.Epic("Service layer")
		t.Feature("Integrity")
		t.Story("Server-level check")
		t.Title("CheckServer returns collected results before the first error")

		ctx := context.Background()
		store, serverModel, firstLog := prepareIntegrityRepository(t)
		secondLog := &models.LogFile{
			ID:       "log-integrity-missing",
			ServerID: serverModel.ID,
			Path:     "/var/log/missing.log",
			LogType:  models.LogTypeApp,
			IsActive: true,
		}
		t.Require().NoError(store.CreateLogFile(ctx, secondLog))

		factory := &testsupport.SSHClientFactory{Execute: func(cmd string) (string, error) {
			if strings.Contains(cmd, "missing.log") {
				return "", errors.New("remote file not found")
			}
			return integrityReadHandler("1\talpha\n2\tbeta")(cmd)
		}}
		service := integrityservice.NewService(factory, store, store)

		t.WithNewStep("Run CheckServer for one good and one failing log file", func(step provider.StepCtx) {
			results, err := service.CheckServer(ctx, serverModel, []*models.LogFile{firstLog, secondLog})
			step.Require().Error(err)
			step.Require().Len(results, 1)
			step.Require().Equal(models.CheckStatusOK, results[0].Status)
		})
	})
}

func chunkHash(contents ...string) string {
	builder := strings.Builder{}
	for _, content := range contents {
		builder.WriteString(hasher.SHA256String(content))
		builder.WriteByte('\n')
	}
	return hasher.SHA256String(builder.String())
}

func prepareIntegrityRepository(t provider.T) (*memory.Storage, *models.Server, *models.LogFile) {
	ctx := context.Background()
	store := memory.New()
	serverModel := &models.Server{
		ID:        "srv-integrity",
		Name:      "integrity-host",
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
		ID:       "log-integrity",
		ServerID: serverModel.ID,
		Path:     "/var/log/app.log",
		LogType:  models.LogTypeApp,
		IsActive: true,
	}
	t.Require().NoError(store.CreateLogFile(ctx, logFile))
	t.Require().NoError(store.CreateLogEntries(ctx, []*models.LogEntry{
		{LogFileID: logFile.ID, LineNumber: 1, Content: "alpha", Hash: hasher.SHA256String("alpha")},
		{LogFileID: logFile.ID, LineNumber: 2, Content: "beta", Hash: hasher.SHA256String("beta")},
	}))

	return store, serverModel, logFile
}

func integrityReadHandler(lines string) testsupport.SSHCommandHandler {
	return func(cmd string) (string, error) {
		if strings.Contains(cmd, "awk") {
			return lines, nil
		}
		return "", nil
	}
}
