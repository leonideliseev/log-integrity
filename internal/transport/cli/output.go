package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"text/tabwriter"
	"time"

	"github.com/lenchik/logmonitor/internal/runtimeinfo"
	checkservice "github.com/lenchik/logmonitor/internal/service/check"
	logfileservice "github.com/lenchik/logmonitor/internal/service/logfile"
	serverservice "github.com/lenchik/logmonitor/internal/service/server"
	"github.com/lenchik/logmonitor/models"
)

// printServers renders a server collection according to the selected output format.
func (a *Application) printServers(items []*models.Server) error {
	sanitized := make([]*models.Server, 0, len(items))
	for _, item := range items {
		sanitized = append(sanitized, sanitizeServer(item))
	}

	if a.output == outputJSON {
		return printJSON(a.out(), sanitized)
	}

	rows := make([][]string, 0, len(sanitized))
	for _, item := range sanitized {
		rows = append(rows, []string{
			item.ID,
			item.Name,
			item.Host,
			strconv.Itoa(item.Port),
			item.Username,
			string(item.OSType),
			string(item.Status),
			string(item.ManagedBy),
		})
	}
	return printTable(a.out(), []string{"ID", "NAME", "HOST", "PORT", "USER", "OS", "STATUS", "MANAGED_BY"}, rows)
}

// printLogFiles renders discovered log files.
func (a *Application) printLogFiles(items []*models.LogFile) error {
	if a.output == outputJSON {
		return printJSON(a.out(), items)
	}

	rows := make([][]string, 0, len(items))
	for _, item := range items {
		rows = append(rows, []string{
			item.ID,
			item.ServerID,
			item.Path,
			string(item.LogType),
			strconv.FormatBool(item.IsActive),
		})
	}
	return printTable(a.out(), []string{"ID", "SERVER_ID", "PATH", "TYPE", "ACTIVE"}, rows)
}

// printEntries renders stored log entries.
func (a *Application) printEntries(items []*models.LogEntry) error {
	if a.output == outputJSON {
		return printJSON(a.out(), items)
	}

	rows := make([][]string, 0, len(items))
	for _, item := range items {
		rows = append(rows, []string{
			item.ID,
			item.LogFileID,
			strconv.FormatInt(item.LineNumber, 10),
			item.Hash,
			item.Content,
		})
	}
	return printTable(a.out(), []string{"ID", "LOG_FILE_ID", "LINE", "HASH", "CONTENT"}, rows)
}

// printChecks renders integrity check history.
func (a *Application) printChecks(items []*models.CheckResult) error {
	if a.output == outputJSON {
		return printJSON(a.out(), items)
	}

	rows := make([][]string, 0, len(items))
	for _, item := range items {
		rows = append(rows, []string{
			item.ID,
			item.LogFileID,
			string(item.Status),
			strconv.FormatInt(item.TotalLines, 10),
			strconv.FormatInt(item.TamperedLines, 10),
			item.ErrorMessage,
		})
	}
	return printTable(a.out(), []string{"ID", "LOG_FILE_ID", "STATUS", "TOTAL_LINES", "TAMPERED_LINES", "ERROR"}, rows)
}

// printProblems renders operator-facing problem items.
func (a *Application) printProblems(items []models.SystemProblem) error {
	if a.output == outputJSON {
		return printJSON(a.out(), items)
	}

	rows := make([][]string, 0, len(items))
	for _, item := range items {
		rows = append(rows, []string{
			string(item.Severity),
			string(item.Type),
			item.ServerID,
			item.ServerName,
			item.LogFileID,
			item.LogPath,
			item.Message,
			item.DetectedAt.UTC().Format(time.RFC3339),
		})
	}
	return printTable(a.out(), []string{"SEVERITY", "TYPE", "SERVER_ID", "SERVER", "LOG_FILE_ID", "LOG_PATH", "MESSAGE", "DETECTED_AT"}, rows)
}

// printDashboard renders aggregated dashboard counters.
func (a *Application) printDashboard(item *serverservice.Dashboard) error {
	if a.output == outputJSON {
		return printJSON(a.out(), item)
	}

	rows := [][]string{
		{"servers.total", strconv.Itoa(item.Servers.Total)},
		{"servers.active", strconv.Itoa(item.Servers.Active)},
		{"servers.degraded", strconv.Itoa(item.Servers.Degraded)},
		{"servers.inactive", strconv.Itoa(item.Servers.Inactive)},
		{"servers.error", strconv.Itoa(item.Servers.Error)},
		{"log_files.total", strconv.Itoa(item.LogFiles.Total)},
		{"log_files.active", strconv.Itoa(item.LogFiles.Active)},
		{"log_files.inactive", strconv.Itoa(item.LogFiles.Inactive)},
		{"problems.total", strconv.Itoa(item.Problems.Total)},
		{"problems.warning", strconv.Itoa(item.Problems.Warning)},
		{"problems.error", strconv.Itoa(item.Problems.Error)},
		{"problems.critical", strconv.Itoa(item.Problems.Critical)},
	}
	return printTable(a.out(), []string{"METRIC", "VALUE"}, rows)
}

