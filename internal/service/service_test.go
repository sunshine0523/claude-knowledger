package service_test

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kindbrave/knowledger/internal/config"
	"github.com/kindbrave/knowledger/internal/core"
	"github.com/kindbrave/knowledger/internal/registry"
	"github.com/kindbrave/knowledger/internal/service"
)

type fakeBackend struct {
	hits []core.SearchHit
}

func (f fakeBackend) Add(context.Context, core.KnowledgeBase, core.AddInput) (core.KnowledgeItem, core.IngestionResult, core.IndexStatus, error) {
	return core.KnowledgeItem{ID: "1"}, core.IngestionResult{Success: true, ItemID: "1"}, core.IndexStatus{State: "not_indexed"}, nil
}

func (f fakeBackend) Search(context.Context, core.KnowledgeBase, core.SearchOptions) ([]core.SearchHit, error) {
	return f.hits, nil
}

func (f fakeBackend) GetItem(context.Context, core.KnowledgeBase, string) (core.KnowledgeItem, error) {
	return core.KnowledgeItem{ID: "1", KBID: "docs", Title: "Doc", Content: "Content"}, nil
}

func (f fakeBackend) ListItems(context.Context, core.KnowledgeBase) ([]core.KnowledgeItem, error) {
	return []core.KnowledgeItem{{ID: "1", KBID: "docs", Title: "Doc", Content: "Content"}}, nil
}

func (f fakeBackend) DeleteItem(context.Context, core.KnowledgeBase, string) error {
	return nil
}

func (f fakeBackend) SupportsSemantic(core.KnowledgeBase) bool { return false }

type recordingBackend struct {
	hits        []core.SearchHit
	semantic    bool
	lastOptions []core.SearchOptions
}

func (r *recordingBackend) Add(context.Context, core.KnowledgeBase, core.AddInput) (core.KnowledgeItem, core.IngestionResult, core.IndexStatus, error) {
	return core.KnowledgeItem{ID: "1"}, core.IngestionResult{Success: true, ItemID: "1"}, core.IndexStatus{State: "not_indexed"}, nil
}

func (r *recordingBackend) Search(_ context.Context, _ core.KnowledgeBase, opt core.SearchOptions) ([]core.SearchHit, error) {
	r.lastOptions = append(r.lastOptions, opt)
	return r.hits, nil
}

func (r *recordingBackend) GetItem(_ context.Context, kb core.KnowledgeBase, itemID string) (core.KnowledgeItem, error) {
	for _, hit := range r.hits {
		if hit.ItemID == itemID {
			content := hit.Snippet
			if content == "" {
				content = hit.ContentPreview
			}
			return core.KnowledgeItem{ID: itemID, KBID: kb.ID, Title: hit.Title, Content: content}, nil
		}
	}
	return core.KnowledgeItem{}, &core.Error{Kind: core.ErrorKindStore, Message: "knowledge item not found"}
}

func (r *recordingBackend) ListItems(context.Context, core.KnowledgeBase) ([]core.KnowledgeItem, error) {
	return nil, nil
}

func (r *recordingBackend) DeleteItem(context.Context, core.KnowledgeBase, string) error {
	return nil
}

func (r *recordingBackend) SupportsSemantic(core.KnowledgeBase) bool { return r.semantic }

func testBackendBuilder(kbs []core.KnowledgeBase) (map[string]core.StoreBackend, error) {
	return map[string]core.StoreBackend{"text": fakeBackend{}, "sqlite": fakeBackend{}}, nil
}

type failingSemanticBackend struct{}

func (failingSemanticBackend) Add(context.Context, core.KnowledgeBase, core.AddInput) (core.KnowledgeItem, core.IngestionResult, core.IndexStatus, error) {
	return core.KnowledgeItem{}, core.IngestionResult{}, core.IndexStatus{}, nil
}

func (failingSemanticBackend) Search(_ context.Context, _ core.KnowledgeBase, opt core.SearchOptions) ([]core.SearchHit, error) {
	if opt.SearchMode == "lexical" {
		return nil, nil
	}
	return nil, errors.New("semantic path unavailable")
}

func (failingSemanticBackend) GetItem(context.Context, core.KnowledgeBase, string) (core.KnowledgeItem, error) {
	return core.KnowledgeItem{}, &core.Error{Kind: core.ErrorKindStore, Message: "knowledge item not found"}
}

func (failingSemanticBackend) ListItems(context.Context, core.KnowledgeBase) ([]core.KnowledgeItem, error) {
	return nil, nil
}

func (failingSemanticBackend) DeleteItem(context.Context, core.KnowledgeBase, string) error {
	return nil
}

func (failingSemanticBackend) SupportsSemantic(core.KnowledgeBase) bool { return true }

type hybridFallbackBackend struct {
	modes       []string
	lexicalHits []core.SearchHit
}

func (h *hybridFallbackBackend) Add(context.Context, core.KnowledgeBase, core.AddInput) (core.KnowledgeItem, core.IngestionResult, core.IndexStatus, error) {
	return core.KnowledgeItem{}, core.IngestionResult{}, core.IndexStatus{}, nil
}

func (h *hybridFallbackBackend) Search(_ context.Context, _ core.KnowledgeBase, opt core.SearchOptions) ([]core.SearchHit, error) {
	h.modes = append(h.modes, opt.SearchMode)
	if opt.SearchMode == "semantic" || opt.SearchMode == "hybrid" {
		return nil, errors.New("semantic path unavailable")
	}
	if opt.SearchMode == "lexical" {
		if h.lexicalHits != nil {
			return h.lexicalHits, nil
		}
		return []core.SearchHit{{ItemID: "lex", KBID: "notes", Score: 1, MatchMode: "lexical"}}, nil
	}
	return nil, errors.New("unexpected mode")
}

