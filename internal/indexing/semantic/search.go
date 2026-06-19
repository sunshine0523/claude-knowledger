package semantic

import (
	"context"
	"sort"

	"github.com/kindbrave/knowledger/internal/core"
	"github.com/kindbrave/knowledger/internal/indexing/chroma"
)

func (idx *Indexer) Search(ctx context.Context, kb core.KnowledgeBase, query string, limit int, mode string) ([]core.SearchHit, error) {
	cfg, ok := idx.configFor(kb)
	if !ok {
		return nil, nil
	}
	tokens := core.TokenizeQuery(query)
	if len(tokens) == 0 {
		return nil, nil
	}
	client, err := idx.client(cfg)
	if err != nil {
		return nil, err
	}
	merged := map[string]chroma.Hit{}
	for _, tok := range tokens {
		raw, err := client.Query(ctx, cfg.Collection, tok, limit)
		if err != nil {
			return nil, err
		}
		for _, h := range raw {
			kbID, _ := h.Metadata["kb_id"].(string)
			if kbID != kb.ID {
				continue
			}
			if prev, exists := merged[h.ItemID]; !exists || h.Score > prev.Score {
				merged[h.ItemID] = h
			}
		}
	}
	byParent := map[string]chroma.Hit{}
	for _, h := range merged {
		parent, _ := h.Metadata["parent_id"].(string)
		if parent == "" {
			continue
		}
		if prev, exists := byParent[parent]; !exists || h.Score > prev.Score {
			byParent[parent] = h
		}
	}
	out := make([]core.SearchHit, 0, len(byParent))
	for parentID, h := range byParent {
		title, _ := h.Metadata["title"].(string)
		out = append(out, core.SearchHit{
			ItemID:         parentID,
			KBID:           kb.ID,
			Title:          title,
			Snippet:        h.Content,
			ContentPreview: h.Content,
			Score:          h.Score,
			MatchMode:      mode,
			SourceBackend:  "chroma",
			Metadata:       h.Metadata,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Score > out[j].Score })
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}
