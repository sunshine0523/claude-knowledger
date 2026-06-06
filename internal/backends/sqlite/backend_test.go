package sqlite_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	sqlitebackend "github.com/kindbrave/knowledger/internal/backends/sqlite"
	"github.com/kindbrave/knowledger/internal/core"
)

func TestSQLiteBackendAddListAndFTSSearch(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "knowledge.db")
	backend, err := sqlitebackend.New(dbPath)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	kb := core.KnowledgeBase{ID: "notes", StoreType: "sqlite", StoreConfig: map[string]any{"path": dbPath}, Enabled: true}

	item, ingest, _, err := backend.Add(ctx, kb, core.AddInput{KBID: "notes", Title: "缓存策略", Content: "SQLite 存事实，Chroma 做语义召回。"})
	if err != nil {
		t.Fatalf("Add returned error: %v", err)
	}
	if !ingest.Success || item.ID == "" {
		t.Fatalf("expected successful ingest with item id")
	}

	items, err := backend.ListItems(ctx, kb)
	if err != nil {
		t.Fatalf("ListItems returned error: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}

	hits, err := backend.Search(ctx, kb, core.SearchOptions{Query: "语义召回", Limit: 10})
	if err != nil {
		t.Fatalf("Search returned error: %v", err)
	}
	if len(hits) != 1 {
		t.Fatalf("expected 1 hit, got %d", len(hits))
	}
}

func TestSQLiteBackendCreatesParentDirectory(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "nested", "storage", "knowledge.db")
	backend, err := sqlitebackend.New(dbPath)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	kb := core.KnowledgeBase{ID: "notes", StoreType: "sqlite", StoreConfig: map[string]any{"path": dbPath}, Enabled: true}
	if _, _, _, err := backend.Add(context.Background(), kb, core.AddInput{KBID: "notes", Title: "test", Content: "content"}); err != nil {
		t.Fatalf("Add returned error: %v", err)
	}
	if _, err := os.Stat(dbPath); err != nil {
		t.Fatalf("expected sqlite db file to exist: %v", err)
	}
}

func TestSQLiteBackendRejectsEmptyPath(t *testing.T) {
	if _, err := sqlitebackend.New(""); err == nil {
		t.Fatalf("expected error for empty sqlite path")
	}
}
