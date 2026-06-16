package web

import (
	"context"
	"encoding/json"
	"html/template"
	"net/http"
	"strings"
	"time"

	"github.com/kindbrave/knowledger/internal/core"
	"github.com/kindbrave/knowledger/internal/registry"
	"github.com/kindbrave/knowledger/internal/service"
	webassets "github.com/kindbrave/knowledger/web"
)

type webService interface {
	ListKnowledgeBaseRecords() ([]registry.KnowledgeBaseRecord, error)
	ListKnowledgeBaseSummaries(context.Context) ([]service.KnowledgeBaseSummary, error)
	ListKnowledgeItems(context.Context, string, string) ([]core.KnowledgeItem, error)
	DeleteKnowledgeItem(context.Context, string, string, string) error
	CreateKnowledgeBase(context.Context, service.CreateKnowledgeBaseInput) (registry.KnowledgeBaseRecord, error)
	DeleteKnowledgeBase(context.Context, string, string) error
	Search(context.Context, core.SearchOptions) (service.SearchResult, error)
	HasProjectScope() bool
	ProjectRoot() string
}

type Server struct {
	tmpl *template.Template
	mux  *http.ServeMux
	svc  webService
}

type apiResponse struct {
	Success  bool       `json:"success"`
	Data     any        `json:"data"`
	Warnings []string   `json:"warnings"`
	Errors   []apiError `json:"errors"`
	Meta     any        `json:"meta"`
}

type apiError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type kbView struct {
	ID                string   `json:"id"`
	Scope             string   `json:"scope"`
	Name              string   `json:"name"`
	StoreType         string   `json:"store_type"`
	Path              string   `json:"path"`
	Enabled           bool     `json:"enabled"`
	DefaultSearchMode string   `json:"default_search_mode"`
	Tags              []string `json:"tags"`
	Source            string   `json:"source"`
	Deletable         bool     `json:"deletable"`
	ItemCount         int      `json:"item_count"`
}

type knowledgeItemView struct {
	ID        string         `json:"id"`
	KBID      string         `json:"kb_id"`
	Type      string         `json:"type"`
	Title     string         `json:"title"`
	Content   string         `json:"content"`
	Summary   string         `json:"summary"`
	SourceRef string         `json:"source_ref"`
	Metadata  map[string]any `json:"metadata"`
	Tags      []string       `json:"tags"`
	CreatedAt string         `json:"created_at"`
	UpdatedAt string         `json:"updated_at"`
}

type kbsPageData struct {
	KnowledgeBases []kbView
	Error          string
}

type createKBRequest struct {
	ID              string   `json:"id"`
	Name            string   `json:"name"`
	StoreType       string   `json:"store_type"`
	Path            string   `json:"path"`
	Enabled         *bool    `json:"enabled"`
	SemanticEnabled *bool    `json:"semantic_enabled"`
	Tags            []string `json:"tags"`
}

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

type dashboardSummary struct {
	TotalKBs    int            `json:"total_kbs"`
	EnabledKBs  int            `json:"enabled_kbs"`
	DisabledKBs int            `json:"disabled_kbs"`
	RuntimeKBs  int            `json:"runtime_kbs"`
	StaticKBs   int            `json:"static_kbs"`
	ProjectKBs  int            `json:"project_kbs"`
	GlobalKBs   int            `json:"global_kbs"`
	StoreTypes  map[string]int `json:"store_types"`
}

type dashboardStatus struct {
	State   string `json:"state"`
	Message string `json:"message"`
}

type dashboardReadiness struct {
	SearchableKBs            int      `json:"searchable_kbs"`
	LexicalConfiguredKBs     int      `json:"lexical_configured_kbs"`
	SemanticConfiguredKBs    int      `json:"semantic_configured_kbs"`
	SemanticQueryImplemented bool     `json:"semantic_query_implemented"`
	Notes                    []string `json:"notes"`
}

type projectView struct {
	InProject   bool   `json:"in_project"`
	ProjectRoot string `json:"project_root"`
}

func NewServer(svc any) *Server {
	tmpl := template.Must(parseTemplates())
	mux := http.NewServeMux()
	s := &Server{tmpl: tmpl, mux: mux}
	if typed, ok := svc.(webService); ok {
		s.svc = typed
	}
	mux.HandleFunc("GET /", s.dashboard)
	mux.HandleFunc("GET /kbs", s.kbs)
	mux.HandleFunc("GET /knowledge", s.knowledge)
	mux.HandleFunc("GET /search-lab", s.searchLab)
	mux.HandleFunc("GET /debug", s.debug)
	mux.HandleFunc("GET /api/kbs", s.apiListKBs)
	mux.HandleFunc("POST /api/kbs", s.apiCreateKB)
	mux.HandleFunc("DELETE /api/kbs/{id}", s.apiDeleteKB)
	mux.HandleFunc("GET /api/kbs/{id}/items", s.apiListKnowledgeItems)
	mux.HandleFunc("DELETE /api/kbs/{id}/items/{item_id}", s.apiDeleteKnowledgeItem)
	mux.HandleFunc("POST /api/search", s.apiSearch)
	mux.HandleFunc("GET /api/dashboard", s.apiDashboard)
	mux.HandleFunc("GET /api/project", s.apiProject)
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.FS(webassets.StaticFS()))))
	return s
}

