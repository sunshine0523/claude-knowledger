package text_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kindbrave/knowledger/internal/backends/text"
	"github.com/kindbrave/knowledger/internal/core"
)

func TestTextBackendAddListAndSearch(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	backend := text.New()
	kb := core.KnowledgeBase{
		ID:          "docs",
		StoreType:   "text",
		StoreConfig: map[string]any{"path": dir},
		Enabled:     true,
	}

	item, ingest, _, err := backend.Add(ctx, kb, core.AddInput{
		KBID:    "docs",
		Title:   "设计原则",
		Content: "统一 core，隐藏底层差异。",
		Tags:    []string{"architecture"},
	})
	if err != nil {
		t.Fatalf("Add returned error: %v", err)
	}
	if !ingest.Success {
		t.Fatalf("expected ingest success")
	}
	if item.Title != "设计原则" {
		t.Fatalf("expected title 设计原则, got %q", item.Title)
	}

	items, err := backend.ListItems(ctx, kb)
	if err != nil {
		t.Fatalf("ListItems returned error: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	if !strings.Contains(items[0].Content, "统一 core") {
		t.Fatalf("expected item content to include stored body, got %#v", items[0])
	}

	hits, err := backend.Search(ctx, kb, core.SearchOptions{Query: "隐藏底层", Limit: 10})
	if err != nil {
		t.Fatalf("Search returned error: %v", err)
	}
	if len(hits) != 1 {
		t.Fatalf("expected 1 hit, got %d", len(hits))
	}

	expectedFile := filepath.Join(dir, item.ID+".md")
	if hits[0].Locator != expectedFile {
		t.Fatalf("expected locator %q, got %q", expectedFile, hits[0].Locator)
	}
}

func TestTextBackendListsAndSearchesSupportedFilesRecursively(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	backend := text.New()
	kb := core.KnowledgeBase{ID: "docs", StoreType: "text", StoreConfig: map[string]any{"path": dir}, Enabled: true}

	if err := os.MkdirAll(filepath.Join(dir, "nested", "deep"), 0o755); err != nil {
		t.Fatalf("MkdirAll returned error: %v", err)
	}
	files := map[string]string{
		"root.md":                  "root content",
		"nested/note.txt":          "txt file contains recursive needle",
		"nested/deep/design.md":    "markdown file contains recursive needle",
		"nested/deep/ignored.json": "ignored file contains recursive needle",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, filepath.FromSlash(name)), []byte(content), 0o644); err != nil {
			t.Fatalf("WriteFile returned error: %v", err)
		}
	}

	items, err := backend.ListItems(ctx, kb)
	if err != nil {
		t.Fatalf("ListItems returned error: %v", err)
	}
	byID := map[string]core.KnowledgeItem{}
	for _, item := range items {
		byID[item.ID] = item
	}
	for _, id := range []string{"root", "nested/note.txt", "nested/deep/design"} {
		if _, ok := byID[id]; !ok {
			t.Fatalf("expected item id %q in %#v", id, items)
		}
	}
	if _, ok := byID["nested/deep/ignored.json"]; ok {
		t.Fatalf("expected unsupported json file to be ignored, got %#v", items)
	}
	if byID["nested/deep/design"].Title != "nested/deep/design.md" {
		t.Fatalf("expected recursive title, got %q", byID["nested/deep/design"].Title)
	}

	hits, err := backend.Search(ctx, kb, core.SearchOptions{Query: "recursive needle", Limit: 10})
	if err != nil {
		t.Fatalf("Search returned error: %v", err)
	}
	hitIDs := map[string]string{}
	for _, hit := range hits {
		hitIDs[hit.ItemID] = hit.Locator
	}
	for id, path := range map[string]string{
		"nested/note.txt":    filepath.Join(dir, "nested", "note.txt"),
		"nested/deep/design": filepath.Join(dir, "nested", "deep", "design.md"),
	} {
		if hitIDs[id] != path {
			t.Fatalf("expected hit %q locator %q, got hits %#v", id, path, hits)
		}
	}
	if len(hits) != 2 {
		t.Fatalf("expected 2 hits, got %#v", hits)
	}
}

