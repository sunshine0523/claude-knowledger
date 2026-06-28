package text

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/kindbrave/claude-knowledger/internal/core"
	"github.com/kindbrave/claude-knowledger/internal/indexing/semantic"
	"gopkg.in/yaml.v3"
)

var supportedTextExtensions = map[string]struct{}{
	".md":  {},
	".txt": {},
}

type Backend struct {
	indexer *semantic.Indexer
}

type Option func(*Backend)

func WithIndexer(idx *semantic.Indexer) Option {
	return func(b *Backend) {
		if idx != nil {
			b.indexer = idx
		}
	}
}

func New(opts ...Option) *Backend {
	b := &Backend{}
	for _, opt := range opts {
		if opt != nil {
			opt(b)
		}
	}
	return b
}

func (b *Backend) supportsKBSemantic(kb core.KnowledgeBase) bool {
	return b.indexer != nil && b.indexer.SupportsKB(kb)
}

func (b *Backend) Add(ctx context.Context, kb core.KnowledgeBase, input core.AddInput) (core.KnowledgeItem, core.IngestionResult, core.IndexStatus, error) {
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
	if !b.supportsKBSemantic(kb) {
		return item, core.IngestionResult{Success: true, ItemID: id}, core.IndexStatus{State: "not_indexed"}, nil
	}
	info, _ := os.Stat(path)
	var mtime int64
	if info != nil {
		mtime = info.ModTime().Unix()
	}
	extra := map[string]any{
		"path":  itemPathRel(dir, path),
		"mtime": mtime,
	}
	if err := b.indexer.UpsertItem(ctx, kb, item, extra); err != nil {
		_ = os.Remove(path)
		return core.KnowledgeItem{}, core.IngestionResult{}, core.IndexStatus{}, fmt.Errorf("semantic index failed; file rolled back: %w", err)
	}
	return item, core.IngestionResult{Success: true, ItemID: id}, core.IndexStatus{State: "indexed"}, nil
}

func (b *Backend) Search(ctx context.Context, kb core.KnowledgeBase, opt core.SearchOptions) ([]core.SearchHit, error) {
	switch opt.SearchMode {
	case "semantic":
		if !b.supportsKBSemantic(kb) {
			return b.lexicalSearch(ctx, kb, opt)
		}
		hits, err := b.indexer.Search(ctx, kb, opt.Query, opt.Limit, "semantic")
		if err != nil {
			return nil, err
		}
		return b.enrichWithFileMeta(kb, hits), nil
	case "hybrid":
		if !b.supportsKBSemantic(kb) {
			return b.lexicalSearch(ctx, kb, opt)
		}
		semanticHits, err := b.indexer.Search(ctx, kb, opt.Query, opt.Limit, "hybrid")
		if err != nil {
			return nil, err
		}
		semanticHits = b.enrichWithFileMeta(kb, semanticHits)
		lexicalHits, err := b.lexicalSearch(ctx, kb, opt)
		if err != nil {
			return nil, err
		}
		return mergeHybridHits(lexicalHits, semanticHits, opt.Limit), nil
	default:
		return b.lexicalSearch(ctx, kb, opt)
	}
}

// mergeHybridHits combines lexical and semantic hits with lexical taking
// priority. Lexical hits are placed first (literal matches are the strongest
// signal, especially for short Chinese queries where embeddings are unreliable),
// then semantic hits are appended for items not already covered. Every hit's
// MatchMode is rewritten to "hybrid" so callers can see how it was retrieved.
func mergeHybridHits(lexicalHits, semanticHits []core.SearchHit, limit int) []core.SearchHit {
	merged := make([]core.SearchHit, 0, len(lexicalHits)+len(semanticHits))
	seen := make(map[string]bool, len(lexicalHits)+len(semanticHits))
	for _, h := range lexicalHits {
		h.MatchMode = "hybrid"
		merged = append(merged, h)
		seen[h.ItemID] = true
	}
	for _, h := range semanticHits {
		if seen[h.ItemID] {
			continue
		}
		h.MatchMode = "hybrid"
		merged = append(merged, h)
		seen[h.ItemID] = true
	}
	if limit > 0 && len(merged) > limit {
		merged = merged[:limit]
	}
	return merged
}

