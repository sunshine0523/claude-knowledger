package registry

import (
	"fmt"
	"sort"

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
			key := recordKey{Scope: core.ScopeProject, ID: item.ID}
			merged[key] = KnowledgeBaseRecord{KnowledgeBase: runtimeToCore(item, core.ScopeProject), Source: SourceRuntime, Deletable: true}
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
