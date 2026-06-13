package cli

import (
	"context"
	"encoding/json"

	"github.com/kindbrave/knowledger/internal/service"
	"github.com/spf13/cobra"
)

func newIndexCommand(svc *service.Service) *cobra.Command {
	var kbID string
	var rebuild bool
	cmd := &cobra.Command{
		Use:   "index",
		Short: "Backfill or rebuild semantic indexes",
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := svc.IndexKnowledge(context.Background(), service.IndexKnowledgeInput{KBID: kbID, Rebuild: rebuild})
			if err != nil {
				return err
			}
			return json.NewEncoder(cmd.OutOrStdout()).Encode(result)
		},
	}
	cmd.Flags().StringVar(&kbID, "kb", "", "knowledge base id")
	cmd.Flags().BoolVar(&rebuild, "rebuild", false, "delete existing semantic vectors before indexing")
	return cmd
}
