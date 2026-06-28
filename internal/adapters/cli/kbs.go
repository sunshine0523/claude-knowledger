package cli

import (
	"context"
	"fmt"
	"strings"

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
			// Format as text similar to MCP
			for i, kb := range kbs {
				if i > 0 {
					fmt.Fprintln(cmd.OutOrStdout())
				}
				writeKnowledgeBaseHeader(cmd.OutOrStdout(), kb)
				writeKnowledgeBaseItems(context.Background(), cmd.OutOrStdout(), svc, kb)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&scopeFilter, "scope-filter", "", "filter by scope: project, global, or all (default all)")
	return cmd
}

func writeKnowledgeBaseHeader(w interface{ Write([]byte) (int, error) }, kb core.KnowledgeBase) {
	scope := kb.Scope
	if scope == "" {
		scope = core.ScopeGlobal
	}
	fmt.Fprintf(w, "[%s:%s]", scope, kb.ID)
	if kb.Name != "" && kb.Name != kb.ID {
		fmt.Fprintf(w, " %s", kb.Name)
	}
	fmt.Fprintf(w, " (store=%s", kb.StoreType)
	if !kb.Enabled {
		fmt.Fprint(w, ", disabled")
	}
	if len(kb.Tags) > 0 {
		fmt.Fprintf(w, ", tags=%s", strings.Join(kb.Tags, ","))
	}
	fmt.Fprintln(w, ")")
}

func writeKnowledgeBaseItems(ctx context.Context, w interface{ Write([]byte) (int, error) }, svc *service.Service, kb core.KnowledgeBase) {
	items, err := svc.ListKnowledgeItems(ctx, kb.Scope, kb.ID)
	if err != nil {
		fmt.Fprintf(w, "  (items unavailable: %s)\n", err.Error())
		return
	}
	if len(items) == 0 {
		fmt.Fprintln(w, "  (empty)")
		return
	}
	for _, item := range items {
		fmt.Fprintf(w, "  - %s\t%s", item.ID, item.Title)
		if len(item.Tags) > 0 {
			fmt.Fprintf(w, " [%s]", strings.Join(item.Tags, ","))
		}
		fmt.Fprintln(w)
	}
}
