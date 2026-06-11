package mcp_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	mcpadapter "github.com/kindbrave/knowledger/internal/adapters/mcp"
	"github.com/kindbrave/knowledger/internal/backends/text"
	"github.com/kindbrave/knowledger/internal/core"
	"github.com/kindbrave/knowledger/internal/service"
	mcpclient "github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
)

func TestNewServerRegistersKnowledgeToolsInOrder(t *testing.T) {
	server := mcpadapter.NewServer(nil)
	tools := server.Tools()
	want := []string{"search_knowledge", "get_knowledge_item", "add_knowledge_item", "list_knowledge_bases"}
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
	if text := firstTextContent(t, listResult.Content); !strings.Contains(text, "docs") {
		t.Fatalf("expected list text to contain docs, got %q", text)
	}
	structured, ok := listResult.StructuredContent.(map[string]any)
	if !ok {
		t.Fatalf("expected list structured content to be object, got %T", listResult.StructuredContent)
	}
	if _, ok := structured["knowledge_bases"]; !ok {
		t.Fatalf("expected list structured content to include knowledge_bases")
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
