package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/kindbrave/claude-knowledger/internal/core"
	"github.com/kindbrave/claude-knowledger/internal/service"
	"github.com/spf13/cobra"
)

func newAddCommand(svc *service.Service) *cobra.Command {
	var kbID, title, content, metadataJSON string
	var tags []string
	cmd := &cobra.Command{
		Use: "add",
		RunE: func(cmd *cobra.Command, args []string) error {
			scope, err := EffectiveScope(ScopeFlagValue(), svc != nil && svc.HasProjectScope())
			if err != nil {
				return err
			}
			var metadata map[string]any
			if strings.TrimSpace(metadataJSON) != "" {
				if err := json.Unmarshal([]byte(metadataJSON), &metadata); err != nil {
					return fmt.Errorf("--metadata must be a JSON object: %w", err)
				}
			}
			if showsEmbeddedChromaHint(svc, scope, kbID) {
				fmt.Fprintln(cmd.ErrOrStderr(), "Embedded Chroma semantic indexing may download runtime/model files on first use; this can take a few minutes.")
			}
			item, ingest, status, err := svc.Add(context.Background(), core.AddInput{KBID: kbID, Scope: scope, Title: title, Content: content, Tags: tags, Metadata: metadata})
			if err != nil {
				return err
			}
			return json.NewEncoder(cmd.OutOrStdout()).Encode(map[string]any{"item": item, "ingestion_result": ingest, "index_status": status})
		},
	}
	cmd.Flags().StringVar(&kbID, "kb", "", "knowledge base id")
	cmd.Flags().StringVar(&title, "title", "", "item title")
	cmd.Flags().StringVar(&content, "content", "", "item content")
	cmd.Flags().StringSliceVar(&tags, "tag", nil, "item tag (repeat or comma-separate for multiple)")
	cmd.Flags().StringVar(&metadataJSON, "metadata", "", "item metadata as a JSON object")
	return cmd
}

func showsEmbeddedChromaHint(svc *service.Service, scope, kbID string) bool {
	if svc == nil {
		return false
	}
	for _, kb := range svc.ListKnowledgeBases() {
		if kb.ID != kbID || kb.Scope != scope {
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
