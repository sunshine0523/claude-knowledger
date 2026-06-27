package semantic

import (
	"context"
	"fmt"
	"os"

	"github.com/kindbrave/claude-knowledger/internal/core"
	"github.com/kindbrave/claude-knowledger/internal/indexing/chroma"
)

type ItemSource func(ctx context.Context) ([]core.KnowledgeItem, error)
type MetaProvider func(item core.KnowledgeItem) map[string]any

func (idx *Indexer) MaintainIndex(ctx context.Context, kb core.KnowledgeBase, opts core.IndexOptions, source ItemSource, meta MetaProvider) (core.IndexResult, error) {
	cfg, ok := idx.configFor(kb)
	if !ok {
		return core.IndexResult{Skipped: 1, Warnings: []string{fmt.Sprintf("%s: semantic indexing is not enabled", kb.ID)}}, nil
	}
	notify := progressNotifier(opts.Progress, kb.ID)

	if opts.Rebuild {
		if err := idx.resetPersistent(cfg); err != nil {
			return core.IndexResult{}, &core.Error{Kind: core.ErrorKindIndex, Message: fmt.Sprintf("reset persistent chroma at %s", cfg.Path), Cause: err}
		}
		if cfg.EffectiveMode() == chroma.ModePersistent && cfg.Path != "" {
			notify(core.IndexProgressEvent{Phase: core.IndexProgressPhaseRebuildReset, Message: cfg.Path})
		}
	}

	client, err := idx.client(cfg)
	if err != nil {
		return core.IndexResult{}, &core.Error{Kind: core.ErrorKindIndex, Message: "semantic index client unavailable", Cause: err}
	}
	items, err := source(ctx)
	if err != nil {
		return core.IndexResult{}, err
	}
	notify(core.IndexProgressEvent{Phase: core.IndexProgressPhaseStart, Total: len(items)})

	if opts.Rebuild {
		return idx.maintainRebuild(ctx, client, cfg.Collection, kb, items, meta, notify)
	}
	return idx.maintainIncremental(ctx, client, cfg.Collection, kb, items, meta, notify)
}

func (idx *Indexer) maintainIncremental(ctx context.Context, client chroma.Client, collection string, kb core.KnowledgeBase, items []core.KnowledgeItem, meta MetaProvider, notify func(core.IndexProgressEvent)) (core.IndexResult, error) {
	existing, err := client.ListByKB(ctx, collection, kb.ID)
	if err != nil {
		return core.IndexResult{}, err
	}
	byParent := map[string][]chroma.ChunkRecord{}
	for _, rec := range existing {
		byParent[rec.ParentID] = append(byParent[rec.ParentID], rec)
	}
	result := core.IndexResult{}
	total := len(items)
	for i, item := range items {
		var extra map[string]any
		if meta != nil {
			extra = meta(item)
		}
		records, present := byParent[item.ID]
		delete(byParent, item.ID)
		diskMtime, hasMtime := mtimeFromMeta(extra)
		if present && hasMtime && allMtimesEqual(records, diskMtime) {
			result.Skipped++
			notify(core.IndexProgressEvent{Phase: core.IndexProgressPhaseSkip, Item: item.ID, Done: i + 1, Total: total})
			continue
		}
		notify(core.IndexProgressEvent{Phase: core.IndexProgressPhaseIndex, Item: item.ID, Done: i + 1, Total: total})
		if err := idx.UpsertItem(ctx, kb, item, extra); err != nil {
			return result, err
		}
		result.Indexed++
	}
	deleteOrphans(ctx, client, collection, kb.ID, byParent, &result, notify)
	notify(core.IndexProgressEvent{Phase: core.IndexProgressPhaseDone, Done: result.Indexed + result.Skipped, Total: total})
	return result, nil
}

func (idx *Indexer) maintainRebuild(ctx context.Context, client chroma.Client, collection string, kb core.KnowledgeBase, items []core.KnowledgeItem, meta MetaProvider, notify func(core.IndexProgressEvent)) (core.IndexResult, error) {
	existing, err := client.ListByKB(ctx, collection, kb.ID)
	if err != nil {
		return core.IndexResult{}, err
	}
	byParent := map[string][]chroma.ChunkRecord{}
	for _, rec := range existing {
		byParent[rec.ParentID] = append(byParent[rec.ParentID], rec)
	}
	result := core.IndexResult{}
	deleteOrphans(ctx, client, collection, kb.ID, byParent, &result, notify)
	total := len(items)
	for i, item := range items {
		var extra map[string]any
		if meta != nil {
			extra = meta(item)
		}
		notify(core.IndexProgressEvent{Phase: core.IndexProgressPhaseIndex, Item: item.ID, Done: i + 1, Total: total})
		if err := idx.UpsertItem(ctx, kb, item, extra); err != nil {
			return result, err
		}
		result.Indexed++
	}
	notify(core.IndexProgressEvent{Phase: core.IndexProgressPhaseDone, Done: result.Indexed + result.Skipped, Total: total})
	return result, nil
}

// resetPersistent removes the persistent chroma data directory associated with
// cfg, after closing and evicting any cached client. This is the only safe
// recovery from a corrupted on-disk HNSW index, where chroma's C++ integrity
// check aborts the process at first collection access.
func (idx *Indexer) resetPersistent(cfg chroma.Config) error {
	if cfg.EffectiveMode() != chroma.ModePersistent || cfg.Path == "" {
		return nil
	}
	idx.mu.Lock()
	key := clientKey(cfg)
	if c := idx.clients[key]; c != nil {
		_ = c.Close()
		delete(idx.clients, key)
	}
	idx.mu.Unlock()
	if err := os.RemoveAll(cfg.Path); err != nil {
		return err
	}
	return nil
}

// deleteOrphans removes records whose parent items are no longer present.
// Records keyed by an empty ParentID predate the parent_id metadata and must
// be addressed by chunk id directly; DeleteByParent rejects an empty
// parentID, so it cannot serve those rows.
func deleteOrphans(ctx context.Context, client chroma.Client, collection, kbID string, byParent map[string][]chroma.ChunkRecord, result *core.IndexResult, notify func(core.IndexProgressEvent)) {
	for parentID, records := range byParent {
		if parentID == "" {
			for _, rec := range records {
				if err := client.Delete(ctx, collection, rec.ID); err != nil {
					result.Errors = append(result.Errors, fmt.Sprintf("cleanup orphan chunk %s: %v", rec.ID, err))
					continue
				}
				result.Deleted++
				notify(core.IndexProgressEvent{Phase: core.IndexProgressPhaseDeleteOrphan, Item: rec.ID})
			}
			continue
		}
		if err := client.DeleteByParent(ctx, collection, kbID, parentID); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("cleanup orphan parent %s: %v", parentID, err))
			continue
		}
		result.Deleted++
		notify(core.IndexProgressEvent{Phase: core.IndexProgressPhaseDeleteOrphan, Item: parentID})
	}
}

func progressNotifier(p core.IndexProgress, kbID string) func(core.IndexProgressEvent) {
	if p == nil {
		return func(core.IndexProgressEvent) {}
	}
	return func(ev core.IndexProgressEvent) {
		ev.KBID = kbID
		p(ev)
	}
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
