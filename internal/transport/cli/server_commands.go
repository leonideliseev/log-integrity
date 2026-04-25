package cli

import (
	"context"
	"fmt"

	"github.com/lenchik/logmonitor/internal/app"
	"github.com/lenchik/logmonitor/models"
	"github.com/spf13/cobra"
)

// newServerCommand groups server management commands.
func (a *Application) newServerCommand() *cobra.Command {
	command := &cobra.Command{
		Use:   "server",
		Short: "Manage monitored servers",
	}

	command.AddCommand(
		a.newServerListCommand(),
		a.newServerGetCommand(),
		a.newServerAddCommand(),
		a.newServerUpdateCommand(),
		a.newServerDeleteCommand(),
		a.newServerRetryCommand(),
	)

	return command
}

// newServerListCommand lists all monitored servers.
func (a *Application) newServerListCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List monitored servers",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return a.withRuntime(cmd, func(ctx context.Context, runtime *app.Runtime) error {
				items, err := runtime.ServerService.List(ctx)
				if err != nil {
					return err
				}
				return a.printServers(items)
			})
		},
	}
}

// newServerGetCommand prints one monitored server by ID.
func (a *Application) newServerGetCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "get <server-id>",
		Short: "Show one monitored server",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.withRuntime(cmd, func(ctx context.Context, runtime *app.Runtime) error {
				item, err := runtime.ServerService.Get(ctx, args[0])
				if err != nil {
					return err
				}
				return a.printServers([]*models.Server{item})
			})
		},
	}
}

// newServerAddCommand creates a new API-managed server.
func (a *Application) newServerAddCommand() *cobra.Command {
	attrs := newServerAttributes()

	command := &cobra.Command{
		Use:     "add",
		Short:   "Add a monitored server",
		Example: "logmonitor server add --name web-1 --host 192.168.1.10 --username ubuntu --auth-type key --auth-value C:/keys/id_rsa --os-type linux",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := attrs.validateForCreate(); err != nil {
				return err
			}

			return a.withRuntime(cmd, func(ctx context.Context, runtime *app.Runtime) error {
				serverModel := attrs.toModel("")
				if err := runtime.ServerService.Create(ctx, serverModel); err != nil {
					return err
				}
				return a.printServers([]*models.Server{serverModel})
			})
		},
	}

	attrs.bindCreateFlags(command)
	return command
}

// newServerUpdateCommand updates a monitored server using changed flags as a patch.
func (a *Application) newServerUpdateCommand() *cobra.Command {
	attrs := newServerAttributes()

	command := &cobra.Command{
		Use:   "update <server-id>",
		Short: "Update a monitored server",
		Example: "logmonitor server update srv_123 --host 192.168.1.11\n" +
			"logmonitor server update srv_123 --status inactive",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.withRuntime(cmd, func(ctx context.Context, runtime *app.Runtime) error {
				current, err := runtime.ServerService.Get(ctx, args[0])
				if err != nil {
					return err
				}

				updated := *current
				attrs.applyChanges(cmd, &updated)
				if err := attrs.validateForUpdate(&updated); err != nil {
					return err
				}

				if err := runtime.ServerService.Update(ctx, &updated); err != nil {
					return err
				}

				refreshed, err := runtime.ServerService.Get(ctx, args[0])
				if err != nil {
					return err
				}
				return a.printServers([]*models.Server{refreshed})
			})
		},
	}

	attrs.bindUpdateFlags(command)
	return command
}

// newServerDeleteCommand deletes one monitored server by ID.
func (a *Application) newServerDeleteCommand() *cobra.Command {
	return &cobra.Command{
		Use:     "delete <server-id>",
		Short:   "Delete a monitored server",
		Example: "logmonitor server delete srv_123",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.withRuntime(cmd, func(ctx context.Context, runtime *app.Runtime) error {
				if err := runtime.ServerService.Delete(ctx, args[0]); err != nil {
					return err
				}
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), "server deleted")
				return nil
			})
		},
	}
}

// newServerRetryCommand clears backoff state for one monitored server.
func (a *Application) newServerRetryCommand() *cobra.Command {
	return &cobra.Command{
		Use:     "retry <server-id>",
		Short:   "Clear temporary failure state for a server",
		Example: "logmonitor server retry srv_123",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.withRuntime(cmd, func(ctx context.Context, runtime *app.Runtime) error {
				item, err := runtime.ServerService.Retry(ctx, args[0])
				if err != nil {
					return err
				}
				return a.printServers([]*models.Server{item})
			})
		},
	}
}

type serverAttributes struct {
	name      string
	host      string
	port      int
	username  string
	authType  string
	authValue string
	osType    string
	status    string
}

// newServerAttributes allocates reusable server flag state for create and update commands.
func newServerAttributes() *serverAttributes {
	return &serverAttributes{}
}

