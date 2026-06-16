package cli

import (
	"context"
	"encoding/json"

	"github.com/kindbrave/knowledger/internal/service"
	"github.com/spf13/cobra"
)

func newIndexCommand(svc *service.Service) *cobra.Command {
	var kbID string
	var rebuild, all bool
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
	return cmd
}
