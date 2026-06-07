package sqlite_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	sqlitebackend "github.com/kindbrave/knowledger/internal/backends/sqlite"
	"github.com/kindbrave/knowledger/internal/core"
	"github.com/kindbrave/knowledger/internal/indexing/chroma"
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
	if items[0].Content != "SQLite 存事实，Chroma 做语义召回。" {
		t.Fatalf("expected content to round-trip, got %#v", items[0])
	}

	hits, err := backend.Search(ctx, kb, core.SearchOptions{Query: "语义召回", Limit: 10})
	if err != nil {
		t.Fatalf("Search returned error: %v", err)
	}
	if len(hits) != 1 {
		t.Fatalf("expected 1 hit, got %d", len(hits))
	}
}

func TestSQLiteBackendDeleteItemRemovesListAndSearchResults(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "knowledge.db")
	backend, err := sqlitebackend.New(dbPath)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	kb := core.KnowledgeBase{ID: "notes", StoreType: "sqlite", StoreConfig: map[string]any{"path": dbPath}, Enabled: true}

	item, _, _, err := backend.Add(ctx, kb, core.AddInput{KBID: "notes", Title: "Delete Me", Content: "temporary content"})
	if err != nil {
		t.Fatalf("Add returned error: %v", err)
	}
	if err := backend.DeleteItem(ctx, kb, item.ID); err != nil {
		t.Fatalf("DeleteItem returned error: %v", err)
	}
	items, err := backend.ListItems(ctx, kb)
	if err != nil {
		t.Fatalf("ListItems returned error: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("expected no items after delete, got %#v", items)
	}
	hits, err := backend.Search(ctx, kb, core.SearchOptions{Query: "temporary", Limit: 10})
	if err != nil {
		t.Fatalf("Search returned error: %v", err)
	}
	if len(hits) != 0 {
		t.Fatalf("expected no hits after delete, got %#v", hits)
	}
	if err := backend.DeleteItem(ctx, kb, item.ID); err == nil {
		t.Fatalf("expected deleting missing item to fail")
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

func TestSQLiteBackendSemanticUpsertFailureRollsBackSQLiteItem(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "knowledge.db")
	client := &fakeSemanticClient{upsertErr: errors.New("chroma unavailable")}
	backend, err := sqlitebackend.New(dbPath, sqlitebackend.WithSemanticClientFactory(func(chroma.Config) (chroma.Client, error) {
		return client, nil
	}))
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	kb := semanticKB(dbPath, t.TempDir())

	_, _, _, err = backend.Add(ctx, kb, core.AddInput{KBID: "notes", Title: "rollback", Content: "should not persist"})
	if err == nil {
		t.Fatalf("expected semantic upsert failure")
	}
	if !strings.Contains(err.Error(), "semantic index failed") || !strings.Contains(err.Error(), "sqlite item rolled back") {
		t.Fatalf("expected rollback context in error, got %v", err)
	}

	items, err := backend.ListItems(ctx, kb)
	if err != nil {
		t.Fatalf("ListItems returned error: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("expected item rollback, got %#v", items)
	}
}

func TestSQLiteBackendSemanticAddUpsertsAndReturnsIndexed(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "knowledge.db")
	client := &fakeSemanticClient{}
	var gotConfig chroma.Config
	backend, err := sqlitebackend.New(dbPath, sqlitebackend.WithSemanticClientFactory(func(cfg chroma.Config) (chroma.Client, error) {
		gotConfig = cfg
		return client, nil
	}))
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	chromaDir := t.TempDir()
	kb := semanticKB(dbPath, chromaDir)

	item, ingest, status, err := backend.Add(ctx, kb, core.AddInput{
		KBID:     "notes",
		Title:    "semantic title",
		Content:  "semantic content",
		Tags:     []string{"sqlite", "semantic"},
		Metadata: map[string]any{"source": "test"},
	})
	if err != nil {
		t.Fatalf("Add returned error: %v", err)
	}
	if !ingest.Success || ingest.ItemID != item.ID {
		t.Fatalf("expected successful ingest for item %q, got %#v", item.ID, ingest)
	}
	if status.State != "indexed" {
		t.Fatalf("expected index status indexed, got %#v", status)
	}
	if len(client.upserts) != 1 {
		t.Fatalf("expected 1 semantic upsert, got %d", len(client.upserts))
	}
	upsert := client.upserts[0]
	if upsert.collection != "notes" || upsert.item.ID != item.ID || upsert.item.KBID != "notes" || upsert.item.Title != "semantic title" || upsert.item.Content != "semantic content" {
		t.Fatalf("unexpected upsert: %#v", upsert)
	}
	if gotConfig.Mode != chroma.ModePersistent || gotConfig.Path != filepath.Join(chromaDir, "chroma", "notes") || gotConfig.Collection != "notes" || !gotConfig.AutoDownload {
		t.Fatalf("unexpected chroma config: %#v", gotConfig)
	}
}

func TestSQLiteBackendSemanticSearchMapsChromaHits(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "knowledge.db")
	client := &fakeSemanticClient{
		queryHits: []chroma.Hit{{
			ItemID:   "42",
			Content:  "semantic snippet",
			Score:    0.75,
			Metadata: map[string]any{"title": "semantic result", "source": "fake"},
		}},
	}
	backend, err := sqlitebackend.New(dbPath, sqlitebackend.WithSemanticClientFactory(func(chroma.Config) (chroma.Client, error) {
		return client, nil
	}))
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	kb := semanticKB(dbPath, t.TempDir())

	hits, err := backend.Search(ctx, kb, core.SearchOptions{Query: "meaning", SearchMode: "semantic", Limit: 3})
	if err != nil {
		t.Fatalf("Search returned error: %v", err)
	}
	if len(hits) != 1 {
		t.Fatalf("expected 1 hit, got %#v", hits)
	}
	hit := hits[0]
	if hit.ItemID != "42" || hit.KBID != "notes" || hit.ItemType != "note" || hit.Title != "semantic result" || hit.Snippet != "semantic snippet" || hit.ContentPreview != "semantic snippet" || hit.Score != 0.75 || hit.MatchMode != "semantic" || hit.SourceBackend != "chroma" {
		t.Fatalf("unexpected hit: %#v", hit)
	}
	if len(client.queries) != 1 || client.queries[0].collection != "notes" || client.queries[0].query != "meaning" || client.queries[0].limit != 3 {
		t.Fatalf("unexpected queries: %#v", client.queries)
	}
}

