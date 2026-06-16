package registry_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kindbrave/knowledger/internal/config"
	"github.com/kindbrave/knowledger/internal/core"
	"github.com/kindbrave/knowledger/internal/registry"
)

func TestFileStoreMissingFileReturnsEmptyList(t *testing.T) {
	store := registry.NewFileStore(filepath.Join(t.TempDir(), "registry.json"))

	items, err := store.List()
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("expected empty list, got %#v", items)
	}
}

func TestFileStoreSaveAndListRoundTrip(t *testing.T) {
	store := registry.NewFileStore(filepath.Join(t.TempDir(), "state", "registry.json"))
	items := []registry.RuntimeKnowledgeBase{{
		ID:          "docs",
		Name:        "Docs",
		StoreType:   "text",
		StoreConfig: map[string]any{"path": "./docs"},
		Enabled:     true,
		Tags:        []string{"docs"},
	}}

	if err := store.Save(items); err != nil {
		t.Fatalf("Save returned error: %v", err)
	}
	got, err := store.List()
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	if len(got) != 1 || got[0].ID != "docs" || got[0].StoreConfig["path"] != "./docs" {
		t.Fatalf("unexpected round trip result: %#v", got)
	}
}

func TestFileStoreMalformedJSONReturnsError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "registry.json")
	if err := os.WriteFile(path, []byte("{"), 0o644); err != nil {
		t.Fatalf("write malformed registry: %v", err)
	}

	store := registry.NewFileStore(path)
	if _, err := store.List(); err == nil {
		t.Fatalf("expected malformed JSON error")
	}
}

func TestRegistryCreatesRuntimeKnowledgeBase(t *testing.T) {
	r := registry.New(nil, registry.NewMemoryStore(nil), nil, "")

	if err := r.Create(core.ScopeGlobal, registry.RuntimeKnowledgeBase{ID: "docs", Name: "Docs", StoreType: "text", StoreConfig: map[string]any{"path": "./docs"}, Enabled: true}); err != nil {
		t.Fatalf("Create returned error: %v", err)
	}

	items, err := r.ListWithSources()
	if err != nil {
		t.Fatalf("ListWithSources returned error: %v", err)
	}
	if len(items) != 1 || items[0].KnowledgeBase.ID != "docs" || items[0].Source != registry.SourceRuntime || !items[0].Deletable {
		t.Fatalf("unexpected source-aware item: %#v", items)
	}
}

func TestRegistryRejectsDuplicateCreate(t *testing.T) {
	static := []config.KnowledgeBaseConfig{{ID: "docs", StoreType: "text", StoreConfig: map[string]any{"path": "./docs"}, Enabled: true}}
	r := registry.New(static, registry.NewMemoryStore(nil), nil, "")

	if err := r.Create(core.ScopeGlobal, registry.RuntimeKnowledgeBase{ID: "docs", StoreType: "text", StoreConfig: map[string]any{"path": "./other"}, Enabled: true}); err == nil {
		t.Fatalf("expected duplicate static create to fail")
	}

	if err := r.Create(core.ScopeGlobal, registry.RuntimeKnowledgeBase{ID: "notes", StoreType: "text", StoreConfig: map[string]any{"path": "./notes"}, Enabled: true}); err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	if err := r.Create(core.ScopeGlobal, registry.RuntimeKnowledgeBase{ID: "notes", StoreType: "text", StoreConfig: map[string]any{"path": "./notes2"}, Enabled: true}); err == nil {
		t.Fatalf("expected duplicate runtime create to fail")
	}
}

