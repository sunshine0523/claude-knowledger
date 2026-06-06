package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kindbrave/knowledger/internal/config"
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
		"enabled":    true,
		"provider":   config.DefaultChromaProvider,
		"base_url":   config.DefaultChromaBaseURL,
		"collection": collection,
		"sync_mode":  config.DefaultChromaSyncMode,
	}
	for key, value := range expected {
		if semantic[key] != value {
			t.Fatalf("expected semantic %s=%#v, got %#v", key, value, semantic[key])
		}
	}
}
