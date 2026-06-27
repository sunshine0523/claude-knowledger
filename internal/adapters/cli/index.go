package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/kindbrave/claude-knowledger/internal/core"
	"github.com/kindbrave/claude-knowledger/internal/service"
	"github.com/spf13/cobra"
)

func newIndexCommand(svc *service.Service) *cobra.Command {
	var kbID string
	var rebuild, all, quiet bool
	cmd := &cobra.Command{
		Use:   "index",
		Short: "Backfill or rebuild semantic indexes",
		RunE: func(cmd *cobra.Command, args []string) error {
			input := service.IndexKnowledgeInput{KBID: kbID, Rebuild: rebuild}
			if all {
				input.Scope = ""
				input.KBID = ""
			} else {
				scope, err := EffectiveScope(ScopeFlagValue(), svc != nil && svc.HasProjectScope())
				if err != nil {
					return err
				}
				input.Scope = scope
			}
			if !quiet {
				input.Progress = stderrIndexProgress(cmd.ErrOrStderr())
			}
			result, err := svc.IndexKnowledge(context.Background(), input)
			if err != nil {
				return err
			}
			return json.NewEncoder(cmd.OutOrStdout()).Encode(result)
		},
	}
	cmd.Flags().StringVar(&kbID, "kb", "", "knowledge base id")
	cmd.Flags().BoolVar(&rebuild, "rebuild", false, "delete existing semantic vectors before indexing")
	cmd.Flags().BoolVar(&all, "all", false, "index all knowledge bases across all scopes")
	cmd.Flags().BoolVar(&quiet, "quiet", false, "suppress progress output to stderr")
	return cmd
}

// stderrIndexProgress returns an IndexProgress that writes one line per event
// to w. Suitable for human consumption; the JSON result still goes to stdout.
func stderrIndexProgress(w io.Writer) core.IndexProgress {
	return func(ev core.IndexProgressEvent) {
		switch ev.Phase {
		case core.IndexProgressPhaseStart:
			if ev.Total > 0 {
				fmt.Fprintf(w, "[%s] scanning %d items\n", ev.KBID, ev.Total)
			} else {
				fmt.Fprintf(w, "[%s] scanning\n", ev.KBID)
			}
		case core.IndexProgressPhaseRebuildReset:
			fmt.Fprintf(w, "[%s] rebuild: cleared persistent index at %s\n", ev.KBID, ev.Message)
		case core.IndexProgressPhaseIndex:
			fmt.Fprintf(w, "[%s] %s indexing %s\n", ev.KBID, formatProgress(ev.Done, ev.Total), ev.Item)
		case core.IndexProgressPhaseSkip:
			fmt.Fprintf(w, "[%s] %s skip %s\n", ev.KBID, formatProgress(ev.Done, ev.Total), ev.Item)
		case core.IndexProgressPhaseDeleteOrphan:
			fmt.Fprintf(w, "[%s] delete orphan %s\n", ev.KBID, ev.Item)
		case core.IndexProgressPhaseDone:
			if ev.Total > 0 {
				fmt.Fprintf(w, "[%s] done (%d/%d)\n", ev.KBID, ev.Done, ev.Total)
			} else {
				fmt.Fprintf(w, "[%s] done\n", ev.KBID)
			}
		}
	}
}

func formatProgress(done, total int) string {
	if total <= 0 {
		return fmt.Sprintf("%d", done)
	}
	return fmt.Sprintf("%d/%d", done, total)
}