func TestRegistryDeletesRuntimeKnowledgeBaseOnly(t *testing.T) {
	static := []config.KnowledgeBaseConfig{{ID: "static", StoreType: "text", StoreConfig: map[string]any{"path": "./static"}, Enabled: true}}
	r := registry.New(static, registry.NewMemoryStore([]registry.RuntimeKnowledgeBase{{ID: "runtime", StoreType: "text", StoreConfig: map[string]any{"path": "./runtime"}, Enabled: true}}), nil, "")

	if err := r.Delete(core.ScopeGlobal, "runtime"); err != nil {
		t.Fatalf("Delete runtime returned error: %v", err)
	}
	items, err := r.List()
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	if len(items) != 1 || items[0].ID != "static" {
		t.Fatalf("expected only static item after delete, got %#v", items)
	}
	if err := r.Delete(core.ScopeGlobal, "static"); err == nil {
		t.Fatalf("expected static delete to fail")
	}
}

func TestRegistryDeleteRuntimeOverrideRevealsStaticKnowledgeBase(t *testing.T) {
	static := []config.KnowledgeBaseConfig{{ID: "docs", Name: "Static Docs", StoreType: "text", StoreConfig: map[string]any{"path": "./static"}, Enabled: true}}
	r := registry.New(static, registry.NewMemoryStore([]registry.RuntimeKnowledgeBase{{ID: "docs", Name: "Runtime Docs", StoreType: "text", StoreConfig: map[string]any{"path": "./runtime"}, Enabled: true}}), nil, "")

	items, err := r.ListWithSources()
	if err != nil {
		t.Fatalf("ListWithSources returned error: %v", err)
	}
	if len(items) != 1 || items[0].Source != registry.SourceRuntime {
		t.Fatalf("expected runtime override before delete, got %#v", items)
	}
	if err := r.Delete(core.ScopeGlobal, "docs"); err != nil {
		t.Fatalf("Delete runtime override returned error: %v", err)
	}
	items, err = r.ListWithSources()
	if err != nil {
		t.Fatalf("ListWithSources after delete returned error: %v", err)
	}
	if len(items) != 1 || items[0].Source != registry.SourceStatic || items[0].KnowledgeBase.Name != "Static Docs" {
		t.Fatalf("expected static item revealed after delete, got %#v", items)
	}
}

func TestRegistryHasProjectStore(t *testing.T) {
	r := registry.New(nil, registry.NewMemoryStore(nil), nil, "")
	if r.HasProjectStore() {
		t.Fatalf("expected HasProjectStore=false when projectStore is nil")
	}
	r2 := registry.New(nil, registry.NewMemoryStore(nil), registry.NewMemoryStore(nil), "/tmp/proj")
	if !r2.HasProjectStore() {
		t.Fatalf("expected HasProjectStore=true when projectStore is non-nil")
	}
}

func TestRegistryMergesStaticAndRuntimeKnowledgeBases(t *testing.T) {
	static := []config.KnowledgeBaseConfig{{
		ID:          "docs",
		Name:        "Docs",
		StoreType:   "text",
		StoreConfig: map[string]any{"path": "./kb/docs"},
		Enabled:     true,
	}}

	store := registry.NewMemoryStore([]registry.RuntimeKnowledgeBase{{
		ID:          "notes",
		Name:        "Notes",
		StoreType:   "sqlite",
		StoreConfig: map[string]any{"path": "./kb/notes.db"},
		Enabled:     true,
	}})

	r := registry.New(static, store, nil, "")
	items, err := r.List()
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}

	if len(items) != 2 {
		t.Fatalf("expected 2 knowledge bases, got %d", len(items))
	}

	if err := r.SetEnabled(core.ScopeGlobal, "notes", false); err != nil {
		t.Fatalf("SetEnabled returned error: %v", err)
	}

	items, err = r.List()
	if err != nil {
		t.Fatalf("List after SetEnabled returned error: %v", err)
	}

	for _, item := range items {
		if item.ID == "notes" && item.Enabled {
			t.Fatalf("expected notes to be disabled")
		}
	}
}

