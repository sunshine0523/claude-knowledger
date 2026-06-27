package cli

import (
	"context"
	"encoding/json"

	"github.com/kindbrave/claude-knowledger/internal/service"
	"github.com/spf13/cobra"
)

func newDeleteCommand(svc *service.Service) *cobra.Command {
	var kbID string
	var itemID string
	cmd := &cobra.Command{
		Use:   "delete",
		Short: "Delete a knowledge item from a knowledge base",
		RunE: func(cmd *cobra.Command, args []string) error {
			scope, err := EffectiveScope(ScopeFlagValue(), svc != nil && svc.HasProjectScope())
			if err != nil {
				return err
			}
			if err := svc.DeleteKnowledgeItem(context.Background(), scope, kbID, itemID); err != nil {
				return err
			}
			return json.NewEncoder(cmd.OutOrStdout()).Encode(map[string]any{
				"deleted": true,
				"scope":   scope,
				"kb_id":   kbID,
				"item_id": itemID,
			})
		},
	}
	cmd.Flags().StringVar(&kbID, "kb", "", "knowledge base id")
	cmd.Flags().StringVar(&itemID, "id", "", "knowledge item id")
	return cmd
}
