package cli

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/kindbrave/claude-knowledger/internal/service"
	"github.com/spf13/cobra"
)

type listItemsSummary struct {
	ID        string   `json:"id"`
	KBID      string   `json:"kb_id"`
	Scope     string   `json:"scope"`
	Type      string   `json:"type,omitempty"`
	Title     string   `json:"title"`
	Tags      []string `json:"tags,omitempty"`
	UpdatedAt string   `json:"updated_at,omitempty"`
}

func newListItemsCommand(svc *service.Service) *cobra.Command {
	var kbID string
	var titlesOnly bool
	cmd := &cobra.Command{
		Use:   "list-items",
		Short: "List items in a knowledge base (id/title/tags only — no content)",
		RunE: func(cmd *cobra.Command, args []string) error {
			scope, err := EffectiveScope(ScopeFlagValue(), svc != nil && svc.HasProjectScope())
			if err != nil {
				return err
			}
			items, err := svc.ListKnowledgeItems(context.Background(), scope, kbID)
			if err != nil {
				return err
			}
			if titlesOnly {
				for _, item := range items {
					fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\n", item.ID, item.Title)
				}
				return nil
			}
			summaries := make([]listItemsSummary, 0, len(items))
			for _, item := range items {
				updated := ""
				if !item.UpdatedAt.IsZero() {
					updated = item.UpdatedAt.UTC().Format("2006-01-02T15:04:05Z")
				}
				summaries = append(summaries, listItemsSummary{
					ID:        item.ID,
					KBID:      item.KBID,
					Scope:     scope,
					Type:      item.Type,
					Title:     item.Title,
					Tags:      item.Tags,
					UpdatedAt: updated,
				})
			}
			return json.NewEncoder(cmd.OutOrStdout()).Encode(summaries)
		},
	}
	cmd.Flags().StringVar(&kbID, "kb", "", "knowledge base id")
	cmd.Flags().BoolVar(&titlesOnly, "titles-only", false, "print one \"id<TAB>title\" line per item")
	return cmd
}
