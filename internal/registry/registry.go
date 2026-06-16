package registry

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/kindbrave/knowledger/internal/config"
	"github.com/kindbrave/knowledger/internal/core"
)

const (
	SourceStatic  = "static"
	SourceRuntime = "runtime"
)

type KnowledgeBaseRecord struct {
	KnowledgeBase core.KnowledgeBase
	Source        string
	Deletable     bool
}

type recordKey struct {
	Scope string
	ID    string
}

type Registry struct {
	static       []config.KnowledgeBaseConfig
	globalStore  Store
	projectStore Store
	projectRoot  string
}

func New(static []config.KnowledgeBaseConfig, globalStore, projectStore Store, projectRoot string) *Registry {
	return &Registry{
		static:       static,
		globalStore:  globalStore,
		projectStore: projectStore,
		projectRoot:  projectRoot,
	}
}

func (r *Registry) HasProjectStore() bool {
	return r.projectStore != nil
}

func staticToCore(item config.KnowledgeBaseConfig) core.KnowledgeBase {
	return core.KnowledgeBase{
		ID:                item.ID,
		Scope:             core.ScopeGlobal,
		Name:              item.Name,
		StoreType:         item.StoreType,
		StoreConfig:       item.StoreConfig,
		Enabled:           item.Enabled,
		DefaultSearchMode: item.DefaultSearchMode,
		Indexing:          item.Indexing,
		Tags:              item.Tags,
	}
}

func runtimeToCore(item RuntimeKnowledgeBase, scope string) core.KnowledgeBase {
	return core.KnowledgeBase{
		ID:                item.ID,
		Scope:             scope,
		Name:              item.Name,
		StoreType:         item.StoreType,
		StoreConfig:       item.StoreConfig,
		Enabled:           item.Enabled,
		DefaultSearchMode: item.DefaultSearchMode,
		Indexing:          item.Indexing,
		Tags:              item.Tags,
	}
}

func (r *Registry) List() ([]core.KnowledgeBase, error) {
	records, err := r.ListWithSources()
	if err != nil {
		return nil, err
	}
	out := make([]core.KnowledgeBase, 0, len(records))
	for _, record := range records {
		out = append(out, record.KnowledgeBase)
	}
	return out, nil
}

func (r *Registry) ListWithSources() ([]KnowledgeBaseRecord, error) {
	merged := map[recordKey]KnowledgeBaseRecord{}

	for _, item := range r.static {
		key := recordKey{Scope: core.ScopeGlobal, ID: item.ID}
		merged[key] = KnowledgeBaseRecord{KnowledgeBase: staticToCore(item), Source: SourceStatic, Deletable: false}
	}

	globalRuntime, err := r.globalStore.List()
	if err != nil {
		return nil, err
	}
	for _, item := range globalRuntime {
		key := recordKey{Scope: core.ScopeGlobal, ID: item.ID}
		merged[key] = KnowledgeBaseRecord{KnowledgeBase: runtimeToCore(item, core.ScopeGlobal), Source: SourceRuntime, Deletable: true}
	}

	if r.projectStore != nil {
		projectRuntime, err := r.projectStore.List()
		if err != nil {
			return nil, err
		}
		for _, item := range projectRuntime {
			resolved := resolveProjectPaths(item, r.projectRoot)
			key := recordKey{Scope: core.ScopeProject, ID: resolved.ID}
			merged[key] = KnowledgeBaseRecord{KnowledgeBase: runtimeToCore(resolved, core.ScopeProject), Source: SourceRuntime, Deletable: true}
		}
	}

	keys := make([]recordKey, 0, len(merged))
	for k := range merged {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].Scope != keys[j].Scope {
			return keys[i].Scope == core.ScopeProject
		}
		return keys[i].ID < keys[j].ID
	})

	out := make([]KnowledgeBaseRecord, 0, len(keys))
	for _, k := range keys {
		out = append(out, merged[k])
	}
	return out, nil
}

func (r *Registry) storeForScope(scope string) (Store, error) {
	switch scope {
	case core.ScopeGlobal:
		return r.globalStore, nil
	case core.ScopeProject:
		if r.projectStore == nil {
			return nil, fmt.Errorf("not in a project directory; cannot operate on scope=project")
		}
		return r.projectStore, nil
	default:
		return nil, fmt.Errorf("unknown scope %q", scope)
	}
}

func (r *Registry) Create(scope string, item RuntimeKnowledgeBase) error {
	scope, err := core.NormalizeScope(scope)
	if err != nil {
		return err
	}
	if item.ID == "" {
		return fmt.Errorf("knowledge base id is required")
	}
	store, err := r.storeForScope(scope)
	if err != nil {
		return err
	}
	if scope == core.ScopeGlobal {
		for _, s := range r.static {
			if s.ID == item.ID {
				return fmt.Errorf("knowledge base %q already exists", item.ID)
			}
		}
	}
	existing, err := store.List()
	if err != nil {
		return err
	}
	for _, e := range existing {
		if e.ID == item.ID {
			return fmt.Errorf("knowledge base %q already exists", item.ID)
		}
	}
	if scope == core.ScopeProject {
		if err := applyProjectDefaults(&item, r.projectRoot); err != nil {
			return err
		}
	}
	existing = append(existing, item)
	return store.Save(existing)
}

