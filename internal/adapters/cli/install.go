package cli

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"
)

type ClaudeInstallRunner func(out, errOut io.Writer) error
type OpenCodeInstallRunner func(out, errOut io.Writer) error

func newInstallCommand(runClaude ClaudeInstallRunner, runOpenCode OpenCodeInstallRunner) *cobra.Command {
	var claude, opencode bool
	cmd := &cobra.Command{
		Use:  "install",
		Args: cobra.ExactArgs(0),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !claude && !opencode {
				return fmt.Errorf("install requires a target: pass --claude or --opencode")
			}
			out := cmd.OutOrStdout()
			errOut := cmd.ErrOrStderr()
			if claude {
				if err := runClaude(out, errOut); err != nil {
					return err
				}
			}
			if opencode {
				if err := runOpenCode(out, errOut); err != nil {
					return err
				}
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&claude, "claude", false, "Install Claude Code integration")
	cmd.Flags().BoolVar(&opencode, "opencode", false, "Install OpenCode integration")
	return cmd
}
