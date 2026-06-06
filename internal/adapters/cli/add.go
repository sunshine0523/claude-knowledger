package cli

import (
	"context"
	"encoding/json"

	"github.com/kindbrave/knowledger/internal/core"
	"github.com/kindbrave/knowledger/internal/service"
	"github.com/spf13/cobra"
)

func newAddCommand(svc *service.Service) *cobra.Command {
	var kbID, title, content string
	cmd := &cobra.Command{
		Use: "add",
		RunE: func(cmd *cobra.Command, args []string) error {
			item, ingest, status, err := svc.Add(context.Background(), core.AddInput{KBID: kbID, Title: title, Content: content})
			if err != nil {
				return err
			}
			return json.NewEncoder(cmd.OutOrStdout()).Encode(map[string]any{"item": item, "ingestion_result": ingest, "index_status": status})
		},
	}
	cmd.Flags().StringVar(&kbID, "kb", "", "knowledge base id")
	cmd.Flags().StringVar(&title, "title", "", "item title")
	cmd.Flags().StringVar(&content, "content", "", "item content")
	return cmd
}