func TestTextBackendGetItemReturnsFullContent(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	backend := text.New()
	kb := core.KnowledgeBase{ID: "docs", StoreType: "text", StoreConfig: map[string]any{"path": dir}, Enabled: true}

	item, _, _, err := backend.Add(ctx, kb, core.AddInput{KBID: "docs", Title: "Full", Content: "text backend full content"})
	if err != nil {
		t.Fatalf("Add returned error: %v", err)
	}

	got, err := backend.GetItem(ctx, kb, item.ID)
	if err != nil {
		t.Fatalf("GetItem returned error: %v", err)
	}
	if got.ID != item.ID || got.KBID != "docs" || got.Type != "document" || got.Title != item.ID+".md" {
		t.Fatalf("unexpected item metadata: %#v", got)
	}
	if !strings.Contains(got.Content, "text backend full content") {
		t.Fatalf("expected full content, got %#v", got)
	}
}

func TestTextBackendListItemsSkipsDirectoryWithSupportedExtension(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	backend := text.New()
	kb := core.KnowledgeBase{ID: "docs", StoreType: "text", StoreConfig: map[string]any{"path": dir}, Enabled: true}

	if err := os.Mkdir(filepath.Join(dir, "dir.md"), 0o755); err != nil {
		t.Fatalf("Mkdir returned error: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "safe.md"), []byte("safe content"), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	items, err := backend.ListItems(ctx, kb)
	if err != nil {
		t.Fatalf("ListItems returned error: %v", err)
	}
	if len(items) != 1 || items[0].ID != "safe" {
		t.Fatalf("expected only safe item and directory skipped, got %#v", items)
	}
}

func TestTextBackendSearchSkipsDirectoryWithSupportedExtension(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	backend := text.New()
	kb := core.KnowledgeBase{ID: "docs", StoreType: "text", StoreConfig: map[string]any{"path": dir}, Enabled: true}

	if err := os.Mkdir(filepath.Join(dir, "dir.md"), 0o755); err != nil {
		t.Fatalf("Mkdir returned error: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "safe.md"), []byte("safe needle"), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	hits, err := backend.Search(ctx, kb, core.SearchOptions{Query: "needle", Limit: 10})
	if err != nil {
		t.Fatalf("Search returned error: %v", err)
	}
	if len(hits) != 1 || hits[0].ItemID != "safe" {
		t.Fatalf("expected only safe hit and directory skipped, got %#v", hits)
	}
}

func TestTextBackendGetItemRejectsDirectoryWithSupportedExtension(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	backend := text.New()
	kb := core.KnowledgeBase{ID: "docs", StoreType: "text", StoreConfig: map[string]any{"path": dir}, Enabled: true}

	if err := os.Mkdir(filepath.Join(dir, "dir.md"), 0o755); err != nil {
		t.Fatalf("Mkdir returned error: %v", err)
	}

	if got, err := backend.GetItem(ctx, kb, "dir"); err == nil {
		t.Fatalf("expected directory item to fail, got %#v", got)
	} else if err.Error() != "store_error: knowledge item not found" {
		t.Fatalf("expected knowledge item not found, got %v", err)
	}
}

func TestTextBackendSkipsSymlinkToDirectoryInsideStore(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	backend := text.New()
	kb := core.KnowledgeBase{ID: "docs", StoreType: "text", StoreConfig: map[string]any{"path": dir}, Enabled: true}

	targetDir := filepath.Join(dir, "target.md")
	if err := os.Mkdir(targetDir, 0o755); err != nil {
		t.Fatalf("Mkdir target returned error: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "safe.md"), []byte("safe directory-link needle"), 0o644); err != nil {
		t.Fatalf("WriteFile safe returned error: %v", err)
	}
	if err := os.Symlink("target.md", filepath.Join(dir, "link.md")); err != nil {
		t.Skipf("symlinks are not supported: %v", err)
	}

	items, err := backend.ListItems(ctx, kb)
	if err != nil {
		t.Fatalf("ListItems returned error: %v", err)
	}
	for _, item := range items {
		if item.ID == "link" || item.ID == "target" {
			t.Fatalf("expected symlink and directory target to be skipped, got items %#v", items)
		}
	}
	if len(items) != 1 || items[0].ID != "safe" {
		t.Fatalf("expected only safe item, got %#v", items)
	}

	hits, err := backend.Search(ctx, kb, core.SearchOptions{Query: "directory-link needle", Limit: 10})
	if err != nil {
		t.Fatalf("Search returned error: %v", err)
	}
	if len(hits) != 1 || hits[0].ItemID != "safe" {
		t.Fatalf("expected only safe hit, got %#v", hits)
	}

	if got, err := backend.GetItem(ctx, kb, "link"); err == nil {
		t.Fatalf("expected symlink to directory to fail, got %#v", got)
	} else if err.Error() != "store_error: knowledge item not found" {
		t.Fatalf("expected knowledge item not found, got %v", err)
	}
}

func TestTextBackendListItemsSkipsSymlinkOutsideStore(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	outsideDir := t.TempDir()
	backend := text.New()
	kb := core.KnowledgeBase{ID: "docs", StoreType: "text", StoreConfig: map[string]any{"path": dir}, Enabled: true}

	if err := os.WriteFile(filepath.Join(dir, "safe.md"), []byte("safe in-store content"), 0o644); err != nil {
		t.Fatalf("WriteFile safe returned error: %v", err)
	}
	outsidePath := filepath.Join(outsideDir, "secret.md")
	if err := os.WriteFile(outsidePath, []byte("outside symlink secret content"), 0o644); err != nil {
		t.Fatalf("WriteFile outside returned error: %v", err)
	}
	if err := os.Symlink(outsidePath, filepath.Join(dir, "leak.md")); err != nil {
		t.Skipf("symlinks are not supported: %v", err)
	}

	items, err := backend.ListItems(ctx, kb)
	if err != nil {
		t.Fatalf("ListItems returned error: %v", err)
	}
	for _, item := range items {
		if item.ID == "leak" {
			t.Fatalf("expected outside symlink leak.md to be skipped, got items %#v", items)
		}
		if strings.Contains(item.Content, "outside symlink secret content") {
			t.Fatalf("expected outside symlink content not to leak, got items %#v", items)
		}
	}
}

func TestTextBackendSearchSkipsSymlinkOutsideStore(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	outsideDir := t.TempDir()
	backend := text.New()
	kb := core.KnowledgeBase{ID: "docs", StoreType: "text", StoreConfig: map[string]any{"path": dir}, Enabled: true}

	if err := os.WriteFile(filepath.Join(dir, "safe.md"), []byte("safe in-store content"), 0o644); err != nil {
		t.Fatalf("WriteFile safe returned error: %v", err)
	}
	outsidePath := filepath.Join(outsideDir, "secret.md")
	if err := os.WriteFile(outsidePath, []byte("outside search needle"), 0o644); err != nil {
		t.Fatalf("WriteFile outside returned error: %v", err)
	}
	if err := os.Symlink(outsidePath, filepath.Join(dir, "leak.md")); err != nil {
		t.Skipf("symlinks are not supported: %v", err)
	}

	hits, err := backend.Search(ctx, kb, core.SearchOptions{Query: "outside search needle", Limit: 10})
	if err != nil {
		t.Fatalf("Search returned error: %v", err)
	}
	if len(hits) != 0 {
		t.Fatalf("expected outside symlink search content not to match, got hits %#v", hits)
	}
}

func TestTextBackendListItemsAndSearchAllowInStoreSymlink(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	backend := text.New()
	kb := core.KnowledgeBase{ID: "docs", StoreType: "text", StoreConfig: map[string]any{"path": dir}, Enabled: true}

	if err := os.WriteFile(filepath.Join(dir, "target.md"), []byte("target content through safe list search link"), 0o644); err != nil {
		t.Fatalf("WriteFile target returned error: %v", err)
	}
	if err := os.Symlink("target.md", filepath.Join(dir, "link.md")); err != nil {
		t.Skipf("symlinks are not supported: %v", err)
	}

	items, err := backend.ListItems(ctx, kb)
	if err != nil {
		t.Fatalf("ListItems returned error: %v", err)
	}
	foundLink := false
	for _, item := range items {
		if item.ID != "link" {
			continue
		}
		foundLink = true
		if item.Title != "link.md" {
			t.Fatalf("expected symlink title link.md, got %#v", item)
		}
		if !strings.Contains(item.Content, "target content through safe list search link") {
			t.Fatalf("expected symlink target content, got %#v", item)
		}
	}
	if !foundLink {
		t.Fatalf("expected in-store symlink item id link, got %#v", items)
	}

	hits, err := backend.Search(ctx, kb, core.SearchOptions{Query: "safe list search link", Limit: 10})
	if err != nil {
		t.Fatalf("Search returned error: %v", err)
	}
	foundHit := false
	for _, hit := range hits {
		if hit.ItemID != "link" {
			continue
		}
		foundHit = true
		if hit.Title != "link.md" || hit.Locator != filepath.Join(dir, "link.md") {
			t.Fatalf("expected symlink search metadata for link.md, got %#v", hit)
		}
		if !strings.Contains(hit.ContentPreview, "target content through safe list search link") {
			t.Fatalf("expected symlink target content preview, got %#v", hit)
		}
	}
	if !foundHit {
		t.Fatalf("expected search hit for in-store symlink link, got %#v", hits)
	}
}

func TestTextBackendGetItemPreservesSymlinkItemMetadata(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	backend := text.New()
	kb := core.KnowledgeBase{ID: "docs", StoreType: "text", StoreConfig: map[string]any{"path": dir}, Enabled: true}

	targetPath := filepath.Join(dir, "target.md")
	linkPath := filepath.Join(dir, "link.md")
	if err := os.WriteFile(targetPath, []byte("target content through symlink"), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}
	targetTime := time.Date(2001, time.February, 3, 4, 5, 6, 0, time.UTC)
	if err := os.Chtimes(targetPath, targetTime, targetTime); err != nil {
		t.Fatalf("Chtimes target returned error: %v", err)
	}
	if err := os.Symlink("target.md", linkPath); err != nil {
		t.Skipf("symlinks are not supported: %v", err)
	}
	linkInfo, err := os.Lstat(linkPath)
	if err != nil {
		t.Fatalf("Lstat link returned error: %v", err)
	}

	got, err := backend.GetItem(ctx, kb, "link")
	if err != nil {
		t.Fatalf("GetItem returned error: %v", err)
	}
	if !strings.Contains(got.Content, "target content through symlink") {
		t.Fatalf("expected target content, got %#v", got)
	}
	if got.ID != "link" || got.Title != "link.md" {
		t.Fatalf("expected symlink metadata ID link and Title link.md, got %#v", got)
	}
	if !got.CreatedAt.Equal(linkInfo.ModTime()) || !got.UpdatedAt.Equal(linkInfo.ModTime()) {
		t.Fatalf("expected symlink timestamps %v, got CreatedAt %v UpdatedAt %v", linkInfo.ModTime(), got.CreatedAt, got.UpdatedAt)
	}
	if got.CreatedAt.Equal(targetTime) || got.UpdatedAt.Equal(targetTime) {
		t.Fatalf("expected symlink timestamps to differ from target timestamp %v, got CreatedAt %v UpdatedAt %v", targetTime, got.CreatedAt, got.UpdatedAt)
	}
}

func TestTextBackendGetItemRejectsUnsupportedSymlinkItemPath(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	backend := text.New()
	kb := core.KnowledgeBase{ID: "docs", StoreType: "text", StoreConfig: map[string]any{"path": dir}, Enabled: true}

	if err := os.WriteFile(filepath.Join(dir, "note.md"), []byte("supported target through unsupported symlink"), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}
	if err := os.Symlink("note.md", filepath.Join(dir, "secret.json")); err != nil {
		t.Skipf("symlinks are not supported: %v", err)
	}

	if got, err := backend.GetItem(ctx, kb, "secret.json"); err == nil {
		t.Fatalf("expected unsupported symlink item path to fail, got %#v", got)
	}
}

func TestTextBackendGetItemRejectsExtensionlessSymlinkItemPath(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	backend := text.New()
	kb := core.KnowledgeBase{ID: "docs", StoreType: "text", StoreConfig: map[string]any{"path": dir}, Enabled: true}

	if err := os.WriteFile(filepath.Join(dir, "note.md"), []byte("supported target through extensionless symlink"), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}
	if err := os.Symlink("note.md", filepath.Join(dir, "secret")); err != nil {
		t.Skipf("symlinks are not supported: %v", err)
	}

	if got, err := backend.GetItem(ctx, kb, "secret"); err == nil {
		t.Fatalf("expected extensionless symlink item path to fail, got %#v", got)
	}
}

func TestTextBackendGetItemRejectsUnsupportedJSONFile(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	backend := text.New()
	kb := core.KnowledgeBase{ID: "docs", StoreType: "text", StoreConfig: map[string]any{"path": dir}, Enabled: true}

	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte("unsupported json content"), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	if got, err := backend.GetItem(ctx, kb, "config.json"); err == nil {
		t.Fatalf("expected unsupported json file to fail, got %#v", got)
	}
}

func TestTextBackendGetItemRejectsSymlinkToUnsupportedJSONFile(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	backend := text.New()
	kb := core.KnowledgeBase{ID: "docs", StoreType: "text", StoreConfig: map[string]any{"path": dir}, Enabled: true}

	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(`{"token":"secret"}`), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}
	if err := os.Symlink("config.json", filepath.Join(dir, "link.md")); err != nil {
		t.Skipf("symlinks are not supported: %v", err)
	}

	if got, err := backend.GetItem(ctx, kb, "link"); err == nil {
		t.Fatalf("expected symlink to unsupported json file to fail, got %#v", got)
	}
}

func TestTextBackendGetItemRejectsNoExtensionFile(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	backend := text.New()
	kb := core.KnowledgeBase{ID: "docs", StoreType: "text", StoreConfig: map[string]any{"path": dir}, Enabled: true}

	if err := os.WriteFile(filepath.Join(dir, "secret"), []byte("extensionless secret content"), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	if got, err := backend.GetItem(ctx, kb, "secret"); err == nil {
		t.Fatalf("expected no-extension file to fail, got %#v", got)
	}
}

func TestTextBackendGetItemRejectsSymlinkOutsideStore(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	outsideDir := t.TempDir()
	backend := text.New()
	kb := core.KnowledgeBase{ID: "docs", StoreType: "text", StoreConfig: map[string]any{"path": dir}, Enabled: true}

	outsidePath := filepath.Join(outsideDir, "secret.md")
	if err := os.WriteFile(outsidePath, []byte("outside symlink secret content"), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}
	if err := os.Symlink(outsidePath, filepath.Join(dir, "secret.md")); err != nil {
		t.Skipf("symlinks are not supported: %v", err)
	}

	if got, err := backend.GetItem(ctx, kb, "secret"); err == nil {
		t.Fatalf("expected symlink outside store to fail, got %#v", got)
	}
}

func TestTextBackendGetItemRejectsTraversal(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	backend := text.New()
	kb := core.KnowledgeBase{ID: "docs", StoreType: "text", StoreConfig: map[string]any{"path": dir}, Enabled: true}

	if _, err := backend.GetItem(ctx, kb, "../outside"); err == nil {
		t.Fatalf("expected path traversal item id to fail")
	}
}

func TestTextBackendDeleteItemRemovesFileAndRejectsTraversal(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	backend := text.New()
	kb := core.KnowledgeBase{ID: "docs", StoreType: "text", StoreConfig: map[string]any{"path": dir}, Enabled: true}

	item, _, _, err := backend.Add(ctx, kb, core.AddInput{KBID: "docs", Title: "Delete", Content: "remove me"})
	if err != nil {
		t.Fatalf("Add returned error: %v", err)
	}
	path := filepath.Join(dir, item.ID+".md")
	if err := backend.DeleteItem(ctx, kb, item.ID); err != nil {
		t.Fatalf("DeleteItem returned error: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected file to be deleted, stat err=%v", err)
	}
	items, err := backend.ListItems(ctx, kb)
	if err != nil {
		t.Fatalf("ListItems returned error: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("expected no items after delete, got %#v", items)
	}
	if err := backend.DeleteItem(ctx, kb, "../outside"); err == nil {
		t.Fatalf("expected path traversal item id to fail")
	}
}
