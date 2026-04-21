package memory

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

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
