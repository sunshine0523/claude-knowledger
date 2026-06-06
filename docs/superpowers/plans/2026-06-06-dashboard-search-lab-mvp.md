# Dashboard 与 Search Lab MVP Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 让 Knowledger Web Dashboard 展示知识库维度总览，并让 Search Lab 通过 Web API 调用现有 `Service.Search` 执行真实搜索。

**Architecture:** 在 `internal/adapters/web/server.go` 内按现有 Web API 模式新增 `POST /api/search` 和 `GET /api/dashboard`。页面继续使用 Go templates 提供静态骨架，`web/static/app.js` 在浏览器中 fetch API 并安全渲染动态内容。MVP 只做 KB 维度统计和搜索调试表格，不新增 service 层 dashboard abstraction，也不接真实 indexing/failure metrics。

**Tech Stack:** Go `net/http`、Go templates、Go tests with `httptest`、vanilla JavaScript、现有 CSS。

---

## 文件结构与职责

- Modify: `internal/adapters/web/server.go`
  - 扩展 Web adapter service interface，增加 search 能力。
  - 注册 `POST /api/search` 和 `GET /api/dashboard`。
  - 新增 search/dashboard request/response view types、validation、summary 计算、JSON response helper。

- Modify: `internal/adapters/web/server_test.go`
  - 增加 fake Web service，用于直接验证 Web adapter 是否把搜索请求正确传给 service。
  - 增加 `/api/search` validation/success tests。
  - 增加 `/api/dashboard` summary tests。
  - 扩展 route/template marker tests。

- Modify: `web/templates/dashboard.html`
  - 从占位页改成 Dashboard 数据容器、统计卡片、store type 区域、KB 明细表、unsupported 状态块。

- Modify: `web/templates/search_lab.html`
  - 从占位页改成查询栏 + 调试表格布局，并加载 `/static/app.js`。

- Modify: `web/static/app.js`
  - 保留现有 KB create/delete 行为。
  - 新增 Dashboard fetch/render。
  - 新增 Search Lab form submit/render。
  - 所有动态内容用 `textContent` 或 DOM text node 渲染。

- Modify: `web/static/styles.css`
  - 增加统计卡片、响应式表单、表格 wrapper、状态块、summary/warnings 的轻量样式。

---

### Task 1: 为 `POST /api/search` 写失败测试

**Files:**
- Modify: `internal/adapters/web/server_test.go`

- [ ] **Step 1: 扩展测试 imports**

在 `internal/adapters/web/server_test.go` 的 import block 中加入 `reflect`：

```go
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
```

- [ ] **Step 2: 在 `fakeBackend` 后新增 fake Web service**

把下面代码放到 `fakeBackend` 定义之后、现有测试函数之前：

```go
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
```

- [ ] **Step 3: 新增 search API tests**

把下面测试追加到 `TestAPIListKBsReturnsKnowledgeBases` 后面：

```go
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
```

- [ ] **Step 4: 新增 API error assertion helper**

把下面 helper 放到 `decodeResponse` 后面：

```go
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
```

- [ ] **Step 5: 运行 search API tests，确认失败**

Run:

```bash
go test ./internal/adapters/web -run 'TestAPISearch' -count=1
```

Expected: FAIL。典型失败是返回 `404` 而不是 `503/400/200`，因为 `/api/search` 尚未注册。

---

### Task 2: 实现 `POST /api/search`

**Files:**
- Modify: `internal/adapters/web/server.go`

- [ ] **Step 1: 更新 imports**

把 `internal/adapters/web/server.go` 的 imports 改成：

```go
import (
	"context"
	"encoding/json"
	"html/template"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/kindbrave/knowledger/internal/core"
	"github.com/kindbrave/knowledger/internal/registry"
	"github.com/kindbrave/knowledger/internal/service"
)
```

- [ ] **Step 2: 扩展 service interface**

把当前 `knowledgeBaseService` 替换为：

```go
type webService interface {
	ListKnowledgeBaseRecords() ([]registry.KnowledgeBaseRecord, error)
	CreateKnowledgeBase(context.Context, service.CreateKnowledgeBaseInput) (registry.KnowledgeBaseRecord, error)
	DeleteKnowledgeBase(context.Context, string) error
	Search(context.Context, core.SearchOptions) (service.SearchResult, error)
}
```