func parseTemplates() (*template.Template, error) {
	return template.ParseFS(webassets.TemplateFS(), "*.html")
}

func (s *Server) Handler() http.Handler { return s.mux }

func (s *Server) dashboard(w http.ResponseWriter, r *http.Request) {
	_ = s.tmpl.ExecuteTemplate(w, "dashboard.html", nil)
}

func (s *Server) kbs(w http.ResponseWriter, r *http.Request) {
	data := kbsPageData{}
	if s.svc == nil {
		data.Error = "knowledge base service is unavailable"
		_ = s.tmpl.ExecuteTemplate(w, "kbs.html", data)
		return
	}
	summaries, err := s.svc.ListKnowledgeBaseSummaries(r.Context())
	if err != nil {
		data.Error = err.Error()
		_ = s.tmpl.ExecuteTemplate(w, "kbs.html", data)
		return
	}
	data.KnowledgeBases = summariesToViews(summaries)
	_ = s.tmpl.ExecuteTemplate(w, "kbs.html", data)
}

func (s *Server) knowledge(w http.ResponseWriter, r *http.Request) {
	_ = s.tmpl.ExecuteTemplate(w, "knowledge.html", nil)
}

func (s *Server) searchLab(w http.ResponseWriter, r *http.Request) {
	_ = s.tmpl.ExecuteTemplate(w, "search_lab.html", nil)
}

func (s *Server) debug(w http.ResponseWriter, r *http.Request) {
	_ = s.tmpl.ExecuteTemplate(w, "debug.html", nil)
}

func (s *Server) apiListKBs(w http.ResponseWriter, r *http.Request) {
	if s.svc == nil {
		writeAPIError(w, http.StatusServiceUnavailable, "service_unavailable", "knowledge base service is unavailable")
		return
	}
	summaries, err := s.svc.ListKnowledgeBaseSummaries(r.Context())
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "list_kbs_failed", err.Error())
		return
	}
	writeAPISuccess(w, http.StatusOK, map[string]any{"knowledge_bases": summariesToViews(summaries)})
}

func (s *Server) apiCreateKB(w http.ResponseWriter, r *http.Request) {
	if s.svc == nil {
		writeAPIError(w, http.StatusServiceUnavailable, "service_unavailable", "knowledge base service is unavailable")
		return
	}
	var req createKBRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid_json", "request body must be valid JSON")
		return
	}
	record, err := s.svc.CreateKnowledgeBase(r.Context(), service.CreateKnowledgeBaseInput{
		ID:              req.ID,
		Name:            req.Name,
		StoreType:       req.StoreType,
		Path:            req.Path,
		Enabled:         req.Enabled,
		SemanticEnabled: req.SemanticEnabled,
		Tags:            req.Tags,
	})
	if err != nil {
		writeAPIError(w, statusForError(err), codeForError(err), err.Error())
		return
	}
	writeAPISuccess(w, http.StatusCreated, map[string]any{"knowledge_base": recordToView(record)})
}

func (s *Server) apiDeleteKB(w http.ResponseWriter, r *http.Request) {
	if s.svc == nil {
		writeAPIError(w, http.StatusServiceUnavailable, "service_unavailable", "knowledge base service is unavailable")
		return
	}
	id := r.PathValue("id")
	if id == "" {
		writeAPIError(w, http.StatusBadRequest, "missing_id", "knowledge base id is required")
		return
	}
	if err := s.svc.DeleteKnowledgeBase(r.Context(), core.ScopeGlobal, id); err != nil { // TODO(plan-2): scope
		writeAPIError(w, statusForError(err), codeForError(err), err.Error())
		return
	}
	writeAPISuccess(w, http.StatusOK, map[string]any{"deleted_id": id})
}

