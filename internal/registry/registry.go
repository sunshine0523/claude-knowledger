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

func (r *Registry) Create(item RuntimeKnowledgeBase) error {
	if item.ID == "" {
		return fmt.Errorf("knowledge base id is required")
	}
	existing, err := r.List()
	if err != nil {
		return err
	}
	for _, kb := range existing {
		if kb.ID == item.ID {
			return fmt.Errorf("knowledge base %q already exists", item.ID)
		}
	}
	items, err := r.globalStore.List()
	if err != nil {
		return err
	}
	items = append(items, item)
	return r.globalStore.Save(items)
}

func (r *Registry) Delete(id string) error {
	items, err := r.globalStore.List()
	if err != nil {
		return err
	}
	for i := range items {
		if items[i].ID == id {
			items = append(items[:i], items[i+1:]...)
			return r.globalStore.Save(items)
		}
	}
	for _, item := range r.static {
		if item.ID == id {
			return fmt.Errorf("knowledge base %q is defined in static config", id)
		}
	}
	return fmt.Errorf("knowledge base %q not found in runtime registry", id)
}

func (r *Registry) RuntimeItems() ([]RuntimeKnowledgeBase, error) {
	return r.globalStore.List()
}

func (r *Registry) SetEnabled(id string, enabled bool) error {
	items, err := r.globalStore.List()
	if err != nil {
		return err
	}
	for i := range items {
		if items[i].ID == id {
			items[i].Enabled = enabled
			return r.globalStore.Save(items)
		}
	}
	return fmt.Errorf("knowledge base %q not found in runtime registry", id)
}
