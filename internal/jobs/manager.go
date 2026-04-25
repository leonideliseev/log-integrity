// Package jobs provides an in-memory async queue with history for manual API operations.
package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/lenchik/logmonitor/models"
)

var (
	// ErrNotFound reports that the requested job does not exist in history.
	ErrNotFound = errors.New("jobs: not found")
	// ErrQueueFull reports that the async queue is currently saturated.
	ErrQueueFull = errors.New("jobs: queue is full")
	// ErrShuttingDown reports that the manager no longer accepts new jobs.
	ErrShuttingDown = errors.New("jobs: shutting down")
)

// Options stores queue sizing and retention settings.
type Options struct {
	Workers      int
	QueueSize    int
	HistoryLimit int
}

// ListFilter narrows job history results for API consumers and future UI screens.
type ListFilter struct {
	Type      models.JobType
	Status    models.JobStatus
	ServerID  string
	LogFileID string
	Offset    int
	Limit     int
}

// Runner executes one queued operation and returns a JSON-serializable result.
type Runner func(ctx context.Context) (any, error)

// TaskSpec describes a queued async operation before it is persisted in history.
type TaskSpec struct {
	Type           models.JobType
	IdempotencyKey string
	Fingerprint    string
	ServerID       string
	LogFileID      string
	Run            Runner
}

type queuedTask struct {
	job *models.Job
	run Runner
}

// Manager coordinates async execution, deduplication and job history.
type Manager struct {
	logger *slog.Logger
	opts   Options

	mu                 sync.RWMutex
	jobs               map[string]*models.Job
	order              []string
	idempotencyKeys    map[string]string
	activeFingerprints map[string]string
	queue              chan queuedTask
	ctx                context.Context
	cancel             context.CancelFunc
	workersStarted     bool
	shuttingDown       bool
	workerWaitGroup    sync.WaitGroup
	workerStartupOnce  sync.Once
	workerShutdownOnce sync.Once
}

// NewManager creates an async job manager with bounded memory usage.
func NewManager(logger *slog.Logger, opts Options) *Manager {
	if opts.Workers <= 0 {
		opts.Workers = 2
	}
	if opts.QueueSize <= 0 {
		opts.QueueSize = 128
	}
	if opts.HistoryLimit <= 0 {
		opts.HistoryLimit = 1000
	}

	return &Manager{
		logger:             logger,
		opts:               opts,
		jobs:               make(map[string]*models.Job),
		order:              make([]string, 0, opts.HistoryLimit),
		idempotencyKeys:    make(map[string]string),
		activeFingerprints: make(map[string]string),
		queue:              make(chan queuedTask, opts.QueueSize),
	}
}

// Start launches worker goroutines bound to the application lifetime context.
func (m *Manager) Start(parentCtx context.Context) {
	m.workerStartupOnce.Do(func() {
		m.ctx, m.cancel = context.WithCancel(parentCtx)
		m.workersStarted = true

		for index := 0; index < m.opts.Workers; index++ {
			m.workerWaitGroup.Add(1)
			go m.worker(index + 1)
		}
	})
}

