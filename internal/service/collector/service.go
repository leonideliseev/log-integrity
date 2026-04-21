// Package collector reads remote log sources and persists newly discovered entries.
package collector

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/lenchik/logmonitor/internal/repository"
	"github.com/lenchik/logmonitor/internal/ssh"
	"github.com/lenchik/logmonitor/models"
	"github.com/lenchik/logmonitor/pkg/hasher"
)

const defaultBatchSize = 5000
const defaultChunkSize = 1000
const defaultChunkHashAlgo = "sha256"

// Line represents one numbered line read from a remote log source.
type Line struct {
	Number  int64
	Content string
}

// Options controls batching, chunking and raw content persistence during collection.
type Options struct {
	BatchSize        int
	ChunkSize        int
	StoreRawContent  bool
	ChunkHashAlgo    string
	IntegrityHMACKey string
}

// Service collects remote logs, hashes lines and persists entries in batches.
type Service struct {
	clientFactory ssh.ClientFactory
	logFiles      repository.LogFileRepository
	entries       repository.LogEntryRepository
	chunks        repository.LogChunkRepository
	batchWriter   repository.LogBatchRepository
	options       Options
}

// NewService creates a collector service with repository and SSH dependencies.
func NewService(clientFactory ssh.ClientFactory, logFiles repository.LogFileRepository, entries repository.LogEntryRepository) *Service {
	return NewServiceWithOptions(clientFactory, logFiles, entries, nil, Options{})
}

// NewServiceWithOptions creates a collector service with explicit high-load settings.
func NewServiceWithOptions(clientFactory ssh.ClientFactory, logFiles repository.LogFileRepository, entries repository.LogEntryRepository, chunks repository.LogChunkRepository, options Options) *Service {
	options = normalizeOptions(options)
	batchWriter, _ := entries.(repository.LogBatchRepository)
	return &Service{
		clientFactory: clientFactory,
		logFiles:      logFiles,
		entries:       entries,
		chunks:        chunks,
		batchWriter:   batchWriter,
		options:       options,
	}
}

// CollectLogFile reads a remote log file and stores only newly discovered lines.
func (s *Service) CollectLogFile(ctx context.Context, serverModel *models.Server, logFile *models.LogFile) (int, error) {
	client := s.clientFactory.NewClient()
	if err := client.Connect(serverModel); err != nil {
		return 0, fmt.Errorf("collector: connect to %q: %w", serverModel.Name, err)
	}
	defer func() {
		_ = client.Close()
	}()

	identity, meta, identityErr := InspectLogFileIdentity(ctx, client, serverModel, logFile)
	shouldReset := identityErr == nil && shouldResetCollection(logFile, identity)

	maxLine := int64(0)
	if !shouldReset {
		var err error
		maxLine, err = s.entries.GetMaxLineNumber(ctx, logFile.ID)
		if err != nil {
			return 0, fmt.Errorf("collector: get max line for %q: %w", logFile.Path, err)
		}
	}

	lines, err := ReadLogLinesAfter(ctx, client, logFile, maxLine)
	if err != nil {
		return 0, err
	}
	if shouldReset {
		if err := s.resetLogFileState(ctx, logFile); err != nil {
			return 0, err
		}
	}
	if identityErr == nil {
		logFile.FileIdentity = identity
		logFile.Meta = meta
	}

	newEntries := make([]*models.LogEntry, 0)
	for _, line := range lines {
		if line.Number <= maxLine {
			continue
		}
		content := line.Content
		if !s.options.StoreRawContent {
			content = ""
		}
		// TODO: add optional encryption for raw log content before it is persisted.
		newEntries = append(newEntries, &models.LogEntry{
			LogFileID:  logFile.ID,
			LineNumber: line.Number,
			Content:    content,
			Hash:       hasher.HashString(line.Content, s.options.IntegrityHMACKey),
		})
	}

	if len(newEntries) > 0 {
		chunks, err := s.buildChunksIfEnabled(ctx, logFile.ID, newEntries)
		if err != nil {
			return 0, err
		}
		if err := s.saveEntriesAndChunks(ctx, logFile.Path, newEntries, chunks); err != nil {
			return 0, err
		}
	}

	if len(lines) > 0 {
		logFile.LastLineNumber = lines[len(lines)-1].Number
	}
	now := time.Now().UTC()
	logFile.LastScannedAt = &now
	if err := s.logFiles.UpdateLogFile(ctx, logFile); err != nil {
		return 0, fmt.Errorf("collector: update last scanned for %q: %w", logFile.Path, err)
	}

	return len(newEntries), nil
}

