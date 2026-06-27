package sqlite_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	sqlitebackend "github.com/kindbrave/claude-knowledger/internal/backends/sqlite"
	"github.com/kindbrave/claude-knowledger/internal/core"
	"github.com/kindbrave/claude-knowledger/internal/indexing/chroma"
	"github.com/kindbrave/claude-knowledger/internal/indexing/semantic"
)

func indexerOption(factory chroma.Factory) sqlitebackend.Option {
	return sqlitebackend.WithIndexer(semantic.NewIndexer(factory, nil))
}

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

func TestSQLiteBackendGetItemReturnsFullContent(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "knowledge.db")
	backend, err := sqlitebackend.New(dbPath)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	kb := core.KnowledgeBase{ID: "notes", StoreType: "sqlite", StoreConfig: map[string]any{"path": dbPath}, Enabled: true}

	item, _, _, err := backend.Add(ctx, kb, core.AddInput{KBID: "notes", Title: "Full", Content: "完整内容应该通过 GetItem 返回。", Tags: []string{"retrieval"}, Metadata: map[string]any{"source": "test"}})
	if err != nil {
		t.Fatalf("Add returned error: %v", err)
	}

	got, err := backend.GetItem(ctx, kb, item.ID)
	if err != nil {
		t.Fatalf("GetItem returned error: %v", err)
	}
	if got.ID != item.ID || got.KBID != "notes" || got.Type != "note" || got.Title != "Full" || got.Content != "完整内容应该通过 GetItem 返回。" {
		t.Fatalf("unexpected item: %#v", got)
	}
	if len(got.Tags) != 1 || got.Tags[0] != "retrieval" {
		t.Fatalf("expected tag retrieval, got %#v", got.Tags)
	}
	if got.Metadata["source"] != "test" {
		t.Fatalf("expected metadata source test, got %#v", got.Metadata)
	}
	if got.CreatedAt.IsZero() || got.UpdatedAt.IsZero() {
		t.Fatalf("expected timestamps to be populated, got %#v", got)
	}
}

func TestSQLiteBackendGetItemIsScopedToKnowledgeBase(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "knowledge.db")
	backend, err := sqlitebackend.New(dbPath)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	notes := core.KnowledgeBase{ID: "notes", StoreType: "sqlite", StoreConfig: map[string]any{"path": dbPath}, Enabled: true}
	other := core.KnowledgeBase{ID: "other", StoreType: "sqlite", StoreConfig: map[string]any{"path": dbPath}, Enabled: true}

	item, _, _, err := backend.Add(ctx, notes, core.AddInput{KBID: "notes", Title: "Scoped", Content: "only notes can read this"})
	if err != nil {
		t.Fatalf("Add returned error: %v", err)
	}

	_, err = backend.GetItem(ctx, other, item.ID)
	var coreErr *core.Error
	if !errors.As(err, &coreErr) || coreErr.Kind != core.ErrorKindStore || coreErr.Message != "knowledge item not found" {
		t.Fatalf("expected other KB GetItem to return not found store error, got %v", err)
	}
}

