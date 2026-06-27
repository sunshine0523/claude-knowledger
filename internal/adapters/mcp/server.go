package mcp

import (
	"context"
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

// serverInstructions is injected into the MCP `initialize` response and
// surfaced by hosts (e.g. Claude Code) inside the system prompt. This
// is the strongest priming channel — stronger than per-tool descriptions,
// because the model reads it every turn without having to scan tool
// metadata. Keep it sharp, behavioral, and bilingual where the user base
// asks questions in mixed languages.
const serverInstructions = `# Knowledger — local knowledge recall, runs BEFORE answering

Knowledger is the project's persistent knowledge base — decisions,
conventions, library/tool usage notes, debugging recipes, domain
references. It captures knowledge ABOUT the code that grep, file
reads, and codegraph cannot find.

## Recall — call BEFORE answering

Call list_knowledge_bases BEFORE answering ANY of these question shapes,
even when the user does not say "knowledge / 知识库 / 记得":

- "How do I use X" / "X 怎么用"
- "What is X"     / "X 是什么"
- "How does X work" / "X 怎么实现"
- "Why did we do X this way"
- "What's our convention for X"
- "Where do we store/track X"
- Any debugging question that could have a saved recipe.

One cheap call. Scan the KB and item titles — if any look relevant,
call get_knowledge_item for the full content.

## Capture — only on explicit user intent

add_knowledge_item when the user says save / capture / remember /
记一下 / 保存到 / 添加到 — and the target KB is unambiguous.
Otherwise list_knowledge_bases and ask which KB to use.

## Skip

Conversational turns, ephemeral state, secrets, or anything fully
derivable from the current diff/file.
`

type Server struct {
	svc    *service.Service
	server *mcpserver.MCPServer
	tools  []mcpgo.Tool
}

type ToolForTest = mcpgo.Tool

type getKnowledgeItemInput struct {
	Scope  string `json:"scope,omitempty"`
	KBID   string `json:"kb_id"`
	ItemID string `json:"item_id"`
}

type listKnowledgeItemsInput struct {
	Scope  string `json:"scope,omitempty"`
	KBID   string `json:"kb_id"`
	Limit  int    `json:"limit,omitempty"`
	Offset int    `json:"offset,omitempty"`
}

// knowledgeItemSummary is the lean view returned by list_knowledge_items —
// no Content/Metadata, so a large KB can be browsed cheaply as a directory.
type knowledgeItemSummary struct {
	ID        string   `json:"id"`
	KBID      string   `json:"kb_id"`
	Scope     string   `json:"scope"`
	Type      string   `json:"type,omitempty"`
	Title     string   `json:"title"`
	Summary   string   `json:"summary,omitempty"`
	Tags      []string `json:"tags,omitempty"`
	UpdatedAt string   `json:"updated_at,omitempty"`
}

type listKnowledgeItemsResult struct {
	Items  []knowledgeItemSummary `json:"items"`
	Total  int                    `json:"total"`
	Offset int                    `json:"offset"`
	Limit  int                    `json:"limit"`
}

type addKnowledgeItemInput struct {
	Scope    string         `json:"scope,omitempty"`
	KBID     string         `json:"kb_id"`
	Title    string         `json:"title"`
	Content  string         `json:"content"`
	Tags     []string       `json:"tags,omitempty"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

type deleteKnowledgeItemInput struct {
	Scope  string `json:"scope,omitempty"`
	KBID   string `json:"kb_id"`
	ItemID string `json:"item_id"`
}

type createKnowledgeBaseInput struct {
	Scope           string   `json:"scope,omitempty"`
	ID              string   `json:"id"`
	Name            string   `json:"name,omitempty"`
	StoreType       string   `json:"store_type"`
	Path            string   `json:"path,omitempty"`
	Enabled         *bool    `json:"enabled,omitempty"`
	SemanticEnabled *bool    `json:"semantic_enabled,omitempty"`
	Tags            []string `json:"tags,omitempty"`
}

type deleteKnowledgeBaseInput struct {
	Scope string `json:"scope,omitempty"`
	ID    string `json:"id"`
}

type indexKnowledgeInput struct {
	Scope   string `json:"scope,omitempty"`
	KBID    string `json:"kb_id,omitempty"`
	Rebuild bool   `json:"rebuild,omitempty"`
}

func NewServer(svc *service.Service) *Server {
	adapter := &Server{svc: svc, server: mcpserver.NewMCPServer(
		serverName,
		serverVersion,
		mcpserver.WithInstructions(serverInstructions),
	)}
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
	scopeProperty := mcpgo.WithString(
		"scope",
		mcpgo.Description("Knowledge base scope. Defaults to project when running in a project directory, otherwise global."),
		mcpgo.Enum("project", "global"),
	)
	getTool := mcpgo.NewTool(
		"get_knowledge_item",
		mcpgo.WithDescription("Fetch the full content and metadata of a single knowledge item by KB id + item id. Use after list_knowledge_items or list_knowledge_bases surfaces a promising hit and you need the complete text — to answer a user's question accurately, cite in a technical design/spec, or apply to code. Cheap and read-only; prefer fetching full content over deciding from a title alone."),
		scopeProperty,
		mcpgo.WithString("kb_id", mcpgo.Required(), mcpgo.Description("Knowledge base ID.")),
		mcpgo.WithString("item_id", mcpgo.Required(), mcpgo.Description("Knowledge item ID.")),
		mcpgo.WithReadOnlyHintAnnotation(true),
		mcpgo.WithDestructiveHintAnnotation(false),
		mcpgo.WithIdempotentHintAnnotation(true),
		mcpgo.WithOpenWorldHintAnnotation(false),
	)
	listItemsTool := mcpgo.NewTool(
		"list_knowledge_items",
		mcpgo.WithDescription("Browse a knowledge base as a lightweight directory (id/title/tags, no content). Use when: (1) the user asks 'what is in KB X', (2) you need to scan a KB exhaustively before answering a question, drafting a technical plan, or writing code. After spotting a promising id, call get_knowledge_item for the full content."),
		scopeProperty,
		mcpgo.WithString("kb_id", mcpgo.Required(), mcpgo.Description("Knowledge base ID.")),
		mcpgo.WithNumber("limit", mcpgo.Description("Maximum number of items to return. 0 means all.")),
		mcpgo.WithNumber("offset", mcpgo.Description("Number of items to skip from the start.")),
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
	deleteTool := mcpgo.NewTool(
		"delete_knowledge_item",
		mcpgo.WithDescription("Delete a knowledge item from a knowledge base."),
		scopeProperty,
		mcpgo.WithString("kb_id", mcpgo.Required(), mcpgo.Description("Knowledge base ID.")),
		mcpgo.WithString("item_id", mcpgo.Required(), mcpgo.Description("Knowledge item ID.")),
		mcpgo.WithReadOnlyHintAnnotation(false),
		mcpgo.WithDestructiveHintAnnotation(true),
		mcpgo.WithIdempotentHintAnnotation(true),
		mcpgo.WithOpenWorldHintAnnotation(false),
	)
	listTool := mcpgo.NewTool(
		"list_knowledge_bases",
		mcpgo.WithDescription("List all configured knowledge bases AND every item id/title/tags. CALL EARLY at the start of any non-trivial task — title/tag scans surface entries that might otherwise be missed. Cheap, read-only; one upfront call beats guessing kb_ids. Use get_knowledge_item for full content."),
		mcpgo.WithReadOnlyHintAnnotation(true),
		mcpgo.WithDestructiveHintAnnotation(false),
		mcpgo.WithIdempotentHintAnnotation(true),
		mcpgo.WithOpenWorldHintAnnotation(false),
	)
	createKBTool := mcpgo.NewTool(
		"create_knowledge_base",
		mcpgo.WithDescription("Create a new knowledge base. Path is required for global scope; for project scope a relative path is resolved against the project root."),
		scopeProperty,
		mcpgo.WithString("id", mcpgo.Required(), mcpgo.Description("Knowledge base ID (letters, digits, underscore, dash, dot; max 64 chars).")),
		mcpgo.WithString("name", mcpgo.Description("Human-readable name. Defaults to id.")),
		mcpgo.WithString("store_type", mcpgo.Required(), mcpgo.Description("Backend store type."), mcpgo.Enum("text", "sqlite")),
		mcpgo.WithString("path", mcpgo.Description("Storage path. Required for global scope; relative paths for project scope are resolved against the project root.")),
		mcpgo.WithBoolean("enabled", mcpgo.Description("Whether the knowledge base is enabled. Defaults to true.")),
		mcpgo.WithBoolean("semantic_enabled", mcpgo.Description("Enable semantic indexing for sqlite store types.")),
		mcpgo.WithArray("tags", mcpgo.Description("Optional knowledge base tags."), mcpgo.WithStringItems()),
		mcpgo.WithReadOnlyHintAnnotation(false),
		mcpgo.WithDestructiveHintAnnotation(false),
		mcpgo.WithIdempotentHintAnnotation(false),
		mcpgo.WithOpenWorldHintAnnotation(false),
	)
	deleteKBTool := mcpgo.NewTool(
		"delete_knowledge_base",
		mcpgo.WithDescription("Delete a runtime-managed knowledge base. Static knowledge bases declared in config files cannot be deleted."),
		scopeProperty,
		mcpgo.WithString("id", mcpgo.Required(), mcpgo.Description("Knowledge base ID.")),
		mcpgo.WithReadOnlyHintAnnotation(false),
		mcpgo.WithDestructiveHintAnnotation(true),
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

	s.tools = []mcpgo.Tool{getTool, listItemsTool, addTool, deleteTool, listTool, createKBTool, deleteKBTool, indexTool}
	s.server.AddTool(getTool, s.handleGetKnowledgeItem)
	s.server.AddTool(listItemsTool, s.handleListKnowledgeItems)
	s.server.AddTool(addTool, s.handleAddKnowledgeItem)
	s.server.AddTool(deleteTool, s.handleDeleteKnowledgeItem)
	s.server.AddTool(listTool, s.handleListKnowledgeBases)
	s.server.AddTool(createKBTool, s.handleCreateKnowledgeBase)
	s.server.AddTool(deleteKBTool, s.handleDeleteKnowledgeBase)
	s.server.AddTool(indexTool, s.handleIndexKnowledge)
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

func (s *Server) handleListKnowledgeItems(ctx context.Context, request mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	if s.svc == nil {
		return mcpgo.NewToolResultError("service is not configured"), nil
	}
	var input listKnowledgeItemsInput
	if err := request.BindArguments(&input); err != nil {
		return mcpgo.NewToolResultErrorFromErr("invalid arguments", err), nil
	}
	scope, err := s.defaultScope(input.Scope)
	if err != nil {
		return mcpgo.NewToolResultError(err.Error()), nil
	}
	items, err := s.svc.ListKnowledgeItems(ctx, scope, input.KBID)
	if err != nil {
		return mcpgo.NewToolResultError(err.Error()), nil
	}
	total := len(items)
	offset := input.Offset
	if offset < 0 {
		offset = 0
	}
	if offset > total {
		offset = total
	}
	end := total
	if input.Limit > 0 && offset+input.Limit < end {
		end = offset + input.Limit
	}
	page := items[offset:end]
	summaries := make([]knowledgeItemSummary, 0, len(page))
	for _, item := range page {
		updated := ""
		if !item.UpdatedAt.IsZero() {
			updated = item.UpdatedAt.UTC().Format("2006-01-02T15:04:05Z")
		}
		summaries = append(summaries, knowledgeItemSummary{
			ID:        item.ID,
			KBID:      item.KBID,
			Scope:     scope,
			Type:      item.Type,
			Title:     item.Title,
			Summary:   item.Summary,
			Tags:      item.Tags,
			UpdatedAt: updated,
		})
	}
	return mcpgo.NewToolResultStructuredOnly(listKnowledgeItemsResult{
		Items:  summaries,
		Total:  total,
		Offset: offset,
		Limit:  input.Limit,
	}), nil
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

func (s *Server) handleDeleteKnowledgeItem(ctx context.Context, request mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	if s.svc == nil {
		return mcpgo.NewToolResultError("service is not configured"), nil
	}
	var input deleteKnowledgeItemInput
	if err := request.BindArguments(&input); err != nil {
		return mcpgo.NewToolResultErrorFromErr("invalid arguments", err), nil
	}
	scope, err := s.defaultScope(input.Scope)
	if err != nil {
		return mcpgo.NewToolResultError(err.Error()), nil
	}
	if err := s.svc.DeleteKnowledgeItem(ctx, scope, input.KBID, input.ItemID); err != nil {
		return mcpgo.NewToolResultError(err.Error()), nil
	}
	return mcpgo.NewToolResultStructuredOnly(map[string]any{
		"deleted": true,
		"scope":   scope,
		"kb_id":   strings.TrimSpace(input.KBID),
		"item_id": strings.TrimSpace(input.ItemID),
	}), nil
}

func (s *Server) handleListKnowledgeBases(ctx context.Context, request mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	_ = request
	if s.svc == nil {
		return mcpgo.NewToolResultError("service is not configured"), nil
	}
	kbs := s.svc.ListKnowledgeBases()
	return mcpgo.NewToolResultText(formatKnowledgeBasesWithItems(ctx, s.svc, kbs)), nil
}

func formatKnowledgeBasesWithItems(ctx context.Context, svc *service.Service, kbs []core.KnowledgeBase) string {
	if len(kbs) == 0 {
		return "no knowledge bases configured"
	}
	var b strings.Builder
	for i, kb := range kbs {
		if i > 0 {
			b.WriteByte('\n')
		}
		writeKnowledgeBaseHeader(&b, kb)
		writeKnowledgeBaseItems(ctx, &b, svc, kb)
	}
	return strings.TrimRight(b.String(), "\n")
}

func writeKnowledgeBaseHeader(b *strings.Builder, kb core.KnowledgeBase) {
	scope := kb.Scope
	if scope == "" {
		scope = core.ScopeGlobal
	}
	fmt.Fprintf(b, "[%s:%s]", scope, kb.ID)
	if kb.Name != "" && kb.Name != kb.ID {
		fmt.Fprintf(b, " %s", kb.Name)
	}
	fmt.Fprintf(b, " (store=%s", kb.StoreType)
	if !kb.Enabled {
		b.WriteString(", disabled")
	}
	if len(kb.Tags) > 0 {
		fmt.Fprintf(b, ", tags=%s", strings.Join(kb.Tags, ","))
	}
	b.WriteString(")\n")
}

func writeKnowledgeBaseItems(ctx context.Context, b *strings.Builder, svc *service.Service, kb core.KnowledgeBase) {
	items, err := svc.ListKnowledgeItems(ctx, kb.Scope, kb.ID)
	if err != nil {
		fmt.Fprintf(b, "  (items unavailable: %s)\n", err.Error())
		return
	}
	if len(items) == 0 {
		b.WriteString("  (empty)\n")
		return
	}
	for _, item := range items {
		fmt.Fprintf(b, "  - %s\t%s", item.ID, item.Title)
		if len(item.Tags) > 0 {
			fmt.Fprintf(b, " [%s]", strings.Join(item.Tags, ","))
		}
		b.WriteByte('\n')
	}
}

func (s *Server) handleCreateKnowledgeBase(ctx context.Context, request mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	if s.svc == nil {
		return mcpgo.NewToolResultError("service is not configured"), nil
	}
	var input createKnowledgeBaseInput
	if err := request.BindArguments(&input); err != nil {
		return mcpgo.NewToolResultErrorFromErr("invalid arguments", err), nil
	}
	scope, err := s.defaultScope(input.Scope)
	if err != nil {
		return mcpgo.NewToolResultError(err.Error()), nil
	}
	record, err := s.svc.CreateKnowledgeBase(ctx, service.CreateKnowledgeBaseInput{
		Scope:           scope,
		ID:              strings.TrimSpace(input.ID),
		Name:            strings.TrimSpace(input.Name),
		StoreType:       strings.TrimSpace(input.StoreType),
		Path:            input.Path,
		Enabled:         input.Enabled,
		SemanticEnabled: input.SemanticEnabled,
		Tags:            input.Tags,
	})
	if err != nil {
		return mcpgo.NewToolResultError(err.Error()), nil
	}
	return mcpgo.NewToolResultStructuredOnly(map[string]any{"knowledge_base": record}), nil
}

func (s *Server) handleDeleteKnowledgeBase(ctx context.Context, request mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	if s.svc == nil {
		return mcpgo.NewToolResultError("service is not configured"), nil
	}
	var input deleteKnowledgeBaseInput
	if err := request.BindArguments(&input); err != nil {
		return mcpgo.NewToolResultErrorFromErr("invalid arguments", err), nil
	}
	scope, err := s.defaultScope(input.Scope)
	if err != nil {
		return mcpgo.NewToolResultError(err.Error()), nil
	}
	id := strings.TrimSpace(input.ID)
	if id == "" {
		return mcpgo.NewToolResultError("knowledge base id is required"), nil
	}
	if err := s.svc.DeleteKnowledgeBase(ctx, scope, id); err != nil {
		return mcpgo.NewToolResultError(err.Error()), nil
	}
	return mcpgo.NewToolResultStructuredOnly(map[string]any{
		"deleted": true,
		"scope":   scope,
		"id":      id,
	}), nil
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
