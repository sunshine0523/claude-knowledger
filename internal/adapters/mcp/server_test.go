package mcp_test

import (
	"testing"

	mcpadapter "github.com/kindbrave/knowledger/internal/adapters/mcp"
)

func TestServerRegistersHighLevelTools(t *testing.T) {
	server := mcpadapter.NewServer(nil)
	tools := server.Tools()
	if len(tools) != 4 {
		t.Fatalf("expected 4 tools, got %d", len(tools))
	}
}
