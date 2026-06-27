package semantic

import (
	"context"
	"sort"
	"strings"

	"github.com/kindbrave/claude-knowledger/internal/core"
	"github.com/kindbrave/claude-knowledger/internal/indexing/chroma"
)

// Search runs a single embedding query against chroma using the full query
// string. Splitting the query into lexical tokens and querying each one
// separately destroys the joint semantics the embedding model is supposed to
// capture, so the whole query is passed through verbatim.
func (idx *Indexer) Search(ctx context.Context, kb core.KnowledgeBase, query string, limit int, mode string) ([]core.SearchHit, error) {
	cfg, ok := idx.configFor(kb)
	if !ok {
		return nil, nil
	}
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, nil
	}
	client, err := idx.client(cfg)
	if err != nil {
		return nil, err
	}
	raw, err := client.Query(ctx, cfg.Collection, query, limit)
	if err != nil {
		return nil, err
	}
	byParent := map[string]chroma.Hit{}
	for _, h := range raw {
		kbID, _ := h.Metadata["kb_id"].(string)
		if kbID != kb.ID {
			continue
		}
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
