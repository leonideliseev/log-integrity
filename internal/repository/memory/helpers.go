package memory

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/lenchik/logmonitor/internal/repository"
	"github.com/lenchik/logmonitor/models"
)

// logFilesByServerIDs returns log file identifiers grouped under a server.
func (s *Storage) logFilesByServerIDs(serverID string) []string {
	byPath := s.logFilesByServer[serverID]
	ids := make([]string, 0, len(byPath))
	for _, id := range byPath {
		ids = append(ids, id)
	}
	return ids
}

// newID generates a simple random identifier with a prefix.
func newID(prefix string) string {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("%s_%d", prefix, time.Now().UTC().UnixNano())
	}
	return prefix + "_" + hex.EncodeToString(buf)
}

// cloneServer returns a detached copy of the server model.
func cloneServer(in *models.Server) *models.Server {
	copyModel := *in
	if in.LastSeenAt != nil {
		lastSeenAt := *in.LastSeenAt
		copyModel.LastSeenAt = &lastSeenAt
	}
	if in.BackoffUntil != nil {
		backoffUntil := *in.BackoffUntil
		copyModel.BackoffUntil = &backoffUntil
	}
	return &copyModel
}

// cloneLogFile returns a detached copy of the log file model.
func cloneLogFile(in *models.LogFile) *models.LogFile {
	copyModel := *in
	if in.LastScannedAt != nil {
		ts := *in.LastScannedAt
		copyModel.LastScannedAt = &ts
	}
	if in.Meta != nil {
		copyModel.Meta = make(map[string]string, len(in.Meta))
		for key, value := range in.Meta {
			copyModel.Meta[key] = value
		}
	}
	return &copyModel
}

// cloneLogEntry returns a detached copy of the entry model.
func cloneLogEntry(in *models.LogEntry) *models.LogEntry {
	copyModel := *in
	return &copyModel
}

// cloneLogChunk returns a detached copy of the log chunk model.
func cloneLogChunk(in *models.LogChunk) *models.LogChunk {
	copyModel := *in
	return &copyModel
}

// cloneCheckResult returns a detached copy of the check result model.
func cloneCheckResult(in *models.CheckResult) *models.CheckResult {
	copyModel := *in
	return &copyModel
}

// paginate slices a result set while keeping the function reusable for different models.
func paginate[T any](items []T, offset, limit int) []T {
	if offset < 0 {
		offset = 0
	}
	if offset >= len(items) {
		return []T{}
	}
	if limit <= 0 || offset+limit > len(items) {
		limit = len(items) - offset
	}
	result := make([]T, limit)
	copy(result, items[offset:offset+limit])
	return result
}

func paged[T any](items []T, offset, limit int) repository.Page[T] {
	total := len(items)
	return repository.Page[T]{
		Items:  paginate(items, offset, limit),
		Total:  total,
		Offset: normalizeOffset(offset),
		Limit:  limit,
	}
}

func normalizeOffset(offset int) int {
	if offset < 0 {
		return 0
	}
	return offset
}

func normalizedSearch(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func matchesSearch(query string, values ...string) bool {
	query = normalizedSearch(query)
	if query == "" {
		return true
	}
	for _, value := range values {
		if strings.Contains(normalizedSearch(value), query) {
			return true
		}
	}
	return false
}

func ascending(order string) bool {
	return strings.EqualFold(strings.TrimSpace(order), "asc")
}
