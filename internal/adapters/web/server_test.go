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

func (fakeBackend) ListItems(_ context.Context, kb core.KnowledgeBase) ([]core.KnowledgeItem, error) {
	return []core.KnowledgeItem{{ID: "1", KBID: kb.ID, Type: "note", Title: "Stored knowledge", Content: "Stored content"}}, nil
}

func (fakeBackend) DeleteItem(context.Context, core.KnowledgeBase, string) error {
	return nil
}

func (fakeBackend) SupportsSemantic(core.KnowledgeBase) bool { return false }

type fakeWebService struct {
	records      []registry.KnowledgeBaseRecord
	items        []core.KnowledgeItem
	listErr      error
	searchResult service.SearchResult
	searchErr    error
	searchCalled bool
	lastSearch   core.SearchOptions
	lastCreate   service.CreateKnowledgeBaseInput
	deletedKB    string
	deletedItem  string
}

func (f *fakeWebService) ListKnowledgeBaseRecords() ([]registry.KnowledgeBaseRecord, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	return f.records, nil
}

func (f *fakeWebService) ListKnowledgeBaseSummaries(context.Context) ([]service.KnowledgeBaseSummary, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	summaries := make([]service.KnowledgeBaseSummary, 0, len(f.records))
	for _, record := range f.records {
		count := 0
		for _, item := range f.items {
			if item.KBID == record.KnowledgeBase.ID {
				count++
			}
		}
		summaries = append(summaries, service.KnowledgeBaseSummary{Record: record, ItemCount: count})
	}
	return summaries, nil
}

func (f *fakeWebService) ListKnowledgeItems(_ context.Context, kbID string) ([]core.KnowledgeItem, error) {
	for _, record := range f.records {
		if record.KnowledgeBase.ID == kbID {
			items := make([]core.KnowledgeItem, 0, len(f.items))
			for _, item := range f.items {
				if item.KBID == kbID {
					items = append(items, item)
				}
			}
			return items, nil
		}
	}
	return nil, &core.Error{Kind: core.ErrorKindConfig, Message: "knowledge base not found"}
}

func (f *fakeWebService) DeleteKnowledgeItem(_ context.Context, kbID string, itemID string) error {
	f.deletedKB = kbID
	f.deletedItem = itemID
	return nil
}

func (f *fakeWebService) CreateKnowledgeBase(_ context.Context, input service.CreateKnowledgeBaseInput) (registry.KnowledgeBaseRecord, error) {
	f.lastCreate = input
	return registry.KnowledgeBaseRecord{KnowledgeBase: core.KnowledgeBase{ID: input.ID, Name: input.Name, StoreType: input.StoreType, StoreConfig: map[string]any{"path": input.Path}, Enabled: true}, Source: registry.SourceRuntime, Deletable: true}, nil
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
	body := res.Body.String()
	for _, expected := range []string{"dashboard-root", "Search Readiness", "Indexing Notes", "dashboard-kbs-body", "/static/app.js"} {
		if !strings.Contains(body, expected) {
			t.Fatalf("expected dashboard page to contain %q, got %s", expected, body)
		}
	}
}

func TestSearchLabRespondsOK(t *testing.T) {
	srv := webadapter.NewServer(nil)
	res := serve(t, srv, http.MethodGet, "/search-lab", nil)

	if res.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", res.Code)
	}
	body := res.Body.String()
	for _, expected := range []string{"search-form", "auto/default", "search-summary", "search-warnings", "search-results-body", "/static/app.js"} {
		if !strings.Contains(body, expected) {
			t.Fatalf("expected search lab page to contain %q, got %s", expected, body)
		}
	}
}

func TestKnowledgePageRespondsOK(t *testing.T) {
	srv := webadapter.NewServer(nil)
	res := serve(t, srv, http.MethodGet, "/knowledge", nil)

	if res.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", res.Code)
	}
	body := res.Body.String()
	for _, expected := range []string{"knowledge-root", "knowledge-kb-select", "knowledge-items-body", "knowledge-content", "/static/app.js"} {
		if !strings.Contains(body, expected) {
			t.Fatalf("expected knowledge page to contain %q, got %s", expected, body)
		}
	}
}

