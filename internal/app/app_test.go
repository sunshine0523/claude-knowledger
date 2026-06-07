package app_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/kindbrave/knowledger/internal/app"
	"github.com/kindbrave/knowledger/internal/config"
	"github.com/kindbrave/knowledger/internal/core"
	"github.com/kindbrave/knowledger/internal/registry"
)

func TestBuildServiceUsesDefaultSQLiteKnowledgeBase(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	configPath := filepath.Join(t.TempDir(), "knowledger.yaml")
	if err := os.WriteFile(configPath, []byte(""), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	svc, err := app.BuildService(configPath)
	if err != nil {
		t.Fatalf("BuildService returned error: %v", err)
	}

	kbs := svc.ListKnowledgeBases()
	if len(kbs) != 1 {
		t.Fatalf("expected 1 knowledge base, got %d", len(kbs))
	}
	if kbs[0].ID != config.DefaultKBID || kbs[0].StoreType != "sqlite" {
		t.Fatalf("expected default sqlite kb, got %#v", kbs[0])
	}

	if err := svc.Close(); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}

	dbPath := filepath.Join(home, ".knowledger", "lexical.db")
	lexicalSvc, err := app.BuildServiceFromConfig(config.Config{KnowledgeBases: []config.KnowledgeBaseConfig{{
		ID:          config.DefaultKBID,
		Name:        config.DefaultKBName,
		StoreType:   "sqlite",
		StoreConfig: map[string]any{"path": dbPath},
		Enabled:     true,
		Indexing:    map[string]any{"semantic": map[string]any{"enabled": false}},
	}}})
	if err != nil {
		t.Fatalf("BuildServiceFromConfig returned error: %v", err)
	}
	defer func() { _ = lexicalSvc.Close() }()

	_, ingest, _, err := lexicalSvc.Add(context.Background(), core.AddInput{KBID: config.DefaultKBID, Title: "Default DB", Content: "SQLite default storage"})
	if err != nil {
		t.Fatalf("Add returned error: %v", err)
	}
	if !ingest.Success {
		t.Fatalf("expected successful ingest")
	}

	result, err := lexicalSvc.Search(context.Background(), core.SearchOptions{Query: "SQLite", SearchMode: "lexical", Limit: 10})
	if err != nil {
		t.Fatalf("Search returned error: %v", err)
	}
	if len(result.Hits) != 1 {
		t.Fatalf("expected 1 search hit, got %d", len(result.Hits))
	}

	if _, err := os.Stat(dbPath); err != nil {
		t.Fatalf("expected sqlite db to exist: %v", err)
	}
}

func TestBuildDefaultService(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	svc, err := app.BuildDefaultService()
	if err != nil {
		t.Fatalf("BuildDefaultService returned error: %v", err)
	}
	kbs := svc.ListKnowledgeBases()
	if len(kbs) != 1 || kbs[0].ID != config.DefaultKBID {
		t.Fatalf("expected default knowledge base, got %#v", kbs)
	}
}

func TestBuildServiceLoadsPersistedRuntimeKnowledgeBases(t *testing.T) {
	runtimePath := filepath.Join(t.TempDir(), "registry.json")
	store := registry.NewFileStore(runtimePath)
	docsPath := t.TempDir()
	if err := store.Save([]registry.RuntimeKnowledgeBase{{ID: "docs", Name: "Docs", StoreType: "text", StoreConfig: map[string]any{"path": docsPath}, Enabled: true}}); err != nil {
		t.Fatalf("Save runtime registry: %v", err)
	}

	svc, err := app.BuildServiceFromConfig(config.Config{RuntimeRegistryPath: runtimePath, KnowledgeBases: []config.KnowledgeBaseConfig{{ID: "default", StoreType: "sqlite", Enabled: true, StoreConfig: map[string]any{"path": filepath.Join(t.TempDir(), "db")}}}})
	if err != nil {
		t.Fatalf("BuildServiceFromConfig returned error: %v", err)
	}
	kbs := svc.ListKnowledgeBases()
	if len(kbs) != 2 {
		t.Fatalf("expected static and runtime KBs, got %#v", kbs)
	}
}

func TestBuildServiceRejectsMultipleSQLitePaths(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cfg := config.Config{KnowledgeBases: []config.KnowledgeBaseConfig{
		{ID: "one", StoreType: "sqlite", Enabled: true, StoreConfig: map[string]any{"path": filepath.Join(t.TempDir(), "one.db")}},
		{ID: "two", StoreType: "sqlite", Enabled: true, StoreConfig: map[string]any{"path": filepath.Join(t.TempDir(), "two.db")}},
	}}

	if _, err := app.BuildServiceFromConfig(cfg); err == nil {
		t.Fatalf("expected error for multiple sqlite paths")
	}
}
