package cli

import (
	"context"
	"fmt"
	"io"

	"github.com/lenchik/logmonitor/config"
	"github.com/lenchik/logmonitor/internal/app"
	"github.com/spf13/cobra"
)

const (
	outputTable = "table"
	outputJSON  = "json"
)

// Application stores global CLI settings shared across all commands.
type Application struct {
	configPath string
	output     string
	stdout     io.Writer
	stderr     io.Writer
}

// NewRootCommand creates the root CLI command for local administration tasks.
func NewRootCommand() *cobra.Command {
	application := &Application{}

	root := &cobra.Command{
		Use:           "logmonitor",
		Short:         "Console utility for remote log monitoring and integrity control",
		Long:          "logmonitor is a console utility for operating the remote log monitoring and integrity control service.",
		Example:       "logmonitor server list\nlogmonitor server add --name web-1 --host 192.168.1.10 --username ubuntu --auth-type key --auth-value C:/keys/id_rsa --os-type linux\nlogmonitor discover --server-id srv_123\nlogmonitor check run --server-id srv_123 --output json",
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	root.PersistentFlags().StringVarP(&application.configPath, "config", "c", "config.yaml", "Path to YAML config")
	root.PersistentFlags().StringVarP(&application.output, "output", "o", outputTable, "Output format: table or json")

	root.AddCommand(
		application.newServeCommand(),
		application.newHealthCommand(),
		application.newReadyCommand(),
		application.newConfigCommand(),
		application.newServerCommand(),
		application.newDiscoverCommand(),
		application.newLogFileCommand(),
		application.newCollectCommand(),
		application.newEntryCommand(),
		application.newCheckCommand(),
		application.newProblemCommand(),
		application.newDashboardCommand(),
		application.newRuntimeCommand(),
	)

	return root
}

// withRuntime loads config, builds shared services and closes resources after one command finishes.
func (a *Application) withRuntime(cmd *cobra.Command, run func(context.Context, *app.Runtime) error) error {
	if err := a.validateOutput(); err != nil {
		return err
	}
	a.stdout = cmd.OutOrStdout()
	a.stderr = cmd.ErrOrStderr()

	cfg, err := config.LoadRuntime(a.configPath)
	if err != nil {
		return fmt.Errorf("cli: load config: %w", err)
	}

	runtime, err := app.NewRuntime(cfg)
	if err != nil {
		return fmt.Errorf("cli: build runtime: %w", err)
	}
	defer func() {
		_ = runtime.Close()
	}()

	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}

	return run(ctx, runtime)
}

// validateOutput ensures the selected printer mode is supported by the CLI.
func (a *Application) validateOutput() error {
	switch a.output {
	case outputTable, outputJSON:
		return nil
	default:
		return fmt.Errorf("cli: unsupported output format %q", a.output)
	}
}

// out returns the preferred stdout writer for command responses.
func (a *Application) out() io.Writer {
	if a != nil && a.stdout != nil {
		return a.stdout
	}
	return io.Discard
}
