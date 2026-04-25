// Package runtimeinfo stores runtime status that can be exposed through the API.
package runtimeinfo

import "sync"

// EnvCheckStatus describes how an environment placeholder was resolved during config loading.
type EnvCheckStatus string

// Environment resolution statuses used by runtime validation responses.
const (
	EnvCheckProvided  EnvCheckStatus = "provided"
	EnvCheckDefaulted EnvCheckStatus = "defaulted"
	EnvCheckMissing   EnvCheckStatus = "missing"
)

// EnvCheck describes one resolved configuration placeholder.
type EnvCheck struct {
	Name    string         `json:"name"`
	Status  EnvCheckStatus `json:"status"`
	Message string         `json:"message"`
}

// Warning describes a non-fatal startup problem that was skipped.
type Warning struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// Snapshot contains runtime metadata useful for operators and API validation endpoints.
type Snapshot struct {
	DryRun           bool       `json:"dry_run"`
	StorageBackend   string     `json:"storage_backend"`
	SchedulerEnabled bool       `json:"scheduler_enabled"`
	Warnings         []Warning  `json:"warnings"`
	EnvChecks        []EnvCheck `json:"env_checks"`
}

// Check describes one readiness probe result.
type Check struct {
	Name    string `json:"name"`
	Ready   bool   `json:"ready"`
	Message string `json:"message,omitempty"`
}

// Readiness contains the full readiness response payload.
type Readiness struct {
	Ready  bool    `json:"ready"`
	Checks []Check `json:"checks"`
}

// State stores mutable runtime metadata in a concurrency-safe way.
type State struct {
	mu               sync.RWMutex
	dryRun           bool
	storageBackend   string
	schedulerEnabled bool
	warnings         []Warning
	envChecks        []EnvCheck
}

// NewState creates an empty runtime state container.
func NewState() *State {
	return &State{}
}

// SetDryRun stores whether the application is running in dry-run mode.
func (s *State) SetDryRun(value bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.dryRun = value
}

// SetStorageBackend stores the active repository backend name.
func (s *State) SetStorageBackend(value string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.storageBackend = value
}

// SetSchedulerEnabled stores whether background jobs are active.
func (s *State) SetSchedulerEnabled(value bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.schedulerEnabled = value
}

// SetEnvChecks replaces the current list of config environment checks.
func (s *State) SetEnvChecks(items []EnvCheck) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.envChecks = cloneEnvChecks(items)
}

// AddWarning appends one non-fatal startup warning.
func (s *State) AddWarning(code, message string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.warnings = append(s.warnings, Warning{
		Code:    code,
		Message: message,
	})
}

// Snapshot returns a detached runtime status snapshot safe for API responses.
func (s *State) Snapshot() Snapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return Snapshot{
		DryRun:           s.dryRun,
		StorageBackend:   s.storageBackend,
		SchedulerEnabled: s.schedulerEnabled,
		Warnings:         cloneWarnings(s.warnings),
		EnvChecks:        cloneEnvChecks(s.envChecks),
	}
}

func cloneWarnings(items []Warning) []Warning {
	if len(items) == 0 {
		return nil
	}
	result := make([]Warning, len(items))
	copy(result, items)
	return result
}

func cloneEnvChecks(items []EnvCheck) []EnvCheck {
	if len(items) == 0 {
		return nil
	}
	result := make([]EnvCheck, len(items))
	copy(result, items)
	return result
}
