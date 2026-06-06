package web

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

type webService interface {
	ListKnowledgeBaseRecords() ([]registry.KnowledgeBaseRecord, error)
	CreateKnowledgeBase(context.Context, service.CreateKnowledgeBaseInput) (registry.KnowledgeBaseRecord, error)
	DeleteKnowledgeBase(context.Context, string) error
	Search(context.Context, core.SearchOptions) (service.SearchResult, error)
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
	Name              string   `json:"name"`
	StoreType         string   `json:"store_type"`
	Path              string   `json:"path"`
	Enabled           bool     `json:"enabled"`
	DefaultSearchMode string   `json:"default_search_mode"`
	Tags              []string `json:"tags"`
	Source            string   `json:"source"`
	Deletable         bool     `json:"deletable"`
}

type kbsPageData struct {
	KnowledgeBases []kbView
	Error          string
}

type createKBRequest struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	StoreType string   `json:"store_type"`
	Path      string   `json:"path"`
	Enabled   *bool    `json:"enabled"`
	Tags      []string `json:"tags"`
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
	StoreTypes  map[string]int `json:"store_types"`
}

type dashboardStatus struct {
	State   string `json:"state"`
	Message string `json:"message"`
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
	mux.HandleFunc("GET /search-lab", s.searchLab)
	mux.HandleFunc("GET /debug", s.debug)
	mux.HandleFunc("GET /api/kbs", s.apiListKBs)
	mux.HandleFunc("POST /api/kbs", s.apiCreateKB)
	mux.HandleFunc("DELETE /api/kbs/{id}", s.apiDeleteKB)
	mux.HandleFunc("POST /api/search", s.apiSearch)
	mux.HandleFunc("GET /api/dashboard", s.apiDashboard)
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.Dir("web/static"))))
	return s
}

func parseTemplates() (*template.Template, error) {
	for _, pattern := range []string{
		filepath.Join("web", "templates", "*.html"),
		filepath.Join("..", "..", "..", "web", "templates", "*.html"),
	} {
		tmpl, err := template.ParseGlob(pattern)
		if err == nil {
			return tmpl, nil
		}
	}
	return template.ParseGlob(filepath.Join("web", "templates", "*.html"))
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
	records, err := s.svc.ListKnowledgeBaseRecords()
	if err != nil {
		data.Error = err.Error()
		_ = s.tmpl.ExecuteTemplate(w, "kbs.html", data)
		return
	}
	data.KnowledgeBases = recordsToViews(records)
	_ = s.tmpl.ExecuteTemplate(w, "kbs.html", data)
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
	records, err := s.svc.ListKnowledgeBaseRecords()
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "list_kbs_failed", err.Error())
		return
	}
	writeAPISuccess(w, http.StatusOK, map[string]any{"knowledge_bases": recordsToViews(records)})
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
		ID:        req.ID,
		Name:      req.Name,
		StoreType: req.StoreType,
		Path:      req.Path,
		Enabled:   req.Enabled,
		Tags:      req.Tags,
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
	if err := s.svc.DeleteKnowledgeBase(r.Context(), id); err != nil {
		writeAPIError(w, statusForError(err), codeForError(err), err.Error())
		return
	}
	writeAPISuccess(w, http.StatusOK, map[string]any{"deleted_id": id})
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

func recordsToViews(records []registry.KnowledgeBaseRecord) []kbView {
	out := make([]kbView, 0, len(records))
	for _, record := range records {
		out = append(out, recordToView(record))
	}
	return out
}

func recordToView(record registry.KnowledgeBaseRecord) kbView {
	kb := record.KnowledgeBase
	path, _ := kb.StoreConfig["path"].(string)
	return kbView{
		ID:                kb.ID,
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
	if strings.Contains(message, "not found in runtime registry") {
		return http.StatusNotFound
	}
	if strings.Contains(message, "required") || strings.Contains(message, "unsupported") || strings.Contains(message, "may contain") || strings.Contains(message, "not available") || strings.Contains(message, "not a directory") {
		return http.StatusBadRequest
	}
	return http.StatusInternalServerError
}

func codeForError(err error) string {
	message := err.Error()
	if strings.Contains(message, "already exists") {
		return "duplicate_kb"
	}
	if strings.Contains(message, "defined in static config") {
		return "static_kb_read_only"
	}
	if strings.Contains(message, "not found in runtime registry") {
		return "kb_not_found"
	}
	if strings.Contains(message, "unsupported") {
		return "unsupported_store_type"
	}
	return "invalid_kb"
}