同时把 `Server` 中的 `svc` 类型改为：

```go
type Server struct {
	tmpl *template.Template
	mux  *http.ServeMux
	svc  webService
}
```

- [ ] **Step 3: 新增 search view/request types**

在 `createKBRequest` 后面加入：

```go
const (
	defaultSearchLimit = 10
	maxSearchLimit     = 100
)

type searchRequest struct {
	Query      string   `json:"query"`
	Limit      *int     `json:"limit"`
	KBIDs      []string `json:"kb_ids"`
	SearchMode string   `json:"search_mode"`
}

type searchHitView struct {
	ItemID         string         `json:"item_id"`
	KBID           string         `json:"kb_id"`
	ItemType       string         `json:"item_type"`
	Title          string         `json:"title"`
	Snippet        string         `json:"snippet"`
	ContentPreview string         `json:"content_preview"`
	Score          float64        `json:"score"`
	MatchMode      string         `json:"match_mode"`
	SourceBackend  string         `json:"source_backend"`
	Locator        string         `json:"locator"`
	Metadata       map[string]any `json:"metadata"`
}
```

- [ ] **Step 4: 更新 `NewServer` type assertion 和 route 注册**

把 `NewServer` 中的 type assertion 改成：

```go
	if typed, ok := svc.(webService); ok {
		s.svc = typed
	}
```

在 API routes 中加入 `POST /api/search`：

```go
	mux.HandleFunc("GET /api/kbs", s.apiListKBs)
	mux.HandleFunc("POST /api/kbs", s.apiCreateKB)
	mux.HandleFunc("DELETE /api/kbs/{id}", s.apiDeleteKB)
	mux.HandleFunc("POST /api/search", s.apiSearch)
```

- [ ] **Step 5: 新增 `apiSearch` handler**

把下面函数放到 `apiDeleteKB` 后面：

```go
func (s *Server) apiSearch(w http.ResponseWriter, r *http.Request) {
	if s.svc == nil {
		writeAPIError(w, http.StatusServiceUnavailable, "service_unavailable", "knowledge base service is unavailable")
		return
	}
	var req searchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid_json", "request body must be valid JSON")
		return
	}

	query := strings.TrimSpace(req.Query)
	if query == "" {
		writeAPIError(w, http.StatusBadRequest, "invalid_query", "query is required")
		return
	}

	limit := defaultSearchLimit
	if req.Limit != nil {
		limit = *req.Limit
	}
	if limit < 1 || limit > maxSearchLimit {
		writeAPIError(w, http.StatusBadRequest, "invalid_limit", "limit must be between 1 and 100")
		return
	}

	searchMode := strings.TrimSpace(req.SearchMode)
	if !validSearchMode(searchMode) {
		writeAPIError(w, http.StatusBadRequest, "invalid_search_mode", "search_mode must be lexical, semantic, hybrid, or empty")
		return
	}

	result, err := s.svc.Search(r.Context(), core.SearchOptions{
		Query:      query,
		Limit:      limit,
		KBIDs:      cleanKBIDs(req.KBIDs),
		SearchMode: searchMode,
	})
	if err != nil {
		writeAPIError(w, statusForError(err), codeForSearchError(err), err.Error())
		return
	}

	hits := searchHitsToViews(result.Hits)
	writeAPISuccessWithMeta(
		w,
		http.StatusOK,
		map[string]any{"query": query, "limit": limit, "hits": hits},
		result.Warnings,
		map[string]any{"hit_count": len(hits)},
	)
}
```

- [ ] **Step 6: 新增 search helper functions**

把下面 helper 放到 `recordToView` 后面：

```go
func cleanKBIDs(ids []string) []string {
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id != "" {
			out = append(out, id)
		}
	}
	return out
}

func validSearchMode(mode string) bool {
	switch mode {
	case "", "lexical", "semantic", "hybrid":
		return true
	default:
		return false
	}
}

func searchHitsToViews(hits []core.SearchHit) []searchHitView {
	out := make([]searchHitView, 0, len(hits))
	for _, hit := range hits {
		out = append(out, searchHitView{
			ItemID:         hit.ItemID,
			KBID:           hit.KBID,
			ItemType:       hit.ItemType,
			Title:          hit.Title,
			Snippet:        hit.Snippet,
			ContentPreview: hit.ContentPreview,
			Score:          hit.Score,
			MatchMode:      hit.MatchMode,
			SourceBackend:  hit.SourceBackend,
			Locator:        hit.Locator,
			Metadata:       hit.Metadata,
		})
	}
	return out
}

func codeForSearchError(err error) string {
	status := statusForError(err)
	if status == http.StatusInternalServerError {
		return "search_failed"
	}
	return codeForError(err)
}
```

