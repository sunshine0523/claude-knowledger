package service

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
	"sync"

	"github.com/kindbrave/knowledger/internal/config"
	"github.com/kindbrave/knowledger/internal/core"
	"github.com/kindbrave/knowledger/internal/registry"
)

type SearchResult struct {
	Hits     []core.SearchHit
	Warnings []string
}

type KnowledgeBaseSummary struct {
	Record    registry.KnowledgeBaseRecord
	ItemCount int
}

type BackendBuilder func([]core.KnowledgeBase) (map[string]core.StoreBackend, error)

type CreateKnowledgeBaseInput struct {
	ID              string
	Name            string
	StoreType       string
	Path            string
	Enabled         *bool
	SemanticEnabled *bool
	Tags            []string
}

type Service struct {
	mu             sync.RWMutex
	knowledgeBases []core.KnowledgeBase
	backends       map[string]core.StoreBackend
	registry       *registry.Registry
	buildBackends  BackendBuilder
}

var knowledgeBaseIDPattern = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)

func New(kbs []core.KnowledgeBase, backends map[string]core.StoreBackend) *Service {
	return &Service{knowledgeBases: kbs, backends: backends}
}

func NewManaged(reg *registry.Registry, buildBackends BackendBuilder) (*Service, error) {
	if reg == nil {
		return nil, fmt.Errorf("registry is required")
	}
	if buildBackends == nil {
		return nil, fmt.Errorf("backend builder is required")
	}
	s := &Service{registry: reg, buildBackends: buildBackends}
	if err := s.Reload(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Service) Search(ctx context.Context, opt core.SearchOptions) (SearchResult, error) {
	kbs, backends := s.snapshot()
	result := SearchResult{}
	for _, kb := range kbs {
		if !kb.Enabled || !matchesKBFilter(kb.ID, opt.KBIDs) {
			continue
		}
		backend, ok := backends[kb.StoreType]
		if !ok {
			return SearchResult{}, &core.Error{Kind: core.ErrorKindConfig, Message: "backend not registered for store type " + kb.StoreType}
		}
		effectiveOpt, warning := searchOptionsForKnowledgeBase(opt, kb, backend)
		if warning != "" {
			result.Warnings = append(result.Warnings, warning)
		}
		kbHits, err := backend.Search(ctx, kb, effectiveOpt)
		if err != nil {
			if (effectiveOpt.SearchMode == "semantic" || effectiveOpt.SearchMode == "hybrid") && backend.SupportsSemantic(kb) {
				fallbackOpt := effectiveOpt
				fallbackOpt.SearchMode = "lexical"
				kbHits, fallbackErr := backend.Search(ctx, kb, fallbackOpt)
				if fallbackErr != nil {
					return SearchResult{}, fallbackErr
				}
				result.Warnings = append(result.Warnings, kb.ID+": semantic path unavailable, lexical fallback used")
				result.Hits = append(result.Hits, kbHits...)
				continue
			}
			return SearchResult{}, err
		}
		result.Hits = append(result.Hits, kbHits...)
	}
	sort.Slice(result.Hits, func(i, j int) bool { return result.Hits[i].Score > result.Hits[j].Score })
	if opt.Limit > 0 && len(result.Hits) > opt.Limit {
		result.Hits = result.Hits[:opt.Limit]
	}
	return result, nil
}

func (s *Service) Add(ctx context.Context, input core.AddInput) (core.KnowledgeItem, core.IngestionResult, core.IndexStatus, error) {
	kbs, backends := s.snapshot()
	for _, kb := range kbs {
		if kb.ID != input.KBID {
			continue
		}
		backend, ok := backends[kb.StoreType]
		if !ok {
			return core.KnowledgeItem{}, core.IngestionResult{}, core.IndexStatus{}, &core.Error{Kind: core.ErrorKindConfig, Message: "backend not registered for store type " + kb.StoreType}
		}
		return backend.Add(ctx, kb, input)
	}
	return core.KnowledgeItem{}, core.IngestionResult{}, core.IndexStatus{}, &core.Error{Kind: core.ErrorKindConfig, Message: "knowledge base not found"}
}

func (s *Service) ListKnowledgeBases() []core.KnowledgeBase {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]core.KnowledgeBase(nil), s.knowledgeBases...)
}

func (s *Service) ListKnowledgeBaseRecords() ([]registry.KnowledgeBaseRecord, error) {
	if s.registry == nil {
		kbs := s.ListKnowledgeBases()
		records := make([]registry.KnowledgeBaseRecord, 0, len(kbs))
		for _, kb := range kbs {
			records = append(records, registry.KnowledgeBaseRecord{KnowledgeBase: kb, Source: registry.SourceStatic, Deletable: false})
		}
		return records, nil
	}
	return s.registry.ListWithSources()
}

