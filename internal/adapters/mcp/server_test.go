package mcp_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	mcpadapter "github.com/kindbrave/knowledger/internal/adapters/mcp"
	"github.com/kindbrave/knowledger/internal/backends/text"
	"github.com/kindbrave/knowledger/internal/core"
	"github.com/kindbrave/knowledger/internal/registry"
	"github.com/kindbrave/knowledger/internal/service"
	mcpclient "github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
)

type indexToolBackend struct {
	called  bool
	rebuild bool
}

func (b *indexToolBackend) Add(context.Context, core.KnowledgeBase, core.AddInput) (core.KnowledgeItem, core.IngestionResult, core.IndexStatus, error) {
	return core.KnowledgeItem{}, core.IngestionResult{}, core.IndexStatus{}, nil
}

func (b *indexToolBackend) Search(context.Context, core.KnowledgeBase, core.SearchOptions) ([]core.SearchHit, error) {
	return nil, nil
}

func (b *indexToolBackend) GetItem(context.Context, core.KnowledgeBase, string) (core.KnowledgeItem, error) {
	return core.KnowledgeItem{}, nil
}

func (b *indexToolBackend) ListItems(context.Context, core.KnowledgeBase) ([]core.KnowledgeItem, error) {
	return nil, nil
}

func (b *indexToolBackend) DeleteItem(context.Context, core.KnowledgeBase, string) error {
	return nil
}

func (b *indexToolBackend) SupportsSemantic(core.KnowledgeBase) bool { return true }

func (b *indexToolBackend) MaintainIndex(_ context.Context, _ core.KnowledgeBase, opt core.IndexOptions) (core.IndexResult, error) {
	b.called = true
	b.rebuild = opt.Rebuild
	return core.IndexResult{Indexed: 1, Deleted: 1}, nil
}

func TestNewServerRegistersKnowledgeToolsInOrder(t *testing.T) {
	server := mcpadapter.NewServer(nil)
	tools := server.Tools()
	want := []string{"search_knowledge", "get_knowledge_item", "list_knowledge_items", "add_knowledge_item", "delete_knowledge_item", "list_knowledge_bases", "create_knowledge_base", "delete_knowledge_base", "index_knowledge"}
	if len(tools) != len(want) {
		t.Fatalf("expected %d tools, got %d", len(want), len(tools))
	}
	for i, name := range want {
		if tools[i].Name != name {
			t.Fatalf("tool %d: expected %q, got %q", i, name, tools[i].Name)
		}
	}
}

func TestSearchKnowledgeSchema(t *testing.T) {
	tool := findTool(t, mcpadapter.NewServer(nil).Tools(), "search_knowledge")
	if !hasRequired(tool, "query") {
		t.Fatalf("expected search_knowledge to require query")
	}
	for _, prop := range []string{"kb_ids", "limit", "search_mode"} {
		if _, ok := tool.InputSchema.Properties[prop]; !ok {
			t.Fatalf("expected search_knowledge schema to have %q property", prop)
		}
	}
}

func TestGetKnowledgeItemSchema(t *testing.T) {
	tool := findTool(t, mcpadapter.NewServer(nil).Tools(), "get_knowledge_item")
	for _, required := range []string{"kb_id", "item_id"} {
		if !hasRequired(tool, required) {
			t.Fatalf("expected get_knowledge_item to require %q", required)
		}
	}
}

func TestListKnowledgeItemsSchema(t *testing.T) {
	tool := findTool(t, mcpadapter.NewServer(nil).Tools(), "list_knowledge_items")
	if !hasRequired(tool, "kb_id") {
		t.Fatalf("expected list_knowledge_items to require kb_id")
	}
	for _, prop := range []string{"scope", "limit", "offset"} {
		if _, ok := tool.InputSchema.Properties[prop]; !ok {
			t.Fatalf("expected list_knowledge_items schema to expose %q property", prop)
		}
	}
	for _, required := range tool.InputSchema.Required {
		if required == "scope" || required == "limit" || required == "offset" {
			t.Fatalf("%q must be optional on list_knowledge_items", required)
		}
	}
}

