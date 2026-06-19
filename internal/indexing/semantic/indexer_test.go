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
