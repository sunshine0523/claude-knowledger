package web

import (
	"context"
	"encoding/json"
	"html/template"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/kindbrave/knowledger/internal/registry"
	"github.com/kindbrave/knowledger/internal/service"
)

type knowledgeBaseService interface {
	ListKnowledgeBaseRecords() ([]registry.KnowledgeBaseRecord, error)
	CreateKnowledgeBase(context.Context, service.CreateKnowledgeBaseInput) (registry.KnowledgeBaseRecord, error)
	DeleteKnowledgeBase(context.Context, string) error
}

type Server struct {
	tmpl *template.Template
	mux  *http.ServeMux
	svc  knowledgeBaseService
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

func NewServer(svc any) *Server {
	tmpl := template.Must(parseTemplates())
	mux := http.NewServeMux()
	s := &Server{tmpl: tmpl, mux: mux}
	if typed, ok := svc.(knowledgeBaseService); ok {
		s.svc = typed
	}
	mux.HandleFunc("GET /", s.dashboard)
	mux.HandleFunc("GET /kbs", s.kbs)
	mux.HandleFunc("GET /search-lab", s.searchLab)
	mux.HandleFunc("GET /debug", s.debug)
	mux.HandleFunc("GET /api/kbs", s.apiListKBs)
	mux.HandleFunc("POST /api/kbs", s.apiCreateKB)
	mux.HandleFunc("DELETE /api/kbs/{id}", s.apiDeleteKB)
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

func writeAPISuccess(w http.ResponseWriter, status int, data any) {
	writeJSON(w, status, apiResponse{Success: true, Data: data, Warnings: []string{}, Errors: []apiError{}, Meta: map[string]any{}})
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