- [ ] **Step 7: 新增 success response helper**

把 `writeAPISuccess` 替换为下面两个函数：

```go
func writeAPISuccess(w http.ResponseWriter, status int, data any) {
	writeAPISuccessWithMeta(w, status, data, []string{}, map[string]any{})
}

func writeAPISuccessWithMeta(w http.ResponseWriter, status int, data any, warnings []string, meta any) {
	if warnings == nil {
		warnings = []string{}
	}
	if meta == nil {
		meta = map[string]any{}
	}
	writeJSON(w, status, apiResponse{Success: true, Data: data, Warnings: warnings, Errors: []apiError{}, Meta: meta})
}
```

- [ ] **Step 8: 运行 search API tests，确认通过**

Run:

```bash
go test ./internal/adapters/web -run 'TestAPISearch' -count=1
```

Expected: PASS。

- [ ] **Step 9: 提交 search API**

```bash
git add internal/adapters/web/server.go internal/adapters/web/server_test.go
git commit -m "feat: add web search API"
```

---

### Task 3: 为 `GET /api/dashboard` 和 route markers 写失败测试

**Files:**
- Modify: `internal/adapters/web/server_test.go`

- [ ] **Step 1: 更新 Dashboard route test**

把现有 `TestDashboardRespondsOK` 替换为：

```go
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
```

- [ ] **Step 2: 新增 Search Lab route marker test**

把下面测试放在 Dashboard route test 后面：

```go
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
```

- [ ] **Step 3: 新增 dashboard API tests**

把下面测试放在 search API tests 后面：

```go
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
```

- [ ] **Step 4: 运行 dashboard/route tests，确认失败**

Run:

```bash
go test ./internal/adapters/web -run 'TestDashboardRespondsOK|TestSearchLabRespondsOK|TestAPIDashboard' -count=1
```

Expected: FAIL。典型失败包括 `/api/dashboard` 返回 `404`，Dashboard/Search Lab 模板中缺少 `dashboard-root` 和 `search-form`。

---

### Task 4: 实现 `GET /api/dashboard`

**Files:**
- Modify: `internal/adapters/web/server.go`

- [ ] **Step 1: 新增 dashboard response types**

在 `searchHitView` 后面加入：

```go
type dashboardSummary struct {
	TotalKBs    int            `json:"total_kbs"`
	EnabledKBs  int            `json:"enabled_kbs"`
	DisabledKBs int            `json:"disabled_kbs"`
	RuntimeKBs  int            `json:"runtime_kbs"`
	StaticKBs   int            `json:"static_kbs"`
	StoreTypes  map[string]int `json:"store_types"`
}

type dashboardStatus struct {
	State   string `json:"state"`
	Message string `json:"message"`
}
```

- [ ] **Step 2: 注册 dashboard API route**

在 `NewServer` 的 API routes 中加入：

```go
	mux.HandleFunc("GET /api/dashboard", s.apiDashboard)
```

最终 API route block 应包含：

```go
	mux.HandleFunc("GET /api/kbs", s.apiListKBs)
	mux.HandleFunc("POST /api/kbs", s.apiCreateKB)
	mux.HandleFunc("DELETE /api/kbs/{id}", s.apiDeleteKB)
	mux.HandleFunc("POST /api/search", s.apiSearch)
	mux.HandleFunc("GET /api/dashboard", s.apiDashboard)
```

- [ ] **Step 3: 新增 `apiDashboard` handler**

把下面函数放到 `apiSearch` 后面：

