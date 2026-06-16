package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/kindbrave/knowledger/internal/core"
	"github.com/kindbrave/knowledger/internal/service"
	"github.com/spf13/cobra"
)

func newSearchCommand(svc *service.Service) *cobra.Command {
	var query string
	var limit int
	var searchMode string
	var kbIDs []string
	cmd := &cobra.Command{
		Use: "search",
		RunE: func(cmd *cobra.Command, args []string) error {
			refs, err := parseKBIDs(kbIDs, ScopeFlagValue(), svc != nil && svc.HasProjectScope())
			if err != nil {
				return err
			}
			result, err := svc.Search(context.Background(), core.SearchOptions{
				Query:      query,
				Limit:      limit,
				KBIDs:      refs,
				SearchMode: searchMode,
			})
			if err != nil {
				return err
			}
			return json.NewEncoder(cmd.OutOrStdout()).Encode(result)
		},
	}
	cmd.Flags().StringVar(&query, "query", "", "search query")
	cmd.Flags().IntVar(&limit, "limit", 10, "maximum number of hits")
	cmd.Flags().StringVar(&searchMode, "search-mode", "", "search mode")
	cmd.Flags().StringSliceVar(&kbIDs, "kb-id", nil, "knowledge base id (may repeat); accepts \"scope:id\" or bare \"id\"")
	return cmd
}

// parseKBIDs converts CLI --kb-id values into ScopedKBRef. Each element is
// either "scope:id" or a bare "id". Bare ids inherit the default scope from
// --scope (or HasProjectScope auto-detection). Empty input means "all".
func parseKBIDs(values []string, scopeFlag string, inProject bool) ([]core.ScopedKBRef, error) {
	if len(values) == 0 {
		return nil, nil
	}
	defaultScope, err := EffectiveScope(scopeFlag, inProject)
	if err != nil {
		return nil, err
	}
	out := make([]core.ScopedKBRef, 0, len(values))
	for _, raw := range values {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		if strings.Contains(raw, ":") {
			parts := strings.SplitN(raw, ":", 2)
			s, err := core.NormalizeScope(parts[0])
			if err != nil {
				return nil, fmt.Errorf("--kb-id %q: %w", raw, err)
			}
			id := strings.TrimSpace(parts[1])
			if id == "" {
				return nil, fmt.Errorf("--kb-id %q: id is empty", raw)
			}
			out = append(out, core.ScopedKBRef{Scope: s, ID: id})
			continue
		}
		out = append(out, core.ScopedKBRef{Scope: defaultScope, ID: raw})
	}
	return out, nil
}