func TestAddKnowledgeItemSchema(t *testing.T) {
	tool := findTool(t, mcpadapter.NewServer(nil).Tools(), "add_knowledge_item")
	for _, required := range []string{"kb_id", "title", "content"} {
		if !hasRequired(tool, required) {
			t.Fatalf("expected add_knowledge_item to require %q", required)
		}
	}
	for _, prop := range []string{"tags", "metadata"} {
		if _, ok := tool.InputSchema.Properties[prop]; !ok {
			t.Fatalf("expected add_knowledge_item schema to have %q property", prop)
		}
	}
}

func TestDeleteKnowledgeItemSchema(t *testing.T) {
	tool := findTool(t, mcpadapter.NewServer(nil).Tools(), "delete_knowledge_item")
	for _, required := range []string{"kb_id", "item_id"} {
		if !hasRequired(tool, required) {
			t.Fatalf("expected delete_knowledge_item to require %q", required)
		}
	}
	if _, ok := tool.InputSchema.Properties["scope"]; !ok {
		t.Fatalf("expected delete_knowledge_item to expose scope property")
	}
	for _, required := range tool.InputSchema.Required {
		if required == "scope" {
			t.Fatalf("scope must be optional on delete_knowledge_item")
		}
	}
}

func TestCreateKnowledgeBaseSchema(t *testing.T) {
	tool := findTool(t, mcpadapter.NewServer(nil).Tools(), "create_knowledge_base")
	for _, required := range []string{"id", "store_type"} {
		if !hasRequired(tool, required) {
			t.Fatalf("expected create_knowledge_base to require %q", required)
		}
	}
	for _, prop := range []string{"scope", "name", "path", "enabled", "semantic_enabled", "tags"} {
		if _, ok := tool.InputSchema.Properties[prop]; !ok {
			t.Fatalf("expected create_knowledge_base schema to have %q property", prop)
		}
	}
	for _, required := range tool.InputSchema.Required {
		if required == "scope" {
			t.Fatalf("scope must be optional on create_knowledge_base")
		}
	}
}

func TestDeleteKnowledgeBaseSchema(t *testing.T) {
	tool := findTool(t, mcpadapter.NewServer(nil).Tools(), "delete_knowledge_base")
	if !hasRequired(tool, "id") {
		t.Fatalf("expected delete_knowledge_base to require id")
	}
	if _, ok := tool.InputSchema.Properties["scope"]; !ok {
		t.Fatalf("expected delete_knowledge_base to expose scope property")
	}
	for _, required := range tool.InputSchema.Required {
		if required == "scope" {
			t.Fatalf("scope must be optional on delete_knowledge_base")
		}
	}
}

func TestIndexKnowledgeSchema(t *testing.T) {
	tool := findTool(t, mcpadapter.NewServer(nil).Tools(), "index_knowledge")
	for _, prop := range []string{"kb_id", "rebuild"} {
		if _, ok := tool.InputSchema.Properties[prop]; !ok {
			t.Fatalf("expected index_knowledge schema to have %q property", prop)
		}
	}
}

