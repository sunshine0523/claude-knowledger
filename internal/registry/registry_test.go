package registry_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kindbrave/knowledger/internal/config"
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
	r := registry.New(nil, registry.NewMemoryStore(nil))

	if err := r.Create(registry.RuntimeKnowledgeBase{ID: "docs", Name: "Docs", StoreType: "text", StoreConfig: map[string]any{"path": "./docs"}, Enabled: true}); err != nil {
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
	r := registry.New(static, registry.NewMemoryStore(nil))

	if err := r.Create(registry.RuntimeKnowledgeBase{ID: "docs", StoreType: "text", StoreConfig: map[string]any{"path": "./other"}, Enabled: true}); err == nil {
		t.Fatalf("expected duplicate static create to fail")
	}

	if err := r.Create(registry.RuntimeKnowledgeBase{ID: "notes", StoreType: "text", StoreConfig: map[string]any{"path": "./notes"}, Enabled: true}); err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	if err := r.Create(registry.RuntimeKnowledgeBase{ID: "notes", StoreType: "text", StoreConfig: map[string]any{"path": "./notes2"}, Enabled: true}); err == nil {
		t.Fatalf("expected duplicate runtime create to fail")
	}
}

func TestRegistryDeletesRuntimeKnowledgeBaseOnly(t *testing.T) {
	static := []config.KnowledgeBaseConfig{{ID: "static", StoreType: "text", StoreConfig: map[string]any{"path": "./static"}, Enabled: true}}
	r := registry.New(static, registry.NewMemoryStore([]registry.RuntimeKnowledgeBase{{ID: "runtime", StoreType: "text", StoreConfig: map[string]any{"path": "./runtime"}, Enabled: true}}))

	if err := r.Delete("runtime"); err != nil {
		t.Fatalf("Delete runtime returned error: %v", err)
	}
	items, err := r.List()
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	if len(items) != 1 || items[0].ID != "static" {
		t.Fatalf("expected only static item after delete, got %#v", items)
	}
	if err := r.Delete("static"); err == nil {
		t.Fatalf("expected static delete to fail")
	}
}

func TestRegistryDeleteRuntimeOverrideRevealsStaticKnowledgeBase(t *testing.T) {
	static := []config.KnowledgeBaseConfig{{ID: "docs", Name: "Static Docs", StoreType: "text", StoreConfig: map[string]any{"path": "./static"}, Enabled: true}}
	r := registry.New(static, registry.NewMemoryStore([]registry.RuntimeKnowledgeBase{{ID: "docs", Name: "Runtime Docs", StoreType: "text", StoreConfig: map[string]any{"path": "./runtime"}, Enabled: true}}))

	items, err := r.ListWithSources()
	if err != nil {
		t.Fatalf("ListWithSources returned error: %v", err)
	}
	if len(items) != 1 || items[0].Source != registry.SourceRuntime {
		t.Fatalf("expected runtime override before delete, got %#v", items)
	}
	if err := r.Delete("docs"); err != nil {
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

	r := registry.New(static, store)
	items, err := r.List()
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}

	if len(items) != 2 {
		t.Fatalf("expected 2 knowledge bases, got %d", len(items))
	}

	if err := r.SetEnabled("notes", false); err != nil {
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
