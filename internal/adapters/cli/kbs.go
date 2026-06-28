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

			// Print table header
			fmt.Fprintln(cmd.OutOrStdout(), "KB-ID\tName\tScope\tStore-Type\tItem-Count")

			// Print each KB as a row
			for _, kb := range kbs {
				items, err := svc.ListKnowledgeItems(context.Background(), kb.Scope, kb.ID)
				itemCount := 0
				if err == nil {
					itemCount = len(items)
				}

				scope := kb.Scope
				if scope == "" {
					scope = core.ScopeGlobal
				}

				name := kb.Name
				if name == "" {
					name = kb.ID
				}

				fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\t%s\t%s\t%d\n",
					kb.ID, name, scope, kb.StoreType, itemCount)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&scopeFilter, "scope-filter", "", "filter by scope: project, global, or all (default all)")
	return cmd
}

