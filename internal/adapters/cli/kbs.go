package cli

import (
	"encoding/json"

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
			return json.NewEncoder(cmd.OutOrStdout()).Encode(kbs)
		},
	}
	cmd.Flags().StringVar(&scopeFilter, "scope-filter", "", "filter by scope: project, global, or all (default all)")
	return cmd
}
