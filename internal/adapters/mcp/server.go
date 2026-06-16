package mcp

import (
	"context"
	"encoding/json"
	"fmt"
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
	Query      string            `json:"query"`
	KBIDs      []json.RawMessage `json:"kb_ids,omitempty"`
	Scope      string            `json:"scope,omitempty"`
	Limit      int               `json:"limit,omitempty"`
	SearchMode string            `json:"search_mode,omitempty"`
}

type getKnowledgeItemInput struct {
	Scope  string `json:"scope,omitempty"`
	KBID   string `json:"kb_id"`
	ItemID string `json:"item_id"`
}

type addKnowledgeItemInput struct {
	Scope    string         `json:"scope,omitempty"`
	KBID     string         `json:"kb_id"`
	Title    string         `json:"title"`
	Content  string         `json:"content"`
	Tags     []string       `json:"tags,omitempty"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

type indexKnowledgeInput struct {
	Scope   string `json:"scope,omitempty"`
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
	Scope         string
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
	s.logModeBanner(os.Stderr)
	logger := log.New(os.Stderr, "knowledger mcp: ", log.LstdFlags)
	return mcpserver.ServeStdio(s.server, mcpserver.WithErrorLogger(logger))
}

// logModeBanner writes a single line describing the scope mode the MCP
// server is running in. Surfaced via stderr so the operator can confirm
// the server discovered the project root they expected.
func (s *Server) logModeBanner(w *os.File) {
	if s.svc != nil && s.svc.HasProjectScope() {
		fmt.Fprintf(w, "knowledger: project mode (root=%s)\n", s.svc.ProjectRoot())
		return
	}
	fmt.Fprintln(w, "knowledger: global mode")
}

// LogModeBannerForTest exposes logModeBanner to the external test package.
func (s *Server) LogModeBannerForTest(w *os.File) { s.logModeBanner(w) }

func (s *Server) registerTools() {
	kbIDsItemSchema := map[string]any{
		"oneOf": []any{
			map[string]any{
				"type":        "string",
				"description": `Bare id ("notes") or "scope:id" ("project:notes").`,
			},
			map[string]any{
				"type": "object",
				"properties": map[string]any{
					"scope": map[string]any{"type": "string", "enum": []string{"project", "global"}},
					"id":    map[string]any{"type": "string"},
				},
				"required":             []string{"id"},
				"additionalProperties": false,
			},
		},
	}
	scopeProperty := mcpgo.WithString(
		"scope",
		mcpgo.Description("Knowledge base scope. Defaults to project when running in a project directory, otherwise global."),
		mcpgo.Enum("project", "global"),
	)
	searchTool := mcpgo.NewTool(
		"search_knowledge",
		mcpgo.WithDescription("Search knowledge bases for relevant items. When running in a project directory, scope defaults to project; otherwise global."),
		mcpgo.WithString("query", mcpgo.Required(), mcpgo.Description("Search query.")),
		mcpgo.WithArray(
			"kb_ids",
			mcpgo.Description(`Optional knowledge base refs. Each element may be a bare id ("notes"), a "scope:id" string ("project:notes"), or an object {"scope":"project","id":"notes"}. Bare ids inherit the request scope.`),
			mcpgo.Items(kbIDsItemSchema),
		),
		scopeProperty,
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
		scopeProperty,
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
		scopeProperty,
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
		scopeProperty,
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
	defaultScope, err := s.defaultScope(input.Scope)
	if err != nil {
		return mcpgo.NewToolResultError(err.Error()), nil
	}
	refs, err := parseSearchKBIDs(input.KBIDs, defaultScope)
	if err != nil {
		return mcpgo.NewToolResultError(err.Error()), nil
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
			Scope:         hit.Scope,
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
	scope, err := s.defaultScope(input.Scope)
	if err != nil {
		return mcpgo.NewToolResultError(err.Error()), nil
	}
	item, err := s.svc.GetKnowledgeItem(ctx, scope, input.KBID, input.ItemID)
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
	scope, err := s.defaultScope(input.Scope)
	if err != nil {
		return mcpgo.NewToolResultError(err.Error()), nil
	}
	item, ingestionResult, indexStatus, err := s.svc.Add(ctx, core.AddInput{KBID: input.KBID, Scope: scope, Title: input.Title, Content: input.Content, Tags: input.Tags, Metadata: input.Metadata})
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
	scope := strings.TrimSpace(input.Scope)
	kbID := strings.TrimSpace(input.KBID)
	if scope == "" && kbID != "" {
		// Single-KB index needs a concrete scope; fall through to default.
		var err error
		scope, err = s.defaultScope("")
		if err != nil {
			return mcpgo.NewToolResultError(err.Error()), nil
		}
	} else if scope != "" {
		normalized, err := core.NormalizeScope(scope)
		if err != nil {
			return mcpgo.NewToolResultError(err.Error()), nil
		}
		scope = normalized
	}
	result, err := s.svc.IndexKnowledge(ctx, service.IndexKnowledgeInput{Scope: scope, KBID: kbID, Rebuild: input.Rebuild})
	if err != nil {
		return mcpgo.NewToolResultError(err.Error()), nil
	}
	return mcpgo.NewToolResultStructuredOnly(result), nil
}

// defaultScope normalises the scope from a tool input. An empty string
// resolves to project when the service is in project mode, else global.
func (s *Server) defaultScope(raw string) (string, error) {
	if strings.TrimSpace(raw) == "" {
		if s.svc != nil && s.svc.HasProjectScope() {
			return core.ScopeProject, nil
		}
		return core.ScopeGlobal, nil
	}
	return core.NormalizeScope(raw)
}

// parseSearchKBIDs accepts each kb_ids element as either:
//   - a JSON object {"scope":"project","id":"notes"}
//   - a JSON string "scope:id" (e.g. "project:notes") — scope optional
//
// Empty/whitespace strings are skipped. Bare strings without ":" use defaultScope.
func parseSearchKBIDs(raws []json.RawMessage, defaultScope string) ([]core.ScopedKBRef, error) {
	out := make([]core.ScopedKBRef, 0, len(raws))
	for _, raw := range raws {
		var asString string
		if err := json.Unmarshal(raw, &asString); err == nil {
			id := strings.TrimSpace(asString)
			if id == "" {
				continue
			}
			if strings.Contains(id, ":") {
				parts := strings.SplitN(id, ":", 2)
				scope, err := core.NormalizeScope(parts[0])
				if err != nil {
					return nil, fmt.Errorf("kb_ids %q: %w", id, err)
				}
				idPart := strings.TrimSpace(parts[1])
				if idPart == "" {
					return nil, fmt.Errorf("kb_ids %q: id is empty", id)
				}
				out = append(out, core.ScopedKBRef{Scope: scope, ID: idPart})
				continue
			}
			out = append(out, core.ScopedKBRef{Scope: defaultScope, ID: id})
			continue
		}
		var asObj struct {
			Scope string `json:"scope"`
			ID    string `json:"id"`
		}
		if err := json.Unmarshal(raw, &asObj); err != nil {
			return nil, fmt.Errorf("kb_ids element must be string or object: %w", err)
		}
		id := strings.TrimSpace(asObj.ID)
		if id == "" {
			continue
		}
		scope := strings.TrimSpace(asObj.Scope)
		if scope == "" {
			scope = defaultScope
		} else {
			normalized, err := core.NormalizeScope(scope)
			if err != nil {
				return nil, fmt.Errorf("kb_ids[%s]: %w", id, err)
			}
			scope = normalized
		}
		out = append(out, core.ScopedKBRef{Scope: scope, ID: id})
	}
	return out, nil
}