func TestStaticAssetsRespondOK(t *testing.T) {
	srv := webadapter.NewServer(nil)

	js := serve(t, srv, http.MethodGet, "/static/app.js", nil)
	if js.Code != http.StatusOK {
		t.Fatalf("expected app.js 200, got %d body=%s", js.Code, js.Body.String())
	}
	if !strings.Contains(js.Body.String(), `dataset.knowledger = "ready"`) {
		t.Fatalf("expected app.js ready marker, got %s", js.Body.String())
	}

	css := serve(t, srv, http.MethodGet, "/static/styles.css", nil)
	if css.Code != http.StatusOK {
		t.Fatalf("expected styles.css 200, got %d body=%s", css.Code, css.Body.String())
	}
	if !strings.Contains(css.Body.String(), ".search-form") {
		t.Fatalf("expected styles.css search form styles, got %s", css.Body.String())
	}
}

func TestWebPagesAndAssetsDoNotDependOnWorkingDirectory(t *testing.T) {
	t.Chdir(t.TempDir())
	srv := webadapter.NewServer(nil)

	dashboard := serve(t, srv, http.MethodGet, "/", nil)
	if dashboard.Code != http.StatusOK {
		t.Fatalf("expected dashboard 200, got %d body=%s", dashboard.Code, dashboard.Body.String())
	}
	if !strings.Contains(dashboard.Body.String(), "dashboard-root") {
		t.Fatalf("expected dashboard-root after cwd change, got %s", dashboard.Body.String())
	}

	searchLab := serve(t, srv, http.MethodGet, "/search-lab", nil)
	if searchLab.Code != http.StatusOK {
		t.Fatalf("expected search lab 200, got %d body=%s", searchLab.Code, searchLab.Body.String())
	}
	if !strings.Contains(searchLab.Body.String(), "search-form") {
		t.Fatalf("expected search-form after cwd change, got %s", searchLab.Body.String())
	}

	knowledge := serve(t, srv, http.MethodGet, "/knowledge", nil)
	if knowledge.Code != http.StatusOK {
		t.Fatalf("expected knowledge 200, got %d body=%s", knowledge.Code, knowledge.Body.String())
	}
	if !strings.Contains(knowledge.Body.String(), "knowledge-root") {
		t.Fatalf("expected knowledge-root after cwd change, got %s", knowledge.Body.String())
	}

	js := serve(t, srv, http.MethodGet, "/static/app.js", nil)
	if js.Code != http.StatusOK {
		t.Fatalf("expected app.js 200 after cwd change, got %d body=%s", js.Code, js.Body.String())
	}

	css := serve(t, srv, http.MethodGet, "/static/styles.css", nil)
	if css.Code != http.StatusOK {
		t.Fatalf("expected styles.css 200 after cwd change, got %d body=%s", css.Code, css.Body.String())
	}
	if !strings.Contains(css.Body.String(), ".search-form") {
		t.Fatalf("expected styles.css search form styles after cwd change, got %s", css.Body.String())
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
	if !strings.Contains(body, "default") || !strings.Contains(body, "static") || !strings.Contains(body, "item_count") {
		t.Fatalf("expected default static KB with item_count in response, got %s", body)
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

func TestAPISearchAcceptsAutoModeAndReturnsNormalizedRequest(t *testing.T) {
	fake := &fakeWebService{searchResult: service.SearchResult{Hits: []core.SearchHit{}}}
	srv := webadapter.NewServer(fake)
	res := serve(t, srv, http.MethodPost, "/api/search", []byte(`{"query":" sqlite ","kb_ids":[" default ",""," docs "],"search_mode":"auto"}`))

	if res.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", res.Code, res.Body.String())
	}
	if fake.lastSearch.SearchMode != "auto" {
		t.Fatalf("expected auto search mode, got %q", fake.lastSearch.SearchMode)
	}
	if !reflect.DeepEqual(fake.lastSearch.KBIDs, []string{"default", "docs"}) {
		t.Fatalf("expected KBIDs [default docs], got %#v", fake.lastSearch.KBIDs)
	}

	var payload struct {
		Success bool `json:"success"`
		Data    struct {
			Query      string   `json:"query"`
			Limit      int      `json:"limit"`
			KBIDs      []string `json:"kb_ids"`
			SearchMode string   `json:"search_mode"`
		} `json:"data"`
		Meta struct {
			HitCount int `json:"hit_count"`
		} `json:"meta"`
	}
	decodeResponse(t, res, &payload)

	if !payload.Success {
		t.Fatalf("expected success response")
	}
	if payload.Data.Query != "sqlite" {
		t.Fatalf("expected normalized query sqlite, got %q", payload.Data.Query)
	}
	if payload.Data.Limit != 10 {
		t.Fatalf("expected default limit 10, got %d", payload.Data.Limit)
	}
	if !reflect.DeepEqual(payload.Data.KBIDs, []string{"default", "docs"}) {
		t.Fatalf("expected response KBIDs [default docs], got %#v", payload.Data.KBIDs)
	}
	if payload.Data.SearchMode != "auto" {
		t.Fatalf("expected response search mode auto, got %q", payload.Data.SearchMode)
	}
	if payload.Meta.HitCount != 0 {
		t.Fatalf("expected hit_count 0, got %d", payload.Meta.HitCount)
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
			KnowledgeBase: core.KnowledgeBase{ID: "default", Name: "Default", StoreType: "sqlite", StoreConfig: map[string]any{"path": "/tmp/default.db"}, Enabled: true, DefaultSearchMode: "hybrid", Indexing: map[string]any{"lexical": map[string]any{"enabled": true}, "semantic": map[string]any{"enabled": true}}, Tags: []string{"primary"}},
			Source:        registry.SourceStatic,
			Deletable:     false,
		},
		{
			KnowledgeBase: core.KnowledgeBase{ID: "vec", Name: "Vector", StoreType: "chroma", StoreConfig: map[string]any{"path": "/tmp/vec"}, Enabled: true, DefaultSearchMode: "semantic", Tags: []string{"vector"}},
			Source:        registry.SourceStatic,
			Deletable:     false,
		},
		{
			KnowledgeBase: core.KnowledgeBase{ID: "disabled-lex", Name: "Disabled Lexical", StoreType: "sqlite", StoreConfig: map[string]any{"path": "/tmp/disabled-lex.db"}, Enabled: true, DefaultSearchMode: "lexical", Indexing: map[string]any{"lexical": map[string]any{"enabled": false}}, Tags: []string{"sqlite"}},
			Source:        registry.SourceRuntime,
			Deletable:     true,
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
			Readiness struct {
				SearchableKBs            int      `json:"searchable_kbs"`
				LexicalConfiguredKBs     int      `json:"lexical_configured_kbs"`
				SemanticConfiguredKBs    int      `json:"semantic_configured_kbs"`
				SemanticQueryImplemented bool     `json:"semantic_query_implemented"`
				Notes                    []string `json:"notes"`
			} `json:"readiness"`
		} `json:"data"`
	}
	decodeResponse(t, res, &payload)

	if !payload.Success {
		t.Fatalf("expected success payload")
	}
	if payload.Data.Summary.TotalKBs != 4 || payload.Data.Summary.EnabledKBs != 3 || payload.Data.Summary.DisabledKBs != 1 {
		t.Fatalf("unexpected summary counts: %#v", payload.Data.Summary)
	}
	if payload.Data.Summary.StaticKBs != 2 || payload.Data.Summary.RuntimeKBs != 2 {
		t.Fatalf("unexpected source counts: %#v", payload.Data.Summary)
	}
	if payload.Data.Summary.StoreTypes["sqlite"] != 2 || payload.Data.Summary.StoreTypes["text"] != 1 || payload.Data.Summary.StoreTypes["chroma"] != 1 {
		t.Fatalf("unexpected store type counts: %#v", payload.Data.Summary.StoreTypes)
	}
	if len(payload.Data.KnowledgeBases) != 4 || payload.Data.KnowledgeBases[0].ID != "default" || payload.Data.KnowledgeBases[1].ID != "vec" || payload.Data.KnowledgeBases[2].ID != "disabled-lex" || payload.Data.KnowledgeBases[3].ID != "docs" {
		t.Fatalf("unexpected knowledge bases: %#v", payload.Data.KnowledgeBases)
	}
	if payload.Data.Indexing.State != "unsupported" || payload.Data.Failures.State != "unsupported" {
		t.Fatalf("expected dashboard states unsupported, got indexing=%#v failures=%#v", payload.Data.Indexing, payload.Data.Failures)
	}
	if payload.Data.Readiness.SearchableKBs != 2 || payload.Data.Readiness.LexicalConfiguredKBs != 1 || payload.Data.Readiness.SemanticConfiguredKBs != 1 {
		t.Fatalf("unexpected readiness counts: %#v", payload.Data.Readiness)
	}
	if !payload.Data.Readiness.SemanticQueryImplemented {
		t.Fatalf("expected semantic query implementation to be true, got %#v", payload.Data.Readiness)
	}
	if len(payload.Data.Readiness.Notes) != 1 || !strings.Contains(payload.Data.Readiness.Notes[0], "query failures fall back to lexical") {
		t.Fatalf("expected embedded Chroma readiness note, got %#v", payload.Data.Readiness.Notes)
	}
}

func TestAPIKnowledgeItemsListAndDelete(t *testing.T) {
	fake := &fakeWebService{
		records: []registry.KnowledgeBaseRecord{{
			KnowledgeBase: core.KnowledgeBase{ID: "docs", Name: "Docs", StoreType: "text", StoreConfig: map[string]any{"path": "/tmp/docs"}, Enabled: true},
			Source:        registry.SourceRuntime,
			Deletable:     true,
		}},
		items: []core.KnowledgeItem{{ID: "item-1", KBID: "docs", Type: "document", Title: "Doc", Content: "Doc body", Tags: []string{"team"}}},
	}
	srv := webadapter.NewServer(fake)

	listRes := serve(t, srv, http.MethodGet, "/api/kbs/docs/items", nil)
	if listRes.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", listRes.Code, listRes.Body.String())
	}
	var payload struct {
		Success bool `json:"success"`
		Data    struct {
			ItemCount int `json:"item_count"`
			Items     []struct {
				ID      string   `json:"id"`
				KBID    string   `json:"kb_id"`
				Title   string   `json:"title"`
				Content string   `json:"content"`
				Tags    []string `json:"tags"`
			} `json:"items"`
		} `json:"data"`
	}
	decodeResponse(t, listRes, &payload)
	if !payload.Success || payload.Data.ItemCount != 1 || len(payload.Data.Items) != 1 || payload.Data.Items[0].Content != "Doc body" {
		t.Fatalf("unexpected item list payload: %#v", payload)
	}

	deleteRes := serve(t, srv, http.MethodDelete, "/api/kbs/docs/items/item-1", nil)
	if deleteRes.Code != http.StatusOK {
		t.Fatalf("expected 200 delete, got %d body=%s", deleteRes.Code, deleteRes.Body.String())
	}
	if fake.deletedKB != "docs" || fake.deletedItem != "item-1" {
		t.Fatalf("expected delete docs/item-1, got %q/%q", fake.deletedKB, fake.deletedItem)
	}
}

