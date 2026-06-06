package mcp

import (
	"github.com/kindbrave/knowledger/internal/service"
	mcpgo "github.com/mark3labs/mcp-go/mcp"
)

type Server struct {
	svc   *service.Service
	tools []mcpgo.Tool
}

func NewServer(svc *service.Service) *Server {
	tools := []mcpgo.Tool{
		mcpgo.NewTool("search", mcpgo.WithDescription("Search across enabled knowledge bases")),
		mcpgo.NewTool("add", mcpgo.WithDescription("Add knowledge to a knowledge base")),
		mcpgo.NewTool("list_kbs", mcpgo.WithDescription("List knowledge bases and capabilities")),
		mcpgo.NewTool("manage_kb", mcpgo.WithDescription("Manage knowledge bases")),
	}
	return &Server{svc: svc, tools: tools}
}

func (s *Server) Tools() []mcpgo.Tool { return s.tools }