func TestSQLiteBackendGetItemReturnsErrorForMissingItem(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "knowledge.db")
	backend, err := sqlitebackend.New(dbPath)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	kb := core.KnowledgeBase{ID: "notes", StoreType: "sqlite", StoreConfig: map[string]any{"path": dbPath}, Enabled: true}

	_, err = backend.GetItem(ctx, kb, "404")
	var coreErr *core.Error
	if !errors.As(err, &coreErr) || coreErr.Kind != core.ErrorKindStore || coreErr.Message != "knowledge item not found" {
		t.Fatalf("expected missing item to return not found store error, got %v", err)
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

func TestSQLiteMultiBackendRoutesByDatabasePath(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	onePath := filepath.Join(root, "one.db")
	twoPath := filepath.Join(root, "two.db")
	oneKB := core.KnowledgeBase{ID: "one", StoreType: "sqlite", StoreConfig: map[string]any{"path": onePath}, Enabled: true}
	twoKB := core.KnowledgeBase{ID: "two", StoreType: "sqlite", StoreConfig: map[string]any{"path": twoPath}, Enabled: true}
	backend, err := sqlitebackend.NewMulti([]core.KnowledgeBase{oneKB, twoKB})
	if err != nil {
		t.Fatalf("NewMulti returned error: %v", err)
	}

	if _, _, _, err := backend.Add(ctx, oneKB, core.AddInput{KBID: "one", Title: "First", Content: "stored in first db"}); err != nil {
		t.Fatalf("Add one returned error: %v", err)
	}
	if _, _, _, err := backend.Add(ctx, twoKB, core.AddInput{KBID: "two", Title: "Second", Content: "stored in second db"}); err != nil {
		t.Fatalf("Add two returned error: %v", err)
	}
	if err := backend.Close(); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}

	oneBackend, err := sqlitebackend.New(onePath)
	if err != nil {
		t.Fatalf("New one returned error: %v", err)
	}
	defer func() { _ = oneBackend.Close() }()
	twoBackend, err := sqlitebackend.New(twoPath)
	if err != nil {
		t.Fatalf("New two returned error: %v", err)
	}
	defer func() { _ = twoBackend.Close() }()

	oneItems, err := oneBackend.ListItems(ctx, oneKB)
	if err != nil {
		t.Fatalf("List one from one db returned error: %v", err)
	}
	if len(oneItems) != 1 || oneItems[0].Title != "First" {
		t.Fatalf("unexpected one db one items: %#v", oneItems)
	}
	if items, err := oneBackend.ListItems(ctx, twoKB); err != nil || len(items) != 0 {
		t.Fatalf("expected no two items in one db, got %#v, err %v", items, err)
	}
	if items, err := twoBackend.ListItems(ctx, oneKB); err != nil || len(items) != 0 {
		t.Fatalf("expected no one items in two db, got %#v, err %v", items, err)
	}
	twoItems, err := twoBackend.ListItems(ctx, twoKB)
	if err != nil {
		t.Fatalf("List two from two db returned error: %v", err)
	}
	if len(twoItems) != 1 || twoItems[0].Title != "Second" {
		t.Fatalf("unexpected two db two items: %#v", twoItems)
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
	backend, err := sqlitebackend.New(dbPath, indexerOption(func(chroma.Config) (chroma.Client, error) {
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
	backend, err := sqlitebackend.New(dbPath, indexerOption(func(cfg chroma.Config) (chroma.Client, error) {
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
	expectedID := item.ID + "#chunk-0"
	if upsert.collection != "notes" || upsert.item.ID != expectedID || upsert.item.KBID != "notes" || upsert.item.Title != "semantic title" || upsert.item.Content != "semantic content" {
		t.Fatalf("unexpected upsert: %#v", upsert)
	}
	if parent, _ := upsert.item.Metadata["parent_id"].(string); parent != item.ID {
		t.Fatalf("expected parent_id=%q, got %#v", item.ID, upsert.item.Metadata)
	}
	if gotConfig.Mode != chroma.ModePersistent || gotConfig.Path != filepath.Join(chromaDir, "chroma", "notes") || gotConfig.Collection != "notes" || !gotConfig.AutoDownload {
		t.Fatalf("unexpected chroma config: %#v", gotConfig)
	}
}

func TestSQLiteBackendSemanticDeleteFailureRollsBackSQLiteItem(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "knowledge.db")
	client := &fakeSemanticClient{}
	backend, err := sqlitebackend.New(dbPath, indexerOption(func(chroma.Config) (chroma.Client, error) {
		return client, nil
	}))
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	kb := semanticKB(dbPath, t.TempDir())

	item, _, _, err := backend.Add(ctx, kb, core.AddInput{KBID: "notes", Title: "keep me", Content: "delete rollback content"})
	if err != nil {
		t.Fatalf("Add returned error: %v", err)
	}
	client.deleteErr = errors.New("chroma delete failed")

	err = backend.DeleteItem(ctx, kb, item.ID)
	if err == nil {
		t.Fatalf("expected semantic delete failure")
	}

	items, err := backend.ListItems(ctx, kb)
	if err != nil {
		t.Fatalf("ListItems returned error: %v", err)
	}
	if len(items) != 1 || items[0].ID != item.ID || items[0].Title != "keep me" {
		t.Fatalf("expected sqlite item to remain after delete failure, got %#v", items)
	}
	if len(client.parentDeletes) != 2 || client.parentDeletes[1].kbID != "notes" || client.parentDeletes[1].parentID != item.ID {
		t.Fatalf("unexpected parent deletes: %#v", client.parentDeletes)
	}
}

func TestSQLiteBackendSemanticAddCleanupUsesDetachedContextAfterCommitFailure(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	dbPath := filepath.Join(t.TempDir(), "knowledge.db")
	client := &fakeSemanticClient{}
	client.afterUpsert = func() {
		cancel()
		client.parentDeleteErr = errors.New("chroma cleanup failed")
	}
	backend, err := sqlitebackend.New(dbPath, indexerOption(func(chroma.Config) (chroma.Client, error) {
		return client, nil
	}))
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	kb := semanticKB(dbPath, t.TempDir())

	_, _, _, err = backend.Add(ctx, kb, core.AddInput{KBID: "notes", Title: "cleanup", Content: "cleanup content"})
	if err == nil {
		t.Fatalf("expected sqlite commit failure after context cancellation")
	}
	if !strings.Contains(err.Error(), "sqlite commit failed after semantic index success") {
		t.Fatalf("expected commit failure context in error, got %v", err)
	}
	if !strings.Contains(err.Error(), "semantic cleanup failed") || !strings.Contains(err.Error(), "chroma cleanup failed") {
		t.Fatalf("expected cleanup failure context in error, got %v", err)
	}
	if len(client.parentDeletes) < 2 {
		t.Fatalf("expected initial DeleteByParent and cleanup DeleteByParent, got %#v", client.parentDeletes)
	}
	if client.deleteCtxCanceled[len(client.deleteCtxCanceled)-1] {
		t.Fatalf("semantic cleanup delete received canceled context")
	}
}

func TestSQLiteBackendSemanticDeleteRestoreUsesDetachedContextAfterCommitFailure(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "knowledge.db")
	client := &fakeSemanticClient{}
	backend, err := sqlitebackend.New(dbPath, indexerOption(func(chroma.Config) (chroma.Client, error) {
		return client, nil
	}))
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	kb := semanticKB(dbPath, t.TempDir())
	item, _, _, err := backend.Add(ctx, kb, core.AddInput{KBID: "notes", Title: "restore", Content: "restore content"})
	if err != nil {
		t.Fatalf("Add returned error: %v", err)
	}

	deleteCtx, cancel := context.WithCancel(context.Background())
	client.afterDelete = func() {
		cancel()
		client.upsertErr = errors.New("chroma restore failed")
	}
	err = backend.DeleteItem(deleteCtx, kb, item.ID)
	if err == nil {
		t.Fatalf("expected sqlite commit failure after context cancellation")
	}
	if !strings.Contains(err.Error(), "semantic restore failed") || !strings.Contains(err.Error(), "chroma restore failed") {
		t.Fatalf("expected restore failure context in error, got %v", err)
	}
	if len(client.upserts) < 1 {
		t.Fatalf("expected initial chunk upsert, got %#v", client.upserts)
	}
	if client.upsertCtxCanceled[len(client.upsertCtxCanceled)-1] {
		t.Fatalf("semantic restore upsert received canceled context")
	}
}

func TestSQLiteBackendSemanticClientCacheKeyDistinguishesAutoDownload(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "knowledge.db")
	var configs []chroma.Config
	backend, err := sqlitebackend.New(dbPath, indexerOption(func(cfg chroma.Config) (chroma.Client, error) {
		configs = append(configs, cfg)
		return &fakeSemanticClient{}, nil
	}))
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	chromaRoot := t.TempDir()
	withAutoDownload := semanticKB(dbPath, chromaRoot)
	withoutAutoDownload := semanticKB(dbPath, chromaRoot)
	withoutAutoDownload.Indexing["semantic"].(map[string]any)["auto_download"] = false

	if _, _, _, err := backend.Add(ctx, withAutoDownload, core.AddInput{KBID: "notes", Title: "auto", Content: "download enabled"}); err != nil {
		t.Fatalf("Add with auto_download=true returned error: %v", err)
	}
	if _, _, _, err := backend.Add(ctx, withoutAutoDownload, core.AddInput{KBID: "notes", Title: "manual", Content: "download disabled"}); err != nil {
		t.Fatalf("Add with auto_download=false returned error: %v", err)
	}

	if len(configs) != 2 {
		t.Fatalf("expected separate semantic clients for different auto_download values, got %d configs: %#v", len(configs), configs)
	}
	if !configs[0].AutoDownload || configs[1].AutoDownload {
		t.Fatalf("expected auto_download configs true then false, got %#v", configs)
	}
}

func TestSQLiteBackendSemanticSearchMapsChromaHits(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "knowledge.db")
	client := &fakeSemanticClient{
		queryHits: []chroma.Hit{{
			ItemID:   "42#chunk-0",
			Content:  "semantic snippet",
			Score:    0.75,
			Metadata: map[string]any{"kb_id": "notes", "parent_id": "42", "title": "semantic result", "source": "fake"},
		}},
	}
	backend, err := sqlitebackend.New(dbPath, indexerOption(func(chroma.Config) (chroma.Client, error) {
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

func TestSQLiteBackendSemanticSearchFiltersHitsByKBMetadata(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "knowledge.db")
	client := &fakeSemanticClient{
		queryHits: []chroma.Hit{
			{
				ItemID:   "wrong-kb#chunk-0",
				Content:  "other kb snippet",
				Score:    0.99,
				Metadata: map[string]any{"kb_id": "other", "parent_id": "wrong-kb", "title": "other result"},
			},
			{
				ItemID:   "missing-kb#chunk-0",
				Content:  "missing kb snippet",
				Score:    0.80,
				Metadata: map[string]any{"parent_id": "missing-kb", "title": "missing kb result"},
			},
			{
				ItemID:   "42#chunk-0",
				Content:  "semantic snippet",
				Score:    0.75,
				Metadata: map[string]any{"kb_id": "notes", "parent_id": "42", "title": "semantic result", "source": "fake"},
			},
		},
	}
	backend, err := sqlitebackend.New(dbPath, indexerOption(func(chroma.Config) (chroma.Client, error) {
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

func TestSQLiteBackendMaintainIndexBackfillsExistingItems(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "knowledge.db")
	client := &fakeSemanticClient{}
	backend, err := sqlitebackend.New(dbPath, indexerOption(func(chroma.Config) (chroma.Client, error) {
		return client, nil
	}))
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	kb := core.KnowledgeBase{ID: "notes", StoreType: "sqlite", StoreConfig: map[string]any{"path": dbPath}, Enabled: true}
	if _, _, _, err := backend.Add(ctx, kb, core.AddInput{KBID: "notes", Title: "First", Content: "first content", Tags: []string{"one"}}); err != nil {
		t.Fatalf("Add first returned error: %v", err)
	}
	if _, _, _, err := backend.Add(ctx, kb, core.AddInput{KBID: "notes", Title: "Second", Content: "second content", Metadata: map[string]any{"source": "test"}}); err != nil {
		t.Fatalf("Add second returned error: %v", err)
	}

	result, err := backend.MaintainIndex(ctx, semanticKB(dbPath, t.TempDir()), core.IndexOptions{})
	if err != nil {
		t.Fatalf("MaintainIndex returned error: %v", err)
	}
	if result.Indexed != 2 || result.Deleted != 0 || result.Skipped != 0 {
		t.Fatalf("unexpected index result: %#v", result)
	}
	if len(client.upserts) != 2 {
		t.Fatalf("expected 2 semantic upserts, got %#v", client.upserts)
	}
	titles := map[string]bool{}
	for _, upsert := range client.upserts {
		if upsert.collection != "notes" || upsert.item.KBID != "notes" || upsert.item.ID == "" || upsert.item.Content == "" {
			t.Fatalf("unexpected upsert: %#v", upsert)
		}
		titles[upsert.item.Title] = true
	}
	if !titles["First"] || !titles["Second"] {
		t.Fatalf("expected both existing items to be indexed, got %#v", client.upserts)
	}
}

func TestSQLiteBackendMaintainIndexRebuildDeletesByKnowledgeBase(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "knowledge.db")
	client := &fakeSemanticClient{}
	backend, err := sqlitebackend.New(dbPath, indexerOption(func(chroma.Config) (chroma.Client, error) {
		return client, nil
	}))
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	kb := core.KnowledgeBase{ID: "notes", StoreType: "sqlite", StoreConfig: map[string]any{"path": dbPath}, Enabled: true}
	first, _, _, err := backend.Add(ctx, kb, core.AddInput{KBID: "notes", Title: "First", Content: "first content"})
	if err != nil {
		t.Fatalf("Add first returned error: %v", err)
	}
	second, _, _, err := backend.Add(ctx, kb, core.AddInput{KBID: "notes", Title: "Second", Content: "second content"})
	if err != nil {
		t.Fatalf("Add second returned error: %v", err)
	}

	client.listResp = []chroma.ChunkRecord{
		{ID: first.ID + "#chunk-0", ParentID: first.ID, Mtime: first.UpdatedAt.Unix()},
		{ID: second.ID + "#chunk-0", ParentID: second.ID, Mtime: second.UpdatedAt.Unix()},
	}
	client.upserts = nil
	client.parentDeletes = nil

	result, err := backend.MaintainIndex(ctx, semanticKB(dbPath, t.TempDir()), core.IndexOptions{Rebuild: true})
	if err != nil {
		t.Fatalf("MaintainIndex returned error: %v", err)
	}
	if result.Indexed != 2 || result.Deleted != 2 {
		t.Fatalf("unexpected index result: %#v", result)
	}
	parents := map[string]bool{}
	for _, d := range client.parentDeletes {
		if d.collection != "notes" || d.kbID != "notes" {
			t.Fatalf("unexpected parent delete: %#v", d)
		}
		parents[d.parentID] = true
	}
	if !parents[first.ID] || !parents[second.ID] {
		t.Fatalf("expected parent deletes for both items, got %#v", client.parentDeletes)
	}
	if len(client.upserts) != 2 {
		t.Fatalf("expected 2 semantic upserts, got %#v", client.upserts)
	}
}

func TestSQLiteBackendMaintainIndexSkipsWhenSemanticDisabled(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "knowledge.db")
	backend, err := sqlitebackend.New(dbPath)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	kb := core.KnowledgeBase{ID: "notes", StoreType: "sqlite", StoreConfig: map[string]any{"path": dbPath}, Enabled: true}

	result, err := backend.MaintainIndex(ctx, kb, core.IndexOptions{})
	if err != nil {
		t.Fatalf("MaintainIndex returned error: %v", err)
	}
	if result.Skipped != 1 || len(result.Warnings) != 1 || !strings.Contains(result.Warnings[0], "semantic indexing is not enabled") {
		t.Fatalf("expected semantic disabled skip, got %#v", result)
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
	upsertErr         error
	queryErr          error
	deleteErr         error
	parentDeleteErr   error
	closeErr          error
	queryHits         []chroma.Hit
	queryHitsByQuery  map[string][]chroma.Hit
	listResp          []chroma.ChunkRecord
	afterUpsert       func()
	afterDelete       func()
	upserts           []fakeSemanticUpsert
	queries           []fakeSemanticQuery
	deletes           []fakeSemanticDelete
	parentDeletes     []fakeSemanticParentDelete
	kbDeletes         []fakeSemanticKBDelete
	upsertCtxCanceled []bool
	deleteCtxCanceled []bool
	closed            bool
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

type fakeSemanticParentDelete struct {
	collection string
	kbID       string
	parentID   string
}

type fakeSemanticKBDelete struct {
	collection string
	kbID       string
}

func (f *fakeSemanticClient) Upsert(ctx context.Context, collection string, item chroma.Item) error {
	f.upserts = append(f.upserts, fakeSemanticUpsert{collection: collection, item: item})
	f.upsertCtxCanceled = append(f.upsertCtxCanceled, ctx.Err() != nil)
	if f.afterUpsert != nil {
		f.afterUpsert()
		f.afterUpsert = nil
	}
	return f.upsertErr
}

func (f *fakeSemanticClient) Query(_ context.Context, collection string, query string, limit int) ([]chroma.Hit, error) {
	f.queries = append(f.queries, fakeSemanticQuery{collection: collection, query: query, limit: limit})
	if f.queryHitsByQuery != nil {
		if hits, ok := f.queryHitsByQuery[query]; ok {
			return hits, f.queryErr
		}
	}
	return f.queryHits, f.queryErr
}

func (f *fakeSemanticClient) Delete(ctx context.Context, collection string, itemID string) error {
	f.deletes = append(f.deletes, fakeSemanticDelete{collection: collection, itemID: itemID})
	f.deleteCtxCanceled = append(f.deleteCtxCanceled, ctx.Err() != nil)
	if f.afterDelete != nil {
		f.afterDelete()
		f.afterDelete = nil
	}
	return f.deleteErr
}

func (f *fakeSemanticClient) DeleteForKnowledgeBase(_ context.Context, collection string, kbID string) error {
	f.kbDeletes = append(f.kbDeletes, fakeSemanticKBDelete{collection: collection, kbID: kbID})
	return f.deleteErr
}

func (f *fakeSemanticClient) DeleteByParent(ctx context.Context, collection, kbID, parentID string) error {
	f.parentDeletes = append(f.parentDeletes, fakeSemanticParentDelete{collection: collection, kbID: kbID, parentID: parentID})
	f.deleteCtxCanceled = append(f.deleteCtxCanceled, ctx.Err() != nil)
	if f.afterDelete != nil {
		f.afterDelete()
		f.afterDelete = nil
	}
	if f.parentDeleteErr != nil {
		return f.parentDeleteErr
	}
	return f.deleteErr
}

func (f *fakeSemanticClient) ListByKB(_ context.Context, _ string, _ string) ([]chroma.ChunkRecord, error) {
	return f.listResp, nil
}

func (f *fakeSemanticClient) Close() error {
	f.closed = true
	return f.closeErr
}


func TestSQLiteBackendFTSSearchTokenizedOR(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "knowledge.db")
	backend, err := sqlitebackend.New(dbPath)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	kb := core.KnowledgeBase{ID: "notes", StoreType: "sqlite", StoreConfig: map[string]any{"path": dbPath}, Enabled: true}

	if _, _, _, err := backend.Add(ctx, kb, core.AddInput{KBID: "notes", Title: "first", Content: "alpha only"}); err != nil {
		t.Fatalf("Add alpha returned error: %v", err)
	}
	if _, _, _, err := backend.Add(ctx, kb, core.AddInput{KBID: "notes", Title: "second", Content: "beta only"}); err != nil {
		t.Fatalf("Add beta returned error: %v", err)
	}
	if _, _, _, err := backend.Add(ctx, kb, core.AddInput{KBID: "notes", Title: "third", Content: "gamma unrelated"}); err != nil {
		t.Fatalf("Add gamma returned error: %v", err)
	}

	hits, err := backend.Search(ctx, kb, core.SearchOptions{Query: "alpha beta", Limit: 10})
	if err != nil {
		t.Fatalf("Search returned error: %v", err)
	}
	if len(hits) != 2 {
		t.Fatalf("expected 2 OR hits, got %d (%#v)", len(hits), hits)
	}
	titles := map[string]bool{}
	for _, h := range hits {
		titles[h.Title] = true
	}
	if !titles["first"] || !titles["second"] || titles["third"] {
		t.Fatalf("expected hits {first, second}, got %#v", titles)
	}
}

func TestSQLiteBackendLikeFallbackTokenizedOR(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "knowledge.db")
	backend, err := sqlitebackend.New(dbPath)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	kb := core.KnowledgeBase{ID: "notes", StoreType: "sqlite", StoreConfig: map[string]any{"path": dbPath}, Enabled: true}

	if _, _, _, err := backend.Add(ctx, kb, core.AddInput{KBID: "notes", Title: "first", Content: "alphabetical"}); err != nil {
		t.Fatalf("Add alphabetical returned error: %v", err)
	}
	if _, _, _, err := backend.Add(ctx, kb, core.AddInput{KBID: "notes", Title: "second", Content: "betacarotene"}); err != nil {
		t.Fatalf("Add betacarotene returned error: %v", err)
	}
	if _, _, _, err := backend.Add(ctx, kb, core.AddInput{KBID: "notes", Title: "third", Content: "unrelated"}); err != nil {
		t.Fatalf("Add unrelated returned error: %v", err)
	}

	hits, err := backend.Search(ctx, kb, core.SearchOptions{Query: "alph beta", Limit: 10})
	if err != nil {
		t.Fatalf("Search returned error: %v", err)
	}
	if len(hits) != 2 {
		t.Fatalf("expected 2 LIKE-fallback OR hits, got %d (%#v)", len(hits), hits)
	}
	titles := map[string]bool{}
	for _, h := range hits {
		titles[h.Title] = true
	}
	if !titles["first"] || !titles["second"] {
		t.Fatalf("expected hits to include first and second, got %#v", titles)
	}
}

// TestSQLiteBackendSemanticSearchPassesFullQuery locks in that semantic mode
// sends the entire query string to chroma in a single embedding lookup. An
// earlier implementation tokenized the query and queried each token
// separately, which destroyed the joint semantics the embedding model is
// meant to capture and produced effectively-noise results.
func TestSQLiteBackendSemanticSearchPassesFullQuery(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "knowledge.db")
	client := &fakeSemanticClient{
		queryHitsByQuery: map[string][]chroma.Hit{
			"alpha beta": {
				{ItemID: "1#chunk-0", Score: 0.9, Metadata: map[string]any{"kb_id": "notes", "parent_id": "1", "title": "first"}},
				{ItemID: "2#chunk-0", Score: 0.7, Metadata: map[string]any{"kb_id": "notes", "parent_id": "2", "title": "second"}},
				{ItemID: "3#chunk-0", Score: 0.6, Metadata: map[string]any{"kb_id": "notes", "parent_id": "3", "title": "third"}},
			},
		},
	}
	backend, err := sqlitebackend.New(dbPath, indexerOption(func(chroma.Config) (chroma.Client, error) {
		return client, nil
	}))
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	kb := semanticKB(dbPath, t.TempDir())

	hits, err := backend.Search(ctx, kb, core.SearchOptions{Query: "alpha beta", SearchMode: "semantic", Limit: 10})
	if err != nil {
		t.Fatalf("Search returned error: %v", err)
	}

	if len(client.queries) != 1 {
		t.Fatalf("expected exactly 1 chroma query (full phrase), got %#v", client.queries)
	}
	if client.queries[0].query != "alpha beta" {
		t.Fatalf("expected full-phrase query %q, got %q", "alpha beta", client.queries[0].query)
	}

	if len(hits) != 3 {
		t.Fatalf("expected 3 hits, got %d (%#v)", len(hits), hits)
	}
	if hits[0].ItemID != "1" || hits[0].Score != 0.9 {
		t.Fatalf("expected itemID 1 with score 0.9 first, got %#v", hits[0])
	}
	if hits[1].ItemID != "2" || hits[1].Score != 0.7 {
		t.Fatalf("expected itemID 2 with score 0.7 second, got %#v", hits[1])
	}
	if hits[2].ItemID != "3" || hits[2].Score != 0.6 {
		t.Fatalf("expected itemID 3 with score 0.6 third, got %#v", hits[2])
	}
	for _, h := range hits {
		if h.MatchMode != "semantic" || h.SourceBackend != "chroma" || h.KBID != "notes" {
			t.Fatalf("unexpected hit envelope: %#v", h)
		}
	}
}
