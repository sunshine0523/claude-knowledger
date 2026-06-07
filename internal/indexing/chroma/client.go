package chroma

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	chromav2 "github.com/amikos-tech/chroma-go/pkg/api/v2"
)

const (
	ModePersistent = "persistent"
	ModeHTTP       = "http"
)

type Config struct {
	Mode         string
	BaseURL      string
	Path         string
	Collection   string
	AutoDownload bool
}

func (c Config) EffectiveMode() string {
	if c.Mode == "" {
		return ModePersistent
	}
	return c.Mode
}

type Item struct {
	ID       string
	KBID     string
	Title    string
	Content  string
	Tags     []string
	Metadata map[string]any
}

type Hit struct {
	ItemID   string
	Content  string
	Score    float64
	Metadata map[string]any
}

func (h Hit) Title() string {
	title, _ := h.Metadata["title"].(string)
	return title
}

type Client interface {
	Upsert(ctx context.Context, collection string, item Item) error
	Query(ctx context.Context, collection string, query string, limit int) ([]Hit, error)
	Delete(ctx context.Context, collection string, itemID string) error
	Close() error
}

type Factory func(Config) (Client, error)

type client struct {
	client            chromav2.Client
	defaultCollection string
	mu                sync.Mutex
	collections       map[string]chromav2.Collection
}

func NewClient(cfg Config) (Client, error) {
	var chromaClient chromav2.Client
	var err error

	switch cfg.EffectiveMode() {
	case ModePersistent:
		chromaClient, err = chromav2.NewPersistentClient(
			chromav2.WithPersistentPath(cfg.Path),
			chromav2.WithPersistentLibraryAutoDownload(cfg.AutoDownload),
		)
	case ModeHTTP:
		chromaClient, err = chromav2.NewHTTPClient(chromav2.WithBaseURL(cfg.BaseURL))
	default:
		return nil, fmt.Errorf("unsupported chroma mode %q", cfg.EffectiveMode())
	}
	if err != nil {
		return nil, err
	}

	return &client{
		client:            chromaClient,
		defaultCollection: cfg.Collection,
		collections:       map[string]chromav2.Collection{},
	}, nil
}

func (c *client) Upsert(ctx context.Context, collection string, item Item) error {
	col, err := c.collection(ctx, collection)
	if err != nil {
		return err
	}

	metadataMap := make(map[string]any, len(item.Metadata)+3)
	for key, value := range item.Metadata {
		metadataMap[key] = value
	}
	metadataMap["kb_id"] = item.KBID
	metadataMap["title"] = item.Title
	if len(item.Tags) > 0 {
		metadataMap["tags"] = item.Tags
	}

	metadata, err := chromav2.NewDocumentMetadataFromMap(metadataMap)
	if err != nil {
		return err
	}

	return col.Upsert(ctx,
		chromav2.WithIDs(chromav2.DocumentID(item.ID)),
		chromav2.WithTexts(item.Content),
		chromav2.WithMetadatas(metadata),
	)
}

func (c *client) Query(ctx context.Context, collection string, query string, limit int) ([]Hit, error) {
	col, err := c.collection(ctx, collection)
	if err != nil {
		return nil, err
	}

	limit = normalizeLimit(limit)
	result, err := col.Query(ctx,
		chromav2.WithQueryTexts(query),
		chromav2.WithNResults(limit),
		chromav2.WithInclude(chromav2.IncludeDocuments, chromav2.IncludeMetadatas, chromav2.IncludeDistances),
	)
	if err != nil {
		return nil, err
	}

	rows, ok := result.(interface{ Rows() []chromav2.ResultRow })
	if !ok {
		return nil, fmt.Errorf("chroma query result does not expose rows")
	}

	hits := make([]Hit, 0, len(rows.Rows()))
	for _, row := range rows.Rows() {
		hits = append(hits, Hit{
			ItemID:   string(row.ID),
			Content:  row.Document,
			Score:    ScoreFromDistance(row.Score),
			Metadata: documentMetadataMap(row.Metadata),
		})
	}
	return hits, nil
}

func normalizeLimit(limit int) int {
	if limit <= 0 {
		return 10
	}
	return limit
}

func (c *client) Delete(ctx context.Context, collection string, itemID string) error {
	col, err := c.collection(ctx, collection)
	if err != nil {
		return err
	}

	return col.Delete(ctx, chromav2.WithIDs(chromav2.DocumentID(itemID)))
}

func (c *client) Close() error {
	return c.client.Close()
}

func (c *client) collection(ctx context.Context, name string) (chromav2.Collection, error) {
	if name == "" {
		name = c.defaultCollection
	}
	if name == "" {
		return nil, fmt.Errorf("chroma collection is required")
	}

	c.mu.Lock()
	cached := c.collections[name]
	c.mu.Unlock()
	if cached != nil {
		return cached, nil
	}

	col, err := c.client.GetOrCreateCollection(ctx, name)
	if err != nil {
		return nil, err
	}

	c.mu.Lock()
	c.collections[name] = col
	c.mu.Unlock()
	return col, nil
}

func documentMetadataMap(metadata chromav2.DocumentMetadata) map[string]any {
	if metadata == nil {
		return map[string]any{}
	}

	if keyed, ok := metadata.(interface{ Keys() []string }); ok {
		out := make(map[string]any, len(keyed.Keys()))
		for _, key := range keyed.Keys() {
			value, ok := metadata.GetRaw(key)
			if !ok {
				continue
			}
			out[key] = rawMetadataValue(value)
		}
		return out
	}

	if marshaler, ok := metadata.(json.Marshaler); ok {
		data, err := marshaler.MarshalJSON()
		if err == nil {
			var out map[string]any
			if err := json.Unmarshal(data, &out); err == nil {
				return out
			}
		}
	}

	return map[string]any{}
}

func rawMetadataValue(value any) any {
	switch value := value.(type) {
	case chromav2.MetadataValue:
		if raw, ok := value.GetRaw(); ok {
			return raw
		}
	case *chromav2.MetadataValue:
		if raw, ok := value.GetRaw(); ok {
			return raw
		}
	}
	return value
}

func ScoreFromDistance(distance float64) float64 {
	if distance < 0 {
		return 0
	}
	return 1 / (1 + distance)
}