func semanticKB(dbPath, chromaRoot string) core.KnowledgeBase {
	return core.KnowledgeBase{
		ID:          "notes",
		StoreType:   "sqlite",
		StoreConfig: map[string]any{"path": dbPath},
		Enabled:     true,
		Indexing: map[string]any{
			"semantic": map[string]any{
				"enabled":       true,
				"provider":      "chroma",
				"mode":          chroma.ModePersistent,
				"path":          filepath.Join(chromaRoot, "chroma", "notes"),
				"collection":    "notes",
				"auto_download": true,
			},
		},
	}
}

type fakeSemanticClient struct {
	upsertErr error
	queryErr  error
	deleteErr error
	closeErr  error
	queryHits []chroma.Hit
	upserts   []fakeSemanticUpsert
	queries   []fakeSemanticQuery
	deletes   []fakeSemanticDelete
	closed    bool
}

type fakeSemanticUpsert struct {
	collection string
	item       chroma.Item
}

type fakeSemanticQuery struct {
	collection string
	query      string
	limit      int
}

type fakeSemanticDelete struct {
	collection string
	itemID     string
}

func (f *fakeSemanticClient) Upsert(_ context.Context, collection string, item chroma.Item) error {
	f.upserts = append(f.upserts, fakeSemanticUpsert{collection: collection, item: item})
	return f.upsertErr
}

func (f *fakeSemanticClient) Query(_ context.Context, collection string, query string, limit int) ([]chroma.Hit, error) {
	f.queries = append(f.queries, fakeSemanticQuery{collection: collection, query: query, limit: limit})
	return f.queryHits, f.queryErr
}

func (f *fakeSemanticClient) Delete(_ context.Context, collection string, itemID string) error {
	f.deletes = append(f.deletes, fakeSemanticDelete{collection: collection, itemID: itemID})
	return f.deleteErr
}

func (f *fakeSemanticClient) Close() error {
	f.closed = true
	return f.closeErr
}