func (b *Backend) lexicalSearch(_ context.Context, kb core.KnowledgeBase, opt core.SearchOptions) ([]core.SearchHit, error) {
	dir, ok := kb.StoreConfig["path"].(string)
	if !ok || dir == "" {
		return nil, &core.Error{Kind: core.ErrorKindConfig, Message: "text backend requires store_config.path"}
	}

	tokens := core.TokenizeQuery(opt.Query)
	if len(tokens) == 0 {
		return nil, nil
	}
	lowerTokens := make([]string, len(tokens))
	for i, t := range tokens {
		lowerTokens[i] = strings.ToLower(t)
	}

	var hits []core.SearchHit
	if err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		content, _, skip, err := readTextItemFile(dir, path)
		if err != nil {
			return err
		}
		if skip {
			return nil
		}
		lowerContent := strings.ToLower(content)
		matched := false
		for _, tok := range lowerTokens {
			if strings.Contains(lowerContent, tok) {
				matched = true
				break
			}
		}
		if !matched {
			return nil
		}
		hits = append(hits, core.SearchHit{
			ItemID:         itemIDForPath(dir, path),
			KBID:           kb.ID,
			ItemType:       "document",
			Title:          itemTitleForPath(dir, path),
			Snippet:        opt.Query,
			ContentPreview: content,
			Score:          1,
			MatchMode:      "lexical",
			SourceBackend:  "text",
			Locator:        path,
		})
		return nil
	}); err != nil {
		return nil, err
	}
	if opt.Limit > 0 && len(hits) > opt.Limit {
		return hits[:opt.Limit], nil
	}
	return hits, nil
}

func (b *Backend) enrichWithFileMeta(kb core.KnowledgeBase, hits []core.SearchHit) []core.SearchHit {
	dir, _ := kb.StoreConfig["path"].(string)
	out := make([]core.SearchHit, 0, len(hits))
	for _, hit := range hits {
		path, err := safeItemPath(dir, hit.ItemID)
		if err != nil {
			continue
		}
		if _, err := os.Stat(path); err != nil {
			continue
		}
		hit.Title = itemTitleForPath(dir, path)
		hit.Locator = path
		hit.SourceBackend = "text"
		hit.ItemType = "document"
		out = append(out, hit)
	}
	return out
}

func itemPathRel(dir, path string) string {
	rel, err := filepath.Rel(dir, path)
	if err != nil {
		return filepath.Base(path)
	}
	return filepath.ToSlash(rel)
}

func (b *Backend) ListItems(_ context.Context, kb core.KnowledgeBase) ([]core.KnowledgeItem, error) {
	dir, ok := kb.StoreConfig["path"].(string)
	if !ok || dir == "" {
		return nil, &core.Error{Kind: core.ErrorKindConfig, Message: "text backend requires store_config.path"}
	}
	if _, err := os.Stat(dir); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var items []core.KnowledgeItem
	if err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		content, info, skip, err := readTextItemFile(dir, path)
		if err != nil {
			return err
		}
		if skip {
			return nil
		}

		// Parse frontmatter and generate summary for markdown files
		var metadata map[string]any
		var summary string
		var title string
		bodyContent := content

		if strings.HasSuffix(strings.ToLower(path), ".md") {
			metadata, bodyContent, title = parseFrontmatter(content)
			summary = generateSummary(bodyContent, 200)
		}

		// Use title from frontmatter if available, otherwise use file path
		if title == "" {
			title = itemTitleForPath(dir, path)
		}

		items = append(items, core.KnowledgeItem{
			ID:        itemIDForPath(dir, path),
			KBID:      kb.ID,
			Type:      "document",
			Title:     title,
			Content:   content,
			Summary:   summary,
			Metadata:  metadata,
			CreatedAt: info.ModTime(),
			UpdatedAt: info.ModTime(),
		})
		return nil
	}); err != nil {
		return nil, err
	}
	return items, nil
}

func (b *Backend) GetItem(_ context.Context, kb core.KnowledgeBase, itemID string) (core.KnowledgeItem, error) {
	dir, ok := kb.StoreConfig["path"].(string)
	if !ok || dir == "" {
		return core.KnowledgeItem{}, &core.Error{Kind: core.ErrorKindConfig, Message: "text backend requires store_config.path"}
	}
	originalPath, err := safeItemPath(dir, itemID)
	if err != nil {
		return core.KnowledgeItem{}, err
	}
	content, originalInfo, skip, err := readTextItemFile(dir, originalPath)
	if err != nil {
		if os.IsNotExist(err) {
			return core.KnowledgeItem{}, &core.Error{Kind: core.ErrorKindStore, Message: "knowledge item not found"}
		}
		return core.KnowledgeItem{}, err
	}
	if skip {
		return core.KnowledgeItem{}, &core.Error{Kind: core.ErrorKindStore, Message: "knowledge item not found"}
	}
	originalBase, err := filepath.Abs(dir)
	if err != nil {
		return core.KnowledgeItem{}, err
	}

	// Parse frontmatter and generate summary for markdown files
	var metadata map[string]any
	var summary string
	var title string
	bodyContent := content

	if strings.HasSuffix(strings.ToLower(originalPath), ".md") {
		metadata, bodyContent, title = parseFrontmatter(content)
		summary = generateSummary(bodyContent, 200)
	}

	// Use title from frontmatter if available, otherwise use file path
	if title == "" {
		title = itemTitleForPath(originalBase, originalPath)
	}

	return core.KnowledgeItem{
		ID:        itemIDForPath(originalBase, originalPath),
		KBID:      kb.ID,
		Type:      "document",
		Title:     title,
		Content:   content,
		Summary:   summary,
		Metadata:  metadata,
		CreatedAt: originalInfo.ModTime(),
		UpdatedAt: originalInfo.ModTime(),
	}, nil
}

