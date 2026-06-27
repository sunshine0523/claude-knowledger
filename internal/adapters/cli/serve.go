package cli

import (
	"fmt"
	"net/http"

	webadapter "github.com/kindbrave/claude-knowledger/internal/adapters/web"
	"github.com/kindbrave/claude-knowledger/internal/service"
	"github.com/spf13/cobra"
)

var listenAndServe = http.ListenAndServe

func newServeCommand(svc *service.Service, address string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Start the web dashboard",
		RunE: func(cmd *cobra.Command, args []string) error {
			server := webadapter.NewServer(svc)
			fmt.Fprintf(cmd.OutOrStdout(), "Knowledger web listening on %s\n", serveURL(address))
			return listenAndServe(address, server.Handler())
		},
	}
	return cmd
}

func serveURL(address string) string {
	if len(address) > 0 && address[0] == ':' {
		return "http://127.0.0.1" + address + "/"
	}
	return "http://" + address + "/"
}
