package semantic

import (
	"context"
	"errors"
	"testing"

	"github.com/kindbrave/knowledger/internal/core"
	"github.com/kindbrave/knowledger/internal/indexing/chroma"
)

func semanticKB(collection, path string) core.KnowledgeBase {
	return core.KnowledgeBase{
		ID:        "notes",
		StoreType: "sqlite",
		Indexing: map[string]any{
			"semantic": map[string]any{
				"enabled":       true,
				"provider":      "chroma",
				"mode":          chroma.ModePersistent,
				"path":          path,
				"collection":    collection,
				"auto_download": true,
			},
		},
	}
}

func TestSupportsKBFalseWhenSemanticDisabled(t *testing.T) {
	idx := NewIndexer(nil, nil)
	kb := core.KnowledgeBase{ID: "x", Indexing: map[string]any{"semantic": map[string]any{"enabled": false, "provider": "chroma"}}}
	if idx.SupportsKB(kb) {
		t.Fatal("expected SupportsKB false for disabled semantic")
	}
}

func TestSupportsKBTrueWhenChromaConfigured(t *testing.T) {
	idx := NewIndexer(nil, nil)
	if !idx.SupportsKB(semanticKB("notes", "/tmp/c")) {
		t.Fatal("expected SupportsKB true")
	}
}

func TestSupportsKBFalseForNonChromaProvider(t *testing.T) {
	idx := NewIndexer(nil, nil)
	kb := core.KnowledgeBase{Indexing: map[string]any{"semantic": map[string]any{"enabled": true, "provider": "weaviate"}}}
	if idx.SupportsKB(kb) {
		t.Fatal("expected SupportsKB false for non-chroma provider")
	}
}

func TestClientCachedByConfigKey(t *testing.T) {
	calls := 0
	factory := func(cfg chroma.Config) (chroma.Client, error) {
		calls++
		return &fakeClient{}, nil
	}
	idx := NewIndexer(factory, nil)
	cfg, ok := idx.configFor(semanticKB("notes", "/tmp/c"))
	if !ok {
		t.Fatal("expected config")
	}
	if _, err := idx.client(cfg); err != nil {
		t.Fatal(err)
	}
	if _, err := idx.client(cfg); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("expected factory called once, got %d", calls)
	}
}

func TestClientFactoryError(t *testing.T) {
	factory := func(chroma.Config) (chroma.Client, error) { return nil, errors.New("boom") }
	idx := NewIndexer(factory, nil)
	cfg, _ := idx.configFor(semanticKB("notes", "/tmp/c"))
	if _, err := idx.client(cfg); err == nil {
		t.Fatal("expected error")
	}
}

func TestCloseClosesAllCachedClients(t *testing.T) {
	c := &fakeClient{}
	idx := NewIndexer(func(chroma.Config) (chroma.Client, error) { return c, nil }, nil)
	cfg, _ := idx.configFor(semanticKB("notes", "/tmp/c"))
	if _, err := idx.client(cfg); err != nil {
		t.Fatal(err)
	}
	if err := idx.Close(); err != nil {
		t.Fatal(err)
	}
	if !c.closed {
		t.Fatal("expected client closed")
	}
}

type fakeUpsertCall struct {
	collection string
	item       chroma.Item
}

type fakeParentDelete struct {
	KB     string
	Parent string
}

type fakeClient struct {
	upsertErr    error
	deleteErr    error
	listErr      error
	queryErr     error
	queryHits    map[string][]chroma.Hit
	upserts      []fakeUpsertCall
	deletesByID  []string
	deletesByPar []fakeParentDelete
	listResp     []chroma.ChunkRecord
	closed       bool
}

