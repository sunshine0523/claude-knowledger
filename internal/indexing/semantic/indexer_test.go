package semantic

import (
	"context"
	"errors"
	"os"
	"path/filepath"
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
	queries      []string
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
	f.queries = append(f.queries, query)
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

func TestSearchAggregatesChunksByParent(t *testing.T) {
	c := &fakeClient{
		queryHits: map[string][]chroma.Hit{
			"how does config work": {
				{ItemID: "42#chunk-0", Content: "low", Score: 0.3, Metadata: map[string]any{"kb_id": "notes", "parent_id": "42", "title": "T"}},
				{ItemID: "42#chunk-1", Content: "high", Score: 0.9, Metadata: map[string]any{"kb_id": "notes", "parent_id": "42", "title": "T"}},
				{ItemID: "99#chunk-0", Content: "other", Score: 0.5, Metadata: map[string]any{"kb_id": "notes", "parent_id": "99", "title": "U"}},
			},
		},
	}
	idx := NewIndexer(func(chroma.Config) (chroma.Client, error) { return c, nil }, nil)
	hits, err := idx.Search(context.Background(), semanticKB("notes", "/tmp/c"), "how does config work", 10, "semantic")
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 2 {
		t.Fatalf("expected 2 parents (42 + 99), got %d: %#v", len(hits), hits)
	}
	if hits[0].ItemID != "42" || hits[0].Score != 0.9 || hits[0].Snippet != "high" {
		t.Fatalf("expected parent 42 with high-score chunk first, got %#v", hits[0])
	}
	if hits[0].MatchMode != "semantic" || hits[0].SourceBackend != "chroma" {
		t.Fatalf("bad match mode/source: %#v", hits[0])
	}
	if hits[0].KBID != "notes" || hits[0].Title != "T" {
		t.Fatalf("bad kb/title: %#v", hits[0])
	}
}

// TestSearchPassesFullQueryNotTokens ensures the query is sent to chroma as
// one embedding lookup, not split into per-token lookups. Embedding the joint
// phrase is what makes semantic search work; tokenizing first reduces it to
// noisy word-level matching.
func TestSearchPassesFullQueryNotTokens(t *testing.T) {
	c := &fakeClient{queryHits: map[string][]chroma.Hit{}}
	idx := NewIndexer(func(chroma.Config) (chroma.Client, error) { return c, nil }, nil)
	if _, err := idx.Search(context.Background(), semanticKB("notes", "/tmp/c"), "how to configure semantic index", 10, "semantic"); err != nil {
		t.Fatal(err)
	}
	if len(c.queries) != 1 {
		t.Fatalf("expected 1 chroma query, got %d: %#v", len(c.queries), c.queries)
	}
	if c.queries[0] != "how to configure semantic index" {
		t.Fatalf("expected full query passed verbatim, got %q", c.queries[0])
	}
}

func TestSearchFiltersByKBID(t *testing.T) {
	c := &fakeClient{
		queryHits: map[string][]chroma.Hit{
			"hello world": {
				{ItemID: "1#chunk-0", Score: 0.9, Metadata: map[string]any{"kb_id": "other", "parent_id": "1"}},
				{ItemID: "2#chunk-0", Score: 0.5, Metadata: map[string]any{"kb_id": "notes", "parent_id": "2"}},
			},
		},
	}
	idx := NewIndexer(func(chroma.Config) (chroma.Client, error) { return c, nil }, nil)
	hits, err := idx.Search(context.Background(), semanticKB("notes", "/tmp/c"), "hello world", 10, "semantic")
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 1 || hits[0].ItemID != "2" {
		t.Fatalf("expected only kb=notes hit, got %#v", hits)
	}
}

func TestSearchEmptyQueryReturnsNil(t *testing.T) {
	c := &fakeClient{}
	idx := NewIndexer(func(chroma.Config) (chroma.Client, error) { return c, nil }, nil)
	hits, err := idx.Search(context.Background(), semanticKB("notes", "/tmp/c"), "   ", 10, "semantic")
	if err != nil {
		t.Fatal(err)
	}
	if hits != nil {
		t.Fatalf("expected nil hits, got %#v", hits)
	}
	if len(c.queries) != 0 {
		t.Fatalf("expected no chroma calls for empty query, got %#v", c.queries)
	}
}

func TestSearchUnsupportedKBReturnsNilNoCall(t *testing.T) {
	c := &fakeClient{}
	idx := NewIndexer(func(chroma.Config) (chroma.Client, error) { return c, nil }, nil)
	kb := core.KnowledgeBase{ID: "x"}
	hits, err := idx.Search(context.Background(), kb, "hi", 10, "semantic")
	if err != nil || hits != nil {
		t.Fatalf("expected nil/nil, got %v / %v", err, hits)
	}
	if len(c.upserts)+len(c.deletesByPar)+len(c.deletesByID) != 0 {
		t.Fatal("expected no client calls")
	}
}

func TestMaintainIndexInsertsNewItem(t *testing.T) {
	c := &fakeClient{}
	idx := NewIndexer(func(chroma.Config) (chroma.Client, error) { return c, nil }, nil)
	kb := semanticKB("notes", "/tmp/c")
	source := func(context.Context) ([]core.KnowledgeItem, error) {
		return []core.KnowledgeItem{{ID: "1", KBID: "notes", Content: "hi"}}, nil
	}
	res, err := idx.MaintainIndex(context.Background(), kb, core.IndexOptions{}, source, func(core.KnowledgeItem) map[string]any { return map[string]any{"mtime": int64(99)} })
	if err != nil {
		t.Fatal(err)
	}
	if res.Indexed != 1 || res.Skipped != 0 || res.Deleted != 0 {
		t.Fatalf("expected 1 indexed, got %#v", res)
	}
	if len(c.upserts) == 0 {
		t.Fatal("expected upsert call")
	}
}

func TestMaintainIndexSkipsUnchangedMtime(t *testing.T) {
	c := &fakeClient{
		listResp: []chroma.ChunkRecord{
			{ID: "1#chunk-0", ParentID: "1", Mtime: 99},
		},
	}
	idx := NewIndexer(func(chroma.Config) (chroma.Client, error) { return c, nil }, nil)
	kb := semanticKB("notes", "/tmp/c")
	source := func(context.Context) ([]core.KnowledgeItem, error) {
		return []core.KnowledgeItem{{ID: "1", KBID: "notes", Content: "hi"}}, nil
	}
	res, err := idx.MaintainIndex(context.Background(), kb, core.IndexOptions{}, source, func(core.KnowledgeItem) map[string]any { return map[string]any{"mtime": int64(99)} })
	if err != nil {
		t.Fatal(err)
	}
	if res.Indexed != 0 || res.Skipped != 1 {
		t.Fatalf("expected skip, got %#v", res)
	}
	if len(c.upserts) != 0 {
		t.Fatal("expected no upsert when mtime matches")
	}
}

func TestMaintainIndexReindexesChangedMtime(t *testing.T) {
	c := &fakeClient{
		listResp: []chroma.ChunkRecord{
			{ID: "1#chunk-0", ParentID: "1", Mtime: 50},
		},
	}
	idx := NewIndexer(func(chroma.Config) (chroma.Client, error) { return c, nil }, nil)
	source := func(context.Context) ([]core.KnowledgeItem, error) {
		return []core.KnowledgeItem{{ID: "1", KBID: "notes", Content: "hi"}}, nil
	}
	res, err := idx.MaintainIndex(context.Background(), semanticKB("notes", "/tmp/c"), core.IndexOptions{}, source, func(core.KnowledgeItem) map[string]any { return map[string]any{"mtime": int64(99)} })
	if err != nil {
		t.Fatal(err)
	}
	if res.Indexed != 1 || res.Skipped != 0 {
		t.Fatalf("expected re-index, got %#v", res)
	}
}

func TestMaintainIndexDeletesOrphans(t *testing.T) {
	c := &fakeClient{
		listResp: []chroma.ChunkRecord{
			{ID: "9#chunk-0", ParentID: "9", Mtime: 1},
		},
	}
	idx := NewIndexer(func(chroma.Config) (chroma.Client, error) { return c, nil }, nil)
	source := func(context.Context) ([]core.KnowledgeItem, error) { return nil, nil }
	res, err := idx.MaintainIndex(context.Background(), semanticKB("notes", "/tmp/c"), core.IndexOptions{}, source, func(core.KnowledgeItem) map[string]any { return nil })
	if err != nil {
		t.Fatal(err)
	}
	if res.Deleted != 1 {
		t.Fatalf("expected 1 deleted orphan, got %#v", res)
	}
	if len(c.deletesByPar) != 1 || c.deletesByPar[0].Parent != "9" {
		t.Fatalf("unexpected delete calls: %#v", c.deletesByPar)
	}
}

func TestMaintainIndexRebuildDeletesAndUpsertsAll(t *testing.T) {
	c := &fakeClient{
		listResp: []chroma.ChunkRecord{
			{ID: "1#chunk-0", ParentID: "1", Mtime: 1},
			{ID: "2#chunk-0", ParentID: "2", Mtime: 2},
		},
	}
	idx := NewIndexer(func(chroma.Config) (chroma.Client, error) { return c, nil }, nil)
	source := func(context.Context) ([]core.KnowledgeItem, error) {
		return []core.KnowledgeItem{{ID: "1", Content: "a"}, {ID: "2", Content: "b"}}, nil
	}
	res, err := idx.MaintainIndex(context.Background(), semanticKB("notes", "/tmp/c"), core.IndexOptions{Rebuild: true}, source, func(core.KnowledgeItem) map[string]any { return nil })
	if err != nil {
		t.Fatal(err)
	}
	if res.Indexed != 2 {
		t.Fatalf("expected indexed=2, got %#v", res)
	}
}

func TestMaintainIndexUnsupportedKBSkipsWithWarning(t *testing.T) {
	idx := NewIndexer(nil, nil)
	res, err := idx.MaintainIndex(context.Background(), core.KnowledgeBase{ID: "x"}, core.IndexOptions{}, func(context.Context) ([]core.KnowledgeItem, error) { return nil, nil }, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.Skipped != 1 || len(res.Warnings) != 1 {
		t.Fatalf("expected skip+warning, got %#v", res)
	}
}

// TestMaintainIndexOrphanWithEmptyParentDeletedByChunkID covers chunks left
// over from before parent_id metadata existed. DeleteByParent rejects empty
// parent ids; the maintainer must address those rows by chunk id instead so
// the index doesn't keep accumulating unreachable rows.
func TestMaintainIndexOrphanWithEmptyParentDeletedByChunkID(t *testing.T) {
	c := &fakeClient{
		listResp: []chroma.ChunkRecord{
			{ID: "legacy-chunk-a", ParentID: ""},
			{ID: "legacy-chunk-b", ParentID: ""},
		},
	}
	idx := NewIndexer(func(chroma.Config) (chroma.Client, error) { return c, nil }, nil)
	source := func(context.Context) ([]core.KnowledgeItem, error) { return nil, nil }
	res, err := idx.MaintainIndex(context.Background(), semanticKB("notes", "/tmp/c"), core.IndexOptions{}, source, func(core.KnowledgeItem) map[string]any { return nil })
	if err != nil {
		t.Fatal(err)
	}
	if res.Deleted != 2 {
		t.Fatalf("expected 2 orphan chunks deleted, got %#v", res)
	}
	if len(c.deletesByPar) != 0 {
		t.Fatalf("expected no DeleteByParent calls for empty parent, got %#v", c.deletesByPar)
	}
	if len(c.deletesByID) != 2 {
		t.Fatalf("expected 2 chunk-id deletes, got %#v", c.deletesByID)
	}
}

// TestMaintainIndexRebuildClearsPersistentPath verifies the recovery path for
// a corrupted on-disk HNSW: --rebuild must remove the persistent data dir
// before opening the chroma client, otherwise the C++ integrity check at first
// collection access aborts the process and there is no way back from Go.
func TestMaintainIndexRebuildClearsPersistentPath(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "data_level0.bin"), []byte("corrupt"), 0o644); err != nil {
		t.Fatal(err)
	}
	c := &fakeClient{}
	idx := NewIndexer(func(chroma.Config) (chroma.Client, error) { return c, nil }, nil)
	source := func(context.Context) ([]core.KnowledgeItem, error) { return nil, nil }
	_, err := idx.MaintainIndex(context.Background(), semanticKB("notes", dir), core.IndexOptions{Rebuild: true}, source, func(core.KnowledgeItem) map[string]any { return nil })
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("expected persistent dir removed, stat err = %v", err)
	}
}