func (h *hybridFallbackBackend) GetItem(_ context.Context, kb core.KnowledgeBase, itemID string) (core.KnowledgeItem, error) {
	for _, hit := range h.lexicalHits {
		if hit.ItemID == itemID {
			content := hit.Snippet
			if content == "" {
				content = hit.ContentPreview
			}
			return core.KnowledgeItem{ID: itemID, KBID: kb.ID, Title: hit.Title, Content: content}, nil
		}
	}
	if itemID == "lex" {
		return core.KnowledgeItem{ID: "lex", KBID: kb.ID, Content: "lex"}, nil
	}
	return core.KnowledgeItem{}, &core.Error{Kind: core.ErrorKindStore, Message: "knowledge item not found"}
}

func (h *hybridFallbackBackend) ListItems(context.Context, core.KnowledgeBase) ([]core.KnowledgeItem, error) {
	return nil, nil
}

func (h *hybridFallbackBackend) DeleteItem(context.Context, core.KnowledgeBase, string) error {
	return nil
}

func (h *hybridFallbackBackend) SupportsSemantic(core.KnowledgeBase) bool { return true }

type itemRecordingBackend struct {
	items       []core.KnowledgeItem
	gotKB       string
	gotItem     string
	deletedKB   string
	deletedItem string
}

type closeRecordingBackend struct {
	closed int
	err    error
}

func (b *closeRecordingBackend) Add(context.Context, core.KnowledgeBase, core.AddInput) (core.KnowledgeItem, core.IngestionResult, core.IndexStatus, error) {
	return core.KnowledgeItem{}, core.IngestionResult{}, core.IndexStatus{}, nil
}

func (b *closeRecordingBackend) Search(context.Context, core.KnowledgeBase, core.SearchOptions) ([]core.SearchHit, error) {
	return nil, nil
}

func (b *closeRecordingBackend) GetItem(context.Context, core.KnowledgeBase, string) (core.KnowledgeItem, error) {
	return core.KnowledgeItem{}, &core.Error{Kind: core.ErrorKindStore, Message: "knowledge item not found"}
}

func (b *closeRecordingBackend) ListItems(context.Context, core.KnowledgeBase) ([]core.KnowledgeItem, error) {
	return nil, nil
}

func (b *closeRecordingBackend) DeleteItem(context.Context, core.KnowledgeBase, string) error {
	return nil
}

func (b *closeRecordingBackend) SupportsSemantic(core.KnowledgeBase) bool { return false }

func (b *closeRecordingBackend) Close() error {
	b.closed++
	return b.err
}

func (b *itemRecordingBackend) Add(context.Context, core.KnowledgeBase, core.AddInput) (core.KnowledgeItem, core.IngestionResult, core.IndexStatus, error) {
	return core.KnowledgeItem{}, core.IngestionResult{}, core.IndexStatus{}, nil
}

func (b *itemRecordingBackend) Search(context.Context, core.KnowledgeBase, core.SearchOptions) ([]core.SearchHit, error) {
	return nil, nil
}

func (b *itemRecordingBackend) GetItem(_ context.Context, kb core.KnowledgeBase, itemID string) (core.KnowledgeItem, error) {
	b.gotKB = kb.ID
	b.gotItem = itemID
	for _, item := range b.items {
		if item.ID == itemID && (item.KBID == "" || item.KBID == kb.ID) {
			item.KBID = kb.ID
			return item, nil
		}
	}
	return core.KnowledgeItem{}, &core.Error{Kind: core.ErrorKindStore, Message: "knowledge item not found"}
}

func (b *itemRecordingBackend) ListItems(_ context.Context, kb core.KnowledgeBase) ([]core.KnowledgeItem, error) {
	items := make([]core.KnowledgeItem, 0, len(b.items))
	for _, item := range b.items {
		if item.KBID == "" || item.KBID == kb.ID {
			item.KBID = kb.ID
			items = append(items, item)
		}
	}
	return items, nil
}

func (b *itemRecordingBackend) DeleteItem(_ context.Context, kb core.KnowledgeBase, itemID string) error {
	b.deletedKB = kb.ID
	b.deletedItem = itemID
	return nil
}

func (b *itemRecordingBackend) SupportsSemantic(core.KnowledgeBase) bool { return false }

type itemSearchBackend struct {
	hits     []core.SearchHit
	items    map[string]core.KnowledgeItem
	getErr   error
	semantic bool
}

type maintenanceBackend struct {
	calls []maintenanceCall
	res   core.IndexResult
	err   error
}

type maintenanceCall struct {
	kbID    string
	rebuild bool
}

func (m *maintenanceBackend) Add(context.Context, core.KnowledgeBase, core.AddInput) (core.KnowledgeItem, core.IngestionResult, core.IndexStatus, error) {
	return core.KnowledgeItem{}, core.IngestionResult{}, core.IndexStatus{}, nil
}

func (m *maintenanceBackend) Search(context.Context, core.KnowledgeBase, core.SearchOptions) ([]core.SearchHit, error) {
	return nil, nil
}

func (m *maintenanceBackend) GetItem(context.Context, core.KnowledgeBase, string) (core.KnowledgeItem, error) {
	return core.KnowledgeItem{}, &core.Error{Kind: core.ErrorKindStore, Message: "knowledge item not found"}
}

func (m *maintenanceBackend) ListItems(context.Context, core.KnowledgeBase) ([]core.KnowledgeItem, error) {
	return nil, nil
}

func (m *maintenanceBackend) DeleteItem(context.Context, core.KnowledgeBase, string) error {
	return nil
}

func (m *maintenanceBackend) SupportsSemantic(core.KnowledgeBase) bool { return true }

func (m *maintenanceBackend) MaintainIndex(_ context.Context, kb core.KnowledgeBase, opt core.IndexOptions) (core.IndexResult, error) {
	m.calls = append(m.calls, maintenanceCall{kbID: kb.ID, rebuild: opt.Rebuild})
	return m.res, m.err
}

