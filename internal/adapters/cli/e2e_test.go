package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/kindbrave/claude-knowledger/internal/adapters/cli"
	"github.com/kindbrave/claude-knowledger/internal/app"
	"github.com/kindbrave/claude-knowledger/internal/config"
	"github.com/kindbrave/claude-knowledger/internal/core"
	"github.com/kindbrave/claude-knowledger/internal/service"
)

func TestEndToEndProjectScopeViaCLI(t *testing.T) {
	tmp := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tmp, ".knowledger"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	projectDataDir := filepath.Join(tmp, "data")
	if err := os.MkdirAll(projectDataDir, 0o755); err != nil {
		t.Fatalf("mkdir project data: %v", err)
	}
	globalDataDir := filepath.Join(t.TempDir(), "global-data")
	if err := os.MkdirAll(globalDataDir, 0o755); err != nil {
		t.Fatalf("mkdir global data: %v", err)
	}

	cfg, err := config.Default()
	if err != nil {
		t.Fatalf("default cfg: %v", err)
	}
	cfg.RuntimeRegistryPath = filepath.Join(t.TempDir(), "global", "registry.json")
	svc, err := app.BuildServiceFromConfig(cfg, tmp)
	if err != nil {
		t.Fatalf("BuildServiceFromConfig: %v", err)
	}
	defer svc.Close()

	// Create a project KB and a global KB with the same id.
	if _, err := svc.CreateKnowledgeBase(context.Background(), service.CreateKnowledgeBaseInput{
		Scope: core.ScopeProject, ID: "notes", StoreType: "text", Path: projectDataDir,
	}); err != nil {
		t.Fatalf("CreateKnowledgeBase project: %v", err)
	}
	if _, err := svc.CreateKnowledgeBase(context.Background(), service.CreateKnowledgeBaseInput{
		Scope: core.ScopeGlobal, ID: "notes", StoreType: "text", Path: globalDataDir,
	}); err != nil {
		t.Fatalf("CreateKnowledgeBase global: %v", err)
	}

	// add to project (no --scope, defaults to project because we're in a project dir)
	root := cli.NewRootCommand(svc)
	root.SetArgs([]string{"add", "--kb", "notes", "--title", "PT", "--content", "hello project"})
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	if err := root.Execute(); err != nil {
		t.Fatalf("add project: %v", err)
	}

	// add to global explicitly via --scope global
	root = cli.NewRootCommand(svc)
	root.SetArgs([]string{"--scope", "global", "add", "--kb", "notes", "--title", "GT", "--content", "hello global"})
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	if err := root.Execute(); err != nil {
		t.Fatalf("add global: %v", err)
	}

	// list-kbs --scope-filter project
	out := new(bytes.Buffer)
	root = cli.NewRootCommand(svc)
	root.SetArgs([]string{"list-kbs", "--scope-filter", "project"})
	root.SetOut(out)
	root.SetErr(io.Discard)
	if err := root.Execute(); err != nil {
		t.Fatalf("list-kbs: %v", err)
	}
	var kbs []core.KnowledgeBase
	if err := json.Unmarshal(out.Bytes(), &kbs); err != nil {
		t.Fatalf("decode list-kbs: %v: %s", err, out.String())
	}
	if len(kbs) != 1 || kbs[0].Scope != core.ScopeProject || kbs[0].ID != "notes" {
		t.Fatalf("expected one project notes KB, got %#v", kbs)
	}

	// search default = both scopes
	out.Reset()
	root = cli.NewRootCommand(svc)
	root.SetArgs([]string{"search", "--query", "hello"})
	root.SetOut(out)
	root.SetErr(io.Discard)
	if err := root.Execute(); err != nil {
		t.Fatalf("search: %v", err)
	}
	var sr service.SearchResult
	if err := json.Unmarshal(out.Bytes(), &sr); err != nil {
		t.Fatalf("decode search: %v: %s", err, out.String())
	}
	scopes := map[string]bool{}
	for _, hit := range sr.Hits {
		scopes[hit.Scope] = true
	}
	if !scopes[core.ScopeProject] || !scopes[core.ScopeGlobal] {
		t.Fatalf("expected hits across both scopes, got %#v (hits=%#v)", scopes, sr.Hits)
	}
}