```go
func (s *Server) apiDashboard(w http.ResponseWriter, r *http.Request) {
	if s.svc == nil {
		writeAPIError(w, http.StatusServiceUnavailable, "service_unavailable", "knowledge base service is unavailable")
		return
	}
	records, err := s.svc.ListKnowledgeBaseRecords()
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "list_kbs_failed", err.Error())
		return
	}
	views := recordsToViews(records)
	writeAPISuccess(w, http.StatusOK, map[string]any{
		"summary":         dashboardSummaryFromViews(views),
		"knowledge_bases": views,
		"indexing": dashboardStatus{
			State:   "unsupported",
			Message: "Index queue metrics are not exposed in the web dashboard MVP.",
		},
		"failures": dashboardStatus{
			State:   "unsupported",
			Message: "Recent indexing failures are not exposed in the web dashboard MVP.",
		},
	})
}
```

- [ ] **Step 4: 新增 dashboard summary helper**

把下面函数放到 `searchHitsToViews` 后面：

```go
func dashboardSummaryFromViews(views []kbView) dashboardSummary {
	summary := dashboardSummary{StoreTypes: map[string]int{}}
	for _, view := range views {
		summary.TotalKBs++
		if view.Enabled {
			summary.EnabledKBs++
		} else {
			summary.DisabledKBs++
		}
		switch view.Source {
		case registry.SourceRuntime:
			summary.RuntimeKBs++
		case registry.SourceStatic:
			summary.StaticKBs++
		}
		if view.StoreType != "" {
			summary.StoreTypes[view.StoreType]++
		}
	}
	return summary
}
```

- [ ] **Step 5: 运行 dashboard API test，确认 API 通过但 route markers 仍可能失败**

Run:

```bash
go test ./internal/adapters/web -run 'TestAPIDashboard' -count=1
```

Expected: PASS。

- [ ] **Step 6: 提交 dashboard API**

```bash
git add internal/adapters/web/server.go internal/adapters/web/server_test.go
git commit -m "feat: add dashboard summary API"
```

---

### Task 5: 更新 Dashboard 与 Search Lab templates

**Files:**
- Modify: `web/templates/dashboard.html`
- Modify: `web/templates/search_lab.html`

- [ ] **Step 1: 替换 Dashboard template**

把 `web/templates/dashboard.html` 全部替换为：

```html
{{ define "dashboard.html" }}
<!doctype html>
<html lang="zh-CN">
  <head>
    <meta charset="utf-8">
    <title>Knowledger Dashboard</title>
    <link rel="stylesheet" href="/static/styles.css">
    <script defer src="/static/app.js"></script>
  </head>
  <body>
    {{ template "nav" . }}
    <main>
      <h1>Knowledger Dashboard</h1>
      <p>查看知识库总览、store type 分布和当前 Web MVP 尚未接入的索引状态。</p>

      <section id="dashboard-root" class="dashboard">
        <p id="dashboard-message" class="message" hidden></p>

        <div class="stats-grid" aria-label="Knowledge base summary">
          <article class="stat-card"><span class="stat-label">Total KBs</span><strong id="stat-total-kbs">—</strong></article>
          <article class="stat-card"><span class="stat-label">Enabled</span><strong id="stat-enabled-kbs">—</strong></article>
          <article class="stat-card"><span class="stat-label">Disabled</span><strong id="stat-disabled-kbs">—</strong></article>
          <article class="stat-card"><span class="stat-label">Runtime</span><strong id="stat-runtime-kbs">—</strong></article>
          <article class="stat-card"><span class="stat-label">Static</span><strong id="stat-static-kbs">—</strong></article>
        </div>

        <section class="panel">
          <h2>Store Types</h2>
          <div id="store-types" class="tag-list"><span class="muted">Loading...</span></div>
        </section>

        <section class="panel">
          <h2>Knowledge Bases</h2>
          <p id="dashboard-empty" class="muted" hidden>No knowledge bases configured.</p>
          <div class="table-wrap">
            <table class="kb-table">
              <thead>
                <tr>
                  <th>ID</th>
                  <th>Name</th>
                  <th>Type</th>
                  <th>Path</th>
                  <th>Enabled</th>
                  <th>Source</th>
                  <th>Default Mode</th>
                  <th>Tags</th>
                </tr>
              </thead>
              <tbody id="dashboard-kbs-body"></tbody>
            </table>
          </div>
        </section>

        <div class="status-grid">
          <section class="panel status-card">
            <h2>Index Queue</h2>
            <p id="indexing-status" class="muted">Loading...</p>
          </section>
          <section class="panel status-card">
            <h2>Recent Failures</h2>
            <p id="failures-status" class="muted">Loading...</p>
          </section>
        </div>
      </section>
    </main>
  </body>
</html>
{{ end }}
```