func (b *itemSearchBackend) Add(context.Context, core.KnowledgeBase, core.AddInput) (core.KnowledgeItem, core.IngestionResult, core.IndexStatus, error) {
	return core.KnowledgeItem{}, core.IngestionResult{}, core.IndexStatus{}, nil
}

func (b *itemSearchBackend) Search(context.Context, core.KnowledgeBase, core.SearchOptions) ([]core.SearchHit, error) {
	return b.hits, nil
}

func (b *itemSearchBackend) GetItem(_ context.Context, kb core.KnowledgeBase, itemID string) (core.KnowledgeItem, error) {
	if b.getErr != nil {
		return core.KnowledgeItem{}, b.getErr
	}
	item, ok := b.items[itemID]
	if !ok || (item.KBID != "" && item.KBID != kb.ID) {
		return core.KnowledgeItem{}, &core.Error{Kind: core.ErrorKindStore, Message: "knowledge item not found"}
	}
	item.KBID = kb.ID
	return item, nil
}

func (b *itemSearchBackend) ListItems(context.Context, core.KnowledgeBase) ([]core.KnowledgeItem, error) {
	return nil, nil
}

func (b *itemSearchBackend) DeleteItem(context.Context, core.KnowledgeBase, string) error {
	return nil
}

func (b *itemSearchBackend) SupportsSemantic(core.KnowledgeBase) bool { return b.semantic }

func countRunes(s string) int { return len([]rune(s)) }

func TestManagedServiceCreatesAndDeletesRuntimeKnowledgeBase(t *testing.T) {
	static := []config.KnowledgeBaseConfig{{ID: "default", StoreType: "sqlite", StoreConfig: map[string]any{"path": filepath.Join(t.TempDir(), "db")}, Enabled: true}}
	reg := registry.New(static, registry.NewMemoryStore(nil), nil, "")
	svc, err := service.NewManaged(reg, testBackendBuilder)
	if err != nil {
		t.Fatalf("NewManaged returned error: %v", err)
	}

	docsPath := t.TempDir()
	record, err := svc.CreateKnowledgeBase(context.Background(), service.CreateKnowledgeBaseInput{ID: "docs", Name: "Docs", StoreType: "text", Path: docsPath})
	if err != nil {
		t.Fatalf("CreateKnowledgeBase returned error: %v", err)
	}
	if record.Source != registry.SourceRuntime || !record.Deletable {
		t.Fatalf("expected runtime deletable record, got %#v", record)
	}
	kbs := svc.ListKnowledgeBases()
	if len(kbs) != 2 {
		t.Fatalf("expected 2 KBs after create, got %#v", kbs)
	}

	if err := svc.DeleteKnowledgeBase(context.Background(), "docs"); err != nil {
		t.Fatalf("DeleteKnowledgeBase returned error: %v", err)
	}
	kbs = svc.ListKnowledgeBases()
	if len(kbs) != 1 || kbs[0].ID != "default" {
		t.Fatalf("expected only default KB after delete, got %#v", kbs)
	}
}

func TestManagedServiceRejectsStaticDelete(t *testing.T) {
	reg := registry.New([]config.KnowledgeBaseConfig{{ID: "default", StoreType: "text", StoreConfig: map[string]any{"path": t.TempDir()}, Enabled: true}}, registry.NewMemoryStore(nil), nil, "")
	svc, err := service.NewManaged(reg, testBackendBuilder)
	if err != nil {
		t.Fatalf("NewManaged returned error: %v", err)
	}
	if err := svc.DeleteKnowledgeBase(context.Background(), "default"); err == nil {
		t.Fatalf("expected static delete to fail")
	}
}

func TestManagedServiceAllowsMultipleSQLitePaths(t *testing.T) {
	basePath := filepath.Join(t.TempDir(), "db")
	reg := registry.New([]config.KnowledgeBaseConfig{{ID: "default", StoreType: "sqlite", StoreConfig: map[string]any{"path": basePath}, Enabled: true}}, registry.NewMemoryStore(nil), nil, "")
	svc, err := service.NewManaged(reg, testBackendBuilder)
	if err != nil {
		t.Fatalf("NewManaged returned error: %v", err)
	}

	otherPath := filepath.Join(t.TempDir(), "other.db")
	if _, err := svc.CreateKnowledgeBase(context.Background(), service.CreateKnowledgeBaseInput{ID: "other", StoreType: "sqlite", Path: otherPath}); err != nil {
		t.Fatalf("expected different sqlite path to succeed, got %v", err)
	}
	runtimeItems, err := reg.RuntimeItems(core.ScopeGlobal)
	if err != nil {
		t.Fatalf("RuntimeItems returned error: %v", err)
	}
	if len(runtimeItems) != 1 || runtimeItems[0].ID != "other" || runtimeItems[0].StoreConfig["path"] != otherPath {
		t.Fatalf("expected runtime sqlite KB with different path to persist, got %#v", runtimeItems)
	}
}

func TestManagedServiceRejectsInvalidCreateInput(t *testing.T) {
	reg := registry.New(nil, registry.NewMemoryStore(nil), nil, "")
	svc, err := service.NewManaged(reg, testBackendBuilder)
	if err != nil {
		t.Fatalf("NewManaged returned error: %v", err)
	}
	if _, err := svc.CreateKnowledgeBase(context.Background(), service.CreateKnowledgeBaseInput{ID: "bad/id", StoreType: "text", Path: t.TempDir()}); err == nil {
		t.Fatalf("expected invalid id to fail")
	}
	if _, err := svc.CreateKnowledgeBase(context.Background(), service.CreateKnowledgeBaseInput{ID: "vec", StoreType: "chroma", Path: t.TempDir()}); err == nil {
		t.Fatalf("expected invalid store type to fail")
	}
	if _, err := svc.CreateKnowledgeBase(context.Background(), service.CreateKnowledgeBaseInput{ID: "missing", StoreType: "text", Path: filepath.Join(t.TempDir(), "missing")}); err == nil {
		t.Fatalf("expected missing enabled text path to fail")
	}
}

