package text_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
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
	if !strings.Contains(items[0].Content, "统一 core") {
		t.Fatalf("expected item content to include stored body, got %#v", items[0])
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

func TestTextBackendListsAndSearchesSupportedFilesRecursively(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	backend := text.New()
	kb := core.KnowledgeBase{ID: "docs", StoreType: "text", StoreConfig: map[string]any{"path": dir}, Enabled: true}

	if err := os.MkdirAll(filepath.Join(dir, "nested", "deep"), 0o755); err != nil {
		t.Fatalf("MkdirAll returned error: %v", err)
	}
	files := map[string]string{
		"root.md":                  "root content",
		"nested/note.txt":          "txt file contains recursive needle",
		"nested/deep/design.md":    "markdown file contains recursive needle",
		"nested/deep/ignored.json": "ignored file contains recursive needle",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, filepath.FromSlash(name)), []byte(content), 0o644); err != nil {
			t.Fatalf("WriteFile returned error: %v", err)
		}
	}

	items, err := backend.ListItems(ctx, kb)
	if err != nil {
		t.Fatalf("ListItems returned error: %v", err)
	}
	byID := map[string]core.KnowledgeItem{}
	for _, item := range items {
		byID[item.ID] = item
	}
	for _, id := range []string{"root", "nested/note.txt", "nested/deep/design"} {
		if _, ok := byID[id]; !ok {
			t.Fatalf("expected item id %q in %#v", id, items)
		}
	}
	if _, ok := byID["nested/deep/ignored.json"]; ok {
		t.Fatalf("expected unsupported json file to be ignored, got %#v", items)
	}
	if byID["nested/deep/design"].Title != "nested/deep/design.md" {
		t.Fatalf("expected recursive title, got %q", byID["nested/deep/design"].Title)
	}

	hits, err := backend.Search(ctx, kb, core.SearchOptions{Query: "recursive needle", Limit: 10})
	if err != nil {
		t.Fatalf("Search returned error: %v", err)
	}
	hitIDs := map[string]string{}
	for _, hit := range hits {
		hitIDs[hit.ItemID] = hit.Locator
	}
	for id, path := range map[string]string{
		"nested/note.txt":    filepath.Join(dir, "nested", "note.txt"),
		"nested/deep/design": filepath.Join(dir, "nested", "deep", "design.md"),
	} {
		if hitIDs[id] != path {
			t.Fatalf("expected hit %q locator %q, got hits %#v", id, path, hits)
		}
	}
	if len(hits) != 2 {
		t.Fatalf("expected 2 hits, got %#v", hits)
	}
}

func TestTextBackendDeleteItemRemovesFileAndRejectsTraversal(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	backend := text.New()
	kb := core.KnowledgeBase{ID: "docs", StoreType: "text", StoreConfig: map[string]any{"path": dir}, Enabled: true}

	item, _, _, err := backend.Add(ctx, kb, core.AddInput{KBID: "docs", Title: "Delete", Content: "remove me"})
	if err != nil {
		t.Fatalf("Add returned error: %v", err)
	}
	path := filepath.Join(dir, item.ID+".md")
	if err := backend.DeleteItem(ctx, kb, item.ID); err != nil {
		t.Fatalf("DeleteItem returned error: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected file to be deleted, stat err=%v", err)
	}
	items, err := backend.ListItems(ctx, kb)
	if err != nil {
		t.Fatalf("ListItems returned error: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("expected no items after delete, got %#v", items)
	}
	if err := backend.DeleteItem(ctx, kb, "../outside"); err == nil {
		t.Fatalf("expected path traversal item id to fail")
	}
}
