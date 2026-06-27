package cli

import (
	"context"
	"encoding/json"

	"github.com/kindbrave/claude-knowledger/internal/service"
	"github.com/spf13/cobra"
)

func newGetCommand(svc *service.Service) *cobra.Command {
	var kbID string
	var itemID string
	cmd := &cobra.Command{
		Use: "get",
		RunE: func(cmd *cobra.Command, args []string) error {
			scope, err := EffectiveScope(ScopeFlagValue(), svc != nil && svc.HasProjectScope())
			if err != nil {
				return err
			}
			item, err := svc.GetKnowledgeItem(context.Background(), scope, kbID, itemID)
			if err != nil {
				return err
			}
			return json.NewEncoder(cmd.OutOrStdout()).Encode(item)
		},
	}
	cmd.Flags().StringVar(&kbID, "kb", "", "knowledge base id")
	cmd.Flags().StringVar(&itemID, "id", "", "knowledge item id")
	return cmd
}
