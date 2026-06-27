package semantic

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"

	"github.com/kindbrave/claude-knowledger/internal/core"
	"github.com/kindbrave/claude-knowledger/internal/indexing/chroma"
	"github.com/kindbrave/claude-knowledger/internal/indexing/chunking"
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

func (idx *Indexer) UpsertItem(ctx context.Context, kb core.KnowledgeBase, item core.KnowledgeItem, extraMeta map[string]any) error {
	cfg, ok := idx.configFor(kb)
	if !ok {
		return nil
	}
	client, err := idx.client(cfg)
	if err != nil {
		return err
	}
	chunks := idx.splitter.Split(item.Content)
	if len(chunks) == 0 {
		return nil
	}
	if err := client.DeleteByParent(ctx, cfg.Collection, kb.ID, item.ID); err != nil {
		return err
	}
	for _, c := range chunks {
		if err := client.Upsert(ctx, cfg.Collection, buildChromaItem(kb, item, c, extraMeta)); err != nil {
			_ = client.DeleteByParent(ctx, cfg.Collection, kb.ID, item.ID)
			return fmt.Errorf("semantic upsert failed for item %s chunk %d/%d: %w", item.ID, c.Index, c.Total, err)
		}
	}
	return nil
}

func (idx *Indexer) DeleteItem(ctx context.Context, kb core.KnowledgeBase, itemID string) error {
	cfg, ok := idx.configFor(kb)
	if !ok {
		return nil
	}
	client, err := idx.client(cfg)
	if err != nil {
		return err
	}
	return client.DeleteByParent(ctx, cfg.Collection, kb.ID, itemID)
}

func buildChromaItem(kb core.KnowledgeBase, item core.KnowledgeItem, c chunking.Chunk, extraMeta map[string]any) chroma.Item {
	metadata := make(map[string]any, len(item.Metadata)+len(extraMeta)+3)
	for k, v := range item.Metadata {
		metadata[k] = v
	}
	for k, v := range extraMeta {
		metadata[k] = v
	}
	metadata["parent_id"] = item.ID
	metadata["chunk_index"] = c.Index
	metadata["chunk_total"] = c.Total
	return chroma.Item{
		ID:       fmt.Sprintf("%s#chunk-%d", item.ID, c.Index),
		KBID:     kb.ID,
		Title:    item.Title,
		Content:  c.Text,
		Tags:     item.Tags,
		Metadata: metadata,
	}
}