// printRuntimeSnapshot renders runtime validation details.
func (a *Application) printRuntimeSnapshot(snapshot runtimeinfo.Snapshot) error {
	if a.output == outputJSON {
		return printJSON(a.out(), snapshot)
	}

	rows := [][]string{
		{"dry_run", strconv.FormatBool(snapshot.DryRun)},
		{"storage_backend", snapshot.StorageBackend},
		{"scheduler_enabled", strconv.FormatBool(snapshot.SchedulerEnabled)},
	}
	for _, warning := range snapshot.Warnings {
		rows = append(rows, []string{
			"warning." + warning.Code,
			warning.Message,
		})
	}
	for _, check := range snapshot.EnvChecks {
		rows = append(rows, []string{
			"env." + check.Name,
			string(check.Status) + ": " + check.Message,
		})
	}
	return printTable(a.out(), []string{"FIELD", "VALUE"}, rows)
}

// printDiscoverResults renders discovery outcomes per server and log file.
func (a *Application) printDiscoverResults(items map[string]serverservice.DiscoverResult) error {
	if a.output == outputJSON {
		return printJSON(a.out(), items)
	}

	rows := make([][]string, 0)
	for serverID, item := range items {
		if item.Error != "" {
			rows = append(rows, []string{serverID, "-", "-", item.Error})
			continue
		}
		if len(item.LogFiles) == 0 {
			rows = append(rows, []string{serverID, "-", "-", "no log files discovered"})
			continue
		}
		for _, logFile := range item.LogFiles {
			rows = append(rows, []string{serverID, logFile.Path, string(logFile.LogType), "ok"})
		}
	}
	return printTable(a.out(), []string{"SERVER_ID", "PATH", "TYPE", "RESULT"}, rows)
}

// printCollectResults renders manual collection outcomes.
func (a *Application) printCollectResults(items map[string]logfileservice.CollectResult) error {
	if a.output == outputJSON {
		return printJSON(a.out(), items)
	}

	rows := make([][]string, 0, len(items))
	for logFileID, item := range items {
		rows = append(rows, []string{
			logFileID,
			strconv.Itoa(item.CollectedEntries),
			item.Error,
		})
	}
	return printTable(a.out(), []string{"LOG_FILE_ID", "COLLECTED_ENTRIES", "ERROR"}, rows)
}

// printRunResults renders manual integrity outcomes.
func (a *Application) printRunResults(items map[string]checkservice.RunResult) error {
	if a.output == outputJSON {
		return printJSON(a.out(), items)
	}

	rows := make([][]string, 0, len(items))
	for logFileID, item := range items {
		status := ""
		tamperedLines := ""
		if item.Result != nil {
			status = string(item.Result.Status)
			tamperedLines = strconv.FormatInt(item.Result.TamperedLines, 10)
		}
		rows = append(rows, []string{
			logFileID,
			status,
			tamperedLines,
			item.Error,
		})
	}
	return printTable(a.out(), []string{"LOG_FILE_ID", "STATUS", "TAMPERED_LINES", "ERROR"}, rows)
}

// sanitizeServer removes secret fields before printing or returning server data.
func sanitizeServer(serverModel *models.Server) *models.Server {
	if serverModel == nil {
		return nil
	}

	copyModel := *serverModel
	copyModel.AuthValue = ""
	return &copyModel
}

// printJSON writes indented JSON to stdout for machine-friendly command usage.
func printJSON(writer io.Writer, value any) error {
	encoder := json.NewEncoder(writer)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}

// printTable writes a simple aligned table to stdout.
func printTable(output io.Writer, headers []string, rows [][]string) error {
	writer := tabwriter.NewWriter(output, 0, 4, 2, ' ', 0)
	if _, err := fmt.Fprintln(writer, joinRow(headers)); err != nil {
		return err
	}
	for _, row := range rows {
		if _, err := fmt.Fprintln(writer, joinRow(row)); err != nil {
			return err
		}
	}
	return writer.Flush()
}

// joinRow converts a row into a tab-separated string suitable for tabwriter.
func joinRow(values []string) string {
	result := ""
	for index, value := range values {
		if index > 0 {
			result += "\t"
		}
		result += value
	}
	return result
}
