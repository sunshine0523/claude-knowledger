package web_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	webadapter "github.com/kindbrave/claude-knowledger/internal/adapters/web"
	"github.com/kindbrave/claude-knowledger/internal/app"
	"github.com/kindbrave/claude-knowledger/internal/config"
	"github.com/kindbrave/claude-knowledger/internal/core"
)

func contains(haystack, needle string) bool { return strings.Contains(haystack, needle) }

func TestEndToEndProjectScopeViaWebAPI(t *testing.T) {
	tmp := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tmp, ".knowledger"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	projectDataDir := filepath.Join(tmp, "data")
	if err := os.MkdirAll(projectDataDir, 0o755); err != nil {
		t.Fatalf("mkdir project data: %v", err)
	}
	cfg, err := config.Default()
	if err != nil {
		t.Fatalf("default cfg: %v", err)
	}
	cfg.RuntimeRegistryPath = filepath.Join(t.TempDir(), "global", "registry.json")
	cfg.KnowledgeBases = nil
	svc, err := app.BuildServiceFromConfig(cfg, tmp)
	if err != nil {
		t.Fatalf("BuildServiceFromConfig: %v", err)
	}
	defer svc.Close()
	srv := webadapter.NewServer(svc)

	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/project", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("/api/project status %d body %s", rec.Code, rec.Body.String())
	}
	var projPayload struct {
		Data struct {
			InProject   bool   `json:"in_project"`
			ProjectRoot string `json:"project_root"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &projPayload); err != nil {
		t.Fatalf("decode project: %v", err)
	}
	if !projPayload.Data.InProject || projPayload.Data.ProjectRoot != tmp {
		t.Fatalf("expected in_project=true root=%q, got %#v", tmp, projPayload.Data)
	}

	rec = httptest.NewRecorder()
	body := []byte(`{"id":"notes","store_type":"text","path":"` + projectDataDir + `"}`)
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/kbs", bytes.NewReader(body)))
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST /api/kbs status %d body %s", rec.Code, rec.Body.String())
	}
	var createPayload struct {
		Data struct {
			KnowledgeBase struct {
				ID    string `json:"id"`
				Scope string `json:"scope"`
			} `json:"knowledge_base"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &createPayload); err != nil {
		t.Fatalf("decode create: %v", err)
	}
	if createPayload.Data.KnowledgeBase.Scope != core.ScopeProject {
		t.Fatalf("expected default scope=project, got %q", createPayload.Data.KnowledgeBase.Scope)
	}

	if _, _, _, err := svc.Add(context.Background(), core.AddInput{Scope: core.ScopeProject, KBID: "notes", Title: "T", Content: "hello world"}); err != nil {
		t.Fatalf("svc.Add: %v", err)
	}

	rec = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/kbs/project/notes/items", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET items status %d body %s", rec.Code, rec.Body.String())
	}
	var itemsPayload struct {
		Data struct {
			ItemCount int `json:"item_count"`
			Items     []struct {
				Title   string `json:"title"`
				Content string `json:"content"`
			} `json:"items"`
			KnowledgeBase struct {
				Scope string `json:"scope"`
			} `json:"knowledge_base"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &itemsPayload); err != nil {
		t.Fatalf("decode items: %v", err)
	}
	if itemsPayload.Data.ItemCount != 1 || len(itemsPayload.Data.Items) != 1 {
		t.Fatalf("expected one item, got %#v", itemsPayload.Data)
	}
	if !contains(itemsPayload.Data.Items[0].Content, "hello world") {
		t.Fatalf("expected item content to contain 'hello world', got %q", itemsPayload.Data.Items[0].Content)
	}
	if itemsPayload.Data.KnowledgeBase.Scope != core.ScopeProject {
		t.Fatalf("expected returned KB scope=project, got %q", itemsPayload.Data.KnowledgeBase.Scope)
	}

	rec = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/search", bytes.NewReader([]byte(`{"query":"hello"}`))))
	if rec.Code != http.StatusOK {
		t.Fatalf("POST /api/search status %d body %s", rec.Code, rec.Body.String())
	}
	var searchPayload struct {
		Data struct {
			Hits []struct {
				Scope string `json:"scope"`
				KBID  string `json:"kb_id"`
			} `json:"hits"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &searchPayload); err != nil {
		t.Fatalf("decode search: %v", err)
	}
	foundProject := false
	for _, hit := range searchPayload.Data.Hits {
		if hit.Scope == core.ScopeProject && hit.KBID == "notes" {
			foundProject = true
		}
	}
	if !foundProject {
		t.Fatalf("expected a hit with scope=project kb=notes, got %#v", searchPayload.Data.Hits)
	}
}