func TestServiceListsAndDeletesKnowledgeItems(t *testing.T) {
	backend := &itemRecordingBackend{items: []core.KnowledgeItem{{ID: "item-1", Title: "Doc", Content: "Body"}}}
	svc := service.New(
		[]core.KnowledgeBase{{ID: "docs", StoreType: "text", Enabled: true}},
		map[string]core.StoreBackend{"text": backend},
	)

	items, err := svc.ListKnowledgeItems(context.Background(), "", "docs")
	if err != nil {
		t.Fatalf("ListKnowledgeItems returned error: %v", err)
	}
	if len(items) != 1 || items[0].ID != "item-1" || items[0].KBID != "docs" {
		t.Fatalf("unexpected items: %#v", items)
	}
	if err := svc.DeleteKnowledgeItem(context.Background(), "", "docs", "item-1"); err != nil {
		t.Fatalf("DeleteKnowledgeItem returned error: %v", err)
	}
	if backend.deletedKB != "docs" || backend.deletedItem != "item-1" {
		t.Fatalf("expected delete docs/item-1, got %q/%q", backend.deletedKB, backend.deletedItem)
	}
}

func TestServiceGetsKnowledgeItem(t *testing.T) {
	backend := &itemRecordingBackend{items: []core.KnowledgeItem{{ID: "item-1", KBID: "docs", Title: "Doc", Content: "full body"}}}
	svc := service.New(
		[]core.KnowledgeBase{{ID: "docs", StoreType: "text", Enabled: true}},
		map[string]core.StoreBackend{"text": backend},
	)

	item, err := svc.GetKnowledgeItem(context.Background(), "", "docs", "item-1")
	if err != nil {
		t.Fatalf("GetKnowledgeItem returned error: %v", err)
	}
	if item.ID != "item-1" || item.KBID != "docs" || item.Content != "full body" {
		t.Fatalf("unexpected item: %#v", item)
	}
	if backend.gotKB != "docs" || backend.gotItem != "item-1" {
		t.Fatalf("expected backend get docs/item-1, got %q/%q", backend.gotKB, backend.gotItem)
	}
}

func TestServiceGetKnowledgeItemValidatesInputs(t *testing.T) {
	svc := service.New(nil, nil)
	if _, err := svc.GetKnowledgeItem(context.Background(), "", "", "item"); err == nil {
		t.Fatalf("expected empty kb id to fail")
	}
	if _, err := svc.GetKnowledgeItem(context.Background(), "", "docs", ""); err == nil {
		t.Fatalf("expected empty item id to fail")
	}
	if _, err := svc.GetKnowledgeItem(context.Background(), "", "missing", "item"); err == nil {
		t.Fatalf("expected missing KB get to fail")
	}
}

func TestServiceReturnsErrorForMissingKnowledgeBaseItems(t *testing.T) {
	svc := service.New(nil, nil)
	if _, err := svc.ListKnowledgeItems(context.Background(), "", "missing"); err == nil {
		t.Fatalf("expected missing KB list to fail")
	}
	if err := svc.DeleteKnowledgeItem(context.Background(), "", "missing", "item"); err == nil {
		t.Fatalf("expected missing KB delete to fail")
	}
}

func TestServiceKnowledgeBaseSummariesIncludeItemCounts(t *testing.T) {
	backend := &itemRecordingBackend{items: []core.KnowledgeItem{{ID: "a", KBID: "docs"}, {ID: "b", KBID: "docs"}}}
	svc := service.New(
		[]core.KnowledgeBase{{ID: "docs", StoreType: "text", Enabled: true}},
		map[string]core.StoreBackend{"text": backend},
	)

	summaries, err := svc.ListKnowledgeBaseSummaries(context.Background())
	if err != nil {
		t.Fatalf("ListKnowledgeBaseSummaries returned error: %v", err)
	}
	if len(summaries) != 1 || summaries[0].ItemCount != 2 || summaries[0].Record.KnowledgeBase.ID != "docs" {
		t.Fatalf("unexpected summaries: %#v", summaries)
	}
}

func TestServiceIndexesSpecificKnowledgeBaseWithRebuild(t *testing.T) {
	backend := &maintenanceBackend{res: core.IndexResult{Indexed: 2, Deleted: 2}}
	svc := service.New(
		[]core.KnowledgeBase{{ID: "notes", StoreType: "sqlite", Enabled: true}},
		map[string]core.StoreBackend{"sqlite": backend},
	)

	result, err := svc.IndexKnowledge(context.Background(), service.IndexKnowledgeInput{KBID: "notes", Rebuild: true})
	if err != nil {
		t.Fatalf("IndexKnowledge returned error: %v", err)
	}
	if len(result.Results) != 1 || result.Results[0].KBID != "notes" || result.Results[0].Result.Indexed != 2 || result.Results[0].Result.Deleted != 2 {
		t.Fatalf("unexpected index result: %#v", result)
	}
	if len(backend.calls) != 1 || backend.calls[0].kbID != "notes" || !backend.calls[0].rebuild {
		t.Fatalf("expected rebuild call for notes, got %#v", backend.calls)
	}
}

