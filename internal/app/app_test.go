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
	"github.com/kindbrave/knowledger/internal/service"
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

func TestBuildServiceAllowsMultipleSQLitePaths(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	root := t.TempDir()
	onePath := filepath.Join(root, "one.db")
	twoPath := filepath.Join(root, "two.db")
	cfg := config.Config{KnowledgeBases: []config.KnowledgeBaseConfig{
		{ID: "one", StoreType: "sqlite", Enabled: true, StoreConfig: map[string]any{"path": onePath}, Indexing: map[string]any{"semantic": map[string]any{"enabled": false}}},
		{ID: "two", StoreType: "sqlite", Enabled: true, StoreConfig: map[string]any{"path": twoPath}, Indexing: map[string]any{"semantic": map[string]any{"enabled": false}}},
	}}

	svc, err := app.BuildServiceFromConfig(cfg)
	if err != nil {
		t.Fatalf("BuildServiceFromConfig returned error: %v", err)
	}
	defer func() { _ = svc.Close() }()

	if _, _, _, err := svc.Add(context.Background(), core.AddInput{KBID: "one", Title: "First", Content: "only in first"}); err != nil {
		t.Fatalf("Add one returned error: %v", err)
	}
	if _, _, _, err := svc.Add(context.Background(), core.AddInput{KBID: "two", Title: "Second", Content: "only in second"}); err != nil {
		t.Fatalf("Add two returned error: %v", err)
	}
	oneItems, err := svc.ListKnowledgeItems(context.Background(), "one")
	if err != nil {
		t.Fatalf("ListKnowledgeItems one returned error: %v", err)
	}
	twoItems, err := svc.ListKnowledgeItems(context.Background(), "two")
	if err != nil {
		t.Fatalf("ListKnowledgeItems two returned error: %v", err)
	}
	if len(oneItems) != 1 || oneItems[0].Title != "First" {
		t.Fatalf("unexpected one items: %#v", oneItems)
	}
	if len(twoItems) != 1 || twoItems[0].Title != "Second" {
		t.Fatalf("unexpected two items: %#v", twoItems)
	}
	if _, err := os.Stat(onePath); err != nil {
		t.Fatalf("expected first sqlite db to exist: %v", err)
	}
	if _, err := os.Stat(twoPath); err != nil {
		t.Fatalf("expected second sqlite db to exist: %v", err)
	}
}

func TestRunDefaultInvokesMCPRunner(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	called := false
	restore := app.SetMCPRunnerForTest(func(svc *service.Service) error {
		called = true
		if svc == nil {
			t.Fatalf("expected service")
		}
		if len(svc.ListKnowledgeBases()) != 1 {
			t.Fatalf("expected default knowledge base, got %#v", svc.ListKnowledgeBases())
		}
		return nil
	})
	defer restore()

	if err := app.RunDefault([]string{"mcp"}); err != nil {
		t.Fatalf("RunDefault returned error: %v", err)
	}
	if !called {
		t.Fatalf("expected MCP runner to be called")
	}
}