func (f *fakeClient) Upsert(_ context.Context, collection string, item chroma.Item) error {
	f.upserts = append(f.upserts, fakeUpsertCall{collection, item})
	return f.upsertErr
}
func (f *fakeClient) Query(_ context.Context, _ string, query string, _ int) ([]chroma.Hit, error) {
	return f.queryHits[query], f.queryErr
}
func (f *fakeClient) Delete(_ context.Context, _ string, itemID string) error {
	f.deletesByID = append(f.deletesByID, itemID)
	return f.deleteErr
}
func (f *fakeClient) DeleteByParent(_ context.Context, _, kbID, parentID string) error {
	f.deletesByPar = append(f.deletesByPar, fakeParentDelete{kbID, parentID})
	return f.deleteErr
}
func (f *fakeClient) ListByKB(_ context.Context, _, _ string) ([]chroma.ChunkRecord, error) {
	return f.listResp, f.listErr
}
func (f *fakeClient) Close() error { f.closed = true; return nil }

func TestUpsertItemSplitsAndUpserts(t *testing.T) {
	c := &fakeClient{}
	idx := NewIndexer(func(chroma.Config) (chroma.Client, error) { return c, nil }, nil)
	kb := semanticKB("notes", "/tmp/c")
	item := core.KnowledgeItem{ID: "42", KBID: "notes", Title: "T", Content: "short content"}

	if err := idx.UpsertItem(context.Background(), kb, item, map[string]any{"mtime": int64(123)}); err != nil {
		t.Fatal(err)
	}
	if len(c.upserts) != 1 {
		t.Fatalf("expected 1 upsert (short content -> 1 chunk), got %d", len(c.upserts))
	}
	got := c.upserts[0].item
	if got.ID != "42#chunk-0" {
		t.Fatalf("expected chunk id 42#chunk-0, got %q", got.ID)
	}
	if got.Metadata["parent_id"] != "42" || got.Metadata["chunk_index"] != 0 || got.Metadata["chunk_total"] != 1 {
		t.Fatalf("bad chunk metadata: %#v", got.Metadata)
	}
	if got.Metadata["mtime"] != int64(123) {
		t.Fatalf("expected extraMeta mtime=123, got %#v", got.Metadata["mtime"])
	}
	if len(c.deletesByPar) != 1 || c.deletesByPar[0].Parent != "42" {
		t.Fatalf("expected DeleteByParent called once before upsert, got %#v", c.deletesByPar)
	}
}

func TestUpsertItemRollsBackOnPartialFailure(t *testing.T) {
	c := &fakeClient{upsertErr: errors.New("boom")}
	idx := NewIndexer(func(chroma.Config) (chroma.Client, error) { return c, nil }, nil)
	kb := semanticKB("notes", "/tmp/c")
	item := core.KnowledgeItem{ID: "42", KBID: "notes", Content: "x"}

	if err := idx.UpsertItem(context.Background(), kb, item, nil); err == nil {
		t.Fatal("expected error")
	}
	if len(c.deletesByPar) != 2 {
		t.Fatalf("expected DeleteByParent called twice, got %d", len(c.deletesByPar))
	}
}

func TestDeleteItemRoutesToDeleteByParent(t *testing.T) {
	c := &fakeClient{}
	idx := NewIndexer(func(chroma.Config) (chroma.Client, error) { return c, nil }, nil)
	kb := semanticKB("notes", "/tmp/c")
	if err := idx.DeleteItem(context.Background(), kb, "42"); err != nil {
		t.Fatal(err)
	}
	if len(c.deletesByPar) != 1 || c.deletesByPar[0].Parent != "42" || c.deletesByPar[0].KB != "notes" {
		t.Fatalf("unexpected deletesByPar: %#v", c.deletesByPar)
	}
}

func TestUpsertItemNoopWhenSemanticDisabled(t *testing.T) {
	c := &fakeClient{}
	idx := NewIndexer(func(chroma.Config) (chroma.Client, error) { return c, nil }, nil)
	kb := core.KnowledgeBase{ID: "x", Indexing: map[string]any{}}
	if err := idx.UpsertItem(context.Background(), kb, core.KnowledgeItem{ID: "1", Content: "y"}, nil); err != nil {
		t.Fatal(err)
	}
	if len(c.upserts) != 0 || len(c.deletesByPar) != 0 {
		t.Fatalf("expected no calls when semantic disabled, got upserts=%d deletes=%d", len(c.upserts), len(c.deletesByPar))
	}
}
