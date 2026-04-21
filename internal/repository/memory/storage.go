package memory

import (
	"sync"

	"github.com/lenchik/logmonitor/models"
)

// Storage keeps an in-memory implementation of the repository layer.
type Storage struct {
	mu sync.RWMutex

	servers          map[string]*models.Server
	logFiles         map[string]*models.LogFile
	logFilesByServer map[string]map[string]string
	entries          map[string]*models.LogEntry
	entriesByLogFile map[string]map[int64]string
	chunks           map[string]*models.LogChunk
	chunksByLogFile  map[string][]string
	checks           map[string]*models.CheckResult
	checksByLogFile  map[string][]string
}

// New creates a fresh in-memory repository instance.
func New() *Storage {
	return &Storage{
		servers:          make(map[string]*models.Server),
		logFiles:         make(map[string]*models.LogFile),
		logFilesByServer: make(map[string]map[string]string),
		entries:          make(map[string]*models.LogEntry),
		entriesByLogFile: make(map[string]map[int64]string),
		chunks:           make(map[string]*models.LogChunk),
		chunksByLogFile:  make(map[string][]string),
		checks:           make(map[string]*models.CheckResult),
		checksByLogFile:  make(map[string][]string),
	}
}

// Close exists to satisfy the repository contract.
func (s *Storage) Close() error {
	return nil
}
