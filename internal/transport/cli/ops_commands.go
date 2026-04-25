package cli

import (
	"context"
	"fmt"

	"github.com/lenchik/logmonitor/internal/app"
	"github.com/spf13/cobra"
)

// newDiscoverCommand creates the manual discovery command.
func (a *Application) newDiscoverCommand() *cobra.Command {
	var serverID string

	command := &cobra.Command{
		Use:   "discover",
		Short: "Run log discovery",
		Long:  "Runs remote log discovery immediately without going through the HTTP API.",
		Example: "logmonitor discover\n" +
			"logmonitor discover --server-id srv_123",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return a.withRuntime(cmd, func(ctx context.Context, runtime *app.Runtime) error {
				items, err := runtime.ServerService.Discover(ctx, serverID)
				if err != nil {
					return err
				}
				return a.printDiscoverResults(items)
			})
		},
	}

	command.Flags().StringVar(&serverID, "server-id", "", "Run discovery only for one server")
	return command
}

// newLogFileCommand groups log file related commands.
func (a *Application) newLogFileCommand() *cobra.Command {
	command := &cobra.Command{
		Use:   "logfile",
		Short: "Inspect discovered log files",
	}

	command.AddCommand(a.newLogFileListCommand())
	return command
}

// newLogFileListCommand lists active log files or log files of one server.
func (a *Application) newLogFileListCommand() *cobra.Command {
	var serverID string

	command := &cobra.Command{
		Use:   "list",
		Short: "List log files",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return a.withRuntime(cmd, func(ctx context.Context, runtime *app.Runtime) error {
				items, err := runtime.LogFileService.List(ctx, serverID)
				if err != nil {
					return err
				}
				return a.printLogFiles(items)
			})
		},
	}

	command.Flags().StringVar(&serverID, "server-id", "", "Filter log files by server")
	return command
}

// newCollectCommand creates the manual collection command.
func (a *Application) newCollectCommand() *cobra.Command {
	var serverID string
	var logFileID string

	command := &cobra.Command{
		Use:   "collect",
		Short: "Collect remote log entries",
		Long:  "Reads remote log files immediately and stores new entries using the same service logic as the server process.",
		Example: "logmonitor collect --server-id srv_123\n" +
			"logmonitor collect --server-id srv_123 --log-file-id log_456",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if serverID == "" {
				return fmt.Errorf("cli: --server-id is required")
			}

			return a.withRuntime(cmd, func(ctx context.Context, runtime *app.Runtime) error {
				items, err := runtime.LogFileService.Collect(ctx, serverID, logFileID)
				if err != nil {
					return err
				}
				return a.printCollectResults(items)
			})
		},
	}

	command.Flags().StringVar(&serverID, "server-id", "", "Collect only from one server")
	command.Flags().StringVar(&logFileID, "log-file-id", "", "Collect only one discovered log file")
	_ = command.MarkFlagRequired("server-id")
	return command
}

// newEntryCommand groups log entry related commands.
func (a *Application) newEntryCommand() *cobra.Command {
	command := &cobra.Command{
		Use:   "entry",
		Short: "Inspect collected log entries",
	}

	command.AddCommand(a.newEntryListCommand())
	return command
}

// newEntryListCommand lists stored log entries with pagination.
func (a *Application) newEntryListCommand() *cobra.Command {
	var logFileID string
	var offset int
	var limit int

	command := &cobra.Command{
		Use:     "list",
		Short:   "List collected log entries",
		Example: "logmonitor entry list --log-file-id log_456 --limit 50",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if logFileID == "" {
				return fmt.Errorf("cli: --log-file-id is required")
			}

			return a.withRuntime(cmd, func(ctx context.Context, runtime *app.Runtime) error {
				items, err := runtime.EntryService.List(ctx, logFileID, offset, limit)
				if err != nil {
					return err
				}
				return a.printEntries(items)
			})
		},
	}

	command.Flags().StringVar(&logFileID, "log-file-id", "", "List entries only for one log file")
	command.Flags().IntVar(&offset, "offset", 0, "Pagination offset")
	command.Flags().IntVar(&limit, "limit", 100, "Pagination limit")
	_ = command.MarkFlagRequired("log-file-id")
	return command
}

