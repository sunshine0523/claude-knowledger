package web_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
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

type fakeWebService struct {
	records      []registry.KnowledgeBaseRecord
	listErr      error
	searchResult service.SearchResult
	searchErr    error
	searchCalled bool
	lastSearch   core.SearchOptions
}

func (f *fakeWebService) ListKnowledgeBaseRecords() ([]registry.KnowledgeBaseRecord, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	return f.records, nil
}

func (f *fakeWebService) CreateKnowledgeBase(context.Context, service.CreateKnowledgeBaseInput) (registry.KnowledgeBaseRecord, error) {
	return registry.KnowledgeBaseRecord{}, nil
}

func (f *fakeWebService) DeleteKnowledgeBase(context.Context, string) error {
	return nil
}

func (f *fakeWebService) Search(_ context.Context, opt core.SearchOptions) (service.SearchResult, error) {
	f.searchCalled = true
	f.lastSearch = opt
	if f.searchErr != nil {
		return service.SearchResult{}, f.searchErr
	}
	return f.searchResult, nil
}

func TestDashboardRespondsOK(t *testing.T) {
	srv := webadapter.NewServer(nil)
	res := serve(t, srv, http.MethodGet, "/", nil)

	if res.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", res.Code)
	}
	if !strings.Contains(res.Body.String(), "dashboard-root") {
		t.Fatalf("expected dashboard page to contain dashboard-root, got %s", res.Body.String())
	}
}

func TestSearchLabRespondsOK(t *testing.T) {
	srv := webadapter.NewServer(nil)
	res := serve(t, srv, http.MethodGet, "/search-lab", nil)

	if res.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", res.Code)
	}
	if !strings.Contains(res.Body.String(), "search-form") {
		t.Fatalf("expected search lab page to contain search-form, got %s", res.Body.String())
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

func TestAPISearchReturnsServiceUnavailableWithoutSearchService(t *testing.T) {
	srv := webadapter.NewServer(nil)
	res := serve(t, srv, http.MethodPost, "/api/search", []byte(`{"query":"sqlite"}`))

	if res.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d body=%s", res.Code, res.Body.String())
	}
	assertAPIErrorCode(t, res, "service_unavailable")
}

func TestAPISearchRejectsInvalidRequests(t *testing.T) {
	srv := webadapter.NewServer(&fakeWebService{})

	cases := []struct {
		name string
		body []byte
		code string
	}{
		{name: "invalid json", body: []byte("{"), code: "invalid_json"},
		{name: "empty query", body: []byte(`{"query":"   "}`), code: "invalid_query"},
		{name: "zero limit", body: []byte(`{"query":"sqlite","limit":0}`), code: "invalid_limit"},
		{name: "negative limit", body: []byte(`{"query":"sqlite","limit":-1}`), code: "invalid_limit"},
		{name: "limit too large", body: []byte(`{"query":"sqlite","limit":101}`), code: "invalid_limit"},
		{name: "invalid search mode", body: []byte(`{"query":"sqlite","limit":10,"search_mode":"vector"}`), code: "invalid_search_mode"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := serve(t, srv, http.MethodPost, "/api/search", tc.body)
			if res.Code != http.StatusBadRequest {
				t.Fatalf("expected 400, got %d body=%s", res.Code, res.Body.String())
			}
			assertAPIErrorCode(t, res, tc.code)
		})
	}
}

func TestAPISearchReturnsHitsAndPassesOptions(t *testing.T) {
	fake := &fakeWebService{searchResult: service.SearchResult{
		Hits: []core.SearchHit{{
			ItemID:         "item-1",
			KBID:           "default",
			ItemType:       "note",
			Title:          "Default DB",
			Snippet:        "SQLite default storage",
			ContentPreview: "SQLite default storage content",
			Score:          0.75,
			MatchMode:      "lexical",
			SourceBackend:  "sqlite",
			Locator:        "knowledge_items:1",
			Metadata:       map[string]any{"source": "test"},
		}},
		Warnings: []string{"semantic path unavailable, lexical fallback used"},
	}}
	srv := webadapter.NewServer(fake)
	res := serve(t, srv, http.MethodPost, "/api/search", []byte(`{"query":" sqlite ","limit":5,"kb_ids":["default","docs"],"search_mode":"hybrid"}`))

	if res.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", res.Code, res.Body.String())
	}
	if !fake.searchCalled {
		t.Fatalf("expected Search to be called")
	}
	if fake.lastSearch.Query != "sqlite" {
		t.Fatalf("expected trimmed query sqlite, got %q", fake.lastSearch.Query)
	}
	if fake.lastSearch.Limit != 5 {
		t.Fatalf("expected limit 5, got %d", fake.lastSearch.Limit)
	}
	if !reflect.DeepEqual(fake.lastSearch.KBIDs, []string{"default", "docs"}) {
		t.Fatalf("expected KBIDs [default docs], got %#v", fake.lastSearch.KBIDs)
	}
	if fake.lastSearch.SearchMode != "hybrid" {
		t.Fatalf("expected hybrid search mode, got %q", fake.lastSearch.SearchMode)
	}

	var payload struct {
		Success  bool     `json:"success"`
		Warnings []string `json:"warnings"`
		Data     struct {
			Query string `json:"query"`
			Limit int    `json:"limit"`
			Hits  []struct {
				ItemID        string         `json:"item_id"`
				KBID          string         `json:"kb_id"`
				ItemType      string         `json:"item_type"`
				Title         string         `json:"title"`
				Snippet       string         `json:"snippet"`
				Score         float64        `json:"score"`
				MatchMode     string         `json:"match_mode"`
				SourceBackend string         `json:"source_backend"`
				Locator       string         `json:"locator"`
				Metadata      map[string]any `json:"metadata"`
			} `json:"hits"`
		} `json:"data"`
		Meta struct {
			HitCount int `json:"hit_count"`
		} `json:"meta"`
	}
	decodeResponse(t, res, &payload)

	if !payload.Success {
		t.Fatalf("expected success response")
	}
	if payload.Data.Query != "sqlite" || payload.Data.Limit != 5 {
		t.Fatalf("unexpected normalized request in response: %#v", payload.Data)
	}
	if payload.Meta.HitCount != 1 {
		t.Fatalf("expected hit_count 1, got %d", payload.Meta.HitCount)
	}
	if len(payload.Warnings) != 1 || payload.Warnings[0] != "semantic path unavailable, lexical fallback used" {
		t.Fatalf("expected warning to round-trip, got %#v", payload.Warnings)
	}
	if len(payload.Data.Hits) != 1 || payload.Data.Hits[0].ItemID != "item-1" || payload.Data.Hits[0].MatchMode != "lexical" {
		t.Fatalf("unexpected hits: %#v", payload.Data.Hits)
	}
}

