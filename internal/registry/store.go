package registry

import (
	"strconv"
	"sync"
)

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

// Store persists the runtime knowledge-base list. Version returns an opaque
// token that changes whenever the underlying data changes; callers compare
// it across calls to detect external mutations (e.g. another process writing
// the same registry file) and reload accordingly. A consistent empty token
// is returned when the store has no data yet.
type Store interface {
	List() ([]RuntimeKnowledgeBase, error)
	Save([]RuntimeKnowledgeBase) error
	Version() (string, error)
}

type MemoryStore struct {
	mu      sync.Mutex
	items   []RuntimeKnowledgeBase
	version uint64
}

func NewMemoryStore(items []RuntimeKnowledgeBase) *MemoryStore {
	return &MemoryStore{items: items}
}

func (m *MemoryStore) List() ([]RuntimeKnowledgeBase, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]RuntimeKnowledgeBase, len(m.items))
	copy(out, m.items)
	return out, nil
}

func (m *MemoryStore) Save(items []RuntimeKnowledgeBase) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.items = make([]RuntimeKnowledgeBase, len(items))
	copy(m.items, items)
	m.version++
	return nil
}

func (m *MemoryStore) Version() (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return strconv.FormatUint(m.version, 10), nil
}