func TestRegistryListWithSourcesMergesAcrossScopes(t *testing.T) {
	static := []config.KnowledgeBaseConfig{{ID: "default", StoreType: "text", StoreConfig: map[string]any{"path": "./d"}, Enabled: true}}
	globalStore := registry.NewMemoryStore([]registry.RuntimeKnowledgeBase{
		{ID: "shared", Name: "Global Shared", StoreType: "text", StoreConfig: map[string]any{"path": "./g"}, Enabled: true},
	})
	projectStore := registry.NewMemoryStore([]registry.RuntimeKnowledgeBase{
		{ID: "shared", Name: "Project Shared", StoreType: "text", StoreConfig: map[string]any{"path": "./p"}, Enabled: true},
		{ID: "local", Name: "Local", StoreType: "text", StoreConfig: map[string]any{"path": "./local"}, Enabled: true},
	})
	r := registry.New(static, globalStore, projectStore, "/tmp/proj")

	records, err := r.ListWithSources()
	if err != nil {
		t.Fatalf("ListWithSources: %v", err)
	}
	if len(records) != 4 {
		t.Fatalf("expected 4 records (1 static, 1 global runtime, 2 project), got %d: %#v", len(records), records)
	}
	if records[0].KnowledgeBase.Scope != core.ScopeProject {
		t.Fatalf("expected first record scope=project, got %q (id=%q)", records[0].KnowledgeBase.Scope, records[0].KnowledgeBase.ID)
	}
	scopesForShared := []string{}
	for _, rec := range records {
		if rec.KnowledgeBase.ID == "shared" {
			scopesForShared = append(scopesForShared, rec.KnowledgeBase.Scope)
		}
	}
	if len(scopesForShared) != 2 {
		t.Fatalf("expected two `shared` records (project+global), got %v", scopesForShared)
	}
}

func TestRegistryCreateRoutesByScope(t *testing.T) {
	globalStore := registry.NewMemoryStore(nil)
	projectStore := registry.NewMemoryStore(nil)
	r := registry.New(nil, globalStore, projectStore, "/tmp/proj")

	if err := r.Create(core.ScopeGlobal, registry.RuntimeKnowledgeBase{ID: "g1", StoreType: "text", StoreConfig: map[string]any{"path": "./g"}, Enabled: true}); err != nil {
		t.Fatalf("global Create: %v", err)
	}
	if err := r.Create(core.ScopeProject, registry.RuntimeKnowledgeBase{ID: "p1", StoreType: "text", StoreConfig: map[string]any{"path": "./p"}, Enabled: true}); err != nil {
		t.Fatalf("project Create: %v", err)
	}

	g, _ := globalStore.List()
	if len(g) != 1 || g[0].ID != "g1" {
		t.Fatalf("global store should contain g1, got %#v", g)
	}
	p, _ := projectStore.List()
	if len(p) != 1 || p[0].ID != "p1" {
		t.Fatalf("project store should contain p1, got %#v", p)
	}
}

func TestRegistryCreateProjectFailsWithoutProjectStore(t *testing.T) {
	r := registry.New(nil, registry.NewMemoryStore(nil), nil, "")
	err := r.Create(core.ScopeProject, registry.RuntimeKnowledgeBase{ID: "x", StoreType: "text", StoreConfig: map[string]any{"path": "./x"}, Enabled: true})
	if err == nil {
		t.Fatalf("expected error creating project KB without project store")
	}
}

func TestRegistryAllowsSameIDAcrossScopes(t *testing.T) {
	r := registry.New(nil, registry.NewMemoryStore(nil), registry.NewMemoryStore(nil), "/tmp/proj")
	if err := r.Create(core.ScopeGlobal, registry.RuntimeKnowledgeBase{ID: "notes", StoreType: "text", StoreConfig: map[string]any{"path": "./g"}, Enabled: true}); err != nil {
		t.Fatalf("global Create: %v", err)
	}
	if err := r.Create(core.ScopeProject, registry.RuntimeKnowledgeBase{ID: "notes", StoreType: "text", StoreConfig: map[string]any{"path": "./p"}, Enabled: true}); err != nil {
		t.Fatalf("project Create with same id should succeed: %v", err)
	}
}

