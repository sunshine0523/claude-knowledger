package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/kindbrave/knowledger/internal/adapters/cli"
	"github.com/kindbrave/knowledger/internal/core"
	"github.com/kindbrave/knowledger/internal/service"
)

type getCommandBackend struct{}

func (getCommandBackend) Add(context.Context, core.KnowledgeBase, core.AddInput) (core.KnowledgeItem, core.IngestionResult, core.IndexStatus, error) {
	return core.KnowledgeItem{}, core.IngestionResult{}, core.IndexStatus{}, nil
}

func (getCommandBackend) Search(context.Context, core.KnowledgeBase, core.SearchOptions) ([]core.SearchHit, error) {
	return nil, nil
}

func (getCommandBackend) GetItem(_ context.Context, kb core.KnowledgeBase, itemID string) (core.KnowledgeItem, error) {
	return core.KnowledgeItem{ID: itemID, KBID: kb.ID, Type: "note", Title: "Stored", Content: "complete content body"}, nil
}

func (getCommandBackend) ListItems(context.Context, core.KnowledgeBase) ([]core.KnowledgeItem, error) {
	return nil, nil
}

func (getCommandBackend) DeleteItem(context.Context, core.KnowledgeBase, string) error {
	return nil
}

func (getCommandBackend) SupportsSemantic(core.KnowledgeBase) bool { return false }

func TestGetCommandOutputsFullKnowledgeItem(t *testing.T) {
	stdout := new(bytes.Buffer)
	svc := service.New(
		[]core.KnowledgeBase{{ID: "notes", StoreType: "sqlite", Enabled: true}},
		map[string]core.StoreBackend{"sqlite": getCommandBackend{}},
	)
	cmd := cli.NewRootCommand(svc)
	cmd.SetOut(stdout)
	cmd.SetErr(new(bytes.Buffer))
	cmd.SetArgs([]string{"get", "--kb", "notes", "--id", "123"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	var item core.KnowledgeItem
	if err := json.Unmarshal(stdout.Bytes(), &item); err != nil {
		t.Fatalf("expected item JSON, got %q: %v", stdout.String(), err)
	}
	if item.ID != "123" || item.KBID != "notes" || item.Content != "complete content body" {
		t.Fatalf("unexpected item: %#v", item)
	}
}

func TestGetCommandRequiresKBAndID(t *testing.T) {
	cmd := cli.NewRootCommand(service.New(nil, nil))
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetErr(new(bytes.Buffer))
	cmd.SetArgs([]string{"get", "--kb", "notes"})

	if err := cmd.Execute(); err == nil {
		t.Fatalf("expected missing id to fail")
	}
}
