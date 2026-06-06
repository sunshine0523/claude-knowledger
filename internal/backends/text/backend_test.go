package text_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/kindbrave/knowledger/internal/backends/text"
	"github.com/kindbrave/knowledger/internal/core"
)

func TestTextBackendAddListAndSearch(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	backend := text.New()
	kb := core.KnowledgeBase{
		ID:          "docs",
		StoreType:   "text",
		StoreConfig: map[string]any{"path": dir},
		Enabled:     true,
	}

	item, ingest, _, err := backend.Add(ctx, kb, core.AddInput{
		KBID:    "docs",
		Title:   "设计原则",
		Content: "统一 core，隐藏底层差异。",
		Tags:    []string{"architecture"},
	})
	if err != nil {
		t.Fatalf("Add returned error: %v", err)
	}
	if !ingest.Success {
		t.Fatalf("expected ingest success")
	}
	if item.Title != "设计原则" {
		t.Fatalf("expected title 设计原则, got %q", item.Title)
	}

	items, err := backend.ListItems(ctx, kb)
	if err != nil {
		t.Fatalf("ListItems returned error: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}

	hits, err := backend.Search(ctx, kb, core.SearchOptions{Query: "隐藏底层", Limit: 10})
	if err != nil {
		t.Fatalf("Search returned error: %v", err)
	}
	if len(hits) != 1 {
		t.Fatalf("expected 1 hit, got %d", len(hits))
	}

	expectedFile := filepath.Join(dir, item.ID+".md")
	if hits[0].Locator != expectedFile {
		t.Fatalf("expected locator %q, got %q", expectedFile, hits[0].Locator)
	}
}
