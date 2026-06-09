package text

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/kindbrave/knowledger/internal/core"
)

var supportedTextExtensions = map[string]struct{}{
	".md":  {},
	".txt": {},
}

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

	var hits []core.SearchHit
	needle := strings.ToLower(opt.Query)
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
		if strings.Contains(strings.ToLower(content), needle) {
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
		}
		return nil
	}); err != nil {
		return nil, err
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
		items = append(items, core.KnowledgeItem{ID: itemIDForPath(dir, path), KBID: kb.ID, Type: "document", Title: itemTitleForPath(dir, path), Content: content, CreatedAt: info.ModTime(), UpdatedAt: info.ModTime()})
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
	return core.KnowledgeItem{
		ID:        itemIDForPath(originalBase, originalPath),
		KBID:      kb.ID,
		Type:      "document",
		Title:     itemTitleForPath(originalBase, originalPath),
		Content:   content,
		CreatedAt: originalInfo.ModTime(),
		UpdatedAt: originalInfo.ModTime(),
	}, nil
}

func (b *Backend) DeleteItem(_ context.Context, kb core.KnowledgeBase, itemID string) error {
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

func (b *Backend) SupportsSemantic(core.KnowledgeBase) bool { return false }