func (r *Registry) Delete(scope, id string) error {
	scope, err := core.NormalizeScope(scope)
	if err != nil {
		return err
	}
	store, err := r.storeForScope(scope)
	if err != nil {
		return err
	}
	items, err := store.List()
	if err != nil {
		return err
	}
	for i := range items {
		if items[i].ID == id {
			items = append(items[:i], items[i+1:]...)
			return store.Save(items)
		}
	}
	if scope == core.ScopeGlobal {
		for _, s := range r.static {
			if s.ID == id {
				return fmt.Errorf("knowledge base %q is defined in static config", id)
			}
		}
	}
	return fmt.Errorf("knowledge base %q not found in %s runtime registry", id, scope)
}

func (r *Registry) SetEnabled(scope, id string, enabled bool) error {
	scope, err := core.NormalizeScope(scope)
	if err != nil {
		return err
	}
	store, err := r.storeForScope(scope)
	if err != nil {
		return err
	}
	items, err := store.List()
	if err != nil {
		return err
	}
	for i := range items {
		if items[i].ID == id {
			items[i].Enabled = enabled
			return store.Save(items)
		}
	}
	return fmt.Errorf("knowledge base %q not found in %s runtime registry", id, scope)
}

func (r *Registry) RuntimeItems(scope string) ([]RuntimeKnowledgeBase, error) {
	scope, err := core.NormalizeScope(scope)
	if err != nil {
		return nil, err
	}
	store, err := r.storeForScope(scope)
	if err != nil {
		return nil, err
	}
	return store.List()
}

// projectHash returns the first 8 hex chars of sha256(filepath.Clean(projectRoot)).
func projectHash(projectRoot string) string {
	sum := sha256.Sum256([]byte(filepath.Clean(projectRoot)))
	return hex.EncodeToString(sum[:])[:8]
}

// applyProjectDefaults fills in default relative paths and prefixes the chroma
// collection name. Called immediately before the project store persists the
// item, so all defaults end up as relative values.
func applyProjectDefaults(item *RuntimeKnowledgeBase, projectRoot string) error {
	if item.StoreConfig == nil {
		item.StoreConfig = map[string]any{}
	}
	rawPath, _ := item.StoreConfig["path"].(string)
	if strings.TrimSpace(rawPath) == "" {
		switch item.StoreType {
		case "sqlite":
			item.StoreConfig["path"] = ".knowledger/db"
		case "text":
			item.StoreConfig["path"] = filepath.Join(".knowledger", "data", item.ID)
		}
	}

	if item.StoreType != "sqlite" {
		return nil
	}
	if item.Indexing == nil {
		item.Indexing = map[string]any{}
	}
	semanticAny, ok := item.Indexing["semantic"]
	if !ok {
		semanticAny = map[string]any{}
		item.Indexing["semantic"] = semanticAny
	}
	semantic, _ := semanticAny.(map[string]any)
	if semantic == nil {
		semantic = map[string]any{}
		item.Indexing["semantic"] = semantic
	}

	if _, hasCollection := semantic["collection"]; !hasCollection {
		semantic["collection"] = "proj-" + projectHash(projectRoot) + "-" + item.ID
	}
	if _, hasPath := semantic["path"]; !hasPath {
		coll, _ := semantic["collection"].(string)
		semantic["path"] = filepath.Join(".knowledger", "chroma", coll)
	}
	// Funnel through config.ApplyKnowledgeBaseDefaults for provider/mode/etc parity,
	// but re-pin the relative path/collection we set explicitly afterward (the helper
	// would home-expand or otherwise overwrite them).
	cfg := config.KnowledgeBaseConfig{
		ID:          item.ID,
		StoreType:   item.StoreType,
		StoreConfig: item.StoreConfig,
		Indexing:    item.Indexing,
	}
	if err := config.ApplyKnowledgeBaseDefaults(&cfg); err != nil {
		return err
	}
	if rel, ok := item.StoreConfig["path"].(string); ok && !filepath.IsAbs(rel) {
		cfg.StoreConfig["path"] = rel
	}
	if semOut, ok := cfg.Indexing["semantic"].(map[string]any); ok {
		if rel, ok := semantic["path"].(string); ok && !filepath.IsAbs(rel) {
			semOut["path"] = rel
		}
	}
	item.StoreConfig = cfg.StoreConfig
	item.Indexing = cfg.Indexing
	return nil
}

// resolveProjectPaths returns a copy of item with relative path values
// expanded against projectRoot. Absolute and `~`-rooted values are passed
// through (with `~` expansion).
func resolveProjectPaths(item RuntimeKnowledgeBase, projectRoot string) RuntimeKnowledgeBase {
	out := item
	out.StoreConfig = cloneMap(item.StoreConfig)
	out.Indexing = cloneMap(item.Indexing)

	if p, ok := out.StoreConfig["path"].(string); ok {
		out.StoreConfig["path"] = expandProjectPath(p, projectRoot)
	}
	if sem, ok := out.Indexing["semantic"].(map[string]any); ok {
		semCopy := cloneMap(sem)
		if p, ok := semCopy["path"].(string); ok {
			semCopy["path"] = expandProjectPath(p, projectRoot)
		}
		out.Indexing["semantic"] = semCopy
	}
	return out
}

func expandProjectPath(p, projectRoot string) string {
	if p == "" {
		return p
	}
	if expanded, err := config.ExpandHomePath(p); err == nil {
		p = expanded
	}
	if filepath.IsAbs(p) {
		return p
	}
	return filepath.Join(projectRoot, p)
}

func cloneMap(in map[string]any) map[string]any {
	if in == nil {
		return nil
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
