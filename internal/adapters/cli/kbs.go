package cli

import (
	"encoding/json"

	"github.com/kindbrave/knowledger/internal/service"
	"github.com/spf13/cobra"
)

func newListKBsCommand(svc *service.Service) *cobra.Command {
	return &cobra.Command{
		Use: "list-kbs",
		RunE: func(cmd *cobra.Command, args []string) error {
			return json.NewEncoder(cmd.OutOrStdout()).Encode(svc.ListKnowledgeBases())
		},
	}
}