// resetLogFileState clears collected data when a log file was rotated or truncated.
func (s *Service) resetLogFileState(ctx context.Context, logFile *models.LogFile) error {
	if err := s.entries.DeleteLogEntriesByLogFile(ctx, logFile.ID); err != nil {
		return fmt.Errorf("collector: reset entries for %q: %w", logFile.Path, err)
	}
	if s.chunks != nil {
		if err := s.chunks.DeleteLogChunksByLogFile(ctx, logFile.ID); err != nil {
			return fmt.Errorf("collector: reset chunks for %q: %w", logFile.Path, err)
		}
	}
	logFile.LastLineNumber = 0
	logFile.LastByteOffset = 0
	return nil
}

// buildChunks groups entries and calculates aggregate hashes for later fast integrity checks.
func (s *Service) buildChunks(ctx context.Context, logFileID string, entries []*models.LogEntry) ([]*models.LogChunk, error) {
	latestChunkNumber := int64(-1)
	if latest, err := s.chunks.GetLatestLogChunk(ctx, logFileID); err == nil {
		latestChunkNumber = latest.ChunkNumber
	} else if !errors.Is(err, repository.ErrNotFound) {
		return nil, fmt.Errorf("collector: get latest chunk for %q: %w", logFileID, err)
	}

	chunks := make([]*models.LogChunk, 0, (len(entries)+s.options.ChunkSize-1)/s.options.ChunkSize)
	for i, batch := range splitEntries(entries, s.options.ChunkSize) {
		if len(batch) == 0 {
			continue
		}
		chunks = append(chunks, &models.LogChunk{
			LogFileID:      logFileID,
			ChunkNumber:    latestChunkNumber + int64(i) + 1,
			FromLineNumber: batch[0].LineNumber,
			ToLineNumber:   batch[len(batch)-1].LineNumber,
			FromByteOffset: 0,
			ToByteOffset:   0,
			EntriesCount:   len(batch),
			Hash:           hashEntryBatch(batch),
			HashAlgorithm:  s.options.ChunkHashAlgo,
			CreatedAt:      time.Now().UTC(),
		})
	}

	return chunks, nil
}

// buildChunksIfEnabled creates aggregate chunks when chunk storage is configured.
func (s *Service) buildChunksIfEnabled(ctx context.Context, logFileID string, entries []*models.LogEntry) ([]*models.LogChunk, error) {
	if s.chunks == nil || s.options.ChunkSize <= 0 {
		return nil, nil
	}
	return s.buildChunks(ctx, logFileID, entries)
}

// saveEntriesAndChunks persists entries and chunks atomically when the repository supports it.
func (s *Service) saveEntriesAndChunks(ctx context.Context, logPath string, entries []*models.LogEntry, chunks []*models.LogChunk) error {
	if len(chunks) > 0 && s.batchWriter != nil {
		for _, batch := range splitEntryChunkBatches(entries, chunks, s.options.BatchSize) {
			if err := s.batchWriter.CreateLogEntriesWithChunks(ctx, batch.entries, batch.chunks); err != nil {
				return fmt.Errorf("collector: save entries and chunks for %q: %w", logPath, err)
			}
		}
		return nil
	}

	for _, batch := range splitEntries(entries, s.options.BatchSize) {
		if err := s.entries.CreateLogEntries(ctx, batch); err != nil {
			return fmt.Errorf("collector: save entries for %q: %w", logPath, err)
		}
	}
	if len(chunks) == 0 {
		return nil
	}
	for _, batch := range splitChunks(chunks, s.options.BatchSize) {
		if err := s.chunks.CreateLogChunks(ctx, batch); err != nil {
			return fmt.Errorf("collector: save chunks for %q: %w", logPath, err)
		}
	}

	return nil
}