// TestMaintainIndexRebuildEvictsCachedClient ensures a cached client pinned
// to the corrupted dir is closed and replaced before the dir is removed.
// Without eviction the cached client would keep file handles to a deleted
// path and the rebuild would silently use the dead client.
func TestMaintainIndexRebuildEvictsCachedClient(t *testing.T) {
	dir := t.TempDir()
	first := &fakeClient{}
	second := &fakeClient{}
	calls := 0
	factory := func(chroma.Config) (chroma.Client, error) {
		calls++
		if calls == 1 {
			return first, nil
		}
		return second, nil
	}
	idx := NewIndexer(factory, nil)
	cfg, _ := idx.configFor(semanticKB("notes", dir))
	if _, err := idx.client(cfg); err != nil {
		t.Fatal(err)
	}
	source := func(context.Context) ([]core.KnowledgeItem, error) { return nil, nil }
	if _, err := idx.MaintainIndex(context.Background(), semanticKB("notes", dir), core.IndexOptions{Rebuild: true}, source, func(core.KnowledgeItem) map[string]any { return nil }); err != nil {
		t.Fatal(err)
	}
	if !first.closed {
		t.Fatal("expected first client closed during rebuild reset")
	}
	if calls != 2 {
		t.Fatalf("expected factory invoked twice (cached+post-reset), got %d", calls)
	}
}

