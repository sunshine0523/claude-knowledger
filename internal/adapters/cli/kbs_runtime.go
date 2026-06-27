package cli

import (
	"context"
	"encoding/json"

	"github.com/kindbrave/claude-knowledger/internal/service"
	"github.com/spf13/cobra"
)

func newCreateKBCommand(svc *service.Service) *cobra.Command {
	var (
		id              string
		name            string
		storeType       string
		path            string
		tags            []string
		enabled         bool
		semanticEnabled bool
	)
	cmd := &cobra.Command{
		Use:   "kb-create",
		Short: "Create a new knowledge base",
		RunE: func(cmd *cobra.Command, args []string) error {
			scope, err := EffectiveScope(ScopeFlagValue(), svc != nil && svc.HasProjectScope())
			if err != nil {
				return err
			}
			input := service.CreateKnowledgeBaseInput{
				Scope:     scope,
				ID:        id,
				Name:      name,
				StoreType: storeType,
				Path:      path,
				Tags:      tags,
			}
			if cmd.Flags().Changed("enabled") {
				input.Enabled = &enabled
			}
			if cmd.Flags().Changed("semantic-enabled") {
				input.SemanticEnabled = &semanticEnabled
			}
			record, err := svc.CreateKnowledgeBase(context.Background(), input)
			if err != nil {
				return err
			}
			return json.NewEncoder(cmd.OutOrStdout()).Encode(map[string]any{"knowledge_base": record})
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "knowledge base id")
	cmd.Flags().StringVar(&name, "name", "", "human-readable name (defaults to id)")
	cmd.Flags().StringVar(&storeType, "store-type", "", "backend store type: text, sqlite")
	cmd.Flags().StringVar(&path, "path", "", "storage path (required for global scope)")
	cmd.Flags().StringSliceVar(&tags, "tag", nil, "knowledge base tag (repeat for multiple)")
	cmd.Flags().BoolVar(&enabled, "enabled", true, "whether the knowledge base is enabled")
	cmd.Flags().BoolVar(&semanticEnabled, "semantic-enabled", false, "enable semantic indexing for sqlite store types")
	return cmd
}

func newDeleteKBCommand(svc *service.Service) *cobra.Command {
	var id string
	cmd := &cobra.Command{
		Use:   "kb-delete",
		Short: "Delete a runtime-managed knowledge base",
		RunE: func(cmd *cobra.Command, args []string) error {
			scope, err := EffectiveScope(ScopeFlagValue(), svc != nil && svc.HasProjectScope())
			if err != nil {
				return err
			}
			if err := svc.DeleteKnowledgeBase(context.Background(), scope, id); err != nil {
				return err
			}
			return json.NewEncoder(cmd.OutOrStdout()).Encode(map[string]any{
				"deleted": true,
				"scope":   scope,
				"id":      id,
			})
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "knowledge base id")
	return cmd
}
