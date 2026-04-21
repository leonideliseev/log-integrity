package ssh

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	gossh "golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"

	"github.com/lenchik/logmonitor/models"
)

// Executor executes commands through a single SSH connection.
type Executor struct {
	client          *gossh.Client
	connectTimeout  time.Duration
	commandTimeout  time.Duration
	hostKeyCallback gossh.HostKeyCallback
}

// Factory creates SSH clients with shared timeout defaults.
type Factory struct {
	options         Options
	hostKeyCallback gossh.HostKeyCallback
}

// Options configures SSH connection safety and timeout behavior.
type Options struct {
	ConnectTimeout        time.Duration
	CommandTimeout        time.Duration
	KnownHostsPath        string
	InsecureIgnoreHostKey bool
}

// NewClient creates an SSH executor with a default timeout fallback.
func NewClient(timeout time.Duration) *Executor {
	options := normalizeOptions(Options{
		ConnectTimeout:        timeout,
		InsecureIgnoreHostKey: true,
	})
	return &Executor{
		connectTimeout:  options.ConnectTimeout,
		commandTimeout:  options.CommandTimeout,
		hostKeyCallback: gossh.InsecureIgnoreHostKey(),
	}
}

// NewClientWithOptions creates an SSH executor with explicit safety settings.
func NewClientWithOptions(options Options) (*Executor, error) {
	options = normalizeOptions(options)
	hostKeyCallback, err := buildHostKeyCallback(options)
	if err != nil {
		return nil, err
	}
	return &Executor{
		connectTimeout:  options.ConnectTimeout,
		commandTimeout:  options.CommandTimeout,
		hostKeyCallback: hostKeyCallback,
	}, nil
}

// NewClientFactory creates a factory that produces SSH clients with shared defaults.
func NewClientFactory(timeout time.Duration) *Factory {
	options := normalizeOptions(Options{
		ConnectTimeout:        timeout,
		InsecureIgnoreHostKey: true,
	})
	return &Factory{
		options:         options,
		hostKeyCallback: gossh.InsecureIgnoreHostKey(),
	}
}

// NewClientFactoryWithOptions creates a factory from explicit SSH settings.
func NewClientFactoryWithOptions(options Options) (*Factory, error) {
	options = normalizeOptions(options)
	hostKeyCallback, err := buildHostKeyCallback(options)
	if err != nil {
		return nil, err
	}
	return &Factory{
		options:         options,
		hostKeyCallback: hostKeyCallback,
	}, nil
}

// NewClient creates a new SSH client instance from the factory.
func (f *Factory) NewClient() Client {
	return &Executor{
		connectTimeout:  f.options.ConnectTimeout,
		commandTimeout:  f.options.CommandTimeout,
		hostKeyCallback: f.hostKeyCallback,
	}
}

// Connect opens an SSH connection to the provided remote server.
func (e *Executor) Connect(serverModel *models.Server) error {
	if serverModel == nil {
		return fmt.Errorf("ssh: server is nil")
	}
	if e.client != nil {
		_ = e.Close()
	}

	authMethod, err := buildAuthMethod(serverModel)
	if err != nil {
		return err
	}

	port := serverModel.Port
	if port == 0 {
		port = 22
	}

	config := &gossh.ClientConfig{
		User:            serverModel.Username,
		Auth:            []gossh.AuthMethod{authMethod},
		HostKeyCallback: e.hostKeyCallback,
		Timeout:         e.connectTimeout,
	}

	client, err := gossh.Dial("tcp", fmt.Sprintf("%s:%d", serverModel.Host, port), config)
	if err != nil {
		return fmt.Errorf("ssh: dial %s:%d: %w", serverModel.Host, port, err)
	}

	e.client = client
	return nil
}

// Execute runs a command on the connected remote server.
func (e *Executor) Execute(cmd string) (string, error) {
	return e.ExecuteContext(context.Background(), cmd)
}