func (s *Service) ListKnowledgeBaseSummaries(ctx context.Context) ([]KnowledgeBaseSummary, error) {
	records, err := s.ListKnowledgeBaseRecords()
	if err != nil {
		return nil, err
	}
	summaries := make([]KnowledgeBaseSummary, 0, len(records))
	for _, record := range records {
		items, err := s.listItemsForKnowledgeBase(ctx, record.KnowledgeBase)
		count := 0
		if err == nil {
			count = len(items)
		}
		summaries = append(summaries, KnowledgeBaseSummary{Record: record, ItemCount: count})
	}
	return summaries, nil
}

func (s *Service) ListKnowledgeItems(ctx context.Context, kbID string) ([]core.KnowledgeItem, error) {
	kbID = strings.TrimSpace(kbID)
	if kbID == "" {
		return nil, &core.Error{Kind: core.ErrorKindConfig, Message: "knowledge base id is required"}
	}
	kb, backend, err := s.backendForKnowledgeBase(kbID)
	if err != nil {
		return nil, err
	}
	return backend.ListItems(ctx, kb)
}

func (s *Service) DeleteKnowledgeItem(ctx context.Context, kbID string, itemID string) error {
	kbID = strings.TrimSpace(kbID)
	itemID = strings.TrimSpace(itemID)
	if kbID == "" {
		return &core.Error{Kind: core.ErrorKindConfig, Message: "knowledge base id is required"}
	}
	if itemID == "" {
		return &core.Error{Kind: core.ErrorKindConfig, Message: "knowledge item id is required"}
	}
	kb, backend, err := s.backendForKnowledgeBase(kbID)
	if err != nil {
		return err
	}
	return backend.DeleteItem(ctx, kb, itemID)
}

func (s *Service) CreateKnowledgeBase(ctx context.Context, input CreateKnowledgeBaseInput) (registry.KnowledgeBaseRecord, error) {
	_ = ctx
	if s.registry == nil || s.buildBackends == nil {
		return registry.KnowledgeBaseRecord{}, fmt.Errorf("runtime registry is not available")
	}
	runtimeKB, err := normalizeCreateInput(input)
	if err != nil {
		return registry.KnowledgeBaseRecord{}, err
	}

	existing, err := s.registry.List()
	if err != nil {
		return registry.KnowledgeBaseRecord{}, err
	}
	for _, kb := range existing {
		if kb.ID == runtimeKB.ID {
			return registry.KnowledgeBaseRecord{}, fmt.Errorf("knowledge base %q already exists", runtimeKB.ID)
		}
	}
	prospective := append(append([]core.KnowledgeBase(nil), existing...), runtimeToCore(runtimeKB))
	if _, err := s.buildBackends(prospective); err != nil {
		return registry.KnowledgeBaseRecord{}, err
	}
	if err := s.registry.Create(runtimeKB); err != nil {
		return registry.KnowledgeBaseRecord{}, err
	}
	if err := s.Reload(); err != nil {
		return registry.KnowledgeBaseRecord{}, err
	}
	records, err := s.registry.ListWithSources()
	if err != nil {
		return registry.KnowledgeBaseRecord{}, err
	}
	for _, record := range records {
		if record.KnowledgeBase.ID == runtimeKB.ID {
			return record, nil
		}
	}
	return registry.KnowledgeBaseRecord{}, fmt.Errorf("knowledge base %q not found after create", runtimeKB.ID)
}

func (s *Service) DeleteKnowledgeBase(ctx context.Context, id string) error {
	_ = ctx
	if s.registry == nil || s.buildBackends == nil {
		return fmt.Errorf("runtime registry is not available")
	}
	if err := s.registry.Delete(id); err != nil {
		return err
	}
	return s.Reload()
}

func (s *Service) Reload() error {
	if s.registry == nil || s.buildBackends == nil {
		return nil
	}
	kbs, err := s.registry.List()
	if err != nil {
		return err
	}
	backends, err := s.buildBackends(kbs)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.knowledgeBases = append([]core.KnowledgeBase(nil), kbs...)
	s.backends = copyBackends(backends)
	return nil
}

func (s *Service) Close() error {
	_, backends := s.snapshot()
	var firstErr error
	for _, backend := range backends {
		closer, ok := backend.(interface{ Close() error })
		if !ok {
			continue
		}
		if err := closer.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func (s *Service) snapshot() ([]core.KnowledgeBase, map[string]core.StoreBackend) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]core.KnowledgeBase(nil), s.knowledgeBases...), copyBackends(s.backends)
}

func (s *Service) listItemsForKnowledgeBase(ctx context.Context, kb core.KnowledgeBase) ([]core.KnowledgeItem, error) {
	_, backend, err := s.backendForKnowledgeBase(kb.ID)
	if err != nil {
		return nil, err
	}
	return backend.ListItems(ctx, kb)
}

