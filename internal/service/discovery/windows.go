package discovery

import (
	"fmt"

	"github.com/lenchik/logmonitor/internal/ssh"
	"github.com/lenchik/logmonitor/models"
)

const windowsDiscoveryCommand = `powershell -NoProfile -Command "$logs = @('Application', 'System', 'Security'); $available = foreach ($name in $logs) { if (Get-WinEvent -ListLog $name -ErrorAction SilentlyContinue) { 'eventlog://' + $name } }; $files = @(); $files += Get-ChildItem -Path 'C:\inetpub\logs\LogFiles' -Recurse -File -Filter '*.log' -ErrorAction SilentlyContinue | ForEach-Object { $_.FullName }; $files += Get-ChildItem -Path 'C:\Windows\Logs' -Recurse -File -Filter '*.log' -ErrorAction SilentlyContinue | ForEach-Object { $_.FullName }; $files += Get-ChildItem -Path 'C:\ProgramData' -Recurse -File -Filter '*.log' -ErrorAction SilentlyContinue | Select-Object -First 100 | ForEach-Object { $_.FullName }; ($available + $files) | Sort-Object -Unique"`

// WindowsDiscoverer discovers Windows event logs and log files.
type WindowsDiscoverer struct{}

// NewWindowsDiscoverer creates a discoverer for Windows event logs and log files.
func NewWindowsDiscoverer() *WindowsDiscoverer {
	return &WindowsDiscoverer{}
}

// Discover runs the Windows discovery command through SSH.
func (d *WindowsDiscoverer) Discover(client ssh.Client) ([]DiscoveredLog, error) {
	output, err := client.Execute(windowsDiscoveryCommand)
	if err != nil {
		return nil, fmt.Errorf("discovery: windows command failed: %w", err)
	}
	return parseDiscoveredOutput(output), nil
}

// SupportedOS reports the operating system handled by this discoverer.
func (d *WindowsDiscoverer) SupportedOS() models.OSType {
	return models.OSWindows
}
