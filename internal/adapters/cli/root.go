package cli

import (
	"io"

	"github.com/kindbrave/knowledger/internal/config"
	"github.com/kindbrave/knowledger/internal/service"
	"github.com/spf13/cobra"
)

func NewRootCommand(svc *service.Service) *cobra.Command {
	return NewRootCommandWithAddress(svc, config.DefaultServerAddress)
}

func NewRootCommandWithAddress(svc *service.Service, address string) *cobra.Command {
	return NewRootCommandWithAddressAndMCPRunner(svc, address, func() error { return nil })
}

func NewRootCommandWithAddressAndMCPRunner(svc *service.Service, address string, runMCP MCPRunner) *cobra.Command {
	return NewRootCommandWithAddressAndRunners(svc, address, runMCP, func(out, errOut io.Writer) error { return nil })
}

func NewRootCommandWithAddressAndRunners(svc *service.Service, address string, runMCP MCPRunner, runClaudeInstall ClaudeInstallRunner) *cobra.Command {
	cmd := &cobra.Command{Use: "knowledger"}
	cmd.PersistentFlags().StringVar(&scopeFlag, "scope", "", "knowledge base scope: project, global. Defaults to project when running in a project directory, else global.")
	cmd.AddCommand(newSearchCommand(svc))
	cmd.AddCommand(newGetCommand(svc))
	cmd.AddCommand(newListItemsCommand(svc))
	cmd.AddCommand(newAddCommand(svc))
	cmd.AddCommand(newDeleteCommand(svc))
	cmd.AddCommand(newIndexCommand(svc))
	cmd.AddCommand(newListKBsCommand(svc))
	cmd.AddCommand(newCreateKBCommand(svc))
	cmd.AddCommand(newDeleteKBCommand(svc))
	cmd.AddCommand(newServeCommand(svc, address))
	cmd.AddCommand(newMCPCommand(runMCP))
	cmd.AddCommand(newInstallCommand(runClaudeInstall))
	return cmd
}
