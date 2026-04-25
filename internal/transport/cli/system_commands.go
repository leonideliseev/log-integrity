package cli

import (
	"context"
	"fmt"

	"github.com/lenchik/logmonitor/internal/runtimeinfo"
	"github.com/spf13/cobra"
)

// newHealthCommand performs the same liveness check semantics as /healthz.
func (a *Application) newHealthCommand() *cobra.Command {
	return &cobra.Command{
		Use:     "health",
		Short:   "Run a local liveness check",
		Long:    "Loads the selected config in CLI mode and returns a success response when lightweight initialization succeeds.",
		Example: "logmonitor health\nlogmonitor health --output json",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return a.withSnapshot(cmd, func(_ context.Context, _ runtimeinfo.Snapshot) error {
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
		Long:    "Loads config and probes repository availability without running migrations or config bootstrap.",
		Example: "logmonitor ready\nlogmonitor ready --output json",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return a.withReadiness(cmd, func(_ context.Context, readiness runtimeinfo.Readiness) error {
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
		Long:    "Loads the selected CLI config, applies environment resolution and prints the runtime validation snapshot without touching persistent storage.",
		Example: "logmonitor config validate\nlogmonitor config validate --output json",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return a.withSnapshot(cmd, func(_ context.Context, snapshot runtimeinfo.Snapshot) error {
				return a.printRuntimeSnapshot(snapshot)
			})
		},
	}
}