func (b *Backend) DeleteItem(ctx context.Context, kb core.KnowledgeBase, itemID string) error {
	dir, ok := kb.StoreConfig["path"].(string)
	if !ok || dir == "" {
		return &core.Error{Kind: core.ErrorKindConfig, Message: "text backend requires store_config.path"}
	}
	path, err := safeItemPath(dir, itemID)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil {
		if os.IsNotExist(err) {
			return &core.Error{Kind: core.ErrorKindStore, Message: "knowledge item not found"}
		}
		return err
	}
	if b.supportsKBSemantic(kb) {
		if err := b.indexer.DeleteItem(ctx, kb, itemID); err != nil {
			log.Printf("text backend: chroma delete failed for %s/%s: %v", kb.ID, itemID, err)
		}
	}
	return nil
}

func isSupportedTextFile(path string) bool {
	_, ok := supportedTextExtensions[strings.ToLower(filepath.Ext(path))]
	return ok
}

func readTextItemFile(dir string, path string) (string, os.FileInfo, bool, error) {
	if !isSupportedTextFile(path) {
		return "", nil, true, nil
	}
	originalInfo, err := os.Lstat(path)
	if err != nil {
		return "", nil, false, err
	}
	if originalInfo.IsDir() {
		return "", nil, true, nil
	}
	_, resolvedPath, err := resolvedContainedPath(dir, path)
	if err != nil {
		if originalInfo.Mode()&os.ModeSymlink != 0 {
			return "", nil, true, nil
		}
		return "", nil, false, err
	}
	if !isSupportedTextFile(resolvedPath) {
		return "", nil, true, nil
	}
	resolvedInfo, err := os.Stat(resolvedPath)
	if err != nil {
		return "", nil, false, err
	}
	if !resolvedInfo.Mode().IsRegular() {
		return "", nil, true, nil
	}
	file, err := os.Open(resolvedPath)
	if err != nil {
		return "", nil, false, err
	}
	defer file.Close()
	data, err := io.ReadAll(file)
	if err != nil {
		return "", nil, false, err
	}
	return string(data), originalInfo, false, nil
}

func itemIDForPath(dir string, path string) string {
	rel, err := filepath.Rel(dir, path)
	if err != nil {
		return filepath.Base(path)
	}
	rel = filepath.ToSlash(rel)
	if filepath.Ext(rel) == ".md" {
		return strings.TrimSuffix(rel, filepath.Ext(rel))
	}
	return rel
}

func itemTitleForPath(dir string, path string) string {
	rel, err := filepath.Rel(dir, path)
	if err != nil {
		return filepath.Base(path)
	}
	return filepath.ToSlash(rel)
}

// parseFrontmatter extracts YAML frontmatter from markdown content.
// Returns metadata, content without frontmatter, and title from frontmatter if available.
func parseFrontmatter(content string) (metadata map[string]any, bodyContent string, title string) {
	metadata = make(map[string]any)
	bodyContent = content

	// Match YAML frontmatter: ---\n...\n---
	re := regexp.MustCompile(`(?s)^---\s*\n(.*?)\n---\s*\n(.*)$`)
	matches := re.FindStringSubmatch(content)

	if len(matches) != 3 {
		return metadata, bodyContent, ""
	}

	// Parse YAML frontmatter
	yamlContent := matches[1]
	bodyContent = matches[2]

	var frontmatter map[string]any
	if err := yaml.Unmarshal([]byte(yamlContent), &frontmatter); err == nil {
		metadata = frontmatter
		// Extract title if present
		if t, ok := frontmatter["title"].(string); ok {
			title = t
		}
	}

	return metadata, bodyContent, title
}

