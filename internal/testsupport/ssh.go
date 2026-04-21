// Package testsupport contains small fakes shared by unit tests.
package testsupport

import (
	"context"
	"sync"

	"github.com/lenchik/logmonitor/internal/ssh"
	"github.com/lenchik/logmonitor/models"
)

// SSHCommandHandler returns remote command output or an execution error.
type SSHCommandHandler func(cmd string) (string, error)

// SSHClientFactory creates fake SSH clients for service and cron unit tests.
type SSHClientFactory struct {
	ConnectErr error
	Execute    SSHCommandHandler

	mu      sync.Mutex
	clients []*SSHClient
}

// NewClient returns a new fake SSH client and records it for later assertions.
func (f *SSHClientFactory) NewClient() ssh.Client {
	f.mu.Lock()
	defer f.mu.Unlock()

	client := &SSHClient{factory: f}
	f.clients = append(f.clients, client)
	return client
}

// Clients returns all fake clients created by the factory.
func (f *SSHClientFactory) Clients() []*SSHClient {
	f.mu.Lock()
	defer f.mu.Unlock()

	result := make([]*SSHClient, len(f.clients))
	copy(result, f.clients)
	return result
}

// Commands returns all commands executed by all fake clients.
func (f *SSHClientFactory) Commands() []string {
	f.mu.Lock()
	clients := make([]*SSHClient, len(f.clients))
	copy(clients, f.clients)
	f.mu.Unlock()

	var commands []string
	for _, client := range clients {
		commands = append(commands, client.Commands()...)
	}
	return commands
}

// SSHClient implements ssh.Client for deterministic unit tests.
type SSHClient struct {
	factory *SSHClientFactory

	mu        sync.Mutex
	connected bool
	commands  []string
}

// Connect records a successful connection unless the factory is configured to fail.
func (c *SSHClient) Connect(_ *models.Server) error {
	if c.factory.ConnectErr != nil {
		return c.factory.ConnectErr
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	c.connected = true
	return nil
}

// Execute runs the fake command handler without a context.
func (c *SSHClient) Execute(cmd string) (string, error) {
	return c.ExecuteContext(context.Background(), cmd)
}

// ExecuteContext records the command and returns the configured fake output.
func (c *SSHClient) ExecuteContext(_ context.Context, cmd string) (string, error) {
	c.mu.Lock()
	c.commands = append(c.commands, cmd)
	c.mu.Unlock()

	if c.factory.Execute == nil {
		return "", nil
	}
	return c.factory.Execute(cmd)
}

// Close marks the fake client as disconnected.
func (c *SSHClient) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.connected = false
	return nil
}

// IsConnected reports the fake connection state.
func (c *SSHClient) IsConnected() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.connected
}

// Commands returns commands executed through this fake client.
func (c *SSHClient) Commands() []string {
	c.mu.Lock()
	defer c.mu.Unlock()

	result := make([]string, len(c.commands))
	copy(result, c.commands)
	return result
}
