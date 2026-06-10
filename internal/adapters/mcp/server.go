package mcp

import (
	"context"
	"log"
	"os"

	"github.com/kindbrave/knowledger/internal/core"
	"github.com/kindbrave/knowledger/internal/service"
	mcpgo "github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
)

const serverName = "knowledger"
const serverVersion = "0.1.0"

type Server struct {
	svc    *service.Service
	server *mcpserver.MCPServer
	tools  []mcpgo.Tool
}

type ToolForTest = mcpgo.Tool

type searchKnowledgeInput struct {
	Query      string   `json:"query"`
	KBIDs      []string `json:"kb_ids,omitempty"`
	Limit      int      `json:"limit,omitempty"`
	SearchMode string   `json:"search_mode,omitempty"`
}

type getKnowledgeItemInput struct {
	KBID   string `json:"kb_id"`
	ItemID string `json:"item_id"`
}

type addKnowledgeItemInput struct {
	KBID     string         `json:"kb_id"`
	Title    string         `json:"title"`
	Content  string         `json:"content"`
	Tags     []string       `json:"tags,omitempty"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

func NewServer(svc *service.Service) *Server {
	adapter := &Server{svc: svc, server: mcpserver.NewMCPServer(serverName, serverVersion)}
	adapter.registerTools()
	return adapter
}

func (s *Server) MCPServer() *mcpserver.MCPServer { return s.server }

func (s *Server) Tools() []mcpgo.Tool { return append([]mcpgo.Tool(nil), s.tools...) }

func (s *Server) ServeStdio() error {
	logger := log.New(os.Stderr, "knowledger mcp: ", log.LstdFlags)
	return mcpserver.ServeStdio(s.server, mcpserver.WithErrorLogger(logger))
}

func (s *Server) registerTools() {
	searchTool := mcpgo.NewTool(
		"search_knowledge",
		mcpgo.WithDescription("Search knowledge bases for relevant items."),
		mcpgo.WithString("query", mcpgo.Required(), mcpgo.Description("Search query.")),
		mcpgo.WithArray("kb_ids", mcpgo.Description("Optional knowledge base IDs to search."), mcpgo.WithStringItems()),
		mcpgo.WithNumber("limit", mcpgo.Description("Maximum number of results."), mcpgo.DefaultNumber(10)),
		mcpgo.WithString("search_mode", mcpgo.Description("Search mode."), mcpgo.Enum("auto", "lexical", "semantic", "hybrid"), mcpgo.DefaultString("auto")),
		mcpgo.WithReadOnlyHintAnnotation(true),
		mcpgo.WithDestructiveHintAnnotation(false),
		mcpgo.WithIdempotentHintAnnotation(true),
		mcpgo.WithOpenWorldHintAnnotation(false),
	)
	getTool := mcpgo.NewTool(
		"get_knowledge_item",
		mcpgo.WithDescription("Get a knowledge item by knowledge base and item ID."),
		mcpgo.WithString("kb_id", mcpgo.Required(), mcpgo.Description("Knowledge base ID.")),
		mcpgo.WithString("item_id", mcpgo.Required(), mcpgo.Description("Knowledge item ID.")),
		mcpgo.WithReadOnlyHintAnnotation(true),
		mcpgo.WithDestructiveHintAnnotation(false),
		mcpgo.WithIdempotentHintAnnotation(true),
		mcpgo.WithOpenWorldHintAnnotation(false),
	)
	addTool := mcpgo.NewTool(
		"add_knowledge_item",
		mcpgo.WithDescription("Add a knowledge item to a knowledge base."),
		mcpgo.WithString("kb_id", mcpgo.Required(), mcpgo.Description("Knowledge base ID.")),
		mcpgo.WithString("title", mcpgo.Required(), mcpgo.Description("Item title.")),
		mcpgo.WithString("content", mcpgo.Required(), mcpgo.Description("Item content.")),
		mcpgo.WithArray("tags", mcpgo.Description("Optional item tags."), mcpgo.WithStringItems()),
		mcpgo.WithObject("metadata", mcpgo.Description("Optional item metadata."), mcpgo.AdditionalProperties(true)),
		mcpgo.WithReadOnlyHintAnnotation(false),
		mcpgo.WithDestructiveHintAnnotation(false),
		mcpgo.WithIdempotentHintAnnotation(false),
		mcpgo.WithOpenWorldHintAnnotation(false),
	)
	listTool := mcpgo.NewTool(
		"list_knowledge_bases",
		mcpgo.WithDescription("List configured knowledge bases."),
		mcpgo.WithReadOnlyHintAnnotation(true),
		mcpgo.WithDestructiveHintAnnotation(false),
		mcpgo.WithIdempotentHintAnnotation(true),
		mcpgo.WithOpenWorldHintAnnotation(false),
	)

	s.tools = []mcpgo.Tool{searchTool, getTool, addTool, listTool}
	s.server.AddTool(searchTool, s.handleSearchKnowledge)
	s.server.AddTool(getTool, s.handleGetKnowledgeItem)
	s.server.AddTool(addTool, s.handleAddKnowledgeItem)
	s.server.AddTool(listTool, s.handleListKnowledgeBases)
}

func (s *Server) handleSearchKnowledge(ctx context.Context, request mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	if s.svc == nil {
		return mcpgo.NewToolResultError("service is not configured"), nil
	}
	var input searchKnowledgeInput
	if err := request.BindArguments(&input); err != nil {
		return mcpgo.NewToolResultErrorFromErr("invalid arguments", err), nil
	}
	limit := input.Limit
	if limit == 0 {
		limit = 10
	}
	result, err := s.svc.Search(ctx, core.SearchOptions{Query: input.Query, KBIDs: input.KBIDs, Limit: limit, SearchMode: input.SearchMode})
	if err != nil {
		return mcpgo.NewToolResultError(err.Error()), nil
	}
	return mcpgo.NewToolResultStructuredOnly(result), nil
}

func (s *Server) handleGetKnowledgeItem(ctx context.Context, request mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	if s.svc == nil {
		return mcpgo.NewToolResultError("service is not configured"), nil
	}
	var input getKnowledgeItemInput
	if err := request.BindArguments(&input); err != nil {
		return mcpgo.NewToolResultErrorFromErr("invalid arguments", err), nil
	}
	item, err := s.svc.GetKnowledgeItem(ctx, input.KBID, input.ItemID)
	if err != nil {
		return mcpgo.NewToolResultError(err.Error()), nil
	}
	return mcpgo.NewToolResultStructuredOnly(item), nil
}

func (s *Server) handleAddKnowledgeItem(ctx context.Context, request mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	if s.svc == nil {
		return mcpgo.NewToolResultError("service is not configured"), nil
	}
	var input addKnowledgeItemInput
	if err := request.BindArguments(&input); err != nil {
		return mcpgo.NewToolResultErrorFromErr("invalid arguments", err), nil
	}
	item, ingestionResult, indexStatus, err := s.svc.Add(ctx, core.AddInput{KBID: input.KBID, Title: input.Title, Content: input.Content, Tags: input.Tags, Metadata: input.Metadata})
	if err != nil {
		return mcpgo.NewToolResultError(err.Error()), nil
	}
	return mcpgo.NewToolResultStructuredOnly(map[string]any{"item": item, "ingestion_result": ingestionResult, "index_status": indexStatus}), nil
}

func (s *Server) handleListKnowledgeBases(ctx context.Context, request mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	_ = ctx
	_ = request
	if s.svc == nil {
		return mcpgo.NewToolResultError("service is not configured"), nil
	}
	return mcpgo.NewToolResultStructuredOnly(s.svc.ListKnowledgeBases()), nil
}
