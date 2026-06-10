package cli

import (
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
	cmd := &cobra.Command{Use: "knowledger"}
	cmd.AddCommand(newSearchCommand(svc))
	cmd.AddCommand(newGetCommand(svc))
	cmd.AddCommand(newAddCommand(svc))
	cmd.AddCommand(newListKBsCommand(svc))
	cmd.AddCommand(newServeCommand(svc, address))
	cmd.AddCommand(newMCPCommand(runMCP))
	return cmd
}