- [ ] **Step 2: 替换 Search Lab template**

把 `web/templates/search_lab.html` 全部替换为：

```html
{{ define "search_lab.html" }}
<!doctype html>
<html lang="zh-CN">
  <head>
    <meta charset="utf-8">
    <title>Search Lab</title>
    <link rel="stylesheet" href="/static/styles.css">
    <script defer src="/static/app.js"></script>
  </head>
  <body>
    {{ template "nav" . }}
    <main>
      <h1>Search Lab</h1>
      <p>观察聚合搜索请求、命中结果、score、match mode 和 backend。</p>

      <section class="panel">
        <form id="search-form" class="search-form">
          <label>Query <input name="query" required placeholder="SQLite default storage"></label>
          <label>Limit <input name="limit" type="number" min="1" max="100" value="10"></label>
          <label>KB IDs <input name="kb_ids" placeholder="default,docs"></label>
          <label>Search Mode
            <select name="search_mode">
              <option value="">default</option>
              <option value="lexical">lexical</option>
              <option value="semantic">semantic</option>
              <option value="hybrid">hybrid</option>
            </select>
          </label>
          <button id="search-submit" type="submit">Search</button>
        </form>
      </section>

      <p id="search-message" class="message" hidden></p>

      <section class="panel">
        <h2>Request Summary</h2>
        <dl id="search-summary" class="summary-list">
          <dt>Status</dt><dd>Run a search to see request details.</dd>
        </dl>
      </section>

      <section class="panel">
        <h2>Warnings</h2>
        <ul id="search-warnings" class="warning-list"><li class="muted">No warnings.</li></ul>
      </section>

      <section class="panel">
        <h2>Results</h2>
        <p id="search-empty" class="muted">Run a search to see results.</p>
        <div class="table-wrap">
          <table class="kb-table">
            <thead>
              <tr>
                <th>Title</th>
                <th>KB</th>
                <th>Score</th>
                <th>Match Mode</th>
                <th>Backend</th>
                <th>Locator</th>
                <th>Snippet</th>
              </tr>
            </thead>
            <tbody id="search-results-body"></tbody>
          </table>
        </div>
      </section>
    </main>
  </body>
</html>
{{ end }}
```

- [ ] **Step 3: 运行 route marker tests，确认通过**

Run:

```bash
go test ./internal/adapters/web -run 'TestDashboardRespondsOK|TestSearchLabRespondsOK' -count=1
```

Expected: PASS。

- [ ] **Step 4: 提交 templates**

```bash
git add web/templates/dashboard.html web/templates/search_lab.html internal/adapters/web/server_test.go
git commit -m "feat: add dashboard and search lab templates"
```

---

### Task 6: 实现 Dashboard/Search Lab JavaScript 与样式

**Files:**
- Modify: `web/static/app.js`
- Modify: `web/static/styles.css`

- [ ] **Step 1: 替换 `web/static/app.js`**

把 `web/static/app.js` 全部替换为：

