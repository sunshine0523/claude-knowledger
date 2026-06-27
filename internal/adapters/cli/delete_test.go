package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/kindbrave/claude-knowledger/internal/adapters/cli"
	"github.com/kindbrave/claude-knowledger/internal/core"
	"github.com/kindbrave/claude-knowledger/internal/service"
)

type deleteCommandBackend struct {
	calls       int32
	lastKBID    string
	lastItemID  string
	returnError error
}

func (b *deleteCommandBackend) Add(context.Context, core.KnowledgeBase, core.AddInput) (core.KnowledgeItem, core.IngestionResult, core.IndexStatus, error) {
	return core.KnowledgeItem{}, core.IngestionResult{}, core.IndexStatus{}, nil
}

func (b *deleteCommandBackend) Search(context.Context, core.KnowledgeBase, core.SearchOptions) ([]core.SearchHit, error) {
	return nil, nil
}

func (b *deleteCommandBackend) GetItem(context.Context, core.KnowledgeBase, string) (core.KnowledgeItem, error) {
	return core.KnowledgeItem{}, nil
}

func (b *deleteCommandBackend) ListItems(context.Context, core.KnowledgeBase) ([]core.KnowledgeItem, error) {
	return nil, nil
}

func (b *deleteCommandBackend) DeleteItem(_ context.Context, kb core.KnowledgeBase, itemID string) error {
	atomic.AddInt32(&b.calls, 1)
	b.lastKBID = kb.ID
	b.lastItemID = itemID
	return b.returnError
}

func (b *deleteCommandBackend) SupportsSemantic(core.KnowledgeBase) bool { return false }

func TestDeleteCommandRemovesItem(t *testing.T) {
	backend := &deleteCommandBackend{}
	svc := service.New(
		[]core.KnowledgeBase{{ID: "notes", Scope: core.ScopeGlobal, StoreType: "sqlite", Enabled: true}},
		map[string]core.StoreBackend{"sqlite": backend},
	)

	stdout := new(bytes.Buffer)
	cmd := cli.NewRootCommand(svc)
	cmd.SetOut(stdout)
	cmd.SetErr(new(bytes.Buffer))
	cmd.SetArgs([]string{"delete", "--kb", "notes", "--id", "abc-123"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	if got := atomic.LoadInt32(&backend.calls); got != 1 {
		t.Fatalf("expected backend DeleteItem to be called once, got %d", got)
	}
	if backend.lastKBID != "notes" || backend.lastItemID != "abc-123" {
		t.Fatalf("unexpected delete args: kb=%q id=%q", backend.lastKBID, backend.lastItemID)
	}

	var payload struct {
		Deleted bool   `json:"deleted"`
		Scope   string `json:"scope"`
		KBID    string `json:"kb_id"`
		ItemID  string `json:"item_id"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal output %q: %v", stdout.String(), err)
	}
	if !payload.Deleted || payload.KBID != "notes" || payload.ItemID != "abc-123" || payload.Scope != core.ScopeGlobal {
		t.Fatalf("unexpected payload: %+v", payload)
	}
}

func TestDeleteCommandPropagatesBackendError(t *testing.T) {
	backend := &deleteCommandBackend{returnError: errors.New("boom")}
	svc := service.New(
		[]core.KnowledgeBase{{ID: "notes", Scope: core.ScopeGlobal, StoreType: "sqlite", Enabled: true}},
		map[string]core.StoreBackend{"sqlite": backend},
	)

	cmd := cli.NewRootCommand(svc)
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetErr(new(bytes.Buffer))
	cmd.SetArgs([]string{"delete", "--kb", "notes", "--id", "x"})

	if err := cmd.Execute(); err == nil {
		t.Fatalf("expected error from backend, got nil")
	}
}

func TestDeleteCommandRequiresKBAndID(t *testing.T) {
	svc := service.New(
		[]core.KnowledgeBase{{ID: "notes", Scope: core.ScopeGlobal, StoreType: "sqlite", Enabled: true}},
		map[string]core.StoreBackend{"sqlite": &deleteCommandBackend{}},
	)
	cmd := cli.NewRootCommand(svc)
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetErr(new(bytes.Buffer))
	cmd.SetArgs([]string{"delete", "--kb", "notes"})

	if err := cmd.Execute(); err == nil {
		t.Fatalf("expected missing item id to fail")
	}
}
