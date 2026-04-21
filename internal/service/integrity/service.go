// Package integrity compares stored hashes with current remote log contents.
package integrity

import (
	"context"
	"fmt"
	"strings"

	"github.com/lenchik/logmonitor/internal/repository"
	collectservice "github.com/lenchik/logmonitor/internal/service/collector"
	"github.com/lenchik/logmonitor/internal/ssh"
	"github.com/lenchik/logmonitor/models"
	"github.com/lenchik/logmonitor/pkg/hasher"
)

// Service runs integrity checks for collected log entries.
type Service struct {
	clientFactory ssh.ClientFactory
	entries       repository.LogEntryRepository
	chunks        repository.LogChunkRepository
	checks        repository.CheckResultRepository
	integrityKey  string
}

// NewService creates an integrity service with repository and SSH dependencies.
func NewService(clientFactory ssh.ClientFactory, entries repository.LogEntryRepository, checks repository.CheckResultRepository) *Service {
	return NewServiceWithChunks(clientFactory, entries, nil, checks)
}

// NewServiceWithChunks creates an integrity service that can use aggregate chunk hashes.
func NewServiceWithChunks(clientFactory ssh.ClientFactory, entries repository.LogEntryRepository, chunks repository.LogChunkRepository, checks repository.CheckResultRepository) *Service {
	return NewServiceWithOptions(clientFactory, entries, chunks, checks, Options{})
}

// Options controls integrity verification behavior.
type Options struct {
	IntegrityHMACKey string
}

// NewServiceWithOptions creates an integrity service with explicit verification settings.
func NewServiceWithOptions(clientFactory ssh.ClientFactory, entries repository.LogEntryRepository, chunks repository.LogChunkRepository, checks repository.CheckResultRepository, options Options) *Service {
	return &Service{
		clientFactory: clientFactory,
		entries:       entries,
		chunks:        chunks,
		checks:        checks,
		integrityKey:  options.IntegrityHMACKey,
	}
}

// CheckLogFile compares stored hashes with the current remote log contents.
func (s *Service) CheckLogFile(ctx context.Context, serverModel *models.Server, logFile *models.LogFile) (*models.CheckResult, []models.TamperedEntry, error) {
	client := s.clientFactory.NewClient()
	if err := client.Connect(serverModel); err != nil {
		result := &models.CheckResult{
			LogFileID:    logFile.ID,
			Status:       models.CheckStatusError,
			ErrorMessage: fmt.Sprintf("connect to %s: %v", serverModel.Name, err),
		}
		_ = s.checks.CreateCheckResult(ctx, result)
		return result, nil, fmt.Errorf("integrity: connect to %q: %w", serverModel.Name, err)
	}
	defer func() {
		_ = client.Close()
	}()

	currentLines, err := collectservice.ReadLogLines(ctx, client, logFile)
	if err != nil {
		result := &models.CheckResult{
			LogFileID:    logFile.ID,
			Status:       models.CheckStatusError,
			ErrorMessage: err.Error(),
		}
		_ = s.checks.CreateCheckResult(ctx, result)
		return result, nil, err
	}

	totalLines, err := s.entries.CountLogEntries(ctx, logFile.ID)
	if err != nil {
		return nil, nil, fmt.Errorf("integrity: count stored entries for %q: %w", logFile.Path, err)
	}

	currentByLine := make(map[int64]string, len(currentLines))
	for _, line := range currentLines {
		currentByLine[line.Number] = line.Content
	}

	tampered, err := s.findTamperedEntries(ctx, logFile.ID, currentByLine)
	if err != nil {
		return nil, nil, fmt.Errorf("integrity: compare entries for %q: %w", logFile.Path, err)
	}

	result := &models.CheckResult{
		LogFileID:     logFile.ID,
		TotalLines:    totalLines,
		TamperedLines: int64(len(tampered)),
		Status:        models.CheckStatusOK,
	}
	if len(tampered) > 0 {
		result.Status = models.CheckStatusTampered
	}

	if err := s.checks.CreateCheckResult(ctx, result); err != nil {
		return nil, nil, fmt.Errorf("integrity: save check result for %q: %w", logFile.Path, err)
	}

	return result, tampered, nil
}

func (s *Service) findTamperedEntries(ctx context.Context, logFileID string, currentByLine map[int64]string) ([]models.TamperedEntry, error) {
	if s.chunks == nil {
		return s.findTamperedEntriesByRange(ctx, logFileID, 0, 0, currentByLine)
	}

	chunks, err := s.chunks.ListLogChunks(ctx, logFileID, 0, 0)
	if err != nil {
		return nil, err
	}
	if len(chunks) == 0 {
		return s.findTamperedEntriesByRange(ctx, logFileID, 0, 0, currentByLine)
	}

	tampered := make([]models.TamperedEntry, 0)
	for _, chunk := range chunks {
		currentHash, ok := s.hashCurrentChunk(currentByLine, chunk)
		if ok && currentHash == chunk.Hash {
			continue
		}

		items, err := s.findTamperedEntriesByRange(ctx, logFileID, chunk.FromLineNumber, chunk.ToLineNumber, currentByLine)
		if err != nil {
			return nil, err
		}
		tampered = append(tampered, items...)
	}

	return tampered, nil
}

func (s *Service) findTamperedEntriesByRange(ctx context.Context, logFileID string, fromLine, toLine int64, currentByLine map[int64]string) ([]models.TamperedEntry, error) {
	var storedEntries []*models.LogEntry
	var err error
	if fromLine > 0 || toLine > 0 {
		storedEntries, err = s.entries.ListLogEntriesByLineRange(ctx, logFileID, fromLine, toLine)
	} else {
		storedEntries, err = s.entries.ListLogEntries(ctx, logFileID, 0, 0)
	}
	if err != nil {
		return nil, err
	}

	tampered := make([]models.TamperedEntry, 0)
	for _, entry := range storedEntries {
		currentContent, ok := currentByLine[entry.LineNumber]
		currentHash := ""
		if ok {
			currentHash = hasher.HashString(currentContent, s.integrityKey)
		}

		if !ok || currentHash != entry.Hash {
			tampered = append(tampered, models.TamperedEntry{
				LineNumber:     entry.LineNumber,
				StoredHash:     entry.Hash,
				CurrentHash:    currentHash,
				CurrentContent: currentContent,
			})
		}
	}

	return tampered, nil
}

func (s *Service) hashCurrentChunk(currentByLine map[int64]string, chunk *models.LogChunk) (string, bool) {
	builder := strings.Builder{}
	count := 0
	for lineNumber := chunk.FromLineNumber; lineNumber <= chunk.ToLineNumber; lineNumber++ {
		content, ok := currentByLine[lineNumber]
		if !ok {
			return "", false
		}
		builder.WriteString(hasher.HashString(content, s.integrityKey))
		builder.WriteByte('\n')
		count++
	}
	if count != chunk.EntriesCount {
		return "", false
	}
	return hasher.SHA256String(builder.String()), true
}

// CheckServer runs integrity checks for all provided log files of one server.
func (s *Service) CheckServer(ctx context.Context, serverModel *models.Server, logFiles []*models.LogFile) ([]*models.CheckResult, error) {
	results := make([]*models.CheckResult, 0, len(logFiles))
	for _, logFile := range logFiles {
		result, _, err := s.CheckLogFile(ctx, serverModel, logFile)
		if err != nil {
			return results, err
		}
		results = append(results, result)
	}
	return results, nil
}