```js
document.documentElement.dataset.knowledger = "ready";

function showKBMessage(message, isError) {
  const el = document.querySelector("#kb-message");
  if (!el) return;
  showMessage(el, message, isError);
}

function showMessage(el, message, isError) {
  if (!el) return;
  el.hidden = false;
  el.textContent = message;
  el.className = isError ? "message error" : "message success";
}

function hideMessage(el) {
  if (!el) return;
  el.hidden = true;
  el.textContent = "";
}

function firstAPIError(payload) {
  if (payload && payload.errors && payload.errors.length > 0) {
    return payload.errors[0].message;
  }
  return "Request failed";
}

async function parseAPIResponse(response) {
  const payload = await response.json().catch(() => null);
  if (!response.ok || !payload || !payload.success) {
    throw new Error(firstAPIError(payload));
  }
  return payload;
}

function tagsFromInput(value) {
  return value
    .split(",")
    .map((tag) => tag.trim())
    .filter(Boolean);
}

function appendTextCell(row, value, asCode) {
  const cell = document.createElement("td");
  if (asCode) {
    const code = document.createElement("code");
    code.textContent = value == null || value === "" ? "—" : String(value);
    cell.appendChild(code);
  } else {
    cell.textContent = value == null || value === "" ? "—" : String(value);
  }
  row.appendChild(cell);
  return cell;
}

function appendTagsCell(row, tags) {
  const cell = document.createElement("td");
  if (!tags || tags.length === 0) {
    cell.textContent = "—";
  } else {
    tags.forEach((tag) => {
      const span = document.createElement("span");
      span.className = "tag";
      span.textContent = tag;
      cell.appendChild(span);
    });
  }
  row.appendChild(cell);
}

const createForm = document.querySelector("#kb-create-form");
if (createForm) {
  createForm.addEventListener("submit", async (event) => {
    event.preventDefault();
    const form = event.currentTarget;
    const data = new FormData(form);
    const payload = {
      id: data.get("id") || "",
      name: data.get("name") || "",
      store_type: data.get("store_type") || "",
      path: data.get("path") || "",
      enabled: data.get("enabled") === "on",
      tags: tagsFromInput(data.get("tags") || ""),
    };

    try {
      const response = await fetch("/api/kbs", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(payload),
      });
      await parseAPIResponse(response);
      showKBMessage("Knowledge base created.", false);
      window.location.reload();
    } catch (error) {
      showKBMessage(error.message, true);
    }
  });
}

document.querySelectorAll(".kb-delete").forEach((button) => {
  button.addEventListener("click", async () => {
    const id = button.dataset.kbId;
    if (!id) return;
    if (!window.confirm(`Delete runtime knowledge base "${id}"? Stored data will not be deleted.`)) {
      return;
    }
    try {
      const response = await fetch(`/api/kbs/${encodeURIComponent(id)}`, { method: "DELETE" });
      await parseAPIResponse(response);
      showKBMessage("Knowledge base deleted.", false);
      window.location.reload();
    } catch (error) {
      showKBMessage(error.message, true);
    }
  });
});

async function loadDashboard() {
  const message = document.querySelector("#dashboard-message");
  try {
    hideMessage(message);
    const response = await fetch("/api/dashboard");
    const payload = await parseAPIResponse(response);
    renderDashboard(payload.data);
  } catch (error) {
    showMessage(message, `Failed to load dashboard: ${error.message}`, true);
  }
}

function renderDashboard(data) {
  const summary = data.summary || {};
  setText("#stat-total-kbs", summary.total_kbs);
  setText("#stat-enabled-kbs", summary.enabled_kbs);
  setText("#stat-disabled-kbs", summary.disabled_kbs);
  setText("#stat-runtime-kbs", summary.runtime_kbs);
  setText("#stat-static-kbs", summary.static_kbs);
  renderStoreTypes(summary.store_types || {});
  renderKnowledgeBases(data.knowledge_bases || []);
  renderStatus("#indexing-status", data.indexing);
  renderStatus("#failures-status", data.failures);
}

function setText(selector, value) {
  const el = document.querySelector(selector);
  if (!el) return;
  el.textContent = value == null ? "0" : String(value);
}

function renderStoreTypes(storeTypes) {
  const el = document.querySelector("#store-types");
  if (!el) return;
  el.replaceChildren();
  const entries = Object.entries(storeTypes).sort(([a], [b]) => a.localeCompare(b));
  if (entries.length === 0) {
    const empty = document.createElement("span");
    empty.className = "muted";
    empty.textContent = "No store types.";
    el.appendChild(empty);
    return;
  }
  entries.forEach(([storeType, count]) => {
    const tag = document.createElement("span");
    tag.className = "tag";
    tag.textContent = `${storeType}: ${count}`;
    el.appendChild(tag);
  });
}

function renderKnowledgeBases(rows) {
  const body = document.querySelector("#dashboard-kbs-body");
  const empty = document.querySelector("#dashboard-empty");
  if (!body) return;
  body.replaceChildren();
  if (empty) empty.hidden = rows.length > 0;

  rows.forEach((kb) => {
    const row = document.createElement("tr");
    appendTextCell(row, kb.id, true);
    appendTextCell(row, kb.name, false);
    appendTextCell(row, kb.store_type, false);
    appendTextCell(row, kb.path, true);
    appendTextCell(row, kb.enabled, false);
    appendTextCell(row, kb.source, false);
    appendTextCell(row, kb.default_search_mode, false);
    appendTagsCell(row, kb.tags || []);
    body.appendChild(row);
  });
}

function renderStatus(selector, status) {
  const el = document.querySelector(selector);
  if (!el) return;
  if (!status) {
    el.textContent = "unsupported";
    return;
  }
  el.textContent = `${status.state}: ${status.message}`;
}

function setupSearchLab(form) {
  form.addEventListener("submit", async (event) => {
    event.preventDefault();
    const message = document.querySelector("#search-message");
    const button = document.querySelector("#search-submit");
    const payload = searchPayloadFromForm(form);

    try {
      hideMessage(message);
      if (button) {
        button.disabled = true;
        button.textContent = "Searching...";
      }
      const response = await fetch("/api/search", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(payload),
      });
      const result = await parseAPIResponse(response);
      renderSearchResults(result);
    } catch (error) {
      showMessage(message, error.message, true);
    } finally {
      if (button) {
        button.disabled = false;
        button.textContent = "Search";
      }
    }
  });
}

function searchPayloadFromForm(form) {
  const data = new FormData(form);
  const limitValue = data.get("limit");
  return {
    query: data.get("query") || "",
    limit: limitValue === "" ? undefined : Number(limitValue),
    kb_ids: tagsFromInput(data.get("kb_ids") || ""),
    search_mode: data.get("search_mode") || "",
  };
}

function renderSearchResults(payload) {
  const data = payload.data || {};
  const hits = data.hits || [];
  renderSearchSummary(data, payload.meta || {});
  renderSearchWarnings(payload.warnings || []);

  const body = document.querySelector("#search-results-body");
  const empty = document.querySelector("#search-empty");
  if (!body) return;
  body.replaceChildren();
  if (empty) {
    empty.hidden = hits.length > 0;
    empty.textContent = hits.length === 0 ? "No hits found." : "";
  }

  hits.forEach((hit) => {
    const row = document.createElement("tr");
    appendTextCell(row, hit.title, false);
    appendTextCell(row, hit.kb_id, true);
    appendTextCell(row, formatScore(hit.score), false);
    appendTextCell(row, hit.match_mode, false);
    appendTextCell(row, hit.source_backend, false);
    appendTextCell(row, hit.locator, true);
    appendTextCell(row, hit.snippet || hit.content_preview, false);
    body.appendChild(row);
  });
}

function renderSearchSummary(data, meta) {
  const el = document.querySelector("#search-summary");
  if (!el) return;
  el.replaceChildren();
  const rows = [
    ["Query", data.query || ""],
    ["Limit", data.limit == null ? "" : data.limit],
    ["Hit Count", meta.hit_count == null ? 0 : meta.hit_count],
  ];
  rows.forEach(([key, value]) => {
    const dt = document.createElement("dt");
    dt.textContent = key;
    const dd = document.createElement("dd");
    dd.textContent = String(value);
    el.appendChild(dt);
    el.appendChild(dd);
  });
}

function renderSearchWarnings(warnings) {
  const el = document.querySelector("#search-warnings");
  if (!el) return;
  el.replaceChildren();
  if (warnings.length === 0) {
    const item = document.createElement("li");
    item.className = "muted";
    item.textContent = "No warnings.";
    el.appendChild(item);
    return;
  }
  warnings.forEach((warning) => {
    const item = document.createElement("li");
    item.textContent = warning;
    el.appendChild(item);
  });
}

function formatScore(score) {
  const value = Number(score);
  if (!Number.isFinite(value)) return "—";
  return value.toFixed(3);
}

const dashboardRoot = document.querySelector("#dashboard-root");
if (dashboardRoot) {
  loadDashboard();
}

const searchForm = document.querySelector("#search-form");
if (searchForm) {
  setupSearchLab(searchForm);
}
```