func TestServiceIndexesAllEnabledKnowledgeBasesAndSkipsUnsupported(t *testing.T) {
	semanticBackend := &maintenanceBackend{res: core.IndexResult{Indexed: 1}}
	textBackend := fakeBackend{}
	svc := service.New(
		[]core.KnowledgeBase{
			{ID: "docs", StoreType: "text", Enabled: true},
			{ID: "notes", StoreType: "sqlite", Enabled: true},
			{ID: "disabled", StoreType: "sqlite", Enabled: false},
		},
		map[string]core.StoreBackend{"text": textBackend, "sqlite": semanticBackend},
	)

	result, err := svc.IndexKnowledge(context.Background(), service.IndexKnowledgeInput{})
	if err != nil {
		t.Fatalf("IndexKnowledge returned error: %v", err)
	}
	if len(result.Results) != 2 {
		t.Fatalf("expected results for enabled KBs only, got %#v", result.Results)
	}
	if result.Results[0].KBID != "docs" || result.Results[0].Result.Skipped != 1 || len(result.Results[0].Result.Warnings) != 1 {
		t.Fatalf("expected unsupported docs skip, got %#v", result.Results[0])
	}
	if result.Results[1].KBID != "notes" || result.Results[1].Result.Indexed != 1 {
		t.Fatalf("expected indexed notes result, got %#v", result.Results[1])
	}
	if len(result.Warnings) != 1 || !strings.Contains(result.Warnings[0], "index maintenance is not supported") {
		t.Fatalf("expected unsupported warning, got %#v", result.Warnings)
	}
	if len(semanticBackend.calls) != 1 || semanticBackend.calls[0].kbID != "notes" {
		t.Fatalf("expected one notes maintenance call, got %#v", semanticBackend.calls)
	}
}

func TestServiceIndexKnowledgeReturnsErrorForMissingKnowledgeBase(t *testing.T) {
	svc := service.New(nil, nil)
	if _, err := svc.IndexKnowledge(context.Background(), service.IndexKnowledgeInput{KBID: "missing"}); err == nil {
		t.Fatalf("expected missing KB to fail")
	}
}

func TestSearchAggregatesAcrossEnabledKnowledgeBases(t *testing.T) {
	svc := service.New(
		[]core.KnowledgeBase{
			{ID: "docs", StoreType: "text", Enabled: true},
			{ID: "notes", StoreType: "sqlite", Enabled: true},
		},
		map[string]core.StoreBackend{
			"text":   fakeBackend{hits: []core.SearchHit{{ItemID: "a", KBID: "docs", Score: 0.8}}},
			"sqlite": fakeBackend{hits: []core.SearchHit{{ItemID: "b", KBID: "notes", Score: 0.9}}},
		},
	)

	result, err := svc.Search(context.Background(), core.SearchOptions{Query: "core", Limit: 10})
	if err != nil {
		t.Fatalf("Search returned error: %v", err)
	}
	if len(result.Hits) != 2 {
		t.Fatalf("expected 2 hits, got %d", len(result.Hits))
	}
	if result.Hits[0].KBID != "notes" {
		t.Fatalf("expected higher score hit first, got %q", result.Hits[0].KBID)
	}
}

func TestSearchReturnsQueryCenteredSnippetInsteadOfFullContent(t *testing.T) {
	prefix := strings.Repeat("前", 150)
	suffix := strings.Repeat("后", 150)
	content := prefix + "needle" + suffix
	backend := &itemSearchBackend{
		hits:  []core.SearchHit{{ItemID: "item-1", KBID: "docs", Title: "Doc", Snippet: content, ContentPreview: content, Score: 1, MatchMode: "lexical", SourceBackend: "text"}},
		items: map[string]core.KnowledgeItem{"item-1": {ID: "item-1", KBID: "docs", Title: "Doc", Content: content}},
	}
	svc := service.New(
		[]core.KnowledgeBase{{ID: "docs", StoreType: "text", Enabled: true}},
		map[string]core.StoreBackend{"text": backend},
	)

	result, err := svc.Search(context.Background(), core.SearchOptions{Query: "needle", Limit: 10})
	if err != nil {
		t.Fatalf("Search returned error: %v", err)
	}
	if len(result.Hits) != 1 {
		t.Fatalf("expected 1 hit, got %#v", result.Hits)
	}
	snippet := result.Hits[0].Snippet
	if snippet == content || result.Hits[0].ContentPreview == content {
		t.Fatalf("expected bounded snippet, got hit %#v", result.Hits[0])
	}
	if !strings.Contains(snippet, "needle") {
		t.Fatalf("expected snippet to contain query term, got %q", snippet)
	}
	if !strings.HasPrefix(snippet, "…") || !strings.HasSuffix(snippet, "…") {
		t.Fatalf("expected ellipses around middle snippet, got %q", snippet)
	}
	expectedRunes := 120 + countRunes("needle") + 120
	if countRunes(strings.Trim(snippet, "…")) != expectedRunes {
		t.Fatalf("expected %d runes inside snippet, got %d in %q", expectedRunes, countRunes(strings.Trim(snippet, "…")), snippet)
	}
	if result.Hits[0].ContentPreview != snippet {
		t.Fatalf("expected ContentPreview to equal Snippet, got %#v", result.Hits[0])
	}
}

func TestSearchSnippetUsesRuneOffsetsWhenLowercaseChangesByteLength(t *testing.T) {
	content := "İ" + strings.Repeat("前", 150) + "needle" + strings.Repeat("后", 150)
	backend := &itemSearchBackend{
		hits:  []core.SearchHit{{ItemID: "item-1", KBID: "docs", Title: "Doc", Snippet: content, ContentPreview: content, Score: 1, MatchMode: "lexical", SourceBackend: "text"}},
		items: map[string]core.KnowledgeItem{"item-1": {ID: "item-1", KBID: "docs", Title: "Doc", Content: content}},
	}
	svc := service.New(
		[]core.KnowledgeBase{{ID: "docs", StoreType: "text", Enabled: true}},
		map[string]core.StoreBackend{"text": backend},
	)

	result, err := svc.Search(context.Background(), core.SearchOptions{Query: "needle", Limit: 10})
	if err != nil {
		t.Fatalf("Search returned error: %v", err)
	}
	if len(result.Hits) != 1 {
		t.Fatalf("expected 1 hit, got %#v", result.Hits)
	}
	snippet := result.Hits[0].Snippet
	if !strings.Contains(snippet, "needle") {
		t.Fatalf("expected snippet to contain query term, got %q", snippet)
	}
	if !strings.HasPrefix(snippet, "…") || !strings.HasSuffix(snippet, "…") {
		t.Fatalf("expected ellipses around middle snippet, got %q", snippet)
	}
	inside := strings.TrimSuffix(strings.TrimPrefix(snippet, "…"), "…")
	if strings.HasPrefix(inside, "İ") {
		t.Fatalf("expected snippet not to start at the content beginning, got %q", snippet)
	}
	parts := strings.Split(inside, "needle")
	if len(parts) != 2 {
		t.Fatalf("expected one query term in snippet, got %q", snippet)
	}
	if countRunes(parts[0]) != 120 || countRunes(parts[1]) != 120 {
		t.Fatalf("expected needle centered with 120 runes on each side, got %d before and %d after in %q", countRunes(parts[0]), countRunes(parts[1]), snippet)
	}
	if countRunes(inside) != 246 {
		t.Fatalf("expected 246 runes inside snippet, got %d in %q", countRunes(inside), snippet)
	}
}

