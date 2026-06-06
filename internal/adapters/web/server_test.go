package web_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	webadapter "github.com/kindbrave/knowledger/internal/adapters/web"
	"github.com/kindbrave/knowledger/internal/config"
	"github.com/kindbrave/knowledger/internal/core"
	"github.com/kindbrave/knowledger/internal/registry"
	"github.com/kindbrave/knowledger/internal/service"
)

type fakeBackend struct{}

func (fakeBackend) Add(context.Context, core.KnowledgeBase, core.AddInput) (core.KnowledgeItem, core.IngestionResult, core.IndexStatus, error) {
	return core.KnowledgeItem{ID: "1"}, core.IngestionResult{Success: true, ItemID: "1"}, core.IndexStatus{State: "not_indexed"}, nil
}

func (fakeBackend) Search(context.Context, core.KnowledgeBase, core.SearchOptions) ([]core.SearchHit, error) {
	return nil, nil
}

func (fakeBackend) ListItems(context.Context, core.KnowledgeBase) ([]core.KnowledgeItem, error) {
	return nil, nil
}

func (fakeBackend) SupportsSemantic(core.KnowledgeBase) bool { return false }

func TestDashboardRespondsOK(t *testing.T) {
	srv := webadapter.NewServer(nil)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	res := httptest.NewRecorder()

	srv.Handler().ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", res.Code)
	}
}

func TestAPIListKBsReturnsKnowledgeBases(t *testing.T) {
	srv := webadapter.NewServer(testService(t))
	res := serve(t, srv, http.MethodGet, "/api/kbs", nil)

	if res.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", res.Code)
	}
	var payload map[string]any
	decodeResponse(t, res, &payload)
	if payload["success"] != true {
		t.Fatalf("expected success payload, got %#v", payload)
	}
	body := res.Body.String()
	if !strings.Contains(body, "default") || !strings.Contains(body, "static") {
		t.Fatalf("expected default static KB in response, got %s", body)
	}
}

func TestAPICreateAndDeleteRuntimeKB(t *testing.T) {
	srv := webadapter.NewServer(testService(t))
	docsPath := t.TempDir()
	createBody := []byte(`{"id":"docs","name":"Docs","store_type":"text","path":"` + docsPath + `","enabled":true}`)

	createRes := serve(t, srv, http.MethodPost, "/api/kbs", createBody)
	if createRes.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d body=%s", createRes.Code, createRes.Body.String())
	}
	listRes := serve(t, srv, http.MethodGet, "/api/kbs", nil)
	if !strings.Contains(listRes.Body.String(), "docs") || !strings.Contains(listRes.Body.String(), "runtime") {
		t.Fatalf("expected runtime docs KB after create, got %s", listRes.Body.String())
	}

	deleteRes := serve(t, srv, http.MethodDelete, "/api/kbs/docs", nil)
	if deleteRes.Code != http.StatusOK {
		t.Fatalf("expected 200 delete, got %d body=%s", deleteRes.Code, deleteRes.Body.String())
	}
	listRes = serve(t, srv, http.MethodGet, "/api/kbs", nil)
	if strings.Contains(listRes.Body.String(), "docs") {
		t.Fatalf("expected docs to be deleted, got %s", listRes.Body.String())
	}
}

func TestAPICreateRejectsInvalidRequests(t *testing.T) {
	srv := webadapter.NewServer(testService(t))

	malformed := serve(t, srv, http.MethodPost, "/api/kbs", []byte("{"))
	if malformed.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for malformed JSON, got %d", malformed.Code)
	}
	invalidType := serve(t, srv, http.MethodPost, "/api/kbs", []byte(`{"id":"vec","store_type":"chroma","path":"/tmp","enabled":true}`))
	if invalidType.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid type, got %d body=%s", invalidType.Code, invalidType.Body.String())
	}
	duplicate := serve(t, srv, http.MethodPost, "/api/kbs", []byte(`{"id":"default","store_type":"sqlite","path":"`+filepath.Join(t.TempDir(), "db")+`","enabled":true}`))
	if duplicate.Code != http.StatusConflict {
		t.Fatalf("expected 409 for duplicate, got %d body=%s", duplicate.Code, duplicate.Body.String())
	}
}

func TestAPIDeleteStaticKBReturnsConflict(t *testing.T) {
	srv := webadapter.NewServer(testService(t))
	res := serve(t, srv, http.MethodDelete, "/api/kbs/default", nil)

	if res.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d body=%s", res.Code, res.Body.String())
	}
}

func TestKBPageRendersManagementUI(t *testing.T) {
	svc := testService(t)
	_, err := svc.CreateKnowledgeBase(context.Background(), service.CreateKnowledgeBaseInput{ID: "docs", StoreType: "text", Path: t.TempDir()})
	if err != nil {
		t.Fatalf("CreateKnowledgeBase returned error: %v", err)
	}
	srv := webadapter.NewServer(svc)
	res := serve(t, srv, http.MethodGet, "/kbs", nil)

	if res.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", res.Code)
	}
	body := res.Body.String()
	for _, expected := range []string{"Knowledge Bases", "kb-create-form", "default", "docs", "kb-delete"} {
		if !strings.Contains(body, expected) {
			t.Fatalf("expected page to contain %q, got %s", expected, body)
		}
	}
	if !strings.Contains(body, "Static knowledge bases are read-only") {
		t.Fatalf("expected static delete control to be disabled, got %s", body)
	}
}

func testService(t *testing.T) *service.Service {
	t.Helper()
	static := []config.KnowledgeBaseConfig{{ID: "default", Name: "Default", StoreType: "sqlite", StoreConfig: map[string]any{"path": filepath.Join(t.TempDir(), "db")}, Enabled: true}}
	reg := registry.New(static, registry.NewMemoryStore(nil))
	svc, err := service.NewManaged(reg, func([]core.KnowledgeBase) (map[string]core.StoreBackend, error) {
		return map[string]core.StoreBackend{"text": fakeBackend{}, "sqlite": fakeBackend{}}, nil
	})
	if err != nil {
		t.Fatalf("NewManaged returned error: %v", err)
	}
	return svc
}

func serve(t *testing.T, srv *webadapter.Server, method, path string, body []byte) *httptest.ResponseRecorder {
	t.Helper()
	var reader *bytes.Reader
	if body == nil {
		reader = bytes.NewReader(nil)
	} else {
		reader = bytes.NewReader(body)
	}
	req := httptest.NewRequest(method, path, reader)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res := httptest.NewRecorder()
	srv.Handler().ServeHTTP(res, req)
	return res
}

func decodeResponse(t *testing.T, res *httptest.ResponseRecorder, out any) {
	t.Helper()
	if err := json.Unmarshal(res.Body.Bytes(), out); err != nil {
		t.Fatalf("decode response: %v body=%s", err, res.Body.String())
	}
}