func TestMCPHandlersRoundTripThroughService(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	ctx := context.Background()
	dir := t.TempDir()
	svc := service.New([]core.KnowledgeBase{{ID: "docs", Name: "Docs", StoreType: "text", StoreConfig: map[string]any{"path": dir}, Enabled: true, DefaultSearchMode: "lexical"}}, map[string]core.StoreBackend{"text": text.New()})
	adapter := mcpadapter.NewServer(svc)

	client, err := mcpclient.NewInProcessClient(adapter.MCPServer())
	if err != nil {
		t.Fatalf("new in-process client: %v", err)
	}
	defer client.Close()
	if err := client.Start(ctx); err != nil {
		t.Fatalf("start client: %v", err)
	}
	initRequest := mcp.InitializeRequest{}
	initRequest.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	initRequest.Params.ClientInfo = mcp.Implementation{Name: "knowledger-test", Version: "0.1.0"}
	initResult, err := client.Initialize(ctx, initRequest)
	if err != nil {
		t.Fatalf("initialize client: %v", err)
	}
	if initResult.ServerInfo.Name != "knowledger" {
		t.Fatalf("expected server name knowledger, got %q", initResult.ServerInfo.Name)
	}

	addRequest := mcp.CallToolRequest{}
	addRequest.Params.Name = "add_knowledge_item"
	addRequest.Params.Arguments = map[string]any{
		"kb_id":   "docs",
		"title":   "MCP Notes",
		"content": "Knowledger speaks MCP over stdio.",
		"tags":    []string{"mcp"},
	}
	addResult, err := client.CallTool(ctx, addRequest)
	if err != nil {
		t.Fatalf("call add_knowledge_item: %v", err)
	}
	if addResult.IsError {
		t.Fatalf("expected add_knowledge_item success, got %q", firstTextContent(t, addResult.Content))
	}
	itemID := extractItemID(t, addResult.Content)

	searchRequest := mcp.CallToolRequest{}
	searchRequest.Params.Name = "search_knowledge"
	searchRequest.Params.Arguments = map[string]any{
		"query":       "stdio",
		"kb_ids":      []string{"docs"},
		"limit":       5,
		"search_mode": "lexical",
	}
	searchResult, err := client.CallTool(ctx, searchRequest)
	if err != nil {
		t.Fatalf("call search_knowledge: %v", err)
	}
	if searchResult.IsError {
		t.Fatalf("expected search_knowledge success, got %q", firstTextContent(t, searchResult.Content))
	}
	searchText := firstTextContent(t, searchResult.Content)
	if !strings.Contains(searchText, "MCP Notes") {
		t.Fatalf("expected search text to contain MCP Notes, got %q", searchText)
	}
	if !strings.Contains(searchText, "Snippet") {
		t.Fatalf("expected search text to include Snippet, got %q", searchText)
	}
	if strings.Contains(searchText, "ContentPreview") {
		t.Fatalf("expected search text to omit ContentPreview, got %q", searchText)
	}

	getRequest := mcp.CallToolRequest{}
	getRequest.Params.Name = "get_knowledge_item"
	getRequest.Params.Arguments = map[string]any{"kb_id": "docs", "item_id": itemID}
	getResult, err := client.CallTool(ctx, getRequest)
	if err != nil {
		t.Fatalf("call get_knowledge_item: %v", err)
	}
	if getResult.IsError {
		t.Fatalf("expected get_knowledge_item success, got %q", firstTextContent(t, getResult.Content))
	}
	if text := firstTextContent(t, getResult.Content); !strings.Contains(text, "Knowledger speaks MCP") {
		t.Fatalf("expected item text to contain content, got %q", text)
	}

	listRequest := mcp.CallToolRequest{}
	listRequest.Params.Name = "list_knowledge_bases"
	listResult, err := client.CallTool(ctx, listRequest)
	if err != nil {
		t.Fatalf("call list_knowledge_bases: %v", err)
	}
	if listResult.IsError {
		t.Fatalf("expected list_knowledge_bases success, got %q", firstTextContent(t, listResult.Content))
	}
	listText := firstTextContent(t, listResult.Content)
	if !strings.Contains(listText, "[global:docs]") {
		t.Fatalf("expected list text to contain KB header [global:docs], got %q", listText)
	}
	if !strings.Contains(listText, "  - "+itemID) {
		t.Fatalf("expected list text to include item id %q under the KB, got %q", itemID, listText)
	}

	deleteRequest := mcp.CallToolRequest{}
	deleteRequest.Params.Name = "delete_knowledge_item"
	deleteRequest.Params.Arguments = map[string]any{"kb_id": "docs", "item_id": itemID}
	deleteResult, err := client.CallTool(ctx, deleteRequest)
	if err != nil {
		t.Fatalf("call delete_knowledge_item: %v", err)
	}
	if deleteResult.IsError {
		t.Fatalf("expected delete_knowledge_item success, got %q", firstTextContent(t, deleteResult.Content))
	}
	deletePayload, ok := deleteResult.StructuredContent.(map[string]any)
	if !ok {
		t.Fatalf("expected delete structured content to be object, got %T", deleteResult.StructuredContent)
	}
	if deleted, _ := deletePayload["deleted"].(bool); !deleted {
		t.Fatalf("expected delete payload deleted=true, got %v", deletePayload)
	}
	if deletePayload["item_id"] != itemID {
		t.Fatalf("expected delete payload item_id=%q, got %v", itemID, deletePayload["item_id"])
	}

	getAfterDelete := mcp.CallToolRequest{}
	getAfterDelete.Params.Name = "get_knowledge_item"
	getAfterDelete.Params.Arguments = map[string]any{"kb_id": "docs", "item_id": itemID}
	missing, err := client.CallTool(ctx, getAfterDelete)
	if err != nil {
		t.Fatalf("call get_knowledge_item after delete: %v", err)
	}
	if !missing.IsError {
		t.Fatalf("expected get_knowledge_item to error after delete, got %q", firstTextContent(t, missing.Content))
	}
}

