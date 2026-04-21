package discovery

import (
	"fmt"

	"github.com/lenchik/logmonitor/internal/ssh"
	"github.com/lenchik/logmonitor/models"
)

const macOSDiscoveryCommand = "find /var/log /Library/Logs -maxdepth 3 -type f \\( -name '*.log' -o -name 'system.log' -o -name 'install.log' \\) 2>/dev/null | sort -u"

// MacOSDiscoverer discovers common macOS log files.
type MacOSDiscoverer struct{}

// NewMacOSDiscoverer creates a discoverer for macOS log locations.
func NewMacOSDiscoverer() *MacOSDiscoverer {
	return &MacOSDiscoverer{}
}

// Discover runs the macOS discovery command through SSH.
func (d *MacOSDiscoverer) Discover(client ssh.Client) ([]DiscoveredLog, error) {
	output, err := client.Execute(macOSDiscoveryCommand)
	if err != nil {
		return nil, fmt.Errorf("discovery: macos command failed: %w", err)
	}
	return parseDiscoveredOutput(output), nil
}

// SupportedOS reports the operating system handled by this discoverer.
func (d *MacOSDiscoverer) SupportedOS() models.OSType {
	return models.OSMacOS
}
