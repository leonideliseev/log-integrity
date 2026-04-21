package discovery

import (
	"fmt"

	"github.com/lenchik/logmonitor/internal/ssh"
	"github.com/lenchik/logmonitor/models"
)

const linuxDiscoveryCommand = "find /var/log -maxdepth 3 -type f \\( -name '*.log' -o -name 'syslog' -o -name 'messages' -o -name 'secure' -o -name 'auth.log' -o -name 'kern.log' -o -name 'dmesg' \\) 2>/dev/null | sort -u"

// LinuxDiscoverer discovers common Linux log files.
type LinuxDiscoverer struct{}

// NewLinuxDiscoverer creates a discoverer for Linux log locations.
func NewLinuxDiscoverer() *LinuxDiscoverer {
	return &LinuxDiscoverer{}
}

// Discover runs the Linux discovery command through SSH.
func (d *LinuxDiscoverer) Discover(client ssh.Client) ([]DiscoveredLog, error) {
	output, err := client.Execute(linuxDiscoveryCommand)
	if err != nil {
		return nil, fmt.Errorf("discovery: linux command failed: %w", err)
	}
	return parseDiscoveredOutput(output), nil
}

// SupportedOS reports the operating system handled by this discoverer.
func (d *LinuxDiscoverer) SupportedOS() models.OSType {
	return models.OSLinux
}
