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

func newAddCommand(svc *service.Service) *cobra.Command {
	var kbID, title, content string
	cmd := &cobra.Command{
		Use: "add",
		RunE: func(cmd *cobra.Command, args []string) error {
			if showsEmbeddedChromaHint(svc, kbID) {
				fmt.Fprintln(cmd.ErrOrStderr(), "Embedded Chroma semantic indexing may download runtime/model files on first use; this can take a few minutes.")
			}
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

func showsEmbeddedChromaHint(svc *service.Service, kbID string) bool {
	if svc == nil {
		return false
	}
	for _, kb := range svc.ListKnowledgeBases() {
		if kb.ID != kbID {
			continue
		}
		semantic, ok := kb.Indexing["semantic"].(map[string]any)
		if !ok {
			return false
		}
		enabled, _ := semantic["enabled"].(bool)
		provider, _ := semantic["provider"].(string)
		mode, _ := semantic["mode"].(string)
		if mode == "" {
			mode = "persistent"
		}
		return enabled && strings.EqualFold(provider, "chroma") && mode == "persistent"
	}
	return false
}
