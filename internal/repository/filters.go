package repository

import "github.com/lenchik/logmonitor/models"

// Page contains one filtered result page and the total number of matching rows.
type Page[T any] struct {
	Items  []T
	Total  int
	Offset int
	Limit  int
}

// ListOptions contains common paging, search, and ordering inputs.
type ListOptions struct {
	Q      string
	Offset int
	Limit  int
	Sort   string
	Order  string
}

// ServerListFilter narrows server list reads.
type ServerListFilter struct {
	ListOptions
	Status    models.ServerStatus
	OSType    models.OSType
	ManagedBy models.ServerManagedBy
	AuthType  models.AuthType
}

// LogFileListFilter narrows log file list reads.
type LogFileListFilter struct {
	ListOptions
	ServerID string
	Active   *bool
	LogType  models.LogType
}

// LogEntryListFilter narrows log entry list reads.
type LogEntryListFilter struct {
	ListOptions
	LogFileID string
	FromLine  int64
	ToLine    int64
}

// CheckResultListFilter narrows check result history reads.
type CheckResultListFilter struct {
	ListOptions
	LogFileID   string
	Status      models.CheckStatus
	Severity    models.ProblemSeverity
	ProblemType models.ProblemType
}
