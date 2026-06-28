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
			// Format as text similar to MCP
			for i, item := range items {
				if i > 0 {
					fmt.Fprintln(cmd.OutOrStdout())
				}
				fmt.Fprintf(cmd.OutOrStdout(), "- %s\t%s\n", item.ID, item.Title)
				if item.Summary != "" {
					fmt.Fprintf(cmd.OutOrStdout(), "  Summary: %s\n", item.Summary)
				}
				if len(item.Metadata) > 0 {
					fmt.Fprintln(cmd.OutOrStdout(), "  Metadata:")
					for k, v := range item.Metadata {
						fmt.Fprintf(cmd.OutOrStdout(), "    %s: %v\n", k, v)
					}
				}
				if len(item.Tags) > 0 {
					fmt.Fprintf(cmd.OutOrStdout(), "  Tags: [%s]\n", strings.Join(item.Tags, ", "))
				}
				if !item.UpdatedAt.IsZero() {
					fmt.Fprintf(cmd.OutOrStdout(), "  Updated: %s\n", item.UpdatedAt.UTC().Format("2006-01-02T15:04:05Z"))
				}
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&kbID, "kb", "", "knowledge base id")
	cmd.Flags().BoolVar(&titlesOnly, "titles-only", false, "print one \"id<TAB>title\" line per item")
	return cmd
}