func TestRegistryDeleteScopedRoutesCorrectly(t *testing.T) {
	globalStore := registry.NewMemoryStore([]registry.RuntimeKnowledgeBase{{ID: "shared", StoreType: "text", StoreConfig: map[string]any{"path": "./g"}, Enabled: true}})
	projectStore := registry.NewMemoryStore([]registry.RuntimeKnowledgeBase{{ID: "shared", StoreType: "text", StoreConfig: map[string]any{"path": "./p"}, Enabled: true}})
	r := registry.New(nil, globalStore, projectStore, "/tmp/proj")

	if err := r.Delete(core.ScopeProject, "shared"); err != nil {
		t.Fatalf("project Delete: %v", err)
	}
	g, _ := globalStore.List()
	if len(g) != 1 {
		t.Fatalf("global store should be untouched, got %#v", g)
	}
	p, _ := projectStore.List()
	if len(p) != 0 {
		t.Fatalf("project store should be empty, got %#v", p)
	}
}

func TestRegistryProjectKBDefaultsArePersistedAsRelativePaths(t *testing.T) {
	projectStore := registry.NewMemoryStore(nil)
	r := registry.New(nil, registry.NewMemoryStore(nil), projectStore, "/tmp/proj")

	if err := r.Create(core.ScopeProject, registry.RuntimeKnowledgeBase{
		ID:        "notes",
		StoreType: "sqlite",
		Enabled:   true,
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	persisted, _ := projectStore.List()
	if len(persisted) != 1 {
		t.Fatalf("expected 1 persisted item, got %d", len(persisted))
	}
	got := persisted[0].StoreConfig["path"]
	if got != ".knowledger/db" {
		t.Fatalf("expected persisted path %q, got %q", ".knowledger/db", got)
	}
}

func TestRegistryProjectKBResolvesRelativePathsOnList(t *testing.T) {
	projectStore := registry.NewMemoryStore([]registry.RuntimeKnowledgeBase{{
		ID:          "notes",
		StoreType:   "sqlite",
		StoreConfig: map[string]any{"path": ".knowledger/db"},
		Enabled:     true,
	}})
	r := registry.New(nil, registry.NewMemoryStore(nil), projectStore, "/tmp/proj")

	records, err := r.ListWithSources()
	if err != nil {
		t.Fatalf("ListWithSources: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(records))
	}
	got := records[0].KnowledgeBase.StoreConfig["path"]
	if got != "/tmp/proj/.knowledger/db" {
		t.Fatalf("expected resolved path %q, got %q", "/tmp/proj/.knowledger/db", got)
	}
	stored, _ := projectStore.List()
	if stored[0].StoreConfig["path"] != ".knowledger/db" {
		t.Fatalf("expected store to keep relative path, got %q", stored[0].StoreConfig["path"])
	}
}

func TestRegistryProjectKBChromaCollectionPrefix(t *testing.T) {
	projectStore := registry.NewMemoryStore(nil)
	r := registry.New(nil, registry.NewMemoryStore(nil), projectStore, "/tmp/proj")

	if err := r.Create(core.ScopeProject, registry.RuntimeKnowledgeBase{
		ID:        "notes",
		StoreType: "sqlite",
		Enabled:   true,
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	persisted, _ := projectStore.List()
	semantic, _ := persisted[0].Indexing["semantic"].(map[string]any)
	collection, _ := semantic["collection"].(string)

	if !strings.HasPrefix(collection, "proj-") {
		t.Fatalf("expected project KB chroma collection to be prefixed; got %q", collection)
	}
	if !strings.HasSuffix(collection, "-notes") {
		t.Fatalf("expected project KB chroma collection to end with -<id>; got %q", collection)
	}
}