func TestMCPIndexKnowledgeHandler(t *testing.T) {
	ctx := context.Background()
	backend := &indexToolBackend{}
	svc := service.New([]core.KnowledgeBase{{ID: "notes", StoreType: "sqlite", Enabled: true}}, map[string]core.StoreBackend{"sqlite": backend})
	adapter := mcpadapter.NewServer(svc)

	client, err := mcpclient.NewInProcessClient(adapter.MCPServer())
	if err != nil {
		t.Fatalf("new in-process client: %v", err)
	}
	defer client.Close()
	if err := client.Start(ctx); err != nil {
		t.Fatalf("start client: %v", err)
	}
	initRequest := mcp.InitializeRequest{}
	initRequest.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	initRequest.Params.ClientInfo = mcp.Implementation{Name: "knowledger-test", Version: "0.1.0"}
	if _, err := client.Initialize(ctx, initRequest); err != nil {
		t.Fatalf("initialize client: %v", err)
	}

	request := mcp.CallToolRequest{}
	request.Params.Name = "index_knowledge"
	request.Params.Arguments = map[string]any{"kb_id": "notes", "rebuild": true}
	result, err := client.CallTool(ctx, request)
	if err != nil {
		t.Fatalf("call index_knowledge: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected index_knowledge success, got %q", firstTextContent(t, result.Content))
	}
	if !backend.called || !backend.rebuild {
		t.Fatalf("expected backend rebuild call, got called=%v rebuild=%v", backend.called, backend.rebuild)
	}
	if text := firstTextContent(t, result.Content); !strings.Contains(text, "notes") || !strings.Contains(text, "indexed") {
		t.Fatalf("expected index result text to include notes and indexed, got %q", text)
	}
}

func TestMCPHandlerReturnsToolErrorForServiceFailure(t *testing.T) {
	ctx := context.Background()
	svc := service.New(nil, map[string]core.StoreBackend{"text": text.New()})
	adapter := mcpadapter.NewServer(svc)

	client, err := mcpclient.NewInProcessClient(adapter.MCPServer())
	if err != nil {
		t.Fatalf("new in-process client: %v", err)
	}
	defer client.Close()
	if err := client.Start(ctx); err != nil {
		t.Fatalf("start client: %v", err)
	}
	initRequest := mcp.InitializeRequest{}
	initRequest.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	initRequest.Params.ClientInfo = mcp.Implementation{Name: "knowledger-test", Version: "0.1.0"}
	if _, err := client.Initialize(ctx, initRequest); err != nil {
		t.Fatalf("initialize client: %v", err)
	}

	request := mcp.CallToolRequest{}
	request.Params.Name = "get_knowledge_item"
	request.Params.Arguments = map[string]any{"kb_id": "missing", "item_id": "1"}
	result, err := client.CallTool(ctx, request)
	if err != nil {
		t.Fatalf("expected tool error result, got protocol error: %v", err)
	}
	if !result.IsError {
		t.Fatalf("expected tool error result")
	}
	if text := firstTextContent(t, result.Content); !strings.Contains(text, "knowledge base not found") {
		t.Fatalf("expected knowledge base not found error, got %q", text)
	}
}

