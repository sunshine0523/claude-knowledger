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

type Registry struct {
	static []config.KnowledgeBaseConfig
	store  Store
}

func New(static []config.KnowledgeBaseConfig, store Store) *Registry {
	return &Registry{static: static, store: store}
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
	runtimeItems, err := r.store.List()
	if err != nil {
		return nil, err
	}

	merged := map[string]KnowledgeBaseRecord{}
	for _, item := range r.static {
		kb := staticToCore(item)
		merged[item.ID] = KnowledgeBaseRecord{KnowledgeBase: kb, Source: SourceStatic, Deletable: false}
	}
	for _, item := range runtimeItems {
		kb := runtimeToCore(item, core.ScopeGlobal)
		merged[item.ID] = KnowledgeBaseRecord{KnowledgeBase: kb, Source: SourceRuntime, Deletable: true}
	}

	keys := make([]string, 0, len(merged))
	for key := range merged {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	out := make([]KnowledgeBaseRecord, 0, len(keys))
	for _, key := range keys {
		out = append(out, merged[key])
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
	items, err := r.store.List()
	if err != nil {
		return err
	}
	items = append(items, item)
	return r.store.Save(items)
}

func (r *Registry) Delete(id string) error {
	items, err := r.store.List()
	if err != nil {
		return err
	}
	for i := range items {
		if items[i].ID == id {
			items = append(items[:i], items[i+1:]...)
			return r.store.Save(items)
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
	return r.store.List()
}

func (r *Registry) SetEnabled(id string, enabled bool) error {
	items, err := r.store.List()
	if err != nil {
		return err
	}
	for i := range items {
		if items[i].ID == id {
			items[i].Enabled = enabled
			return r.store.Save(items)
		}
	}
	return fmt.Errorf("knowledge base %q not found in runtime registry", id)
}
