package cli

import (
	"context"
	"encoding/json"

	"github.com/kindbrave/knowledger/internal/core"
	"github.com/kindbrave/knowledger/internal/service"
	"github.com/spf13/cobra"
)

func newSearchCommand(svc *service.Service) *cobra.Command {
	var query string
	var limit int
	var searchMode string
	cmd := &cobra.Command{
		Use: "search",
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := svc.Search(context.Background(), core.SearchOptions{Query: query, Limit: limit, SearchMode: searchMode})
			if err != nil {
				return err
			}
			return json.NewEncoder(cmd.OutOrStdout()).Encode(result)
		},
	}
	cmd.Flags().StringVar(&query, "query", "", "search query")
	cmd.Flags().IntVar(&limit, "limit", 10, "maximum number of hits")
	cmd.Flags().StringVar(&searchMode, "search-mode", "", "search mode")
	return cmd
}
