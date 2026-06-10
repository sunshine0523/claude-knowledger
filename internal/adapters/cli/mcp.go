package cli

import "github.com/spf13/cobra"

type MCPRunner func() error

func newMCPCommand(run MCPRunner) *cobra.Command {
	return &cobra.Command{
		Use:   "mcp",
		Short: "Start the MCP stdio server",
		RunE: func(cmd *cobra.Command, args []string) error {
			return run()
		},
	}
}