// Shutdown stops accepting new jobs, cancels running ones and waits for workers to exit.
func (m *Manager) Shutdown(ctx context.Context) error {
	m.workerShutdownOnce.Do(func() {
		m.mu.Lock()
		m.shuttingDown = true
		close(m.queue)
		m.mu.Unlock()

		if m.cancel != nil {
			m.cancel()
		}
	})

	done := make(chan struct{})
	go func() {
		defer close(done)
		m.workerWaitGroup.Wait()
	}()

	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Submit enqueues a new job or reuses an existing one when idempotency rules match.
func (m *Manager) Submit(spec TaskSpec) (*models.Job, bool, error) {
	now := time.Now().UTC()
	spec.IdempotencyKey = strings.TrimSpace(spec.IdempotencyKey)
	spec.Fingerprint = strings.TrimSpace(spec.Fingerprint)

	m.mu.Lock()
	defer m.mu.Unlock()

	if m.shuttingDown {
		return nil, false, ErrShuttingDown
	}
	if spec.IdempotencyKey != "" {
		if jobID, ok := m.idempotencyKeys[spec.IdempotencyKey]; ok {
			job, exists := m.jobs[jobID]
			if exists {
				return cloneJob(job), true, nil
			}
			delete(m.idempotencyKeys, spec.IdempotencyKey)
		}
	}
	if spec.Fingerprint != "" {
		if jobID, ok := m.activeFingerprints[spec.Fingerprint]; ok {
			job, exists := m.jobs[jobID]
			if exists {
				if spec.IdempotencyKey != "" {
					m.idempotencyKeys[spec.IdempotencyKey] = jobID
				}
				return cloneJob(job), true, nil
			}
			delete(m.activeFingerprints, spec.Fingerprint)
		}
	}

	job := &models.Job{
		ID:             uuid.NewString(),
		Type:           spec.Type,
		Status:         models.JobStatusQueued,
		IdempotencyKey: spec.IdempotencyKey,
		Fingerprint:    spec.Fingerprint,
		ServerID:       spec.ServerID,
		LogFileID:      spec.LogFileID,
		CreatedAt:      now,
	}

	select {
	case m.queue <- queuedTask{job: job, run: spec.Run}:
		m.jobs[job.ID] = job
		m.order = append(m.order, job.ID)
		if spec.IdempotencyKey != "" {
			m.idempotencyKeys[spec.IdempotencyKey] = job.ID
		}
		if spec.Fingerprint != "" {
			m.activeFingerprints[spec.Fingerprint] = job.ID
		}
		return cloneJob(job), false, nil
	default:
		return nil, false, ErrQueueFull
	}
}

// GetJob returns one stored job snapshot.
func (m *Manager) GetJob(jobID string) (*models.Job, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	job, ok := m.jobs[jobID]
	if !ok {
		return nil, ErrNotFound
	}
	return cloneJob(job), nil
}

// ListJobs returns filtered job history ordered from newest to oldest.
func (m *Manager) ListJobs(filter ListFilter) ([]*models.Job, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	matches := make([]*models.Job, 0, len(m.order))
	for index := len(m.order) - 1; index >= 0; index-- {
		job, ok := m.jobs[m.order[index]]
		if !ok || !matchesFilter(job, filter) {
			continue
		}
		matches = append(matches, cloneJob(job))
	}

	if filter.Offset >= len(matches) {
		return []*models.Job{}, nil
	}
	start := filter.Offset
	end := len(matches)
	if filter.Limit > 0 && start+filter.Limit < end {
		end = start + filter.Limit
	}
	return matches[start:end], nil
}

// worker executes queued tasks and persists status transitions in history.
func (m *Manager) worker(workerID int) {
	defer m.workerWaitGroup.Done()

	for task := range m.queue {
		if m.ctx != nil && m.ctx.Err() != nil {
			m.finishCanceled(task.job.ID)
			continue
		}
		m.startRunning(task.job.ID)

		result, err := task.run(m.ctx)
		switch {
		case err != nil && errors.Is(err, context.Canceled):
			m.finishCanceled(task.job.ID)
		case err != nil:
			m.finishFailed(task.job.ID, err)
		default:
			m.finishSucceeded(task.job.ID, result)
		}

		if m.logger != nil {
			m.logger.Debug("job processed", "worker", workerID, "job_id", task.job.ID, "type", task.job.Type)
		}
	}
}

// startRunning marks one queued job as actively executed by a worker.
func (m *Manager) startRunning(jobID string) {
	now := time.Now().UTC()

	m.mu.Lock()
	defer m.mu.Unlock()

	job, ok := m.jobs[jobID]
	if !ok {
		return
	}
	job.Status = models.JobStatusRunning
	job.StartedAt = timePointer(now)
	job.FinishedAt = nil
}

// finishSucceeded stores the final successful result payload for one job.
func (m *Manager) finishSucceeded(jobID string, result any) {
	now := time.Now().UTC()
	var payload json.RawMessage
	if result != nil {
		data, err := json.Marshal(result)
		if err == nil {
			payload = data
		}
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	job, ok := m.jobs[jobID]
	if !ok {
		return
	}
	job.Status = models.JobStatusSucceeded
	job.Error = ""
	job.Result = cloneRawMessage(payload)
	job.FinishedAt = timePointer(now)
	m.releaseActiveFingerprint(job)
	m.trimHistoryLocked()
}

// finishFailed stores the terminal error for one job.
func (m *Manager) finishFailed(jobID string, err error) {
	now := time.Now().UTC()

	m.mu.Lock()
	defer m.mu.Unlock()

	job, ok := m.jobs[jobID]
	if !ok {
		return
	}
	job.Status = models.JobStatusFailed
	job.Error = err.Error()
	job.FinishedAt = timePointer(now)
	m.releaseActiveFingerprint(job)
	m.trimHistoryLocked()
}

// finishCanceled marks one queued or running job as canceled during shutdown.
func (m *Manager) finishCanceled(jobID string) {
	now := time.Now().UTC()

	m.mu.Lock()
	defer m.mu.Unlock()

	job, ok := m.jobs[jobID]
	if !ok {
		return
	}
	job.Status = models.JobStatusCanceled
	job.Error = context.Canceled.Error()
	job.FinishedAt = timePointer(now)
	m.releaseActiveFingerprint(job)
	m.trimHistoryLocked()
}

// releaseActiveFingerprint removes running-only deduplication once a job is terminal.
func (m *Manager) releaseActiveFingerprint(job *models.Job) {
	if job == nil || job.Fingerprint == "" {
		return
	}
	if currentID, ok := m.activeFingerprints[job.Fingerprint]; ok && currentID == job.ID {
		delete(m.activeFingerprints, job.Fingerprint)
	}
}

// trimHistoryLocked bounds memory usage while preserving active jobs.
func (m *Manager) trimHistoryLocked() {
	if m.opts.HistoryLimit <= 0 {
		return
	}

	for len(m.jobs) > m.opts.HistoryLimit {
		trimmed := false
		for index, jobID := range m.order {
			job, ok := m.jobs[jobID]
			if !ok || !isTerminalJobStatus(job.Status) {
				continue
			}

			delete(m.jobs, jobID)
			if job.IdempotencyKey != "" {
				if currentID, exists := m.idempotencyKeys[job.IdempotencyKey]; exists && currentID == jobID {
					delete(m.idempotencyKeys, job.IdempotencyKey)
				}
			}
			m.order = append(m.order[:index], m.order[index+1:]...)
			trimmed = true
			break
		}
		if !trimmed {
			return
		}
	}
}

// matchesFilter applies API query constraints to one history item.
func matchesFilter(job *models.Job, filter ListFilter) bool {
	if filter.Type != "" && job.Type != filter.Type {
		return false
	}
	if filter.Status != "" && job.Status != filter.Status {
		return false
	}
	if filter.ServerID != "" && job.ServerID != filter.ServerID {
		return false
	}
	if filter.LogFileID != "" && job.LogFileID != filter.LogFileID {
		return false
	}
	return true
}

// isTerminalJobStatus reports whether a job already completed and can be trimmed from history.
func isTerminalJobStatus(status models.JobStatus) bool {
	switch status {
	case models.JobStatusSucceeded, models.JobStatusFailed, models.JobStatusCanceled:
		return true
	default:
		return false
	}
}

// cloneJob returns a detached snapshot safe for concurrent API reads.
func cloneJob(job *models.Job) *models.Job {
	if job == nil {
		return nil
	}

	clone := *job
	clone.Result = cloneRawMessage(job.Result)
	if job.StartedAt != nil {
		startedAt := *job.StartedAt
		clone.StartedAt = &startedAt
	}
	if job.FinishedAt != nil {
		finishedAt := *job.FinishedAt
		clone.FinishedAt = &finishedAt
	}
	return &clone
}

// cloneRawMessage copies JSON bytes so history snapshots stay immutable outside the manager.
func cloneRawMessage(input json.RawMessage) json.RawMessage {
	if input == nil {
		return nil
	}
	result := make(json.RawMessage, len(input))
	copy(result, input)
	return result
}

// timePointer allocates a stable pointer for stored timestamps.
func timePointer(value time.Time) *time.Time {
	return &value
}
