package semantic

import (
	"strings"

	"github.com/kindbrave/knowledger/internal/core"
	"github.com/kindbrave/knowledger/internal/indexing/chroma"
)

func (idx *Indexer) SupportsKB(kb core.KnowledgeBase) bool {
	_, ok := idx.configFor(kb)
	return ok
}

func (idx *Indexer) configFor(kb core.KnowledgeBase) (chroma.Config, bool) {
	semanticRaw, ok := kb.Indexing["semantic"]
	if !ok {
		return chroma.Config{}, false
	}
	semantic, ok := semanticRaw.(map[string]any)
	if !ok {
		return chroma.Config{}, false
	}
	enabled, _ := semantic["enabled"].(bool)
	if !enabled {
		return chroma.Config{}, false
	}
	provider, _ := semantic["provider"].(string)
	if !strings.EqualFold(provider, "chroma") {
		return chroma.Config{}, false
	}
	cfg := chroma.Config{Collection: kb.ID, AutoDownload: true}
	cfg.Mode, _ = semantic["mode"].(string)
	cfg.BaseURL, _ = semantic["base_url"].(string)
	cfg.Path, _ = semantic["path"].(string)
	if collection, ok := semantic["collection"].(string); ok && collection != "" {
		cfg.Collection = collection
	}
	if autoDownload, ok := semantic["auto_download"].(bool); ok {
		cfg.AutoDownload = autoDownload
	}
	return cfg, true
}