// TestMaintainIndexProgressEventsIncremental locks down the order and shape
// of progress events the CLI relies on for its stderr output.
func TestMaintainIndexProgressEventsIncremental(t *testing.T) {
	c := &fakeClient{
		listResp: []chroma.ChunkRecord{
			{ID: "1#chunk-0", ParentID: "1", Mtime: 99},
			{ID: "ghost#chunk-0", ParentID: "ghost", Mtime: 1},
		},
	}
	idx := NewIndexer(func(chroma.Config) (chroma.Client, error) { return c, nil }, nil)
	source := func(context.Context) ([]core.KnowledgeItem, error) {
		return []core.KnowledgeItem{
			{ID: "1", Content: "hi"},
			{ID: "2", Content: "new"},
		}, nil
	}
	var events []core.IndexProgressEvent
	progress := core.IndexProgress(func(ev core.IndexProgressEvent) { events = append(events, ev) })
	if _, err := idx.MaintainIndex(context.Background(), semanticKB("notes", "/tmp/c"), core.IndexOptions{Progress: progress}, source, func(core.KnowledgeItem) map[string]any { return map[string]any{"mtime": int64(99)} }); err != nil {
		t.Fatal(err)
	}
	want := []struct {
		phase core.IndexProgressPhase
		item  string
	}{
		{core.IndexProgressPhaseStart, ""},
		{core.IndexProgressPhaseSkip, "1"},
		{core.IndexProgressPhaseIndex, "2"},
		{core.IndexProgressPhaseDeleteOrphan, "ghost"},
		{core.IndexProgressPhaseDone, ""},
	}
	if len(events) != len(want) {
		t.Fatalf("expected %d events, got %d: %#v", len(want), len(events), events)
	}
	for i, w := range want {
		if events[i].Phase != w.phase || events[i].Item != w.item || events[i].KBID != "notes" {
			t.Fatalf("event %d = %#v, want phase=%s item=%q", i, events[i], w.phase, w.item)
		}
	}
	if events[0].Total != 2 || events[len(events)-1].Total != 2 {
		t.Fatalf("expected total=2 on start/done, got start=%d done=%d", events[0].Total, events[len(events)-1].Total)
	}
}