// CollectServer collects entries for every provided log file of a server.
func (s *Service) CollectServer(ctx context.Context, serverModel *models.Server, logFiles []*models.LogFile) (int, error) {
	total := 0
	for _, logFile := range logFiles {
		collected, err := s.CollectLogFile(ctx, serverModel, logFile)
		if err != nil {
			return total, err
		}
		total += collected
	}
	return total, nil
}

// ReadLogLines executes a remote command and parses numbered log lines from output.
func ReadLogLines(ctx context.Context, client ssh.Client, logFile *models.LogFile) ([]Line, error) {
	return ReadLogLinesAfter(ctx, client, logFile, 0)
}

// ReadLogLinesAfter executes a remote command and parses lines after the provided line number.
func ReadLogLinesAfter(ctx context.Context, client ssh.Client, logFile *models.LogFile, afterLine int64) ([]Line, error) {
	command := buildReadCommand(logFile, afterLine)
	output, err := client.ExecuteContext(ctx, command)
	if err != nil {
		return nil, fmt.Errorf("collector: read %q: %w", logFile.Path, err)
	}
	return parseLineOutput(output), nil
}

// InspectLogFileIdentity reads best-effort file identity attributes for different operating systems.
func InspectLogFileIdentity(ctx context.Context, client ssh.Client, serverModel *models.Server, logFile *models.LogFile) (models.FileIdentity, map[string]string, error) {
	if strings.HasPrefix(strings.ToLower(logFile.Path), "eventlog://") {
		logName := logFile.Path[len("eventlog://"):]
		return models.FileIdentity{EventLog: logName}, map[string]string{"source_type": "windows_event_log"}, nil
	}

	command := buildIdentityCommand(serverModel, logFile)
	output, err := client.ExecuteContext(ctx, command)
	if err != nil {
		return models.FileIdentity{}, nil, fmt.Errorf("collector: inspect identity for %q: %w", logFile.Path, err)
	}
	identity, meta := parseIdentityOutput(output)
	return identity, meta, nil
}

// buildReadCommand chooses the correct remote read command for the log source type.
func buildReadCommand(logFile *models.LogFile, afterLine int64) string {
	if afterLine < 0 {
		afterLine = 0
	}

	if strings.HasPrefix(strings.ToLower(logFile.Path), "eventlog://") {
		logName := logFile.Path[len("eventlog://"):]
		return `powershell -NoProfile -Command "$events = Get-WinEvent -LogName '` + escapePowerShellSingleQuotes(logName) + `' -MaxEvents 200 -ErrorAction Stop | Where-Object { $_.RecordId -gt ` + strconv.FormatInt(afterLine, 10) + ` } | Sort-Object RecordId; foreach ($event in $events) { '{0}` + "`t" + `{1}' -f $event.RecordId, $event.Message }"`
	}

	if looksLikeWindowsPath(logFile.Path) {
		return `powershell -NoProfile -Command "$i=0; Get-Content -LiteralPath '` + escapePowerShellSingleQuotes(logFile.Path) + `' -ErrorAction Stop | ForEach-Object { $i++; if ($i -gt ` + strconv.FormatInt(afterLine, 10) + `) { '{0}` + "`t" + `{1}' -f $i, $_ } }"`
	}

	escapedPath := escapeSingleQuotes(logFile.Path)
	return "sh -lc \"p='" + escapedPath + "'; start=" + strconv.FormatInt(afterLine, 10) + "; if [ ! -f \\\"$p\\\" ]; then echo 'log file not found:' \\\"$p\\\" >&2; exit 1; fi; awk -v start=\\\"$start\\\" 'NR > start { print NR \\\"\\\\t\\\" $0 }' \\\"$p\\\"\""
}

