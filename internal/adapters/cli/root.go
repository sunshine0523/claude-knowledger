package cli

import (
	"github.com/kindbrave/knowledger/internal/config"
	"github.com/kindbrave/knowledger/internal/service"
	"github.com/spf13/cobra"
)

func NewRootCommand(svc *service.Service) *cobra.Command {
	return NewRootCommandWithAddress(svc, config.DefaultServerAddress)
}

func NewRootCommandWithAddress(svc *service.Service, address string) *cobra.Command {
	cmd := &cobra.Command{Use: "knowledger"}
	cmd.AddCommand(newSearchCommand(svc))
	cmd.AddCommand(newAddCommand(svc))
	cmd.AddCommand(newListKBsCommand(svc))
	cmd.AddCommand(newServeCommand(svc, address))
	return cmd
}
