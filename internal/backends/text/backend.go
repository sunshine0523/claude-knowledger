package text

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/kindbrave/knowledger/internal/core"
)

type Backend struct{}

func New() *Backend { return &Backend{} }

func (b *Backend) Add(_ context.Context, kb core.KnowledgeBase, input core.AddInput) (core.KnowledgeItem, core.IngestionResult, core.IndexStatus, error) {
	dir, ok := kb.StoreConfig["path"].(string)
	if !ok || dir == "" {
		return core.KnowledgeItem{}, core.IngestionResult{}, core.IndexStatus{}, &core.Error{Kind: core.ErrorKindConfig, Message: "text backend requires store_config.path"}
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return core.KnowledgeItem{}, core.IngestionResult{}, core.IndexStatus{}, err
	}

	now := time.Now().UTC()
	id := fmt.Sprintf("%d", now.UnixNano())
	path := filepath.Join(dir, id+".md")
	body := fmt.Sprintf("---\ntitle: %s\ntags: %s\n---\n\n%s\n", input.Title, strings.Join(input.Tags, ","), input.Content)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		return core.KnowledgeItem{}, core.IngestionResult{}, core.IndexStatus{}, err
	}

	item := core.KnowledgeItem{ID: id, KBID: kb.ID, Type: "document", Title: input.Title, Content: input.Content, Tags: input.Tags, CreatedAt: now, UpdatedAt: now}
	return item, core.IngestionResult{Success: true, ItemID: id}, core.IndexStatus{State: "not_indexed"}, nil
}

func (b *Backend) Search(_ context.Context, kb core.KnowledgeBase, opt core.SearchOptions) ([]core.SearchHit, error) {
	dir, ok := kb.StoreConfig["path"].(string)
	if !ok || dir == "" {
		return nil, &core.Error{Kind: core.ErrorKindConfig, Message: "text backend requires store_config.path"}
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var hits []core.SearchHit
	needle := strings.ToLower(opt.Query)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		content := string(data)
		if strings.Contains(strings.ToLower(content), needle) {
			hits = append(hits, core.SearchHit{
				ItemID:         strings.TrimSuffix(entry.Name(), ".md"),
				KBID:           kb.ID,
				ItemType:       "document",
				Title:          entry.Name(),
				Snippet:        opt.Query,
				ContentPreview: content,
				Score:          1,
				MatchMode:      "lexical",
				SourceBackend:  "text",
				Locator:        path,
			})
		}
	}
	if opt.Limit > 0 && len(hits) > opt.Limit {
		return hits[:opt.Limit], nil
	}
	return hits, nil
}

func (b *Backend) ListItems(_ context.Context, kb core.KnowledgeBase) ([]core.KnowledgeItem, error) {
	dir, ok := kb.StoreConfig["path"].(string)
	if !ok || dir == "" {
		return nil, &core.Error{Kind: core.ErrorKindConfig, Message: "text backend requires store_config.path"}
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	items := make([]core.KnowledgeItem, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		items = append(items, core.KnowledgeItem{ID: strings.TrimSuffix(entry.Name(), ".md"), KBID: kb.ID, Type: "document", Title: entry.Name()})
	}
	return items, nil
}

func (b *Backend) SupportsSemantic(core.KnowledgeBase) bool { return false }