func findTool(t *testing.T, tools []mcpadapter.ToolForTest, name string) mcpadapter.ToolForTest {
	t.Helper()
	for _, tool := range tools {
		if tool.Name == name {
			return tool
		}
	}
	t.Fatalf("tool %q not found", name)
	return mcpadapter.ToolForTest{}
}

func hasRequired(tool mcpadapter.ToolForTest, name string) bool {
	for _, required := range tool.InputSchema.Required {
		if required == name {
			return true
		}
	}
	return false
}

func firstTextContent(t *testing.T, content []mcp.Content) string {
	t.Helper()
	if len(content) == 0 {
		t.Fatalf("expected at least one content item")
	}
	text, ok := content[0].(mcp.TextContent)
	if !ok {
		t.Fatalf("expected first content to be text, got %T", content[0])
	}
	return text.Text
}

func extractItemID(t *testing.T, content []mcp.Content) string {
	t.Helper()
	var payload struct {
		Item struct {
			ID string `json:"ID"`
			Id string `json:"id"`
		} `json:"item"`
	}
	if err := json.Unmarshal([]byte(firstTextContent(t, content)), &payload); err != nil {
		t.Fatalf("unmarshal add result: %v", err)
	}
	if payload.Item.ID != "" {
		return payload.Item.ID
	}
	if payload.Item.Id != "" {
		return payload.Item.Id
	}
	t.Fatalf("add result did not include item ID")
	return ""
}

// --- scope-aware coverage ---

func TestSearchKnowledgeSchemaIncludesScopeAndKBIDsItems(t *testing.T) {
	tool := findTool(t, mcpadapter.NewServer(nil).Tools(), "search_knowledge")
	if _, ok := tool.InputSchema.Properties["scope"]; !ok {
		t.Fatalf("expected search_knowledge schema to expose scope")
	}
	kbIDs, ok := tool.InputSchema.Properties["kb_ids"].(map[string]any)
	if !ok {
		t.Fatalf("expected kb_ids property to be an object schema, got %T", tool.InputSchema.Properties["kb_ids"])
	}
	items, ok := kbIDs["items"]
	if !ok {
		t.Fatalf("expected kb_ids schema to declare items")
	}
	itemsBytes, err := json.Marshal(items)
	if err != nil {
		t.Fatalf("marshal items schema: %v", err)
	}
	if !strings.Contains(string(itemsBytes), "oneOf") {
		t.Fatalf("expected kb_ids items to declare oneOf, got %s", itemsBytes)
	}
}

func TestGetAddIndexSchemasIncludeOptionalScope(t *testing.T) {
	tools := mcpadapter.NewServer(nil).Tools()
	for _, name := range []string{"get_knowledge_item", "add_knowledge_item", "delete_knowledge_item", "index_knowledge"} {
		tool := findTool(t, tools, name)
		if _, ok := tool.InputSchema.Properties["scope"]; !ok {
			t.Fatalf("expected %s to expose scope property", name)
		}
		for _, required := range tool.InputSchema.Required {
			if required == "scope" {
				t.Fatalf("scope must be optional on %s", name)
			}
		}
	}
}

