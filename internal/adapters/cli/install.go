package cli

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"
)

type ClaudeInstallRunner func(out, errOut io.Writer) error

func newInstallCommand(run ClaudeInstallRunner) *cobra.Command {
	var claude bool
	cmd := &cobra.Command{
		Use:  "install",
		Args: cobra.ExactArgs(0),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !claude {
				return fmt.Errorf("install currently supports only --claude")
			}
			return run(cmd.OutOrStdout(), cmd.ErrOrStderr())
		},
	}
	cmd.Flags().BoolVar(&claude, "claude", false, "Install Claude integration")
	return cmd
}