- [ ] **Step 2: 追加 CSS 样式**

在 `web/static/styles.css` 末尾追加：

```css
.stats-grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(9rem, 1fr)); gap: 1rem; margin: 1.5rem 0; }
.stat-card { background: white; border: 1px solid #e5e7eb; border-radius: 0.75rem; padding: 1rem; }
.stat-card strong { display: block; font-size: 2rem; margin-top: 0.35rem; }
.stat-label { color: #6b7280; font-size: 0.85rem; text-transform: uppercase; }
.tag-list { display: flex; flex-wrap: wrap; gap: 0.35rem; }
.table-wrap { overflow-x: auto; }
.status-grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(16rem, 1fr)); gap: 1rem; }
.status-card { margin: 0; }
.search-form { display: grid; grid-template-columns: minmax(16rem, 2fr) repeat(3, minmax(9rem, 1fr)); gap: 1rem; align-items: end; }
.search-form label { display: grid; gap: 0.35rem; font-weight: 600; }
.summary-list { display: grid; grid-template-columns: max-content 1fr; gap: 0.4rem 1rem; }
.summary-list dt { font-weight: 700; }
.summary-list dd { margin: 0; }
.warning-list { margin: 0; padding-left: 1.25rem; }
.muted { color: #6b7280; }
```

