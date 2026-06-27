package config_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kindbrave/claude-knowledger/internal/config"
)

func TestLoadAppliesDefaultsAndReadsKnowledgeBases(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "knowledger.yaml")

	err := os.WriteFile(configPath, []byte(`server:
  address: ":34125"
knowledge_bases:
  - id: docs
    name: Docs
    store_type: text
    store_config:
      path: ./kb/docs
    enabled: true
`), 0o644)
	if err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	if cfg.Server.Address != ":34125" {
		t.Fatalf("expected server address :34125, got %q", cfg.Server.Address)
	}

	if cfg.DefaultSearchMode != "auto" {
		t.Fatalf("expected default search mode auto, got %q", cfg.DefaultSearchMode)
	}

	if len(cfg.KnowledgeBases) != 1 {
		t.Fatalf("expected 1 knowledge base, got %d", len(cfg.KnowledgeBases))
	}

	if cfg.KnowledgeBases[0].ID != "docs" {
		t.Fatalf("expected kb id docs, got %q", cfg.KnowledgeBases[0].ID)
	}
	if cfg.KnowledgeBases[0].StoreConfig["path"] != "./kb/docs" {
		t.Fatalf("expected text path to remain unchanged, got %#v", cfg.KnowledgeBases[0].StoreConfig["path"])
	}
}

func TestLoadEmptyConfigCreatesDefaultSQLiteKnowledgeBase(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	cfg := loadConfig(t, "")

	if cfg.DefaultSearchMode != config.DefaultSearchMode {
		t.Fatalf("expected default search mode %q, got %q", config.DefaultSearchMode, cfg.DefaultSearchMode)
	}
	if cfg.Server.Address != config.DefaultServerAddress {
		t.Fatalf("expected server address %q, got %q", config.DefaultServerAddress, cfg.Server.Address)
	}
	if cfg.RuntimeRegistryPath != filepath.Join(home, ".knowledger", "registry.json") {
		t.Fatalf("expected runtime registry path under home, got %q", cfg.RuntimeRegistryPath)
	}
	assertDefaultSQLiteKB(t, cfg, filepath.Join(home, ".knowledger", "db"))
}

func TestLoadExpandsRuntimeRegistryPath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	cfg := loadConfig(t, "runtime_registry_path: ~/custom-registry.json\n")

	if cfg.RuntimeRegistryPath != filepath.Join(home, "custom-registry.json") {
		t.Fatalf("expected expanded runtime registry path, got %q", cfg.RuntimeRegistryPath)
	}
}

func TestLoadEmptyKnowledgeBaseListCreatesDefaultSQLiteKnowledgeBase(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	cfg := loadConfig(t, "knowledge_bases: []\n")

	assertDefaultSQLiteKB(t, cfg, filepath.Join(home, ".knowledger", "db"))
}

func TestLoadSQLiteKnowledgeBaseDefaultsMissingStoreConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	cfg := loadConfig(t, `knowledge_bases:
  - id: notes
    name: Notes
    store_type: sqlite
    enabled: true
`)

	if len(cfg.KnowledgeBases) != 1 {
		t.Fatalf("expected 1 knowledge base, got %d", len(cfg.KnowledgeBases))
	}
	kb := cfg.KnowledgeBases[0]
	if kb.StoreConfig["path"] != filepath.Join(home, ".knowledger", "db") {
		t.Fatalf("expected default sqlite path, got %#v", kb.StoreConfig["path"])
	}
	assertSemanticDefaults(t, kb.Indexing, "notes")
}

func TestLoadSQLiteKnowledgeBaseDefaultsEmptyStoreConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	cfg := loadConfig(t, `knowledge_bases:
  - id: notes
    name: Notes
    store_type: sqlite
    enabled: true
    store_config: {}
`)

	kb := cfg.KnowledgeBases[0]
	if kb.StoreConfig["path"] != filepath.Join(home, ".knowledger", "db") {
		t.Fatalf("expected default sqlite path, got %#v", kb.StoreConfig["path"])
	}
}

