package models

import (
	"encoding/json"
	"time"
)

// JobType identifies an asynchronous operation category.
type JobType string

// Supported asynchronous job types.
const (
	JobTypeDiscover  JobType = "discover"
	JobTypeCollect   JobType = "collect"
	JobTypeIntegrity JobType = "integrity"
)

// JobStatus describes the current lifecycle state of an asynchronous job.
type JobStatus string

// Supported asynchronous job statuses.
const (
	JobStatusQueued    JobStatus = "queued"
	JobStatusRunning   JobStatus = "running"
	JobStatusSucceeded JobStatus = "succeeded"
	JobStatusFailed    JobStatus = "failed"
	JobStatusCanceled  JobStatus = "canceled"
)

// Job stores one queued or executed asynchronous operation.
type Job struct {
	ID             string          `json:"id"`
	Type           JobType         `json:"type"`
	Status         JobStatus       `json:"status"`
	IdempotencyKey string          `json:"idempotency_key,omitempty"`
	Fingerprint    string          `json:"fingerprint,omitempty"`
	ServerID       string          `json:"server_id,omitempty"`
	LogFileID      string          `json:"log_file_id,omitempty"`
	Error          string          `json:"error,omitempty"`
	Result         json.RawMessage `json:"result,omitempty"`
	CreatedAt      time.Time       `json:"created_at"`
	StartedAt      *time.Time      `json:"started_at,omitempty"`
	FinishedAt     *time.Time      `json:"finished_at,omitempty"`
}