func (s *Server) apiListKnowledgeItems(w http.ResponseWriter, r *http.Request) {
	if s.svc == nil {
		writeAPIError(w, http.StatusServiceUnavailable, "service_unavailable", "knowledge base service is unavailable")
		return
	}
	id := r.PathValue("id")
	if id == "" {
		writeAPIError(w, http.StatusBadRequest, "missing_id", "knowledge base id is required")
		return
	}
	items, err := s.svc.ListKnowledgeItems(r.Context(), core.ScopeGlobal, id) // TODO(plan-2): scope
	if err != nil {
		writeAPIError(w, statusForError(err), codeForError(err), err.Error())
		return
	}
	summaries, err := s.svc.ListKnowledgeBaseSummaries(r.Context())
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "list_kbs_failed", err.Error())
		return
	}
	var selected kbView
	for _, view := range summariesToViews(summaries) {
		if view.ID == id {
			selected = view
			break
		}
	}
	writeAPISuccess(w, http.StatusOK, map[string]any{"knowledge_base": selected, "items": knowledgeItemsToViews(items), "item_count": len(items)})
}

func (s *Server) apiDeleteKnowledgeItem(w http.ResponseWriter, r *http.Request) {
	if s.svc == nil {
		writeAPIError(w, http.StatusServiceUnavailable, "service_unavailable", "knowledge base service is unavailable")
		return
	}
	kbID := r.PathValue("id")
	itemID := r.PathValue("item_id")
	if kbID == "" {
		writeAPIError(w, http.StatusBadRequest, "missing_id", "knowledge base id is required")
		return
	}
	if itemID == "" {
		writeAPIError(w, http.StatusBadRequest, "missing_item_id", "knowledge item id is required")
		return
	}
	if err := s.svc.DeleteKnowledgeItem(r.Context(), core.ScopeGlobal, kbID, itemID); err != nil { // TODO(plan-2): scope
		writeAPIError(w, statusForError(err), codeForError(err), err.Error())
		return
	}
	writeAPISuccess(w, http.StatusOK, map[string]any{"kb_id": kbID, "deleted_id": itemID})
}

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
		writeAPIError(w, http.StatusBadRequest, "invalid_search_mode", "search_mode must be auto, lexical, semantic, hybrid, or empty")
		return
	}
	if searchMode == "" {
		searchMode = "auto"
	}
	kbIDs := cleanKBIDs(req.KBIDs)

	result, err := s.svc.Search(r.Context(), core.SearchOptions{
		Query:      query,
		Limit:      limit,
		KBIDs:      kbIDs,
		SearchMode: searchMode,
	})
	if err != nil {
		writeAPIError(w, statusForError(err), codeForSearchError(err), err.Error())
		return
	}

	hits := searchHitsToViews(result.Hits)
	// TODO(plan-2): expose scope to search request and response.
	writeAPISuccessWithMeta(
		w,
		http.StatusOK,
		map[string]any{"query": query, "limit": limit, "kb_ids": kbIDs, "search_mode": searchMode, "hits": hits},
		result.Warnings,
		map[string]any{"hit_count": len(hits)},
	)
}

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
	summaries, err := s.svc.ListKnowledgeBaseSummaries(r.Context())
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "list_kbs_failed", err.Error())
		return
	}
	views := summariesToViews(summaries)
	writeAPISuccess(w, http.StatusOK, map[string]any{
		"summary":         dashboardSummaryFromViews(views),
		"knowledge_bases": views,
		"readiness":       dashboardReadinessFromRecords(records),
		"indexing": dashboardStatus{
			State:   "unsupported",
			Message: "Runtime indexing status is not exposed by the web dashboard yet; use readiness for configuration-derived search availability.",
		},
		"failures": dashboardStatus{
			State:   "unsupported",
			Message: "Recent indexing failures are not exposed by the web dashboard yet.",
		},
	})
}

func (s *Server) apiProject(w http.ResponseWriter, r *http.Request) {
	_ = r
	if s.svc == nil {
		writeAPIError(w, http.StatusServiceUnavailable, "service_unavailable", "knowledge base service is unavailable")
		return
	}
	view := projectView{InProject: s.svc.HasProjectScope(), ProjectRoot: s.svc.ProjectRoot()}
	writeAPISuccess(w, http.StatusOK, view)
}

func recordsToViews(records []registry.KnowledgeBaseRecord) []kbView {
	out := make([]kbView, 0, len(records))
	for _, record := range records {
		out = append(out, recordToView(record))
	}
	return out
}

func summariesToViews(summaries []service.KnowledgeBaseSummary) []kbView {
	out := make([]kbView, 0, len(summaries))
	for _, summary := range summaries {
		view := recordToView(summary.Record)
		view.ItemCount = summary.ItemCount
		out = append(out, view)
	}
	return out
}

func recordToView(record registry.KnowledgeBaseRecord) kbView {
	kb := record.KnowledgeBase
	path, _ := kb.StoreConfig["path"].(string)
	return kbView{
		ID:                kb.ID,
		Scope:             kb.Scope,
		Name:              kb.Name,
		StoreType:         kb.StoreType,
		Path:              path,
		Enabled:           kb.Enabled,
		DefaultSearchMode: kb.DefaultSearchMode,
		Tags:              kb.Tags,
		Source:            record.Source,
		Deletable:         record.Deletable,
	}
}