func TestLoadSQLiteKnowledgeBaseExpandsHomePath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	cfg := loadConfig(t, `knowledge_bases:
  - id: notes
    name: Notes
    store_type: sqlite
    enabled: true
    store_config:
      path: ~/custom.db
`)

	if cfg.KnowledgeBases[0].StoreConfig["path"] != filepath.Join(home, "custom.db") {
		t.Fatalf("expected expanded sqlite path, got %#v", cfg.KnowledgeBases[0].StoreConfig["path"])
	}
}

func TestLoadSQLiteKnowledgeBasePreservesLegacyHTTPChromaConfig(t *testing.T) {
	cfg := loadConfig(t, `knowledge_bases:
  - id: notes
    name: Notes
    store_type: sqlite
    enabled: true
    indexing:
      semantic:
        enabled: true
        provider: chroma
        base_url: http://127.0.0.1:8000
        collection: notes
`)

	semantic := cfg.KnowledgeBases[0].Indexing["semantic"].(map[string]any)
	if semantic["mode"] != "http" {
		t.Fatalf("expected legacy base_url config to default to http mode, got %#v", semantic["mode"])
	}
	if semantic["base_url"] != "http://127.0.0.1:8000" {
		t.Fatalf("expected base_url to be preserved, got %#v", semantic["base_url"])
	}
	if _, ok := semantic["path"]; ok {
		t.Fatalf("expected legacy http config not to receive persistent path, got %#v", semantic)
	}
}

func TestLoadSQLiteKnowledgeBaseExpandsSemanticPersistentPath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	cfg := loadConfig(t, `knowledge_bases:
  - id: notes
    name: Notes
    store_type: sqlite
    enabled: true
    indexing:
      semantic:
        enabled: true
        provider: chroma
        mode: persistent
        path: ~/custom-chroma
`)

	semantic := cfg.KnowledgeBases[0].Indexing["semantic"].(map[string]any)
	if semantic["path"] != filepath.Join(home, "custom-chroma") {
		t.Fatalf("expected expanded semantic path, got %#v", semantic["path"])
	}
}

func TestLoadSQLiteKnowledgeBasePreservesRelativePath(t *testing.T) {
	cfg := loadConfig(t, `knowledge_bases:
  - id: notes
    name: Notes
    store_type: sqlite
    enabled: true
    store_config:
      path: ./data/notes.db
`)

	if cfg.KnowledgeBases[0].StoreConfig["path"] != "./data/notes.db" {
		t.Fatalf("expected relative sqlite path to remain unchanged, got %#v", cfg.KnowledgeBases[0].StoreConfig["path"])
	}
}

func TestLoadSQLiteKnowledgeBaseRejectsNonStringPath(t *testing.T) {
	_, err := loadConfigError(t, `knowledge_bases:
  - id: notes
    name: Notes
    store_type: sqlite
    enabled: true
    store_config:
      path: 123
`)
	if err == nil {
		t.Fatalf("expected error for non-string sqlite path")
	}
}

func TestLoadSQLiteKnowledgeBaseRejectsNonStringSemanticMode(t *testing.T) {
	_, err := loadConfigError(t, `knowledge_bases:
  - id: notes
    name: Notes
    store_type: sqlite
    enabled: true
    indexing:
      semantic:
        mode: 123
`)
	if err == nil {
		t.Fatalf("expected error for non-string semantic mode")
	}
}

func TestApplyDefaultsTextWithoutSemanticLeavesIndexingAlone(t *testing.T) {
	cfg := config.Config{KnowledgeBases: []config.KnowledgeBaseConfig{
		{ID: "docs", Name: "Docs", StoreType: "text", Enabled: true, StoreConfig: map[string]any{"path": "/tmp/docs"}},
	}}
	if err := config.ApplyDefaults(&cfg); err != nil {
		t.Fatal(err)
	}
	semantic, ok := cfg.KnowledgeBases[0].Indexing["semantic"]
	if ok && semantic != nil {
		m := semantic.(map[string]any)
		if enabled, _ := m["enabled"].(bool); enabled {
			t.Fatalf("text without explicit semantic must not flip enabled true: %#v", m)
		}
	}
}