// generateSummary creates a summary from content (first few sentences or first paragraph).
func generateSummary(content string, maxLen int) string {
	if maxLen == 0 {
		maxLen = 200
	}

	// Remove multiple newlines and trim
	content = strings.TrimSpace(content)
	if content == "" {
		return ""
	}

	// Try to get first paragraph
	paragraphs := strings.Split(content, "\n\n")
	firstPara := strings.TrimSpace(paragraphs[0])

	// Remove markdown headers
	firstPara = regexp.MustCompile(`^#+\s+`).ReplaceAllString(firstPara, "")

	// If first paragraph is too long, truncate at sentence boundary
	if len(firstPara) > maxLen {
		sentences := regexp.MustCompile(`[.!?]+\s+`).Split(firstPara, -1)
		summary := ""
		for _, sentence := range sentences {
			if len(summary)+len(sentence) > maxLen {
				break
			}
			if summary != "" {
				summary += ". "
			}
			summary += strings.TrimSpace(sentence)
		}
		if summary != "" {
			return summary + "..."
		}
		// Fallback: hard truncate
		return firstPara[:maxLen] + "..."
	}

	return firstPara
}

func safeItemPath(dir string, itemID string) (string, error) {
	if itemID == "" || strings.Contains(itemID, `\`) {
		return "", &core.Error{Kind: core.ErrorKindConfig, Message: "invalid knowledge item id"}
	}
	parts := strings.Split(filepath.ToSlash(itemID), "/")
	for _, part := range parts {
		if part == "" || part == "." || part == ".." {
			return "", &core.Error{Kind: core.ErrorKindConfig, Message: "invalid knowledge item id"}
		}
	}
	rel := filepath.Clean(filepath.FromSlash(itemID))
	if rel == "." || filepath.IsAbs(rel) {
		return "", &core.Error{Kind: core.ErrorKindConfig, Message: "invalid knowledge item id"}
	}

	candidates := []string{rel + ".md", rel}
	if isSupportedTextFile(rel) {
		candidates = []string{rel}
	}
	base, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	var fallback string
	for _, candidate := range candidates {
		path := filepath.Join(base, candidate)
		absPath, err := filepath.Abs(path)
		if err != nil {
			return "", err
		}
		if err := ensureContained(base, absPath); err != nil {
			return "", err
		}
		if fallback == "" {
			fallback = absPath
		}
		if _, err := os.Stat(absPath); err == nil {
			return absPath, nil
		} else if !os.IsNotExist(err) {
			return "", err
		}
	}
	return fallback, nil
}

func resolvedContainedPath(dir string, path string) (string, string, error) {
	base, err := filepath.EvalSymlinks(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return "", "", &core.Error{Kind: core.ErrorKindStore, Message: "knowledge item not found"}
		}
		return "", "", err
	}
	resolvedPath, err := filepath.EvalSymlinks(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", "", &core.Error{Kind: core.ErrorKindStore, Message: "knowledge item not found"}
		}
		return "", "", err
	}
	base, err = filepath.Abs(base)
	if err != nil {
		return "", "", err
	}
	resolvedPath, err = filepath.Abs(resolvedPath)
	if err != nil {
		return "", "", err
	}
	if err := ensureContained(base, resolvedPath); err != nil {
		return "", "", err
	}
	return base, resolvedPath, nil
}

func ensureContained(base string, path string) error {
	relToBase, err := filepath.Rel(base, path)
	if err != nil || relToBase == ".." || strings.HasPrefix(relToBase, ".."+string(filepath.Separator)) || filepath.IsAbs(relToBase) {
		return &core.Error{Kind: core.ErrorKindConfig, Message: "invalid knowledge item id"}
	}
	return nil
}

func (b *Backend) MaintainIndex(ctx context.Context, kb core.KnowledgeBase, opts core.IndexOptions) (core.IndexResult, error) {
	if !b.supportsKBSemantic(kb) {
		return core.IndexResult{}, nil
	}
	dir, _ := kb.StoreConfig["path"].(string)
	source := func(c context.Context) ([]core.KnowledgeItem, error) { return b.ListItems(c, kb) }
	metaProvider := func(item core.KnowledgeItem) map[string]any {
		path, err := safeItemPath(dir, item.ID)
		if err != nil {
			return nil
		}
		info, err := os.Stat(path)
		if err != nil {
			return nil
		}
		return map[string]any{
			"path":  itemPathRel(dir, path),
			"mtime": info.ModTime().Unix(),
		}
	}
	return b.indexer.MaintainIndex(ctx, kb, opts, source, metaProvider)
}

func (b *Backend) SupportsSemantic(kb core.KnowledgeBase) bool { return b.supportsKBSemantic(kb) }

func (b *Backend) Close() error {
	if b.indexer != nil {
		return b.indexer.Close()
	}
	return nil
}