// ExecuteContext runs a command and closes the SSH session when context is canceled.
func (e *Executor) ExecuteContext(ctx context.Context, cmd string) (string, error) {
	if e.client == nil {
		return "", fmt.Errorf("ssh: client is not connected")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if e.commandTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, e.commandTimeout)
		defer cancel()
	}

	session, err := e.client.NewSession()
	if err != nil {
		return "", fmt.Errorf("ssh: create session: %w", err)
	}
	defer func() {
		_ = session.Close()
	}()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	session.Stdout = &stdout
	session.Stderr = &stderr

	if err := session.Start(cmd); err != nil {
		return "", fmt.Errorf("ssh: start command %q: %w", cmd, err)
	}

	done := make(chan error, 1)
	go func() {
		done <- session.Wait()
	}()

	select {
	case err := <-done:
		result := strings.TrimSpace(stdout.String() + stderr.String())
		if err != nil {
			if result == "" {
				return "", fmt.Errorf("ssh: execute command %q: %w", cmd, err)
			}
			return result, fmt.Errorf("ssh: execute command %q: %w", cmd, err)
		}
		return result, nil
	case <-ctx.Done():
		_ = session.Close()
		return "", fmt.Errorf("ssh: execute command %q canceled: %w", cmd, ctx.Err())
	}
}

// Close closes the current SSH connection if it exists.
func (e *Executor) Close() error {
	if e.client == nil {
		return nil
	}

	err := e.client.Close()
	e.client = nil
	return err
}

// IsConnected reports whether the executor currently holds an open client.
func (e *Executor) IsConnected() bool {
	return e.client != nil
}

// buildAuthMethod converts server auth settings into an SSH auth method.
func buildAuthMethod(serverModel *models.Server) (gossh.AuthMethod, error) {
	switch serverModel.AuthType {
	case models.AuthPassword:
		return gossh.Password(serverModel.AuthValue), nil
	case models.AuthKey:
		privateKey, err := os.ReadFile(serverModel.AuthValue)
		if err != nil {
			return nil, fmt.Errorf("ssh: read private key %q: %w", serverModel.AuthValue, err)
		}
		signer, err := gossh.ParsePrivateKey(privateKey)
		if err != nil {
			return nil, fmt.Errorf("ssh: parse private key %q: %w", serverModel.AuthValue, err)
		}
		return gossh.PublicKeys(signer), nil
	default:
		return nil, fmt.Errorf("ssh: unsupported auth type %q", serverModel.AuthType)
	}
}

// normalizeOptions applies safe SSH timeout defaults.
func normalizeOptions(options Options) Options {
	if options.ConnectTimeout <= 0 {
		options.ConnectTimeout = 10 * time.Second
	}
	if options.CommandTimeout < 0 {
		options.CommandTimeout = 0
	}
	return options
}

// buildHostKeyCallback creates either strict known_hosts validation or explicit insecure mode.
func buildHostKeyCallback(options Options) (gossh.HostKeyCallback, error) {
	if options.InsecureIgnoreHostKey {
		return gossh.InsecureIgnoreHostKey(), nil
	}
	if options.KnownHostsPath == "" {
		return nil, fmt.Errorf("ssh: known_hosts path is required when insecure host key mode is disabled")
	}
	knownHostsPath := expandHomePath(options.KnownHostsPath)
	callback, err := knownhosts.New(knownHostsPath)
	if err != nil {
		return nil, fmt.Errorf("ssh: load known_hosts %q: %w", knownHostsPath, err)
	}
	return callback, nil
}

// expandHomePath supports ~/.ssh/known_hosts style paths in config.
func expandHomePath(path string) string {
	if path != "~" && !strings.HasPrefix(path, "~/") && !strings.HasPrefix(path, `~\`) {
		return path
	}
	homeDir, err := os.UserHomeDir()
	if err != nil || homeDir == "" {
		return path
	}
	if path == "~" {
		return homeDir
	}
	return filepath.Join(homeDir, path[2:])
}