// buildIdentityCommand chooses the best-effort remote stat command for the log source type.
func buildIdentityCommand(serverModel *models.Server, logFile *models.LogFile) string {
	switch serverModel.OSType {
	case models.OSWindows:
		return `powershell -NoProfile -Command "$p='` + escapePowerShellSingleQuotes(logFile.Path) + `'; $i=Get-Item -LiteralPath $p -ErrorAction Stop; 'file_id=' + $i.FullName; 'size_bytes=' + $i.Length; 'mod_time_unix=' + ([DateTimeOffset]$i.LastWriteTimeUtc).ToUnixTimeSeconds(); 'volume_id=' + $i.PSDrive.Name"`
	case models.OSMacOS:
		return "sh -lc \"stat -f 'device_id=%d\ninode=%i\nsize_bytes=%z\nmod_time_unix=%m' '" + escapeSingleQuotes(logFile.Path) + "'\""
	default:
		return "sh -lc \"stat -c 'device_id=%d\ninode=%i\nsize_bytes=%s\nmod_time_unix=%Y' '" + escapeSingleQuotes(logFile.Path) + "'\""
	}
}

// parseIdentityOutput converts key-value command output into file identity fields and metadata.
func parseIdentityOutput(output string) (models.FileIdentity, map[string]string) {
	identity := models.FileIdentity{}
	meta := make(map[string]string)
	for _, raw := range strings.Split(strings.ReplaceAll(output, "\r\n", "\n"), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		switch key {
		case "device_id":
			identity.DeviceID = value
		case "inode":
			identity.Inode = value
		case "volume_id":
			identity.VolumeID = value
		case "file_id":
			identity.FileID = value
		case "size_bytes":
			identity.SizeBytes, _ = strconv.ParseInt(value, 10, 64)
		case "mod_time_unix":
			identity.ModTimeUnix, _ = strconv.ParseInt(value, 10, 64)
		default:
			meta[key] = value
		}
	}
	return identity, meta
}

func shouldResetCollection(logFile *models.LogFile, current models.FileIdentity) bool {
	previous := logFile.FileIdentity
	if !hasIdentity(previous) || !hasIdentity(current) {
		return false
	}

	switch {
	case previous.EventLog != "" || current.EventLog != "":
		return previous.EventLog != current.EventLog
	case previous.DeviceID != "" && previous.Inode != "" && current.DeviceID != "" && current.Inode != "":
		return previous.DeviceID != current.DeviceID || previous.Inode != current.Inode || fileShrank(previous, current)
	case previous.FileID != "" && current.FileID != "":
		return previous.FileID != current.FileID || fileShrank(previous, current)
	default:
		return fileShrank(previous, current)
	}
}

func hasIdentity(identity models.FileIdentity) bool {
	return identity.DeviceID != "" ||
		identity.Inode != "" ||
		identity.VolumeID != "" ||
		identity.FileID != "" ||
		identity.EventLog != "" ||
		identity.SizeBytes > 0
}

func fileShrank(previous, current models.FileIdentity) bool {
	return previous.SizeBytes > 0 && current.SizeBytes > 0 && current.SizeBytes < previous.SizeBytes
}

// parseLineOutput converts numbered command output into structured line models.
func parseLineOutput(output string) []Line {
	lines := make([]Line, 0)

	for _, raw := range strings.Split(strings.ReplaceAll(output, "\r\n", "\n"), "\n") {
		item := strings.TrimRight(raw, "\r")
		if strings.TrimSpace(item) == "" {
			continue
		}

		numberPart := ""
		content := ""

		// Prefer tab-separated output first because Windows commands are built that way.
		if idx := strings.Index(item, "\t"); idx >= 0 {
			numberPart = strings.TrimSpace(item[:idx])
			content = item[idx+1:]
		} else {
			fields := strings.Fields(item)
			if len(fields) == 0 {
				continue
			}
			numberPart = fields[0]
			content = strings.TrimSpace(strings.TrimPrefix(item, fields[0]))
		}

		number, err := strconv.ParseInt(numberPart, 10, 64)
		if err != nil {
			continue
		}

		lines = append(lines, Line{Number: number, Content: content})
	}

	sort.Slice(lines, func(i, j int) bool {
		return lines[i].Number < lines[j].Number
	})

	return lines
}

