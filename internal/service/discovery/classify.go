// Package discovery detects remote log files on supported operating systems.
package discovery

import (
	"path"
	"strings"

	"github.com/lenchik/logmonitor/models"
)

// classifyLogType infers a log type from a discovered remote path.
func classifyLogType(logPath string) models.LogType {
	normalized := strings.ToLower(strings.ReplaceAll(logPath, "\\", "/"))
	base := path.Base(normalized)

	switch {
	case strings.HasPrefix(normalized, "eventlog://"), strings.HasSuffix(normalized, ".evtx"):
		return models.LogTypeEventLog
	case strings.Contains(normalized, "/nginx/"):
		return models.LogTypeNginx
	case strings.Contains(normalized, "/apache"), strings.Contains(normalized, "/httpd/"):
		return models.LogTypeApache
	case base == "auth.log", base == "secure":
		return models.LogTypeAuth
	case base == "syslog", base == "messages", base == "system.log":
		return models.LogTypeSyslog
	case base == "kern.log", base == "kernel.log", base == "dmesg":
		return models.LogTypeKernel
	case strings.HasSuffix(base, ".log"):
		return models.LogTypeApp
	default:
		return models.LogTypeUnknown
	}
}

// parseDiscoveredOutput converts command output into unique discovered log entries.
func parseDiscoveredOutput(output string) []DiscoveredLog {
	if output == "" {
		return nil
	}

	seen := make(map[string]struct{})
	result := make([]DiscoveredLog, 0)

	for _, line := range strings.Split(strings.ReplaceAll(output, "\r\n", "\n"), "\n") {
		item := strings.TrimSpace(line)
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}

		result = append(result, DiscoveredLog{
			Path:    item,
			LogType: classifyLogType(item),
		})
	}

	return result
}
