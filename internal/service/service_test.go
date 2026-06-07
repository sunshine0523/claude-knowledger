package service_test

import (
	"context"
	"errors"
	"fmt"
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

func (f fakeBackend) ListItems(context.Context, core.KnowledgeBase) ([]core.KnowledgeItem, error) {
	return nil, nil
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

func (r *recordingBackend) ListItems(context.Context, core.KnowledgeBase) ([]core.KnowledgeItem, error) {
	return nil, nil
}

func (r *recordingBackend) SupportsSemantic(core.KnowledgeBase) bool { return r.semantic }

func testBackendBuilder(kbs []core.KnowledgeBase) (map[string]core.StoreBackend, error) {
	var sqlitePath string
	for _, kb := range kbs {
		if kb.StoreType != "sqlite" {
			continue
		}
		path, _ := kb.StoreConfig["path"].(string)
		if sqlitePath == "" {
			sqlitePath = path
			continue
		}
		if path != sqlitePath {
			return nil, fmt.Errorf("multiple sqlite database paths are not supported")
		}
	}
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

func (failingSemanticBackend) ListItems(context.Context, core.KnowledgeBase) ([]core.KnowledgeItem, error) {
	return nil, nil
}

func (failingSemanticBackend) SupportsSemantic(core.KnowledgeBase) bool { return true }

type hybridFallbackBackend struct {
	modes []string
}

func (h *hybridFallbackBackend) Add(context.Context, core.KnowledgeBase, core.AddInput) (core.KnowledgeItem, core.IngestionResult, core.IndexStatus, error) {
	return core.KnowledgeItem{}, core.IngestionResult{}, core.IndexStatus{}, nil
}

func (h *hybridFallbackBackend) Search(_ context.Context, _ core.KnowledgeBase, opt core.SearchOptions) ([]core.SearchHit, error) {
	h.modes = append(h.modes, opt.SearchMode)
	if opt.SearchMode == "hybrid" {
		return nil, errors.New("semantic path unavailable")
	}
	if opt.SearchMode == "lexical" {
		return []core.SearchHit{{ItemID: "lex", KBID: "notes", Score: 1, MatchMode: "lexical"}}, nil
	}
	return nil, errors.New("unexpected mode")
}

func (h *hybridFallbackBackend) ListItems(context.Context, core.KnowledgeBase) ([]core.KnowledgeItem, error) {
	return nil, nil
}

func (h *hybridFallbackBackend) SupportsSemantic(core.KnowledgeBase) bool { return true }

func TestManagedServiceCreatesAndDeletesRuntimeKnowledgeBase(t *testing.T) {
	static := []config.KnowledgeBaseConfig{{ID: "default", StoreType: "sqlite", StoreConfig: map[string]any{"path": filepath.Join(t.TempDir(), "db")}, Enabled: true}}
	reg := registry.New(static, registry.NewMemoryStore(nil))
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
	reg := registry.New([]config.KnowledgeBaseConfig{{ID: "default", StoreType: "text", StoreConfig: map[string]any{"path": t.TempDir()}, Enabled: true}}, registry.NewMemoryStore(nil))
	svc, err := service.NewManaged(reg, testBackendBuilder)
	if err != nil {
		t.Fatalf("NewManaged returned error: %v", err)
	}
	if err := svc.DeleteKnowledgeBase(context.Background(), "default"); err == nil {
		t.Fatalf("expected static delete to fail")
	}
}

func TestManagedServiceValidatesSQLitePathConstraintBeforePersisting(t *testing.T) {
	basePath := filepath.Join(t.TempDir(), "db")
	reg := registry.New([]config.KnowledgeBaseConfig{{ID: "default", StoreType: "sqlite", StoreConfig: map[string]any{"path": basePath}, Enabled: true}}, registry.NewMemoryStore(nil))
	svc, err := service.NewManaged(reg, testBackendBuilder)
	if err != nil {
		t.Fatalf("NewManaged returned error: %v", err)
	}

	if _, err := svc.CreateKnowledgeBase(context.Background(), service.CreateKnowledgeBaseInput{ID: "notes", StoreType: "sqlite", Path: basePath}); err != nil {
		t.Fatalf("expected same sqlite path to succeed, got %v", err)
	}
	if _, err := svc.CreateKnowledgeBase(context.Background(), service.CreateKnowledgeBaseInput{ID: "other", StoreType: "sqlite", Path: filepath.Join(t.TempDir(), "other.db")}); err == nil {
		t.Fatalf("expected different sqlite path to fail")
	}
	runtimeItems, err := reg.RuntimeItems()
	if err != nil {
		t.Fatalf("RuntimeItems returned error: %v", err)
	}
	if len(runtimeItems) != 1 || runtimeItems[0].ID != "notes" {
		t.Fatalf("expected failed sqlite create not to persist, got %#v", runtimeItems)
	}
}

func TestManagedServiceRejectsInvalidCreateInput(t *testing.T) {
	reg := registry.New(nil, registry.NewMemoryStore(nil))
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
