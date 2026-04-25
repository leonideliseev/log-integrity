package cli

import (
	"context"
	"fmt"

	generalapp "github.com/lenchik/logmonitor/internal/app/general"
	"github.com/spf13/cobra"
)

// newHealthCommand performs the same liveness check semantics as /healthz.
func (a *Application) newHealthCommand() *cobra.Command {
	return &cobra.Command{
		Use:     "health",
		Short:   "Run a local liveness check",
		Long:    "Builds the standalone CLI runtime and returns a success response when the process can initialize correctly.",
		Example: "logmonitor health\nlogmonitor health --output json",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return a.withRuntime(cmd, func(_ context.Context, _ *generalapp.Runtime) error {
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
		Long:    "Builds the standalone runtime and prints the shared readiness payload without HTTP-specific checks.",
		Example: "logmonitor ready\nlogmonitor ready --output json",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return a.withRuntime(cmd, func(ctx context.Context, runtime *generalapp.Runtime) error {
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
		Long:    "Loads the selected CLI config, applies environment resolution and prints the runtime validation snapshot.",
		Example: "logmonitor config validate\nlogmonitor config validate --output json",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return a.withRuntime(cmd, func(_ context.Context, runtime *generalapp.Runtime) error {
				return a.printRuntimeSnapshot(runtime.RuntimeState.Snapshot())
			})
		},
	}
}
