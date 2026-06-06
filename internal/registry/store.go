package registry

type RuntimeKnowledgeBase struct {
	ID                string         `json:"id"`
	Name              string         `json:"name"`
	StoreType         string         `json:"store_type"`
	StoreConfig       map[string]any `json:"store_config"`
	Enabled           bool           `json:"enabled"`
	DefaultSearchMode string         `json:"default_search_mode"`
	Indexing          map[string]any `json:"indexing"`
	Tags              []string       `json:"tags"`
}

type Store interface {
	List() ([]RuntimeKnowledgeBase, error)
	Save([]RuntimeKnowledgeBase) error
}

type MemoryStore struct {
	items []RuntimeKnowledgeBase
}

func NewMemoryStore(items []RuntimeKnowledgeBase) *MemoryStore {
	return &MemoryStore{items: items}
}

func (m *MemoryStore) List() ([]RuntimeKnowledgeBase, error) {
	out := make([]RuntimeKnowledgeBase, len(m.items))
	copy(out, m.items)
	return out, nil
}

func (m *MemoryStore) Save(items []RuntimeKnowledgeBase) error {
	m.items = make([]RuntimeKnowledgeBase, len(items))
	copy(m.items, items)
	return nil
}