func TestApplyDefaultsTextWithSemanticFillsChromaPath(t *testing.T) {
	cfg := config.Config{KnowledgeBases: []config.KnowledgeBaseConfig{
		{ID: "docs", Name: "Docs", StoreType: "text", Enabled: true,
			StoreConfig: map[string]any{"path": "/tmp/docs"},
			Indexing:    map[string]any{"semantic": map[string]any{"enabled": true, "provider": "chroma"}}},
	}}
	if err := config.ApplyDefaults(&cfg); err != nil {
		t.Fatal(err)
	}
	sem := cfg.KnowledgeBases[0].Indexing["semantic"].(map[string]any)
	if sem["mode"] != "persistent" {
		t.Fatalf("expected persistent mode default, got %v", sem["mode"])
	}
	path, _ := sem["path"].(string)
	if path == "" || !strings.Contains(path, "chroma") {
		t.Fatalf("expected default chroma path, got %q", path)
	}
}

func TestApplyDefaultsSQLiteUnchanged(t *testing.T) {
	cfg := config.Config{KnowledgeBases: []config.KnowledgeBaseConfig{
		{ID: "default", Name: "Default", StoreType: "sqlite", Enabled: true},
	}}
	if err := config.ApplyDefaults(&cfg); err != nil {
		t.Fatal(err)
	}
	sem := cfg.KnowledgeBases[0].Indexing["semantic"].(map[string]any)
	if enabled, _ := sem["enabled"].(bool); !enabled {
		t.Fatal("sqlite semantic should still default-on")
	}
}

func loadConfig(t *testing.T, content string) config.Config {
	t.Helper()
	cfg, err := loadConfigError(t, content)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	return cfg
}

func loadConfigError(t *testing.T, content string) (config.Config, error) {
	t.Helper()
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "knowledger.yaml")
	if err := os.WriteFile(configPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return config.Load(configPath)
}

func assertDefaultSQLiteKB(t *testing.T, cfg config.Config, expectedPath string) {
	t.Helper()
	if len(cfg.KnowledgeBases) != 1 {
		t.Fatalf("expected 1 knowledge base, got %d", len(cfg.KnowledgeBases))
	}
	kb := cfg.KnowledgeBases[0]
	if kb.ID != config.DefaultKBID {
		t.Fatalf("expected default kb id %q, got %q", config.DefaultKBID, kb.ID)
	}
	if kb.Name != config.DefaultKBName {
		t.Fatalf("expected default kb name %q, got %q", config.DefaultKBName, kb.Name)
	}
	if kb.StoreType != "sqlite" {
		t.Fatalf("expected sqlite store type, got %q", kb.StoreType)
	}
	if !kb.Enabled {
		t.Fatalf("expected default kb to be enabled")
	}
	if kb.StoreConfig["path"] != expectedPath {
		t.Fatalf("expected default sqlite path %q, got %#v", expectedPath, kb.StoreConfig["path"])
	}
	assertSemanticDefaults(t, kb.Indexing, config.DefaultKBID)
}

func assertSemanticDefaults(t *testing.T, indexing map[string]any, collection string) {
	t.Helper()
	lexical, ok := indexing["lexical"].(map[string]any)
	if !ok {
		t.Fatalf("expected lexical indexing map, got %#v", indexing["lexical"])
	}
	if lexical["enabled"] != true {
		t.Fatalf("expected lexical indexing enabled, got %#v", lexical["enabled"])
	}
	semantic, ok := indexing["semantic"].(map[string]any)
	if !ok {
		t.Fatalf("expected semantic indexing map, got %#v", indexing["semantic"])
	}
	expected := map[string]any{
		"enabled":       true,
		"provider":      config.DefaultChromaProvider,
		"mode":          config.DefaultChromaMode,
		"path":          filepath.Join(os.Getenv("HOME"), ".knowledger", "chroma", collection),
		"collection":    collection,
		"auto_download": true,
		"sync_mode":     config.DefaultChromaSyncMode,
	}
	for key, value := range expected {
		if semantic[key] != value {
			t.Fatalf("expected semantic %s=%#v, got %#v", key, value, semantic[key])
		}
	}
}
