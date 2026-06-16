package cli_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kindbrave/knowledger/internal/adapters/cli"
	"github.com/kindbrave/knowledger/internal/app"
	"github.com/kindbrave/knowledger/internal/config"
	"github.com/kindbrave/knowledger/internal/core"
	"github.com/kindbrave/knowledger/internal/service"
)

type addCommandBackend struct{}

func (addCommandBackend) Add(context.Context, core.KnowledgeBase, core.AddInput) (core.KnowledgeItem, core.IngestionResult, core.IndexStatus, error) {
	return core.KnowledgeItem{ID: "1", KBID: "notes", Title: "title", Content: "content"}, core.IngestionResult{Success: true, ItemID: "1"}, core.IndexStatus{State: "indexed"}, nil
}

func (addCommandBackend) Search(context.Context, core.KnowledgeBase, core.SearchOptions) ([]core.SearchHit, error) {
	return nil, nil
}

func (addCommandBackend) GetItem(context.Context, core.KnowledgeBase, string) (core.KnowledgeItem, error) {
	return core.KnowledgeItem{ID: "1", KBID: "notes", Title: "title", Content: "content"}, nil
}

func (addCommandBackend) ListItems(context.Context, core.KnowledgeBase) ([]core.KnowledgeItem, error) {
	return nil, nil
}

func (addCommandBackend) DeleteItem(context.Context, core.KnowledgeBase, string) error {
	return nil
}

func (addCommandBackend) SupportsSemantic(core.KnowledgeBase) bool { return true }

func TestAddCommandShowsEmbeddedChromaInitializationHint(t *testing.T) {
	stderr := new(bytes.Buffer)
	stdout := new(bytes.Buffer)
	svc := service.New([]core.KnowledgeBase{{
		ID:        "notes",
		Scope:     core.ScopeGlobal,
		StoreType: "sqlite",
		Enabled:   true,
		Indexing: map[string]any{"semantic": map[string]any{
			"enabled":       true,
			"provider":      "chroma",
			"mode":          "persistent",
			"auto_download": true,
		}},
	}}, map[string]core.StoreBackend{"sqlite": addCommandBackend{}})
	cmd := cli.NewRootCommand(svc)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs([]string{"add", "--kb", "notes", "--title", "title", "--content", "content"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	if !strings.Contains(stderr.String(), "Embedded Chroma semantic indexing may download runtime/model files on first use") {
		t.Fatalf("expected embedded Chroma initialization hint on stderr, got %q", stderr.String())
	}
}

func TestAddCommandResolvesProjectScopeByDefaultInProject(t *testing.T) {
	tmp := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tmp, ".knowledger"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	cfg, err := config.Default()
	if err != nil {
		t.Fatalf("default config: %v", err)
	}
	cfg.RuntimeRegistryPath = filepath.Join(t.TempDir(), "global", "registry.json")
	svc, err := app.BuildServiceFromConfig(cfg, tmp)
	if err != nil {
		t.Fatalf("BuildServiceFromConfig: %v", err)
	}
	defer svc.Close()
	got, err := cli.EffectiveScope("", svc.HasProjectScope())
	if err != nil {
		t.Fatalf("EffectiveScope: %v", err)
	}
	if got != core.ScopeProject {
		t.Fatalf("expected project scope, got %q", got)
	}
}