- [ ] **Step 3: 运行 Web tests，确认无回归**

Run:

```bash
go test ./internal/adapters/web -count=1
```

Expected: PASS。

- [ ] **Step 4: 提交 JS/CSS**

```bash
git add web/static/app.js web/static/styles.css
git commit -m "feat: render dashboard and search lab UI"
```

---

### Task 7: 全量验证与 smoke test

**Files:**
- No code files changed in this task.

- [ ] **Step 1: 运行全量 Go tests**

Run:

```bash
go test ./...
```

Expected: PASS for all packages。

- [ ] **Step 2: 运行 FTS5 tagged tests**

Run:

```bash
CGO_ENABLED=1 go test -tags fts5 ./...
```

Expected: PASS for all packages。如果环境缺少 CGO/SQLite FTS5 支持，记录实际错误，不要声称通过。

- [ ] **Step 3: 启动 Web server 做手动 smoke**

Run:

```bash
go run ./cmd/knowledger serve
```

Expected: server starts and listens at `http://127.0.0.1:34125/`。

- [ ] **Step 4: 检查 Dashboard**

Open:

```text
http://127.0.0.1:34125/
```

Expected:

- 页面包含 `Knowledger Dashboard`。
- 顶部统计卡显示 Total/Enabled/Disabled/Runtime/Static 数值。
- Store Types 至少显示 `sqlite: 1`，除非当前配置确实没有 sqlite KB。
- Knowledge Bases 表中显示当前 KB。
- Index Queue 和 Recent Failures 显示 `unsupported` message。

- [ ] **Step 5: 检查 Search Lab 正常搜索**

先在另一个 terminal 添加测试内容：

```bash
TMP_HOME="$(mktemp -d)"
HOME="$TMP_HOME" go run ./cmd/knowledger add --kb default --title "Default DB" --content "SQLite default storage"
HOME="$TMP_HOME" go run ./cmd/knowledger serve
```

Open:

```text
http://127.0.0.1:34125/search-lab
```

在 Search Lab 中输入：

```text
Query: SQLite
Limit: 10
KB IDs: default
Search Mode: default
```

Expected:

- Results 表出现 `Default DB`。
- Score 显示三位小数。
- Match Mode 和 Backend 有值。
- Warnings 区域显示 `No warnings.` 或 service 返回的 warning。

- [ ] **Step 6: 检查 Search Lab 错误输入**

在 Search Lab 中提交空 query。

Expected: 页面显示 API 错误消息，Network response 是 `400 invalid_query`。

把 limit 改为 `101` 后提交。

Expected: 页面显示 API 错误消息，Network response 是 `400 invalid_limit`。

- [ ] **Step 7: 最终检查工作区**

Run:

```bash
git status --short
```

Expected: 只剩允许保留的未跟踪文件，且本功能相关文件已提交。如果还有本功能相关修改未提交，提交它们：

```bash
git add internal/adapters/web/server.go internal/adapters/web/server_test.go web/templates/dashboard.html web/templates/search_lab.html web/static/app.js web/static/styles.css
git commit -m "test: verify dashboard and search lab MVP"
```

---

## 自检结果

- Spec coverage: 已覆盖 `POST /api/search`、`GET /api/dashboard`、Dashboard KB 维度统计、Search Lab 查询栏 + 调试表格、现有 `app.js`/template 路线、Go API/route tests、unsupported indexing/failures 边界。
- Placeholder scan: 已检查常见占位语、未定义函数名和“以后补充”式步骤，未发现问题。
- Type consistency: `webService.Search(context.Context, core.SearchOptions) (service.SearchResult, error)`、`searchRequest`、`searchHitView`、`dashboardSummary`、DOM IDs 与各任务中的测试和模板一致。
