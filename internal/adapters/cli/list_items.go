package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/kindbrave/claude-knowledger/internal/service"
	"github.com/spf13/cobra"
)

type listItemsSummary struct {
	ID        string         `json:"id"`
	KBID      string         `json:"kb_id"`
	Scope     string         `json:"scope"`
	Type      string         `json:"type,omitempty"`
	Title     string         `json:"title"`
	Summary   string         `json:"summary,omitempty"`
	Metadata  map[string]any `json:"metadata,omitempty"`
	Tags      []string       `json:"tags,omitempty"`
	UpdatedAt string         `json:"updated_at,omitempty"`
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

			// Print table header
			fmt.Fprintln(cmd.OutOrStdout(), "Item-ID\tTitle\tMetadata\tSummary")

			// Format as table rows
			for _, item := range items {
				// Format metadata as compact string
				metadataStr := formatMetadataInline(item.Metadata)

				// Use summary as is (already cleaned)
				summary := item.Summary

				fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\t%s\t%s\n",
					item.ID, item.Title, metadataStr, summary)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&kbID, "kb", "", "knowledge base id")
	cmd.Flags().BoolVar(&titlesOnly, "titles-only", false, "print one \"id<TAB>title\" line per item")
	return cmd
}

// formatMetadataInline converts metadata map to inline string representation
func formatMetadataInline(metadata map[string]any) string {
	if len(metadata) == 0 {
		return "-"
	}
	var parts []string
	for k, v := range metadata {
		parts = append(parts, fmt.Sprintf("%s:%v", k, v))
	}
	return strings.Join(parts, "; ")
}
