package cli

import (
	"context"
	"fmt"

	"github.com/kindbrave/claude-knowledger/internal/core"
	"github.com/kindbrave/claude-knowledger/internal/service"
	"github.com/spf13/cobra"
)

func newListKBsCommand(svc *service.Service) *cobra.Command {
	var scopeFilter string
	cmd := &cobra.Command{
		Use:   "list-kbs",
		Short: "List knowledge bases (optionally filtered by --scope-filter)",
		RunE: func(cmd *cobra.Command, args []string) error {
			kbs := svc.ListKnowledgeBases()
			filter := scopeFilter
			if filter != "" && filter != "all" {
				normalized, err := core.NormalizeScope(filter)
				if err != nil {
					return err
				}
				filtered := make([]core.KnowledgeBase, 0, len(kbs))
				for _, kb := range kbs {
					if kb.Scope == normalized {
						filtered = append(filtered, kb)
					}
				}
				kbs = filtered
			}

			// For each KB, print header and then items table
			for i, kb := range kbs {
				if i > 0 {
					fmt.Fprintln(cmd.OutOrStdout())
				}

				scope := kb.Scope
				if scope == "" {
					scope = core.ScopeGlobal
				}

				name := kb.Name
				if name == "" {
					name = kb.ID
				}

				// Print KB header
				fmt.Fprintf(cmd.OutOrStdout(), "=== [%s:%s] %s (store=%s) ===\n",
					scope, kb.ID, name, kb.StoreType)

				// Get and print items
				items, err := svc.ListKnowledgeItems(context.Background(), kb.Scope, kb.ID)
				if err != nil {
					fmt.Fprintf(cmd.OutOrStdout(), "Error listing items: %s\n", err.Error())
					continue
				}

				if len(items) == 0 {
					fmt.Fprintln(cmd.OutOrStdout(), "(empty)")
					continue
				}

				// Print items table header
				fmt.Fprintln(cmd.OutOrStdout(), "Item-ID\tTitle\tMetadata\tSummary")

				// Print each item
				for _, item := range items {
					// Format metadata (use function from list_items.go)
					metadataStr := formatMetadataInline(item.Metadata)

					fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\t%s\t%s\n",
						item.ID, item.Title, metadataStr, item.Summary)
				}
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&scopeFilter, "scope-filter", "", "filter by scope: project, global, or all (default all)")
	return cmd
}