// bindCreateFlags registers required flags for server creation.
func (s *serverAttributes) bindCreateFlags(command *cobra.Command) {
	command.Flags().StringVar(&s.name, "name", "", "Server display name")
	command.Flags().StringVar(&s.host, "host", "", "Server host or IP")
	command.Flags().IntVar(&s.port, "port", 22, "SSH port")
	command.Flags().StringVar(&s.username, "username", "", "SSH username")
	command.Flags().StringVar(&s.authType, "auth-type", "", "SSH auth type: password or key")
	command.Flags().StringVar(&s.authValue, "auth-value", "", "SSH password or path to private key")
	command.Flags().StringVar(&s.osType, "os-type", "", "OS type: linux, windows or macos")
	_ = command.MarkFlagRequired("name")
	_ = command.MarkFlagRequired("host")
	_ = command.MarkFlagRequired("username")
	_ = command.MarkFlagRequired("auth-type")
	_ = command.MarkFlagRequired("auth-value")
}

// bindUpdateFlags registers optional flags used as a server patch.
func (s *serverAttributes) bindUpdateFlags(command *cobra.Command) {
	command.Flags().StringVar(&s.name, "name", "", "Server display name")
	command.Flags().StringVar(&s.host, "host", "", "Server host or IP")
	command.Flags().IntVar(&s.port, "port", 0, "SSH port")
	command.Flags().StringVar(&s.username, "username", "", "SSH username")
	command.Flags().StringVar(&s.authType, "auth-type", "", "SSH auth type: password or key")
	command.Flags().StringVar(&s.authValue, "auth-value", "", "SSH password or path to private key")
	command.Flags().StringVar(&s.osType, "os-type", "", "OS type: linux, windows or macos")
	command.Flags().StringVar(&s.status, "status", "", "Server status: active or inactive")
}

// applyChanges merges only changed flags into the loaded server model.
func (s *serverAttributes) applyChanges(command *cobra.Command, serverModel *models.Server) {
	if command.Flags().Changed("name") {
		serverModel.Name = s.name
	}
	if command.Flags().Changed("host") {
		serverModel.Host = s.host
	}
	if command.Flags().Changed("port") {
		serverModel.Port = s.port
	}
	if command.Flags().Changed("username") {
		serverModel.Username = s.username
	}
	if command.Flags().Changed("auth-type") {
		serverModel.AuthType = models.AuthType(s.authType)
	}
	if command.Flags().Changed("auth-value") {
		serverModel.AuthValue = s.authValue
	}
	if command.Flags().Changed("os-type") {
		serverModel.OSType = models.OSType(s.osType)
	}
	if command.Flags().Changed("status") {
		serverModel.Status = models.ServerStatus(s.status)
	}
}

// validateForCreate validates required server creation fields before entering the service layer.
func (s *serverAttributes) validateForCreate() error {
	return validateServerInput(s.name, s.host, s.port, s.username, s.authType, s.authValue, s.osType, "")
}

// validateForUpdate validates the merged server model before update.
func (s *serverAttributes) validateForUpdate(serverModel *models.Server) error {
	return validateServerInput(serverModel.Name, serverModel.Host, serverModel.Port, serverModel.Username, string(serverModel.AuthType), serverModel.AuthValue, string(serverModel.OSType), string(serverModel.Status))
}

// toModel converts validated command flags into a domain server model.
func (s *serverAttributes) toModel(serverID string) *models.Server {
	return &models.Server{
		ID:        serverID,
		Name:      s.name,
		Host:      s.host,
		Port:      s.port,
		Username:  s.username,
		AuthType:  models.AuthType(s.authType),
		AuthValue: s.authValue,
		OSType:    models.OSType(s.osType),
		Status:    models.ServerStatus(s.status),
	}
}

// validateServerInput mirrors transport-level validation for CLI commands.
func validateServerInput(name, host string, port int, username, authType, authValue, osType, status string) error {
	if name == "" {
		return fmt.Errorf("server name is required")
	}
	if host == "" {
		return fmt.Errorf("server host is required")
	}
	if username == "" {
		return fmt.Errorf("server username is required")
	}
	if port <= 0 || port > 65535 {
		return fmt.Errorf("server port must be between 1 and 65535")
	}
	if authType != string(models.AuthPassword) && authType != string(models.AuthKey) {
		return fmt.Errorf("server auth-type must be password or key")
	}
	if authValue == "" {
		return fmt.Errorf("server auth-value is required")
	}
	if osType != "" && osType != string(models.OSLinux) && osType != string(models.OSWindows) && osType != string(models.OSMacOS) {
		return fmt.Errorf("server os-type must be empty, linux, windows or macos")
	}
	if status != "" && status != string(models.ServerStatusActive) && status != string(models.ServerStatusInactive) {
		return fmt.Errorf("server status must be empty, active or inactive")
	}
	return nil
}
