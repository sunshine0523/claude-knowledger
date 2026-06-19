package semantic

import (
	"context"
	"fmt"

	"github.com/kindbrave/knowledger/internal/core"
	"github.com/kindbrave/knowledger/internal/indexing/chroma"
)

type ItemSource func(ctx context.Context) ([]core.KnowledgeItem, error)
type MetaProvider func(item core.KnowledgeItem) map[string]any

func (idx *Indexer) MaintainIndex(ctx context.Context, kb core.KnowledgeBase, opts core.IndexOptions, source ItemSource, meta MetaProvider) (core.IndexResult, error) {
	cfg, ok := idx.configFor(kb)
	if !ok {
		return core.IndexResult{Skipped: 1, Warnings: []string{fmt.Sprintf("%s: semantic indexing is not enabled", kb.ID)}}, nil
	}
	client, err := idx.client(cfg)
	if err != nil {
		return core.IndexResult{}, &core.Error{Kind: core.ErrorKindIndex, Message: "semantic index client unavailable", Cause: err}
	}
	items, err := source(ctx)
	if err != nil {
		return core.IndexResult{}, err
	}
	if opts.Rebuild {
		return idx.maintainRebuild(ctx, client, cfg.Collection, kb, items, meta)
	}
	existing, err := client.ListByKB(ctx, cfg.Collection, kb.ID)
	if err != nil {
		return core.IndexResult{}, err
	}
	byParent := map[string][]chroma.ChunkRecord{}
	for _, rec := range existing {
		byParent[rec.ParentID] = append(byParent[rec.ParentID], rec)
	}
	result := core.IndexResult{}
	for _, item := range items {
		var extra map[string]any
		if meta != nil {
			extra = meta(item)
		}
		records, present := byParent[item.ID]
		delete(byParent, item.ID)
		diskMtime, hasMtime := mtimeFromMeta(extra)
		if present && hasMtime && allMtimesEqual(records, diskMtime) {
			result.Skipped++
			continue
		}
		if err := idx.UpsertItem(ctx, kb, item, extra); err != nil {
			return result, err
		}
		result.Indexed++
	}
	for parentID := range byParent {
		if err := client.DeleteByParent(ctx, cfg.Collection, kb.ID, parentID); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("cleanup orphan parent %s: %v", parentID, err))
			continue
		}
		result.Deleted++
	}
	return result, nil
}

func (idx *Indexer) maintainRebuild(ctx context.Context, client chroma.Client, collection string, kb core.KnowledgeBase, items []core.KnowledgeItem, meta MetaProvider) (core.IndexResult, error) {
	existing, err := client.ListByKB(ctx, collection, kb.ID)
	if err != nil {
		return core.IndexResult{}, err
	}
	seen := map[string]bool{}
	for _, rec := range existing {
		if seen[rec.ParentID] {
			continue
		}
		seen[rec.ParentID] = true
		if err := client.DeleteByParent(ctx, collection, kb.ID, rec.ParentID); err != nil {
			return core.IndexResult{}, err
		}
	}
	result := core.IndexResult{Deleted: len(seen)}
	for _, item := range items {
		var extra map[string]any
		if meta != nil {
			extra = meta(item)
		}
		if err := idx.UpsertItem(ctx, kb, item, extra); err != nil {
			return result, err
		}
		result.Indexed++
	}
	return result, nil
}

func mtimeFromMeta(meta map[string]any) (int64, bool) {
	if meta == nil {
		return 0, false
	}
	switch v := meta["mtime"].(type) {
	case int64:
		return v, true
	case int:
		return int64(v), true
	case float64:
		return int64(v), true
	}
	return 0, false
}

func allMtimesEqual(records []chroma.ChunkRecord, mtime int64) bool {
	for _, r := range records {
		if r.Mtime != mtime {
			return false
		}
	}
	return true
}