func TestSearchUsesContentStartWhenQueryTermIsNotLiteral(t *testing.T) {
	content := strings.Repeat("文", 300)
	backend := &itemSearchBackend{
		hits:     []core.SearchHit{{ItemID: "item-1", KBID: "docs", Title: "Doc", Snippet: "semantic full fallback", ContentPreview: "semantic full fallback", Score: 1, MatchMode: "semantic", SourceBackend: "chroma"}},
		items:    map[string]core.KnowledgeItem{"item-1": {ID: "item-1", KBID: "docs", Title: "Doc", Content: content}},
		semantic: true,
	}
	svc := service.New(
		[]core.KnowledgeBase{{ID: "docs", StoreType: "sqlite", Enabled: true}},
		map[string]core.StoreBackend{"sqlite": backend},
	)

	result, err := svc.Search(context.Background(), core.SearchOptions{Query: "missing", SearchMode: "semantic", Limit: 10})
	if err != nil {
		t.Fatalf("Search returned error: %v", err)
	}
	if len(result.Hits) != 1 {
		t.Fatalf("expected 1 hit, got %#v", result.Hits)
	}
	snippet := result.Hits[0].Snippet
	if countRunes(strings.TrimSuffix(snippet, "…")) != 240 {
		t.Fatalf("expected 240-rune prefix snippet, got %d in %q", countRunes(strings.TrimSuffix(snippet, "…")), snippet)
	}
	if !strings.HasSuffix(snippet, "…") {
		t.Fatalf("expected suffix ellipsis for truncated prefix, got %q", snippet)
	}
	if result.Hits[0].ContentPreview != snippet {
		t.Fatalf("expected ContentPreview to equal Snippet, got %#v", result.Hits[0])
	}
}

func TestSearchUsesContentStartWhenFirstQueryTermIsAbsent(t *testing.T) {
	content := strings.Repeat("x", 300) + "needle"
	backend := &itemSearchBackend{
		hits:  []core.SearchHit{{ItemID: "item-1", KBID: "docs", Title: "Doc", Snippet: content, ContentPreview: content, Score: 1, MatchMode: "lexical", SourceBackend: "text"}},
		items: map[string]core.KnowledgeItem{"item-1": {ID: "item-1", KBID: "docs", Title: "Doc", Content: content}},
	}
	svc := service.New(
		[]core.KnowledgeBase{{ID: "docs", StoreType: "text", Enabled: true}},
		map[string]core.StoreBackend{"text": backend},
	)

	result, err := svc.Search(context.Background(), core.SearchOptions{Query: "missing needle", Limit: 10})
	if err != nil {
		t.Fatalf("Search returned error: %v", err)
	}
	if len(result.Hits) != 1 {
		t.Fatalf("expected 1 hit, got %#v", result.Hits)
	}
	expected := strings.Repeat("x", 240) + "…"
	if result.Hits[0].Snippet != expected {
		t.Fatalf("expected first-term-missing fallback snippet %q, got %q", expected, result.Hits[0].Snippet)
	}
	if strings.Contains(result.Hits[0].Snippet, "needle") {
		t.Fatalf("expected first-term-missing fallback not to include later query term, got %q", result.Hits[0].Snippet)
	}
	if result.Hits[0].ContentPreview != result.Hits[0].Snippet {
		t.Fatalf("expected ContentPreview to equal Snippet, got %#v", result.Hits[0])
	}
}

func TestSearchKeepsHitAndWarnsWhenSnippetContentLookupFails(t *testing.T) {
	longFallback := strings.Repeat("p", 300)
	backend := &itemSearchBackend{
		hits:   []core.SearchHit{{ItemID: "missing", KBID: "docs", Title: "Doc", Snippet: "needle", ContentPreview: longFallback, Score: 1, MatchMode: "lexical", SourceBackend: "text"}},
		items:  map[string]core.KnowledgeItem{},
		getErr: &core.Error{Kind: core.ErrorKindStore, Message: "knowledge item not found"},
	}
	svc := service.New(
		[]core.KnowledgeBase{{ID: "docs", StoreType: "text", Enabled: true}},
		map[string]core.StoreBackend{"text": backend},
	)

	result, err := svc.Search(context.Background(), core.SearchOptions{Query: "needle", Limit: 10})
	if err != nil {
		t.Fatalf("Search returned error: %v", err)
	}
	if len(result.Hits) != 1 {
		t.Fatalf("expected hit to be preserved, got %#v", result.Hits)
	}
	expected := strings.Repeat("p", 240) + "…"
	if result.Hits[0].Snippet != expected {
		t.Fatalf("expected fallback snippet to prefer content preview %q, got %q", expected, result.Hits[0].Snippet)
	}
	if result.Hits[0].Snippet == "needle" || strings.Contains(result.Hits[0].Snippet, "needle") {
		t.Fatalf("expected fallback snippet not to prefer hit snippet, got %q", result.Hits[0].Snippet)
	}
	if result.Hits[0].ContentPreview != result.Hits[0].Snippet {
		t.Fatalf("expected ContentPreview to equal Snippet, got %#v", result.Hits[0])
	}
	if len(result.Warnings) != 1 || !strings.Contains(result.Warnings[0], "could not load full content") {
		t.Fatalf("expected lookup warning, got %#v", result.Warnings)
	}
}

