package cli

import (
	"context"
	"fmt"

	"github.com/lenchik/logmonitor/config"
	"github.com/lenchik/logmonitor/internal/app"
	"github.com/spf13/cobra"
)

// newServeCommand starts the full HTTP server from the CLI binary.
func (a *Application) newServeCommand() *cobra.Command {
	return &cobra.Command{
		Use:     "serve",
		Short:   "Start the HTTP server",
		Long:    "Starts the same application runtime as cmd/server, including HTTP transport, background jobs and graceful shutdown.",
		Example: "logmonitor serve --config config.yaml",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := config.LoadRuntime(a.configPath)
			if err != nil {
				return fmt.Errorf("cli: load config: %w", err)
			}

			application, err := app.New(cfg)
			if err != nil {
				return fmt.Errorf("cli: create app: %w", err)
			}

			ctx := cmd.Context()
			if ctx == nil {
				ctx = context.Background()
			}

			return application.Run(ctx)
		},
	}
}

// newHealthCommand performs the same liveness check semantics as /healthz.
func (a *Application) newHealthCommand() *cobra.Command {
	return &cobra.Command{
		Use:     "health",
		Short:   "Run a local liveness check",
		Long:    "Builds the local runtime and returns the same success semantics as the HTTP /healthz probe.",
		Example: "logmonitor health\nlogmonitor health --output json",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return a.withRuntime(cmd, func(_ context.Context, _ *app.Runtime) error {
				if a.output == outputJSON {
					return printJSON(a.out(), map[string]string{"status": "ok"})
				}
				_, err := fmt.Fprintln(a.out(), "ok")
				return err
			})
		},
	}
}

// newReadyCommand performs the same local readiness evaluation as /readyz.
func (a *Application) newReadyCommand() *cobra.Command {
	return &cobra.Command{
		Use:     "ready",
		Short:   "Run a local readiness check",
		Long:    "Builds the runtime and prints the same readiness payload that the HTTP /readyz probe would return.",
		Example: "logmonitor ready\nlogmonitor ready --output json",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return a.withRuntime(cmd, func(ctx context.Context, runtime *app.Runtime) error {
				readiness := runtime.Readiness(ctx)
				if !readiness.Ready {
					if err := a.printReadiness(readiness); err != nil {
						return err
					}
					return fmt.Errorf("cli: runtime is not ready")
				}
				return a.printReadiness(readiness)
			})
		},
	}
}

// newConfigCommand groups configuration-oriented helper commands.
func (a *Application) newConfigCommand() *cobra.Command {
	command := &cobra.Command{
		Use:   "config",
		Short: "Inspect effective configuration state",
	}

	command.AddCommand(a.newConfigValidateCommand())
	return command
}

// newConfigValidateCommand validates config loading and prints runtime validation details.
func (a *Application) newConfigValidateCommand() *cobra.Command {
	return &cobra.Command{
		Use:     "validate",
		Short:   "Validate config and show runtime warnings",
		Long:    "Loads the selected config, applies environment resolution and prints the same validation snapshot exposed by the HTTP runtime endpoint.",
		Example: "logmonitor config validate\nlogmonitor config validate --output json",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return a.withRuntime(cmd, func(_ context.Context, runtime *app.Runtime) error {
				return a.printRuntimeSnapshot(runtime.RuntimeState.Snapshot())
			})
		},
	}
}