func TestMCPProjectScopeDefaultsThroughService(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	ctx := context.Background()
	projectRoot := t.TempDir()
	projectDataDir := filepath.Join(projectRoot, "data-project")
	globalDataDir := t.TempDir()
	if err := os.MkdirAll(projectDataDir, 0o755); err != nil {
		t.Fatalf("mkdir project data: %v", err)
	}

	projectStore := registry.NewMemoryStore([]registry.RuntimeKnowledgeBase{{
		ID: "notes", Name: "Project Notes", StoreType: "text",
		StoreConfig: map[string]any{"path": projectDataDir}, Enabled: true,
	}})
	globalStore := registry.NewMemoryStore([]registry.RuntimeKnowledgeBase{{
		ID: "notes", Name: "Global Notes", StoreType: "text",
		StoreConfig: map[string]any{"path": globalDataDir}, Enabled: true,
	}})
	reg := registry.New(nil, globalStore, projectStore, projectRoot)
	build := func(_ []core.KnowledgeBase) (map[string]core.StoreBackend, error) {
		return map[string]core.StoreBackend{"text": text.New()}, nil
	}
	svc, err := service.NewManaged(reg, build)
	if err != nil {
		t.Fatalf("NewManaged: %v", err)
	}
	defer svc.Close()
	if !svc.HasProjectScope() {
		t.Fatalf("expected project scope")
	}

	adapter := mcpadapter.NewServer(svc)
	client, err := mcpclient.NewInProcessClient(adapter.MCPServer())
	if err != nil {
		t.Fatalf("new in-process client: %v", err)
	}
	defer client.Close()
	if err := client.Start(ctx); err != nil {
		t.Fatalf("start client: %v", err)
	}
	initRequest := mcp.InitializeRequest{}
	initRequest.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	initRequest.Params.ClientInfo = mcp.Implementation{Name: "knowledger-test", Version: "0.1.0"}
	if _, err := client.Initialize(ctx, initRequest); err != nil {
		t.Fatalf("initialize: %v", err)
	}

	// add omits scope -> defaults to project (since HasProjectScope=true)
	addRequest := mcp.CallToolRequest{}
	addRequest.Params.Name = "add_knowledge_item"
	addRequest.Params.Arguments = map[string]any{
		"kb_id":   "notes",
		"title":   "Project Doc",
		"content": "Belongs to the project KB.",
	}
	addResult, err := client.CallTool(ctx, addRequest)
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	if addResult.IsError {
		t.Fatalf("add error: %s", firstTextContent(t, addResult.Content))
	}

	// search with bare kb_id "notes" should hit project scope only
	searchRequest := mcp.CallToolRequest{}
	searchRequest.Params.Name = "search_knowledge"
	searchRequest.Params.Arguments = map[string]any{
		"query":       "Belongs",
		"kb_ids":      []any{"notes"},
		"search_mode": "lexical",
	}
	searchResult, err := client.CallTool(ctx, searchRequest)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if searchResult.IsError {
		t.Fatalf("search error: %s", firstTextContent(t, searchResult.Content))
	}
	structured, ok := searchResult.StructuredContent.(map[string]any)
	if !ok {
		t.Fatalf("expected structured search result, got %T", searchResult.StructuredContent)
	}
	hits, ok := structured["Hits"].([]any)
	if !ok {
		t.Fatalf("expected Hits slice, got %T", structured["Hits"])
	}
	if len(hits) != 1 {
		t.Fatalf("expected exactly 1 hit (project scope only), got %d", len(hits))
	}
	hit, _ := hits[0].(map[string]any)
	if hit["Scope"] != core.ScopeProject {
		t.Fatalf("expected hit scope=project, got %v", hit["Scope"])
	}
	if hit["KBID"] != "notes" {
		t.Fatalf("expected hit kb=notes, got %v", hit["KBID"])
	}

	// search with scope:id syntax also works
	searchRequest.Params.Arguments = map[string]any{
		"query":       "Belongs",
		"kb_ids":      []any{"project:notes"},
		"search_mode": "lexical",
	}
	res2, err := client.CallTool(ctx, searchRequest)
	if err != nil || res2.IsError {
		t.Fatalf("search scope:id: err=%v isError=%v body=%s", err, res2 != nil && res2.IsError, firstTextContent(t, res2.Content))
	}

	// search with object kb_ids form
	searchRequest.Params.Arguments = map[string]any{
		"query":       "Belongs",
		"kb_ids":      []any{map[string]any{"scope": "project", "id": "notes"}},
		"search_mode": "lexical",
	}
	res3, err := client.CallTool(ctx, searchRequest)
	if err != nil || res3.IsError {
		t.Fatalf("search object: err=%v isError=%v body=%s", err, res3 != nil && res3.IsError, firstTextContent(t, res3.Content))
	}
}