// newCheckCommand groups integrity related commands.
func (a *Application) newCheckCommand() *cobra.Command {
	command := &cobra.Command{
		Use:   "check",
		Short: "Inspect and run integrity checks",
	}

	command.AddCommand(
		a.newCheckListCommand(),
		a.newCheckRunCommand(),
	)
	return command
}

// newCheckListCommand lists stored integrity checks with pagination.
func (a *Application) newCheckListCommand() *cobra.Command {
	var logFileID string
	var offset int
	var limit int

	command := &cobra.Command{
		Use:     "list",
		Short:   "List integrity check history",
		Example: "logmonitor check list --log-file-id log_456",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if logFileID == "" {
				return fmt.Errorf("cli: --log-file-id is required")
			}

			return a.withRuntime(cmd, func(ctx context.Context, runtime *app.Runtime) error {
				items, err := runtime.CheckService.List(ctx, logFileID, offset, limit)
				if err != nil {
					return err
				}
				return a.printChecks(items)
			})
		},
	}

	command.Flags().StringVar(&logFileID, "log-file-id", "", "List checks only for one log file")
	command.Flags().IntVar(&offset, "offset", 0, "Pagination offset")
	command.Flags().IntVar(&limit, "limit", 100, "Pagination limit")
	_ = command.MarkFlagRequired("log-file-id")
	return command
}

// newCheckRunCommand launches integrity checks synchronously.
func (a *Application) newCheckRunCommand() *cobra.Command {
	var serverID string
	var logFileID string

	command := &cobra.Command{
		Use:   "run",
		Short: "Run integrity checks",
		Long:  "Runs integrity verification immediately for one server or one concrete discovered log file.",
		Example: "logmonitor check run --server-id srv_123\n" +
			"logmonitor check run --server-id srv_123 --log-file-id log_456",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if serverID == "" {
				return fmt.Errorf("cli: --server-id is required")
			}

			return a.withRuntime(cmd, func(ctx context.Context, runtime *app.Runtime) error {
				items, err := runtime.CheckService.Run(ctx, serverID, logFileID)
				if err != nil {
					return err
				}
				return a.printRunResults(items)
			})
		},
	}

	command.Flags().StringVar(&serverID, "server-id", "", "Run checks only for one server")
	command.Flags().StringVar(&logFileID, "log-file-id", "", "Run checks only for one discovered log file")
	_ = command.MarkFlagRequired("server-id")
	return command
}

// newProblemCommand groups operator-facing problem commands.
func (a *Application) newProblemCommand() *cobra.Command {
	command := &cobra.Command{
		Use:   "problem",
		Short: "Inspect aggregated monitoring problems",
	}

	command.AddCommand(a.newProblemListCommand())
	return command
}

// newProblemListCommand prints current aggregated problem items.
func (a *Application) newProblemListCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List aggregated problems",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return a.withRuntime(cmd, func(ctx context.Context, runtime *app.Runtime) error {
				items, err := runtime.ServerService.ListProblems(ctx)
				if err != nil {
					return err
				}
				return a.printProblems(items)
			})
		},
	}
}

// newDashboardCommand prints aggregated counters useful for quick operator overviews.
func (a *Application) newDashboardCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "dashboard",
		Short: "Show dashboard summary",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return a.withRuntime(cmd, func(ctx context.Context, runtime *app.Runtime) error {
				item, err := runtime.ServerService.Dashboard(ctx)
				if err != nil {
					return err
				}
				return a.printDashboard(item)
			})
		},
	}
}

// newRuntimeCommand groups runtime validation commands.
func (a *Application) newRuntimeCommand() *cobra.Command {
	command := &cobra.Command{
		Use:   "runtime",
		Short: "Inspect runtime configuration state",
	}

	command.AddCommand(a.newRuntimeValidationCommand())
	return command
}

// newRuntimeValidationCommand prints runtime startup warnings and env resolution details.
func (a *Application) newRuntimeValidationCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "validation",
		Short: "Show runtime validation snapshot",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return a.withRuntime(cmd, func(_ context.Context, runtime *app.Runtime) error {
				return a.printRuntimeSnapshot(runtime.RuntimeState.Snapshot())
			})
		},
	}
}