func TestAPIDashboardReturnsServiceUnavailableWithoutService(t *testing.T) {
	srv := webadapter.NewServer(nil)
	res := serve(t, srv, http.MethodGet, "/api/dashboard", nil)

	if res.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d body=%s", res.Code, res.Body.String())
	}
	assertAPIErrorCode(t, res, "service_unavailable")
}

func TestAPIDashboardReturnsKnowledgeBaseSummary(t *testing.T) {
	fake := &fakeWebService{records: []registry.KnowledgeBaseRecord{
		{
			KnowledgeBase: core.KnowledgeBase{ID: "default", Name: "Default", StoreType: "sqlite", StoreConfig: map[string]any{"path": "/tmp/default.db"}, Enabled: true, DefaultSearchMode: "hybrid", Tags: []string{"primary"}},
			Source:        registry.SourceStatic,
			Deletable:     false,
		},
		{
			KnowledgeBase: core.KnowledgeBase{ID: "docs", Name: "Docs", StoreType: "text", StoreConfig: map[string]any{"path": "/tmp/docs"}, Enabled: false, DefaultSearchMode: "lexical", Tags: []string{"docs", "team"}},
			Source:        registry.SourceRuntime,
			Deletable:     true,
		},
	}}
	srv := webadapter.NewServer(fake)
	res := serve(t, srv, http.MethodGet, "/api/dashboard", nil)

	if res.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", res.Code, res.Body.String())
	}
	var payload struct {
		Success bool `json:"success"`
		Data    struct {
			Summary struct {
				TotalKBs    int            `json:"total_kbs"`
				EnabledKBs  int            `json:"enabled_kbs"`
				DisabledKBs int            `json:"disabled_kbs"`
				RuntimeKBs  int            `json:"runtime_kbs"`
				StaticKBs   int            `json:"static_kbs"`
				StoreTypes  map[string]int `json:"store_types"`
			} `json:"summary"`
			KnowledgeBases []struct {
				ID                string   `json:"id"`
				StoreType         string   `json:"store_type"`
				Path              string   `json:"path"`
				Enabled           bool     `json:"enabled"`
				DefaultSearchMode string   `json:"default_search_mode"`
				Tags              []string `json:"tags"`
				Source            string   `json:"source"`
				Deletable         bool     `json:"deletable"`
			} `json:"knowledge_bases"`
			Indexing struct {
				State   string `json:"state"`
				Message string `json:"message"`
			} `json:"indexing"`
			Failures struct {
				State   string `json:"state"`
				Message string `json:"message"`
			} `json:"failures"`
		} `json:"data"`
	}
	decodeResponse(t, res, &payload)

	if !payload.Success {
		t.Fatalf("expected success payload")
	}
	if payload.Data.Summary.TotalKBs != 2 || payload.Data.Summary.EnabledKBs != 1 || payload.Data.Summary.DisabledKBs != 1 {
		t.Fatalf("unexpected summary counts: %#v", payload.Data.Summary)
	}
	if payload.Data.Summary.StaticKBs != 1 || payload.Data.Summary.RuntimeKBs != 1 {
		t.Fatalf("unexpected source counts: %#v", payload.Data.Summary)
	}
	if payload.Data.Summary.StoreTypes["sqlite"] != 1 || payload.Data.Summary.StoreTypes["text"] != 1 {
		t.Fatalf("unexpected store type counts: %#v", payload.Data.Summary.StoreTypes)
	}
	if len(payload.Data.KnowledgeBases) != 2 || payload.Data.KnowledgeBases[0].ID != "default" || payload.Data.KnowledgeBases[1].ID != "docs" {
		t.Fatalf("unexpected knowledge bases: %#v", payload.Data.KnowledgeBases)
	}
	if payload.Data.Indexing.State != "unsupported" || payload.Data.Failures.State != "unsupported" {
		t.Fatalf("expected unsupported indexing/failures, got indexing=%#v failures=%#v", payload.Data.Indexing, payload.Data.Failures)
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

func assertAPIErrorCode(t *testing.T, res *httptest.ResponseRecorder, expected string) {
	t.Helper()
	var payload struct {
		Errors []struct {
			Code string `json:"code"`
		} `json:"errors"`
	}
	decodeResponse(t, res, &payload)
	if len(payload.Errors) == 0 {
		t.Fatalf("expected error code %q, got no errors in %s", expected, res.Body.String())
	}
	if payload.Errors[0].Code != expected {
		t.Fatalf("expected error code %q, got %q body=%s", expected, payload.Errors[0].Code, res.Body.String())
	}
}