func TestMCPCreateAndDeleteKnowledgeBaseRoundTrip(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	ctx := context.Background()
	globalDir := t.TempDir()
	storePath := filepath.Join(t.TempDir(), "newkb")
	if err := os.MkdirAll(storePath, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	globalStore := registry.NewMemoryStore(nil)
	reg := registry.New(nil, globalStore, nil, "")
	_ = globalDir
	build := func(_ []core.KnowledgeBase) (map[string]core.StoreBackend, error) {
		return map[string]core.StoreBackend{"text": text.New()}, nil
	}
	svc, err := service.NewManaged(reg, build)
	if err != nil {
		t.Fatalf("NewManaged: %v", err)
	}
	defer svc.Close()

	adapter := mcpadapter.NewServer(svc)
	client, err := mcpclient.NewInProcessClient(adapter.MCPServer())
	if err != nil {
		t.Fatalf("new in-process client: %v", err)
	}
	defer client.Close()
	if err := client.Start(ctx); err != nil {
		t.Fatalf("start client: %v", err)
	}
	initRequest := mcp.InitializeRequest{}
	initRequest.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	initRequest.Params.ClientInfo = mcp.Implementation{Name: "knowledger-test", Version: "0.1.0"}
	if _, err := client.Initialize(ctx, initRequest); err != nil {
		t.Fatalf("initialize: %v", err)
	}

	createRequest := mcp.CallToolRequest{}
	createRequest.Params.Name = "create_knowledge_base"
	createRequest.Params.Arguments = map[string]any{
		"id":         "newkb",
		"name":       "New KB",
		"store_type": "text",
		"path":       storePath,
		"tags":       []string{"alpha"},
	}
	createResult, err := client.CallTool(ctx, createRequest)
	if err != nil {
		t.Fatalf("call create_knowledge_base: %v", err)
	}
	if createResult.IsError {
		t.Fatalf("expected create_knowledge_base success, got %q", firstTextContent(t, createResult.Content))
	}
	createPayload, ok := createResult.StructuredContent.(map[string]any)
	if !ok {
		t.Fatalf("expected structured create result, got %T", createResult.StructuredContent)
	}
	if _, ok := createPayload["knowledge_base"]; !ok {
		t.Fatalf("expected knowledge_base in payload, got %v", createPayload)
	}

	listRequest := mcp.CallToolRequest{}
	listRequest.Params.Name = "list_knowledge_bases"
	listResult, err := client.CallTool(ctx, listRequest)
	if err != nil || listResult.IsError {
		t.Fatalf("list after create failed: err=%v isError=%v body=%s", err, listResult != nil && listResult.IsError, firstTextContent(t, listResult.Content))
	}
	if !strings.Contains(firstTextContent(t, listResult.Content), "newkb") {
		t.Fatalf("expected newkb in list output, got %q", firstTextContent(t, listResult.Content))
	}

	// Duplicate create should error.
	dupResult, err := client.CallTool(ctx, createRequest)
	if err != nil {
		t.Fatalf("call duplicate create_knowledge_base: %v", err)
	}
	if !dupResult.IsError {
		t.Fatalf("expected duplicate create_knowledge_base to error")
	}

	deleteRequest := mcp.CallToolRequest{}
	deleteRequest.Params.Name = "delete_knowledge_base"
	deleteRequest.Params.Arguments = map[string]any{"id": "newkb"}
	deleteResult, err := client.CallTool(ctx, deleteRequest)
	if err != nil {
		t.Fatalf("call delete_knowledge_base: %v", err)
	}
	if deleteResult.IsError {
		t.Fatalf("expected delete_knowledge_base success, got %q", firstTextContent(t, deleteResult.Content))
	}
	deletePayload, ok := deleteResult.StructuredContent.(map[string]any)
	if !ok {
		t.Fatalf("expected structured delete result, got %T", deleteResult.StructuredContent)
	}
	if deleted, _ := deletePayload["deleted"].(bool); !deleted {
		t.Fatalf("expected deleted=true, got %v", deletePayload)
	}
	if deletePayload["id"] != "newkb" {
		t.Fatalf("expected delete id=newkb, got %v", deletePayload["id"])
	}

	// After delete, listing should not contain newkb anymore.
	listAfter, err := client.CallTool(ctx, listRequest)
	if err != nil || listAfter.IsError {
		t.Fatalf("list after delete failed: err=%v isError=%v", err, listAfter != nil && listAfter.IsError)
	}
	if strings.Contains(firstTextContent(t, listAfter.Content), "newkb") {
		t.Fatalf("expected newkb to be absent after delete, got %q", firstTextContent(t, listAfter.Content))
	}
}

func TestMCPCreateKnowledgeBaseRequiresRuntimeRegistry(t *testing.T) {
	ctx := context.Background()
	svc := service.New(nil, map[string]core.StoreBackend{"text": text.New()})
	adapter := mcpadapter.NewServer(svc)
	client, err := mcpclient.NewInProcessClient(adapter.MCPServer())
	if err != nil {
		t.Fatalf("new in-process client: %v", err)
	}
	defer client.Close()
	if err := client.Start(ctx); err != nil {
		t.Fatalf("start client: %v", err)
	}
	initRequest := mcp.InitializeRequest{}
	initRequest.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	initRequest.Params.ClientInfo = mcp.Implementation{Name: "knowledger-test", Version: "0.1.0"}
	if _, err := client.Initialize(ctx, initRequest); err != nil {
		t.Fatalf("initialize: %v", err)
	}

	request := mcp.CallToolRequest{}
	request.Params.Name = "create_knowledge_base"
	request.Params.Arguments = map[string]any{
		"id":         "x",
		"store_type": "text",
		"path":       t.TempDir(),
	}
	result, err := client.CallTool(ctx, request)
	if err != nil {
		t.Fatalf("call create_knowledge_base: %v", err)
	}
	if !result.IsError {
		t.Fatalf("expected error when registry is unavailable")
	}
}

func TestMCPLogModeBannerProjectMode(t *testing.T) {
	projectRoot := t.TempDir()
	reg := registry.New(nil, registry.NewMemoryStore(nil), registry.NewMemoryStore(nil), projectRoot)
	build := func(_ []core.KnowledgeBase) (map[string]core.StoreBackend, error) {
		return map[string]core.StoreBackend{}, nil
	}
	svc, err := service.NewManaged(reg, build)
	if err != nil {
		t.Fatalf("NewManaged: %v", err)
	}
	defer svc.Close()
	out := captureStderr(t, func(w *os.File) {
		mcpadapter.NewServer(svc).LogModeBannerForTest(w)
	})
	want := "knowledger: project mode (root=" + projectRoot + ")"
	if !strings.Contains(out, want) {
		t.Fatalf("expected banner to contain %q, got %q", want, out)
	}
}

func TestMCPLogModeBannerGlobalMode(t *testing.T) {
	reg := registry.New(nil, registry.NewMemoryStore(nil), nil, "")
	build := func(_ []core.KnowledgeBase) (map[string]core.StoreBackend, error) {
		return map[string]core.StoreBackend{}, nil
	}
	svc, err := service.NewManaged(reg, build)
	if err != nil {
		t.Fatalf("NewManaged: %v", err)
	}
	defer svc.Close()
	out := captureStderr(t, func(w *os.File) {
		mcpadapter.NewServer(svc).LogModeBannerForTest(w)
	})
	if !strings.Contains(out, "knowledger: global mode") {
		t.Fatalf("expected global mode banner, got %q", out)
	}
}

func captureStderr(t *testing.T, fn func(w *os.File)) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	fn(w)
	w.Close()
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, r); err != nil {
		t.Fatalf("copy: %v", err)
	}
	r.Close()
	return buf.String()
}