// normalizeOptions applies safe defaults for collection settings.
func normalizeOptions(options Options) Options {
	if options.BatchSize <= 0 {
		options.BatchSize = defaultBatchSize
	}
	if options.ChunkSize <= 0 {
		options.ChunkSize = defaultChunkSize
	}
	if options.ChunkHashAlgo == "" {
		options.ChunkHashAlgo = defaultChunkHashAlgo
	}
	if options.ChunkSize > options.BatchSize {
		options.ChunkSize = options.BatchSize
	}
	return options
}

type entryChunkBatch struct {
	entries []*models.LogEntry
	chunks  []*models.LogChunk
}

func splitEntryChunkBatches(entries []*models.LogEntry, chunks []*models.LogChunk, batchSize int) []entryChunkBatch {
	if len(chunks) == 0 {
		return []entryChunkBatch{{entries: entries}}
	}

	result := make([]entryChunkBatch, 0, (len(entries)+batchSize-1)/batchSize)
	current := entryChunkBatch{}
	for _, chunk := range chunks {
		chunkEntries := entriesForChunk(entries, chunk)
		if len(current.entries) > 0 && len(current.entries)+len(chunkEntries) > batchSize {
			result = append(result, current)
			current = entryChunkBatch{}
		}
		current.entries = append(current.entries, chunkEntries...)
		current.chunks = append(current.chunks, chunk)
	}
	if len(current.entries) > 0 || len(current.chunks) > 0 {
		result = append(result, current)
	}
	return result
}

func entriesForChunk(entries []*models.LogEntry, chunk *models.LogChunk) []*models.LogEntry {
	result := make([]*models.LogEntry, 0, chunk.EntriesCount)
	for _, entry := range entries {
		if entry.LineNumber >= chunk.FromLineNumber && entry.LineNumber <= chunk.ToLineNumber {
			result = append(result, entry)
		}
	}
	return result
}

// splitEntries splits entries into bounded batches.
func splitEntries(entries []*models.LogEntry, batchSize int) [][]*models.LogEntry {
	if batchSize <= 0 || batchSize >= len(entries) {
		return [][]*models.LogEntry{entries}
	}
	result := make([][]*models.LogEntry, 0, (len(entries)+batchSize-1)/batchSize)
	for start := 0; start < len(entries); start += batchSize {
		end := start + batchSize
		if end > len(entries) {
			end = len(entries)
		}
		result = append(result, entries[start:end])
	}
	return result
}

// splitChunks splits chunks into bounded batches.
func splitChunks(chunks []*models.LogChunk, batchSize int) [][]*models.LogChunk {
	if batchSize <= 0 || batchSize >= len(chunks) {
		return [][]*models.LogChunk{chunks}
	}
	result := make([][]*models.LogChunk, 0, (len(chunks)+batchSize-1)/batchSize)
	for start := 0; start < len(chunks); start += batchSize {
		end := start + batchSize
		if end > len(chunks) {
			end = len(chunks)
		}
		result = append(result, chunks[start:end])
	}
	return result
}

// hashEntryBatch calculates one aggregate hash from ordered entry hashes.
func hashEntryBatch(entries []*models.LogEntry) string {
	builder := strings.Builder{}
	for _, entry := range entries {
		builder.WriteString(entry.Hash)
		builder.WriteByte('\n')
	}
	return hasher.SHA256String(builder.String())
}

// looksLikeWindowsPath detects Windows-style file paths.
func looksLikeWindowsPath(path string) bool {
	return strings.Contains(path, ":\\") || strings.Contains(path, "\\")
}

// escapeSingleQuotes makes a string safe for single-quoted shell snippets.
func escapeSingleQuotes(value string) string {
	return strings.ReplaceAll(value, `'`, `'\''`)
}

// escapePowerShellSingleQuotes makes a string safe for PowerShell single-quoted strings.
func escapePowerShellSingleQuotes(value string) string {
	return strings.ReplaceAll(value, `'`, `''`)
}