func TestAPIKnowledgeItemsMissingKBReturnsNotFound(t *testing.T) {
	srv := webadapter.NewServer(&fakeWebService{})
	res := serve(t, srv, http.MethodGet, "/api/kbs/missing/items", nil)
	if res.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d body=%s", res.Code, res.Body.String())
	}
	assertAPIErrorCode(t, res, "kb_not_found")
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

func TestAPICreatePassesSemanticEnabled(t *testing.T) {
	fake := &fakeWebService{}
	srv := webadapter.NewServer(fake)
	res := serve(t, srv, http.MethodPost, "/api/kbs", []byte(`{"id":"notes","store_type":"sqlite","path":"/tmp/notes.db","semantic_enabled":false}`))
	if res.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d body=%s", res.Code, res.Body.String())
	}
	if fake.lastCreate.SemanticEnabled == nil || *fake.lastCreate.SemanticEnabled != false {
		t.Fatalf("expected semantic_enabled=false to be passed through, got %#v", fake.lastCreate.SemanticEnabled)
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
	for _, expected := range []string{"Knowledge Bases", "kb-create-form", "default", "docs", "kb-delete", "Knowledge Count", "language-select", "/knowledge", "semantic_enabled", "Enable embedded Chroma semantic search for SQLite"} {
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