func TestSearchResolvesAutoModeToLexicalWhenSemanticSearchIsUnavailable(t *testing.T) {
	backend := &recordingBackend{hits: []core.SearchHit{{ItemID: "a", KBID: "docs", Score: 0.8}}}
	svc := service.New(
		[]core.KnowledgeBase{{ID: "docs", StoreType: "text", Enabled: true, DefaultSearchMode: "auto"}},
		map[string]core.StoreBackend{"text": backend},
	)

	result, err := svc.Search(context.Background(), core.SearchOptions{Query: "core", SearchMode: "auto", Limit: 10})
	if err != nil {
		t.Fatalf("Search returned error: %v", err)
	}
	if len(result.Warnings) != 0 {
		t.Fatalf("expected no warnings, got %#v", result.Warnings)
	}
	if len(backend.lastOptions) != 1 {
		t.Fatalf("expected 1 backend search call, got %d", len(backend.lastOptions))
	}
	if backend.lastOptions[0].SearchMode != "lexical" {
		t.Fatalf("expected backend search mode lexical, got %q", backend.lastOptions[0].SearchMode)
	}
}

func TestSearchUsesKnowledgeBaseDefaultSearchModeWhenRequestModeIsAuto(t *testing.T) {
	backend := &recordingBackend{hits: []core.SearchHit{{ItemID: "a", KBID: "docs", Score: 0.8}}, semantic: true}
	svc := service.New(
		[]core.KnowledgeBase{{ID: "docs", StoreType: "sqlite", Enabled: true, DefaultSearchMode: "hybrid"}},
		map[string]core.StoreBackend{"sqlite": backend},
	)

	result, err := svc.Search(context.Background(), core.SearchOptions{Query: "core", SearchMode: "auto", Limit: 10})
	if err != nil {
		t.Fatalf("Search returned error: %v", err)
	}
	if len(result.Warnings) != 0 {
		t.Fatalf("expected no warnings, got %#v", result.Warnings)
	}
	if len(backend.lastOptions) != 1 {
		t.Fatalf("expected 1 backend search call, got %d", len(backend.lastOptions))
	}
	if backend.lastOptions[0].SearchMode != "hybrid" {
		t.Fatalf("expected backend search mode hybrid, got %q", backend.lastOptions[0].SearchMode)
	}
}

func TestSearchReturnsWarningForExplicitSemanticModeWithoutSemanticBackend(t *testing.T) {
	backend := &recordingBackend{hits: []core.SearchHit{{ItemID: "a", KBID: "docs", Score: 0.8}}}
	svc := service.New(
		[]core.KnowledgeBase{{ID: "docs", StoreType: "text", Enabled: true}},
		map[string]core.StoreBackend{"text": backend},
	)

	result, err := svc.Search(context.Background(), core.SearchOptions{Query: "core", SearchMode: "semantic", Limit: 10})
	if err != nil {
		t.Fatalf("Search returned error: %v", err)
	}
	if len(result.Hits) != 1 {
		t.Fatalf("expected 1 lexical hit, got %d", len(result.Hits))
	}
	if len(result.Warnings) != 1 || !strings.Contains(result.Warnings[0], "lexical results returned") {
		t.Fatalf("expected lexical fallback warning, got %#v", result.Warnings)
	}
	if len(backend.lastOptions) != 1 {
		t.Fatalf("expected 1 backend search call, got %d", len(backend.lastOptions))
	}
	if backend.lastOptions[0].SearchMode != "lexical" {
		t.Fatalf("expected backend search mode lexical, got %q", backend.lastOptions[0].SearchMode)
	}
}

func TestSearchReturnsWarningsWhenSemanticPathFallsBack(t *testing.T) {
	svc := service.New(
		[]core.KnowledgeBase{{ID: "notes", StoreType: "sqlite", Enabled: true}},
		map[string]core.StoreBackend{"sqlite": failingSemanticBackend{}},
	)

	result, err := svc.Search(context.Background(), core.SearchOptions{Query: "core", SearchMode: "hybrid", Limit: 10})
	if err != nil {
		t.Fatalf("Search returned error: %v", err)
	}
	if len(result.Warnings) != 1 {
		t.Fatalf("expected 1 warning, got %d", len(result.Warnings))
	}
}

func TestSearchRetriesLexicalWhenHybridSemanticPathFails(t *testing.T) {
	backend := &hybridFallbackBackend{}
	svc := service.New(
		[]core.KnowledgeBase{{ID: "notes", StoreType: "sqlite", Enabled: true}},
		map[string]core.StoreBackend{"sqlite": backend},
	)

	result, err := svc.Search(context.Background(), core.SearchOptions{Query: "core", SearchMode: "hybrid", Limit: 10})
	if err != nil {
		t.Fatalf("Search returned error: %v", err)
	}
	if len(result.Hits) != 1 {
		t.Fatalf("expected 1 lexical hit, got %d", len(result.Hits))
	}
	if result.Hits[0].ItemID != "lex" {
		t.Fatalf("expected lexical hit ItemID lex, got %q", result.Hits[0].ItemID)
	}
	if len(result.Warnings) != 1 {
		t.Fatalf("expected 1 warning, got %d", len(result.Warnings))
	}
	expectedModes := []string{"hybrid", "lexical"}
	if len(backend.modes) != len(expectedModes) {
		t.Fatalf("expected backend modes %#v, got %#v", expectedModes, backend.modes)
	}
	for i, expectedMode := range expectedModes {
		if backend.modes[i] != expectedMode {
			t.Fatalf("expected backend modes %#v, got %#v", expectedModes, backend.modes)
		}
	}
}

