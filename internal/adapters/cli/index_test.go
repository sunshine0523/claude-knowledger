package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/kindbrave/claude-knowledger/internal/adapters/cli"
	"github.com/kindbrave/claude-knowledger/internal/core"
	"github.com/kindbrave/claude-knowledger/internal/service"
)

type indexCommandBackend struct {
	calls []indexCommandCall
}

type indexCommandCall struct {
	kbID    string
	rebuild bool
}

func (b *indexCommandBackend) Add(context.Context, core.KnowledgeBase, core.AddInput) (core.KnowledgeItem, core.IngestionResult, core.IndexStatus, error) {
	return core.KnowledgeItem{}, core.IngestionResult{}, core.IndexStatus{}, nil
}

func (b *indexCommandBackend) Search(context.Context, core.KnowledgeBase, core.SearchOptions) ([]core.SearchHit, error) {
	return nil, nil
}

func (b *indexCommandBackend) GetItem(context.Context, core.KnowledgeBase, string) (core.KnowledgeItem, error) {
	return core.KnowledgeItem{}, nil
}

func (b *indexCommandBackend) ListItems(context.Context, core.KnowledgeBase) ([]core.KnowledgeItem, error) {
	return nil, nil
}

func (b *indexCommandBackend) DeleteItem(context.Context, core.KnowledgeBase, string) error {
	return nil
}

func (b *indexCommandBackend) SupportsSemantic(core.KnowledgeBase) bool { return true }

func (b *indexCommandBackend) MaintainIndex(_ context.Context, kb core.KnowledgeBase, opt core.IndexOptions) (core.IndexResult, error) {
	b.calls = append(b.calls, indexCommandCall{kbID: kb.ID, rebuild: opt.Rebuild})
	return core.IndexResult{Indexed: 3, Deleted: 3}, nil
}

func TestIndexCommandOutputsIndexResult(t *testing.T) {
	stdout := new(bytes.Buffer)
	backend := &indexCommandBackend{}
	svc := service.New(
		[]core.KnowledgeBase{{ID: "notes", StoreType: "sqlite", Enabled: true}},
		map[string]core.StoreBackend{"sqlite": backend},
	)
	cmd := cli.NewRootCommand(svc)
	cmd.SetOut(stdout)
	cmd.SetErr(new(bytes.Buffer))
	cmd.SetArgs([]string{"index", "--kb", "notes", "--rebuild"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if len(backend.calls) != 1 || backend.calls[0].kbID != "notes" || !backend.calls[0].rebuild {
		t.Fatalf("expected rebuild call for notes, got %#v", backend.calls)
	}
	var result service.IndexKnowledgeResult
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("expected result JSON, got %q: %v", stdout.String(), err)
	}
	if len(result.Results) != 1 || result.Results[0].KBID != "notes" || result.Results[0].Result.Indexed != 3 || result.Results[0].Result.Deleted != 3 {
		t.Fatalf("unexpected index result: %#v", result)
	}
}