func (s *Service) backendForKnowledgeBase(kbID string) (core.KnowledgeBase, core.StoreBackend, error) {
	kbs, backends := s.snapshot()
	for _, kb := range kbs {
		if kb.ID != kbID {
			continue
		}
		backend, ok := backends[kb.StoreType]
		if !ok {
			return core.KnowledgeBase{}, nil, &core.Error{Kind: core.ErrorKindConfig, Message: "backend not registered for store type " + kb.StoreType}
		}
		return kb, backend, nil
	}
	return core.KnowledgeBase{}, nil, &core.Error{Kind: core.ErrorKindConfig, Message: "knowledge base not found"}
}

func copyBackends(backends map[string]core.StoreBackend) map[string]core.StoreBackend {
	out := make(map[string]core.StoreBackend, len(backends))
	for key, value := range backends {
		out[key] = value
	}
	return out
}

func normalizeCreateInput(input CreateKnowledgeBaseInput) (registry.RuntimeKnowledgeBase, error) {
	if input.ID == "" {
		return registry.RuntimeKnowledgeBase{}, fmt.Errorf("knowledge base id is required")
	}
	if len(input.ID) > 64 || !knowledgeBaseIDPattern.MatchString(input.ID) {
		return registry.RuntimeKnowledgeBase{}, fmt.Errorf("knowledge base id may contain only letters, digits, underscore, dash, and dot")
	}
	if input.StoreType != "text" && input.StoreType != "sqlite" {
		return registry.RuntimeKnowledgeBase{}, fmt.Errorf("unsupported knowledge base store type %q", input.StoreType)
	}
	if input.Path == "" {
		return registry.RuntimeKnowledgeBase{}, fmt.Errorf("knowledge base path is required")
	}
	path, err := config.ExpandHomePath(input.Path)
	if err != nil {
		return registry.RuntimeKnowledgeBase{}, err
	}
	enabled := true
	if input.Enabled != nil {
		enabled = *input.Enabled
	}
	if input.StoreType == "text" && enabled {
		info, err := os.Stat(path)
		if err != nil {
			return registry.RuntimeKnowledgeBase{}, fmt.Errorf("text knowledge base path %q is not available: %w", path, err)
		}
		if !info.IsDir() {
			return registry.RuntimeKnowledgeBase{}, fmt.Errorf("text knowledge base path %q is not a directory", path)
		}
	}
	name := input.Name
	if name == "" {
		name = input.ID
	}
	kb := config.KnowledgeBaseConfig{
		ID:                input.ID,
		Name:              name,
		StoreType:         input.StoreType,
		StoreConfig:       map[string]any{"path": path},
		Enabled:           enabled,
		DefaultSearchMode: config.DefaultSearchMode,
		Tags:              input.Tags,
	}
	if err := config.ApplyKnowledgeBaseDefaults(&kb); err != nil {
		return registry.RuntimeKnowledgeBase{}, err
	}
	if input.StoreType == "sqlite" && input.SemanticEnabled != nil {
		semantic, _ := kb.Indexing["semantic"].(map[string]any)
		semantic["enabled"] = *input.SemanticEnabled
	}
	return registry.RuntimeKnowledgeBase{
		ID:                kb.ID,
		Name:              kb.Name,
		StoreType:         kb.StoreType,
		StoreConfig:       kb.StoreConfig,
		Enabled:           kb.Enabled,
		DefaultSearchMode: kb.DefaultSearchMode,
		Indexing:          kb.Indexing,
		Tags:              kb.Tags,
	}, nil
}

func runtimeToCore(item registry.RuntimeKnowledgeBase) core.KnowledgeBase {
	return core.KnowledgeBase{
		ID:                item.ID,
		Name:              item.Name,
		StoreType:         item.StoreType,
		StoreConfig:       item.StoreConfig,
		Enabled:           item.Enabled,
		DefaultSearchMode: item.DefaultSearchMode,
		Indexing:          item.Indexing,
		Tags:              item.Tags,
	}
}

func searchOptionsForKnowledgeBase(opt core.SearchOptions, kb core.KnowledgeBase, backend core.StoreBackend) (core.SearchOptions, string) {
	effective := opt
	requested := opt.SearchMode
	if requested == "" || requested == "auto" {
		requested = kb.DefaultSearchMode
	}
	if requested == "" || requested == "auto" {
		requested = "lexical"
	}
	if requested == "semantic" || requested == "hybrid" {
		if !backend.SupportsSemantic(kb) {
			effective.SearchMode = "lexical"
			return effective, fmt.Sprintf("%s: %s search is not implemented for %s backend yet; lexical results returned", kb.ID, requested, kb.StoreType)
		}
	}
	effective.SearchMode = requested
	return effective, ""
}

func matchesKBFilter(kbID string, filter []string) bool {
	if len(filter) == 0 {
		return true
	}
	for _, id := range filter {
		if id == kbID {
			return true
		}
	}
	return false
}
