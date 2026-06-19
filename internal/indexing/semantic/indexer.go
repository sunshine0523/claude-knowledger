package semantic

import (
	"errors"
	"strconv"
	"strings"
	"sync"

	"github.com/kindbrave/knowledger/internal/indexing/chroma"
	"github.com/kindbrave/knowledger/internal/indexing/chunking"
)

type Indexer struct {
	factory  chroma.Factory
	splitter chunking.Splitter
	mu       sync.Mutex
	clients  map[string]chroma.Client
}

func NewIndexer(factory chroma.Factory, splitter chunking.Splitter) *Indexer {
	if factory == nil {
		factory = chroma.NewClient
	}
	if splitter == nil {
		splitter = chunking.Default()
	}
	return &Indexer{factory: factory, splitter: splitter, clients: map[string]chroma.Client{}}
}

func (idx *Indexer) Close() error {
	idx.mu.Lock()
	clients := make([]chroma.Client, 0, len(idx.clients))
	for _, c := range idx.clients {
		clients = append(clients, c)
	}
	idx.clients = nil
	idx.mu.Unlock()

	var err error
	for _, c := range clients {
		err = errors.Join(err, c.Close())
	}
	return err
}

func (idx *Indexer) client(cfg chroma.Config) (chroma.Client, error) {
	idx.mu.Lock()
	defer idx.mu.Unlock()
	if idx.clients == nil {
		idx.clients = map[string]chroma.Client{}
	}
	key := clientKey(cfg)
	if c := idx.clients[key]; c != nil {
		return c, nil
	}
	c, err := idx.factory(cfg)
	if err != nil {
		return nil, err
	}
	idx.clients[key] = c
	return c, nil
}

func clientKey(cfg chroma.Config) string {
	return strings.Join([]string{cfg.EffectiveMode(), cfg.BaseURL, cfg.Path, strconv.FormatBool(cfg.AutoDownload)}, "\x00")
}