func cleanKBIDs(ids []string) []core.ScopedKBRef {
	out := make([]core.ScopedKBRef, 0, len(ids))
	for _, raw := range ids {
		id := strings.TrimSpace(raw)
		if id == "" {
			continue
		}
		out = append(out, core.ScopedKBRef{ID: id})
	}
	return out
}

func validSearchMode(mode string) bool {
	switch mode {
	case "", "auto", "lexical", "semantic", "hybrid":
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

func knowledgeItemsToViews(items []core.KnowledgeItem) []knowledgeItemView {
	out := make([]knowledgeItemView, 0, len(items))
	for _, item := range items {
		out = append(out, knowledgeItemView{
			ID:        item.ID,
			KBID:      item.KBID,
			Type:      item.Type,
			Title:     item.Title,
			Content:   item.Content,
			Summary:   item.Summary,
			SourceRef: item.SourceRef,
			Metadata:  item.Metadata,
			Tags:      item.Tags,
			CreatedAt: formatTime(item.CreatedAt),
			UpdatedAt: formatTime(item.UpdatedAt),
		})
	}
	return out
}

func formatTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339)
}

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
		switch view.Scope {
		case core.ScopeProject:
			summary.ProjectKBs++
		case core.ScopeGlobal:
			summary.GlobalKBs++
		}
		if view.StoreType != "" {
			summary.StoreTypes[view.StoreType]++
		}
	}
	return summary
}

func dashboardReadinessFromRecords(records []registry.KnowledgeBaseRecord) dashboardReadiness {
	readiness := dashboardReadiness{
		Notes: []string{"SQLite semantic search uses Chroma when semantic indexing is enabled; query failures fall back to lexical results with warnings."},
	}
	for _, record := range records {
		kb := record.KnowledgeBase
		if !kb.Enabled || !searchableStoreType(kb.StoreType) {
			continue
		}
		readiness.SearchableKBs++
		if lexicalSearchConfigured(kb) {
			readiness.LexicalConfiguredKBs++
		}
		if indexingEnabled(kb.Indexing, "semantic") {
			readiness.SemanticConfiguredKBs++
			readiness.SemanticQueryImplemented = true
		}
	}
	return readiness
}

func searchableStoreType(storeType string) bool {
	return storeType == "text" || storeType == "sqlite"
}

func lexicalSearchConfigured(kb core.KnowledgeBase) bool {
	if enabled, ok := indexingEnabledValue(kb.Indexing, "lexical"); ok {
		return enabled
	}
	return searchableStoreType(kb.StoreType)
}

func indexingEnabled(indexing map[string]any, key string) bool {
	enabled, ok := indexingEnabledValue(indexing, key)
	return ok && enabled
}

func indexingEnabledValue(indexing map[string]any, key string) (bool, bool) {
	if indexing == nil {
		return false, false
	}
	config, ok := indexing[key].(map[string]any)
	if !ok {
		return false, false
	}
	enabled, ok := config["enabled"].(bool)
	return enabled, ok
}

func codeForSearchError(err error) string {
	if statusForError(err) == http.StatusInternalServerError {
		return "search_failed"
	}
	return codeForError(err)
}

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

func writeAPIError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, apiResponse{Success: false, Data: nil, Warnings: []string{}, Errors: []apiError{{Code: code, Message: message}}, Meta: map[string]any{}})
}

func writeJSON(w http.ResponseWriter, status int, response apiResponse) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(response)
}

func statusForError(err error) int {
	message := err.Error()
	if strings.Contains(message, "already exists") || strings.Contains(message, "defined in static config") {
		return http.StatusConflict
	}
	if strings.Contains(message, "not found") {
		return http.StatusNotFound
	}
	if strings.Contains(message, "required") || strings.Contains(message, "unsupported") || strings.Contains(message, "may contain") || strings.Contains(message, "not available") || strings.Contains(message, "not a directory") || strings.Contains(message, "invalid") {
		return http.StatusBadRequest
	}
	return http.StatusInternalServerError
}

func codeForError(err error) string {
	message := err.Error()
	if strings.Contains(message, "knowledge item not found") {
		return "item_not_found"
	}
	if strings.Contains(message, "knowledge base not found") || strings.Contains(message, "not found in runtime registry") {
		return "kb_not_found"
	}
	if strings.Contains(message, "knowledge item id") || strings.Contains(message, "invalid knowledge item id") {
		return "invalid_item_id"
	}
	if strings.Contains(message, "already exists") {
		return "duplicate_kb"
	}
	if strings.Contains(message, "defined in static config") {
		return "static_kb_read_only"
	}
	if strings.Contains(message, "unsupported") {
		return "unsupported_store_type"
	}
	return "invalid_kb"
}
