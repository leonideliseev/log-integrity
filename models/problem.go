package models

import "time"

// ProblemSeverity describes how urgently an operator should react to a detected issue.
type ProblemSeverity string

// Problem severities help rank operational issues in API responses.
const (
	ProblemSeverityWarning  ProblemSeverity = "warning"
	ProblemSeverityError    ProblemSeverity = "error"
	ProblemSeverityCritical ProblemSeverity = "critical"
)

// ProblemType identifies a concrete operational issue category.
type ProblemType string

// Problem types separate availability issues from integrity violations.
const (
	ProblemTypeServerDegraded      ProblemType = "server_degraded"
	ProblemTypeServerUnreachable   ProblemType = "server_unreachable"
	ProblemTypeServerBackoff       ProblemType = "server_backoff"
	ProblemTypeIntegrityTampered   ProblemType = "integrity_tampered"
	ProblemTypeIntegrityCheckError ProblemType = "integrity_check_error"
)

// SystemProblem represents one actionable problem visible to the operator.
type SystemProblem struct {
	Severity   ProblemSeverity `json:"severity"`
	Type       ProblemType     `json:"type"`
	ServerID   string          `json:"server_id,omitempty"`
	ServerName string          `json:"server_name,omitempty"`
	LogFileID  string          `json:"log_file_id,omitempty"`
	LogPath    string          `json:"log_path,omitempty"`
	Message    string          `json:"message"`
	DetectedAt time.Time       `json:"detected_at"`
}

// SeverityForCheckStatus maps integrity check status to API severity.
func SeverityForCheckStatus(status CheckStatus) ProblemSeverity {
	switch status {
	case CheckStatusTampered:
		return ProblemSeverityCritical
	case CheckStatusError:
		return ProblemSeverityError
	default:
		return ""
	}
}

// ProblemTypeForCheckStatus maps integrity check status to a concrete problem type.
func ProblemTypeForCheckStatus(status CheckStatus) ProblemType {
	switch status {
	case CheckStatusTampered:
		return ProblemTypeIntegrityTampered
	case CheckStatusError:
		return ProblemTypeIntegrityCheckError
	default:
		return ""
	}
}