func TestServiceCloseClosesBackends(t *testing.T) {
	backend := &closeRecordingBackend{}
	svc := service.New(
		[]core.KnowledgeBase{{ID: "docs", StoreType: "text", Enabled: true}},
		map[string]core.StoreBackend{"text": backend},
	)

	if err := svc.Close(); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}
	if backend.closed != 1 {
		t.Fatalf("expected backend closed once, got %d", backend.closed)
	}
}

func TestSearchFallsBackForSemanticFailure(t *testing.T) {
	backend := &hybridFallbackBackend{lexicalHits: []core.SearchHit{{ItemID: "lex", KBID: "docs", Score: 1, MatchMode: "lexical"}}}
	svc := service.New([]core.KnowledgeBase{{ID: "docs", StoreType: "sqlite", Enabled: true, DefaultSearchMode: "semantic"}}, map[string]core.StoreBackend{"sqlite": backend})

	result, err := svc.Search(context.Background(), core.SearchOptions{Query: "core", SearchMode: "semantic", Limit: 10})
	if err != nil {
		t.Fatalf("Search returned error: %v", err)
	}
	if len(result.Hits) != 1 || result.Hits[0].ItemID != "lex" {
		t.Fatalf("expected lexical fallback hit, got %#v", result.Hits)
	}
	if len(result.Warnings) != 1 || !strings.Contains(result.Warnings[0], "semantic path unavailable") {
		t.Fatalf("expected semantic fallback warning, got %#v", result.Warnings)
	}
}

func TestCreateSQLiteKnowledgeBaseDefaultsToPersistentSemantic(t *testing.T) {
	reg := registry.New(nil, registry.NewMemoryStore(nil), nil, "")
	svc, err := service.NewManaged(reg, testBackendBuilder)
	if err != nil {
		t.Fatalf("NewManaged returned error: %v", err)
	}
	record, err := svc.CreateKnowledgeBase(context.Background(), service.CreateKnowledgeBaseInput{ID: "notes", StoreType: "sqlite", Path: filepath.Join(t.TempDir(), "db")})
	if err != nil {
		t.Fatalf("CreateKnowledgeBase returned error: %v", err)
	}
	semantic := record.KnowledgeBase.Indexing["semantic"].(map[string]any)
	if semantic["mode"] != "persistent" || semantic["path"] == "" || semantic["auto_download"] != true {
		t.Fatalf("expected persistent semantic defaults, got %#v", semantic)
	}
}

func TestCreateSQLiteKnowledgeBaseCanDisableSemantic(t *testing.T) {
	reg := registry.New(nil, registry.NewMemoryStore(nil), nil, "")
	svc, err := service.NewManaged(reg, testBackendBuilder)
	if err != nil {
		t.Fatalf("NewManaged returned error: %v", err)
	}
	disabled := false
	record, err := svc.CreateKnowledgeBase(context.Background(), service.CreateKnowledgeBaseInput{ID: "notes", StoreType: "sqlite", Path: filepath.Join(t.TempDir(), "db"), SemanticEnabled: &disabled})
	if err != nil {
		t.Fatalf("CreateKnowledgeBase returned error: %v", err)
	}
	semantic := record.KnowledgeBase.Indexing["semantic"].(map[string]any)
	if semantic["enabled"] != false {
		t.Fatalf("expected semantic disabled, got %#v", semantic)
	}
}

type scopeAwareFakeBackend struct {
	addFunc func(context.Context, core.KnowledgeBase, core.AddInput) (core.KnowledgeItem, core.IngestionResult, core.IndexStatus, error)
}

func (f *scopeAwareFakeBackend) Add(ctx context.Context, kb core.KnowledgeBase, in core.AddInput) (core.KnowledgeItem, core.IngestionResult, core.IndexStatus, error) {
	if f.addFunc != nil {
		return f.addFunc(ctx, kb, in)
	}
	return core.KnowledgeItem{}, core.IngestionResult{}, core.IndexStatus{}, nil
}
func (f *scopeAwareFakeBackend) Search(context.Context, core.KnowledgeBase, core.SearchOptions) ([]core.SearchHit, error) {
	return nil, nil
}
func (f *scopeAwareFakeBackend) GetItem(context.Context, core.KnowledgeBase, string) (core.KnowledgeItem, error) {
	return core.KnowledgeItem{}, nil
}
func (f *scopeAwareFakeBackend) ListItems(context.Context, core.KnowledgeBase) ([]core.KnowledgeItem, error) {
	return nil, nil
}
func (f *scopeAwareFakeBackend) DeleteItem(context.Context, core.KnowledgeBase, string) error {
	return nil
}
func (f *scopeAwareFakeBackend) SupportsSemantic(core.KnowledgeBase) bool { return false }

func TestServiceAddRoutesByScope(t *testing.T) {
	addCalls := []core.KnowledgeBase{}
	backend := &scopeAwareFakeBackend{
		addFunc: func(_ context.Context, kb core.KnowledgeBase, _ core.AddInput) (core.KnowledgeItem, core.IngestionResult, core.IndexStatus, error) {
			addCalls = append(addCalls, kb)
			return core.KnowledgeItem{ID: "x", KBID: kb.ID}, core.IngestionResult{Success: true, ItemID: "x"}, core.IndexStatus{}, nil
		},
	}
	svc := service.New(
		[]core.KnowledgeBase{
			{ID: "notes", Scope: core.ScopeGlobal, StoreType: "text", Enabled: true},
			{ID: "notes", Scope: core.ScopeProject, StoreType: "text", Enabled: true},
		},
		map[string]core.StoreBackend{"text": backend},
	)
	if _, _, _, err := svc.Add(context.Background(), core.AddInput{KBID: "notes", Scope: core.ScopeProject, Title: "t", Content: "c"}); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if len(addCalls) != 1 || addCalls[0].Scope != core.ScopeProject {
		t.Fatalf("expected one Add call to project KB, got %#v", addCalls)
	}
}
