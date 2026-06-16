package mcp

import (
	"context"
	"log"
	"os"
	"strings"

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

type indexKnowledgeInput struct {
	KBID    string `json:"kb_id,omitempty"`
	Rebuild bool   `json:"rebuild,omitempty"`
}

type searchKnowledgeResult struct {
	Hits     []searchKnowledgeHit
	Warnings []string
}

type searchKnowledgeHit struct {
	ItemID        string
	KBID          string
	ItemType      string
	Title         string
	Snippet       string
	Score         float64
	MatchMode     string
	SourceBackend string
	Locator       string
	Metadata      map[string]any
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
	indexTool := mcpgo.NewTool(
		"index_knowledge",
		mcpgo.WithDescription("Backfill or rebuild semantic indexes for one knowledge base or all enabled knowledge bases."),
		mcpgo.WithString("kb_id", mcpgo.Description("Optional knowledge base ID. Omit to index all enabled knowledge bases.")),
		mcpgo.WithBoolean("rebuild", mcpgo.Description("Delete existing semantic vectors before indexing.")),
		mcpgo.WithReadOnlyHintAnnotation(false),
		mcpgo.WithDestructiveHintAnnotation(true),
		mcpgo.WithIdempotentHintAnnotation(true),
		mcpgo.WithOpenWorldHintAnnotation(false),
	)

	s.tools = []mcpgo.Tool{searchTool, getTool, addTool, listTool, indexTool}
	s.server.AddTool(searchTool, s.handleSearchKnowledge)
	s.server.AddTool(getTool, s.handleGetKnowledgeItem)
	s.server.AddTool(addTool, s.handleAddKnowledgeItem)
	s.server.AddTool(listTool, s.handleListKnowledgeBases)
	s.server.AddTool(indexTool, s.handleIndexKnowledge)
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
	// TODO(plan-3): add scope field to MCP search input
	refs := make([]core.ScopedKBRef, 0, len(input.KBIDs))
	for _, raw := range input.KBIDs {
		id := strings.TrimSpace(raw)
		if id == "" {
			continue
		}
		refs = append(refs, core.ScopedKBRef{ID: id})
	}
	result, err := s.svc.Search(ctx, core.SearchOptions{Query: input.Query, KBIDs: refs, Limit: limit, SearchMode: input.SearchMode})
	if err != nil {
		return mcpgo.NewToolResultError(err.Error()), nil
	}
	return mcpgo.NewToolResultStructuredOnly(toSearchKnowledgeResult(result)), nil
}

func toSearchKnowledgeResult(result service.SearchResult) searchKnowledgeResult {
	hits := make([]searchKnowledgeHit, 0, len(result.Hits))
	for _, hit := range result.Hits {
		hits = append(hits, searchKnowledgeHit{
			ItemID:        hit.ItemID,
			KBID:          hit.KBID,
			ItemType:      hit.ItemType,
			Title:         hit.Title,
			Snippet:       hit.Snippet,
			Score:         hit.Score,
			MatchMode:     hit.MatchMode,
			SourceBackend: hit.SourceBackend,
			Locator:       hit.Locator,
			Metadata:      hit.Metadata,
		})
	}
	return searchKnowledgeResult{Hits: hits, Warnings: result.Warnings}
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
	return mcpgo.NewToolResultStructuredOnly(map[string]any{"knowledge_bases": s.svc.ListKnowledgeBases()}), nil
}

func (s *Server) handleIndexKnowledge(ctx context.Context, request mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	if s.svc == nil {
		return mcpgo.NewToolResultError("service is not configured"), nil
	}
	var input indexKnowledgeInput
	if err := request.BindArguments(&input); err != nil {
		return mcpgo.NewToolResultErrorFromErr("invalid arguments", err), nil
	}
	result, err := s.svc.IndexKnowledge(ctx, service.IndexKnowledgeInput{KBID: input.KBID, Rebuild: input.Rebuild})
	if err != nil {
		return mcpgo.NewToolResultError(err.Error()), nil
	}
	return mcpgo.NewToolResultStructuredOnly(result), nil
}
