# Retrieval Snippets and Full-Content Get Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make search return query-centered snippets instead of full content, and add a `get` path that returns the full knowledge item by KB and item ID.

**Architecture:** Add `GetItem` to the backend boundary so service can fetch canonical content by item ID. Keep backend search responsible for retrieval and ranking, then normalize final search hits in `service.Search` by replacing `Snippet` and `ContentPreview` with bounded snippets. Add a CLI `get` command that calls `Service.GetKnowledgeItem` and prints the complete item JSON.

**Tech Stack:** Go, Cobra CLI, SQLite backend, text backend, existing `core.StoreBackend` interface, `go test`.

---

## File Structure

- Modify `internal/core/backend.go`: add `GetItem` to `StoreBackend`.
- Modify `internal/backends/sqlite/backend.go`: add `MultiBackend.GetItem`, `Backend.GetItem`, and a private row-scanning helper reused by `ListItems`.
- Modify `internal/backends/sqlite/backend_test.go`: add tests for `GetItem` success, KB isolation, and not-found behavior.
- Modify `internal/backends/text/backend.go`: add `GetItem` so text backend still satisfies `StoreBackend`.
- Modify `internal/backends/text/backend_test.go`: add tests for text `GetItem` and path traversal rejection.
- Modify `internal/service/service.go`: add `GetKnowledgeItem`, snippet helpers, and search hit normalization.
- Modify `internal/service/service_test.go`: update fakes for `GetItem`, add service tests for full-item retrieval and snippet behavior.
- Modify `internal/adapters/cli/root.go`: register the new `get` command.
- Create `internal/adapters/cli/get.go`: implement `knowledger get --kb <kb-id> --id <item-id>`.
- Modify `internal/adapters/cli/root_test.go`: assert help includes `get`.
- Modify `internal/adapters/cli/add_test.go`: update fake backend for `GetItem`.
- Create `internal/adapters/cli/get_test.go`: test full item output from CLI.
- Modify `internal/adapters/web/server_test.go`: update fake backend for `GetItem` so interface changes compile.

Do not add a web full-content endpoint in this plan; the approved design only requires service/backend and CLI.

---

### Task 1: Extend the backend interface and SQLite retrieval

**Files:**
- Modify: `internal/core/backend.go:20-25`
- Modify: `internal/backends/sqlite/backend.go:105-135`
- Modify: `internal/backends/sqlite/backend.go:410-454`
- Test: `internal/backends/sqlite/backend_test.go`

- [ ] **Step 1: Write failing SQLite `GetItem` tests**

Add these tests after `TestSQLiteBackendAddListAndFTSSearch` in `internal/backends/sqlite/backend_test.go`:

```go
func TestSQLiteBackendGetItemReturnsFullContent(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "knowledge.db")
	backend, err := sqlitebackend.New(dbPath)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	kb := core.KnowledgeBase{ID: "notes", StoreType: "sqlite", StoreConfig: map[string]any{"path": dbPath}, Enabled: true}

	item, _, _, err := backend.Add(ctx, kb, core.AddInput{KBID: "notes", Title: "Full", Content: "完整内容应该通过 GetItem 返回。", Tags: []string{"retrieval"}, Metadata: map[string]any{"source": "test"}})
	if err != nil {
		t.Fatalf("Add returned error: %v", err)
	}

	got, err := backend.GetItem(ctx, kb, item.ID)
	if err != nil {
		t.Fatalf("GetItem returned error: %v", err)
	}
	if got.ID != item.ID || got.KBID != "notes" || got.Type != "note" || got.Title != "Full" || got.Content != "完整内容应该通过 GetItem 返回。" {
		t.Fatalf("unexpected item: %#v", got)
	}
	if len(got.Tags) != 1 || got.Tags[0] != "retrieval" {
		t.Fatalf("expected tag retrieval, got %#v", got.Tags)
	}
	if got.Metadata["source"] != "test" {
		t.Fatalf("expected metadata source test, got %#v", got.Metadata)
	}
}

func TestSQLiteBackendGetItemIsScopedToKnowledgeBase(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "knowledge.db")
	backend, err := sqlitebackend.New(dbPath)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	notes := core.KnowledgeBase{ID: "notes", StoreType: "sqlite", StoreConfig: map[string]any{"path": dbPath}, Enabled: true}
	other := core.KnowledgeBase{ID: "other", StoreType: "sqlite", StoreConfig: map[string]any{"path": dbPath}, Enabled: true}

	item, _, _, err := backend.Add(ctx, notes, core.AddInput{KBID: "notes", Title: "Scoped", Content: "only notes can read this"})
	if err != nil {
		t.Fatalf("Add returned error: %v", err)
	}

	if _, err := backend.GetItem(ctx, other, item.ID); err == nil {
		t.Fatalf("expected other KB GetItem to fail")
	}
}

func TestSQLiteBackendGetItemReturnsErrorForMissingItem(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "knowledge.db")
	backend, err := sqlitebackend.New(dbPath)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	kb := core.KnowledgeBase{ID: "notes", StoreType: "sqlite", StoreConfig: map[string]any{"path": dbPath}, Enabled: true}

	if _, err := backend.GetItem(ctx, kb, "404"); err == nil {
		t.Fatalf("expected missing item to fail")
	}
}
```

- [ ] **Step 2: Run SQLite tests to verify interface failure**

Run:

```bash
go test ./internal/backends/sqlite
```

Expected: FAIL because `Backend` has no `GetItem` method and `core.StoreBackend` has not yet been extended.

- [ ] **Step 3: Add `GetItem` to the core backend interface**

Change `internal/core/backend.go` to:

```go
package core

import "context"

type SearchOptions struct {
	Query      string
	Limit      int
	KBIDs      []string
	SearchMode string
}

type AddInput struct {
	KBID     string
	Title    string
	Content  string
	Tags     []string
	Metadata map[string]any
}

type StoreBackend interface {
	Add(context.Context, KnowledgeBase, AddInput) (KnowledgeItem, IngestionResult, IndexStatus, error)
	Search(context.Context, KnowledgeBase, SearchOptions) ([]SearchHit, error)
	GetItem(context.Context, KnowledgeBase, string) (KnowledgeItem, error)
	ListItems(context.Context, KnowledgeBase) ([]KnowledgeItem, error)
	DeleteItem(context.Context, KnowledgeBase, string) error
	SupportsSemantic(KnowledgeBase) bool
}
```

- [ ] **Step 4: Add SQLite `GetItem` implementation and row helper**

In `internal/backends/sqlite/backend.go`, add this method after `MultiBackend.Search`:

```go
func (m *MultiBackend) GetItem(ctx context.Context, kb core.KnowledgeBase, itemID string) (core.KnowledgeItem, error) {
	backend, err := m.backend(kb)
	if err != nil {
		return core.KnowledgeItem{}, err
	}
	return backend.GetItem(ctx, kb, itemID)
}
```

Then replace `Backend.ListItems` with this block and helper:

```go
func (b *Backend) GetItem(ctx context.Context, kb core.KnowledgeBase, itemID string) (core.KnowledgeItem, error) {
	row := b.db.QueryRowContext(ctx, `
		SELECT id, title, content, tags, metadata_json, created_at, updated_at
		FROM knowledge_items
		WHERE kb_id = ? AND id = ?
	`, kb.ID, itemID)
	item, err := scanItem(row, kb.ID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return core.KnowledgeItem{}, &core.Error{Kind: core.ErrorKindStore, Message: "knowledge item not found"}
		}
		return core.KnowledgeItem{}, err
	}
	return item, nil
}

func (b *Backend) ListItems(ctx context.Context, kb core.KnowledgeBase) ([]core.KnowledgeItem, error) {
	rows, err := b.db.QueryContext(ctx, `
		SELECT id, title, content, tags, metadata_json, created_at, updated_at
		FROM knowledge_items
		WHERE kb_id = ?
		ORDER BY id DESC
	`, kb.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []core.KnowledgeItem
	for rows.Next() {
		item, err := scanItem(rows, kb.ID)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

type itemScanner interface {
	Scan(dest ...any) error
}

func scanItem(scanner itemScanner, kbID string) (core.KnowledgeItem, error) {
	var id int64
	var title, content, tags, metadataJSON, createdAtRaw, updatedAtRaw string
	if err := scanner.Scan(&id, &title, &content, &tags, &metadataJSON, &createdAtRaw, &updatedAtRaw); err != nil {
		return core.KnowledgeItem{}, err
	}
	metadata := map[string]any{}
	if err := json.Unmarshal([]byte(metadataJSON), &metadata); err != nil {
		return core.KnowledgeItem{}, err
	}
	createdAt, err := time.Parse(time.RFC3339, createdAtRaw)
	if err != nil {
		return core.KnowledgeItem{}, err
	}
	updatedAt, err := time.Parse(time.RFC3339, updatedAtRaw)
	if err != nil {
		return core.KnowledgeItem{}, err
	}
	return core.KnowledgeItem{
		ID:        fmt.Sprintf("%d", id),
		KBID:      kbID,
		Type:      "note",
		Title:     title,
		Content:   content,
		Metadata:  metadata,
		Tags:      splitTags(tags),
		CreatedAt: createdAt,
		UpdatedAt: updatedAt,
	}, nil
}
```

This helper works for both `*sql.Row` and `*sql.Rows` because both expose `Scan(dest ...any) error`.

- [ ] **Step 5: Run SQLite tests to verify they pass**

Run:

```bash
go test ./internal/backends/sqlite
```

Expected: PASS for sqlite tests. Other packages may still fail until their backend fakes implement `GetItem`.

- [ ] **Step 6: Checkpoint**

If commits are authorized for this session, run:

```bash
git add internal/core/backend.go internal/backends/sqlite/backend.go internal/backends/sqlite/backend_test.go
git commit -m "feat: add sqlite knowledge item lookup"
```

If commits are not authorized, skip this step and continue without committing.

---

### Task 2: Keep text backend compatible with `GetItem`

**Files:**
- Modify: `internal/backends/text/backend.go:89-141`
- Test: `internal/backends/text/backend_test.go`

- [ ] **Step 1: Write failing text backend `GetItem` tests**

Add these tests before `TestTextBackendDeleteItemRemovesFileAndRejectsTraversal` in `internal/backends/text/backend_test.go`:

```go
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

func TestTextBackendGetItemRejectsTraversal(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	backend := text.New()
	kb := core.KnowledgeBase{ID: "docs", StoreType: "text", StoreConfig: map[string]any{"path": dir}, Enabled: true}

	if _, err := backend.GetItem(ctx, kb, "../outside"); err == nil {
		t.Fatalf("expected path traversal item id to fail")
	}
}
```

- [ ] **Step 2: Run text backend tests to verify failure**

Run:

```bash
go test ./internal/backends/text
```

Expected: FAIL because `text.Backend` does not implement `GetItem`.

- [ ] **Step 3: Add `GetItem` to text backend**

In `internal/backends/text/backend.go`, add this method after `ListItems`:

```go
func (b *Backend) GetItem(_ context.Context, kb core.KnowledgeBase, itemID string) (core.KnowledgeItem, error) {
	dir, ok := kb.StoreConfig["path"].(string)
	if !ok || dir == "" {
		return core.KnowledgeItem{}, &core.Error{Kind: core.ErrorKindConfig, Message: "text backend requires store_config.path"}
	}
	path, err := safeItemPath(dir, itemID)
	if err != nil {
		return core.KnowledgeItem{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return core.KnowledgeItem{}, &core.Error{Kind: core.ErrorKindStore, Message: "knowledge item not found"}
		}
		return core.KnowledgeItem{}, err
	}
	info, err := os.Stat(path)
	if err != nil {
		return core.KnowledgeItem{}, err
	}
	return core.KnowledgeItem{
		ID:        itemIDForPath(dir, path),
		KBID:      kb.ID,
		Type:      "document",
		Title:     itemTitleForPath(dir, path),
		Content:   string(data),
		CreatedAt: info.ModTime(),
		UpdatedAt: info.ModTime(),
	}, nil
}
```

- [ ] **Step 4: Run text backend tests to verify pass**

Run:

```bash
go test ./internal/backends/text
```

Expected: PASS.

- [ ] **Step 5: Checkpoint**

If commits are authorized for this session, run:

```bash
git add internal/backends/text/backend.go internal/backends/text/backend_test.go
git commit -m "feat: add text knowledge item lookup"
```

If commits are not authorized, skip this step and continue without committing.

---

### Task 3: Add service full-item retrieval and update fakes

**Files:**
- Modify: `internal/service/service.go:156-182`
- Modify: `internal/service/service_test.go:16-182`
- Modify: `internal/adapters/cli/add_test.go:14-32`
- Modify: `internal/adapters/web/server_test.go:21-39`

- [ ] **Step 1: Write failing service `GetKnowledgeItem` tests**

In `internal/service/service_test.go`, add this test after `TestServiceListsAndDeletesKnowledgeItems`:

```go
func TestServiceGetsKnowledgeItem(t *testing.T) {
	backend := &itemRecordingBackend{items: []core.KnowledgeItem{{ID: "item-1", KBID: "docs", Title: "Doc", Content: "full body"}}}
	svc := service.New(
		[]core.KnowledgeBase{{ID: "docs", StoreType: "text", Enabled: true}},
		map[string]core.StoreBackend{"text": backend},
	)

	item, err := svc.GetKnowledgeItem(context.Background(), "docs", "item-1")
	if err != nil {
		t.Fatalf("GetKnowledgeItem returned error: %v", err)
	}
	if item.ID != "item-1" || item.KBID != "docs" || item.Content != "full body" {
		t.Fatalf("unexpected item: %#v", item)
	}
	if backend.gotKB != "docs" || backend.gotItem != "item-1" {
		t.Fatalf("expected backend get docs/item-1, got %q/%q", backend.gotKB, backend.gotItem)
	}
}

func TestServiceGetKnowledgeItemValidatesInputs(t *testing.T) {
	svc := service.New(nil, nil)
	if _, err := svc.GetKnowledgeItem(context.Background(), "", "item"); err == nil {
		t.Fatalf("expected empty kb id to fail")
	}
	if _, err := svc.GetKnowledgeItem(context.Background(), "docs", ""); err == nil {
		t.Fatalf("expected empty item id to fail")
	}
	if _, err := svc.GetKnowledgeItem(context.Background(), "missing", "item"); err == nil {
		t.Fatalf("expected missing KB get to fail")
	}
}
```

- [ ] **Step 2: Run service tests to verify failure**

Run:

```bash
go test ./internal/service
```

Expected: FAIL because `Service.GetKnowledgeItem` and fake backend `GetItem` methods do not exist.

- [ ] **Step 3: Update service test fakes for `GetItem`**

In `internal/service/service_test.go`, add these methods to each fake type.

For `fakeBackend`:

```go
func (f fakeBackend) GetItem(context.Context, core.KnowledgeBase, string) (core.KnowledgeItem, error) {
	return core.KnowledgeItem{ID: "1", KBID: "docs", Title: "Doc", Content: "Content"}, nil
}
```

For `recordingBackend`:

```go
func (r *recordingBackend) GetItem(context.Context, core.KnowledgeBase, string) (core.KnowledgeItem, error) {
	return core.KnowledgeItem{}, &core.Error{Kind: core.ErrorKindStore, Message: "knowledge item not found"}
}
```

For `failingSemanticBackend`:

```go
func (failingSemanticBackend) GetItem(context.Context, core.KnowledgeBase, string) (core.KnowledgeItem, error) {
	return core.KnowledgeItem{}, &core.Error{Kind: core.ErrorKindStore, Message: "knowledge item not found"}
}
```

For `hybridFallbackBackend`:

```go
func (h *hybridFallbackBackend) GetItem(context.Context, core.KnowledgeBase, string) (core.KnowledgeItem, error) {
	return core.KnowledgeItem{}, &core.Error{Kind: core.ErrorKindStore, Message: "knowledge item not found"}
}
```

For `closeRecordingBackend`:

```go
func (b *closeRecordingBackend) GetItem(context.Context, core.KnowledgeBase, string) (core.KnowledgeItem, error) {
	return core.KnowledgeItem{}, &core.Error{Kind: core.ErrorKindStore, Message: "knowledge item not found"}
}
```

Update `itemRecordingBackend` fields from:

```go
type itemRecordingBackend struct {
	items       []core.KnowledgeItem
	deletedKB   string
	deletedItem string
}
```

to:

```go
type itemRecordingBackend struct {
	items       []core.KnowledgeItem
	gotKB       string
	gotItem     string
	deletedKB   string
	deletedItem string
}
```

Then add:

```go
func (b *itemRecordingBackend) GetItem(_ context.Context, kb core.KnowledgeBase, itemID string) (core.KnowledgeItem, error) {
	b.gotKB = kb.ID
	b.gotItem = itemID
	for _, item := range b.items {
		if item.ID == itemID && (item.KBID == "" || item.KBID == kb.ID) {
			item.KBID = kb.ID
			return item, nil
		}
	}
	return core.KnowledgeItem{}, &core.Error{Kind: core.ErrorKindStore, Message: "knowledge item not found"}
}
```

- [ ] **Step 4: Add `Service.GetKnowledgeItem`**

In `internal/service/service.go`, add this method after `ListKnowledgeItems`:

```go
func (s *Service) GetKnowledgeItem(ctx context.Context, kbID string, itemID string) (core.KnowledgeItem, error) {
	kbID = strings.TrimSpace(kbID)
	itemID = strings.TrimSpace(itemID)
	if kbID == "" {
		return core.KnowledgeItem{}, &core.Error{Kind: core.ErrorKindConfig, Message: "knowledge base id is required"}
	}
	if itemID == "" {
		return core.KnowledgeItem{}, &core.Error{Kind: core.ErrorKindConfig, Message: "knowledge item id is required"}
	}
	kb, backend, err := s.backendForKnowledgeBase(kbID)
	if err != nil {
		return core.KnowledgeItem{}, err
	}
	return backend.GetItem(ctx, kb, itemID)
}
```

- [ ] **Step 5: Update non-service test fakes for new interface**

In `internal/adapters/cli/add_test.go`, add this method to `addCommandBackend`:

```go
func (addCommandBackend) GetItem(context.Context, core.KnowledgeBase, string) (core.KnowledgeItem, error) {
	return core.KnowledgeItem{ID: "1", KBID: "notes", Title: "title", Content: "content"}, nil
}
```

In `internal/adapters/web/server_test.go`, add this method to `fakeBackend`:

```go
func (fakeBackend) GetItem(_ context.Context, kb core.KnowledgeBase, itemID string) (core.KnowledgeItem, error) {
	if itemID != "1" {
		return core.KnowledgeItem{}, &core.Error{Kind: core.ErrorKindStore, Message: "knowledge item not found"}
	}
	return core.KnowledgeItem{ID: "1", KBID: kb.ID, Type: "note", Title: "Stored knowledge", Content: "Stored content"}, nil
}
```

- [ ] **Step 6: Run service and adapter compile tests**

Run:

```bash
go test ./internal/service ./internal/adapters/cli ./internal/adapters/web
```

Expected: PASS for service and current adapters. CLI `get` does not exist yet, so no CLI get tests have been added.

- [ ] **Step 7: Checkpoint**

If commits are authorized for this session, run:

```bash
git add internal/service/service.go internal/service/service_test.go internal/adapters/cli/add_test.go internal/adapters/web/server_test.go
git commit -m "feat: add service knowledge item lookup"
```

If commits are not authorized, skip this step and continue without committing.

---

### Task 4: Normalize search hits to bounded snippets in service

**Files:**
- Modify: `internal/service/service.go:67-104`
- Modify: `internal/service/service.go:390-419`
- Modify: `internal/service/service_test.go`

- [ ] **Step 1: Write failing tests for query-centered snippets**

Add these tests after `TestSearchAggregatesAcrossEnabledKnowledgeBases` in `internal/service/service_test.go`:

```go
func TestSearchReturnsQueryCenteredSnippetInsteadOfFullContent(t *testing.T) {
	prefix := strings.Repeat("前", 150)
	suffix := strings.Repeat("后", 150)
	content := prefix + "needle" + suffix
	backend := &itemSearchBackend{
		hits:  []core.SearchHit{{ItemID: "item-1", KBID: "docs", Title: "Doc", Snippet: content, ContentPreview: content, Score: 1, MatchMode: "lexical", SourceBackend: "text"}},
		items: map[string]core.KnowledgeItem{"item-1": {ID: "item-1", KBID: "docs", Title: "Doc", Content: content}},
	}
	svc := service.New(
		[]core.KnowledgeBase{{ID: "docs", StoreType: "text", Enabled: true}},
		map[string]core.StoreBackend{"text": backend},
	)

	result, err := svc.Search(context.Background(), core.SearchOptions{Query: "needle", Limit: 10})
	if err != nil {
		t.Fatalf("Search returned error: %v", err)
	}
	if len(result.Hits) != 1 {
		t.Fatalf("expected 1 hit, got %#v", result.Hits)
	}
	snippet := result.Hits[0].Snippet
	if snippet == content || result.Hits[0].ContentPreview == content {
		t.Fatalf("expected bounded snippet, got hit %#v", result.Hits[0])
	}
	if !strings.Contains(snippet, "needle") {
		t.Fatalf("expected snippet to contain query term, got %q", snippet)
	}
	if !strings.HasPrefix(snippet, "…") || !strings.HasSuffix(snippet, "…") {
		t.Fatalf("expected ellipses around middle snippet, got %q", snippet)
	}
	if countRunes(strings.Trim(snippet, "…")) != 245 {
		t.Fatalf("expected 120 + len(needle) + 120 runes inside snippet, got %d in %q", countRunes(strings.Trim(snippet, "…")), snippet)
	}
	if result.Hits[0].ContentPreview != snippet {
		t.Fatalf("expected ContentPreview to equal Snippet, got %#v", result.Hits[0])
	}
}

func TestSearchUsesContentStartWhenQueryTermIsNotLiteral(t *testing.T) {
	content := strings.Repeat("文", 300)
	backend := &itemSearchBackend{
		hits:  []core.SearchHit{{ItemID: "item-1", KBID: "docs", Title: "Doc", Snippet: "semantic full fallback", ContentPreview: "semantic full fallback", Score: 1, MatchMode: "semantic", SourceBackend: "chroma"}},
		items: map[string]core.KnowledgeItem{"item-1": {ID: "item-1", KBID: "docs", Title: "Doc", Content: content}},
	}
	svc := service.New(
		[]core.KnowledgeBase{{ID: "docs", StoreType: "sqlite", Enabled: true}},
		map[string]core.StoreBackend{"sqlite": backend},
	)

	result, err := svc.Search(context.Background(), core.SearchOptions{Query: "missing", SearchMode: "semantic", Limit: 10})
	if err != nil {
		t.Fatalf("Search returned error: %v", err)
	}
	if len(result.Hits) != 1 {
		t.Fatalf("expected 1 hit, got %#v", result.Hits)
	}
	snippet := result.Hits[0].Snippet
	if countRunes(strings.TrimSuffix(snippet, "…")) != 240 {
		t.Fatalf("expected 240-rune prefix snippet, got %d in %q", countRunes(strings.TrimSuffix(snippet, "…")), snippet)
	}
	if !strings.HasSuffix(snippet, "…") {
		t.Fatalf("expected suffix ellipsis for truncated prefix, got %q", snippet)
	}
}

func TestSearchKeepsHitAndWarnsWhenSnippetContentLookupFails(t *testing.T) {
	longFallback := strings.Repeat("x", 300)
	backend := &itemSearchBackend{
		hits:   []core.SearchHit{{ItemID: "missing", KBID: "docs", Title: "Doc", Snippet: longFallback, ContentPreview: longFallback, Score: 1, MatchMode: "lexical", SourceBackend: "text"}},
		items:  map[string]core.KnowledgeItem{},
		getErr: &core.Error{Kind: core.ErrorKindStore, Message: "knowledge item not found"},
	}
	svc := service.New(
		[]core.KnowledgeBase{{ID: "docs", StoreType: "text", Enabled: true}},
		map[string]core.StoreBackend{"text": backend},
	)

	result, err := svc.Search(context.Background(), core.SearchOptions{Query: "needle", Limit: 10})
	if err != nil {
		t.Fatalf("Search returned error: %v", err)
	}
	if len(result.Hits) != 1 {
		t.Fatalf("expected hit to be preserved, got %#v", result.Hits)
	}
	if countRunes(strings.TrimSuffix(result.Hits[0].Snippet, "…")) != 240 {
		t.Fatalf("expected fallback snippet truncated to 240 runes, got %q", result.Hits[0].Snippet)
	}
	if len(result.Warnings) != 1 || !strings.Contains(result.Warnings[0], "could not load full content") {
		t.Fatalf("expected lookup warning, got %#v", result.Warnings)
	}
}
```

Also add this helper fake near other fakes in `internal/service/service_test.go`:

```go
type itemSearchBackend struct {
	hits     []core.SearchHit
	items    map[string]core.KnowledgeItem
	getErr   error
	semantic bool
}

func (b *itemSearchBackend) Add(context.Context, core.KnowledgeBase, core.AddInput) (core.KnowledgeItem, core.IngestionResult, core.IndexStatus, error) {
	return core.KnowledgeItem{}, core.IngestionResult{}, core.IndexStatus{}, nil
}

func (b *itemSearchBackend) Search(context.Context, core.KnowledgeBase, core.SearchOptions) ([]core.SearchHit, error) {
	return b.hits, nil
}

func (b *itemSearchBackend) GetItem(_ context.Context, kb core.KnowledgeBase, itemID string) (core.KnowledgeItem, error) {
	if b.getErr != nil {
		return core.KnowledgeItem{}, b.getErr
	}
	item, ok := b.items[itemID]
	if !ok || (item.KBID != "" && item.KBID != kb.ID) {
		return core.KnowledgeItem{}, &core.Error{Kind: core.ErrorKindStore, Message: "knowledge item not found"}
	}
	item.KBID = kb.ID
	return item, nil
}

func (b *itemSearchBackend) ListItems(context.Context, core.KnowledgeBase) ([]core.KnowledgeItem, error) {
	return nil, nil
}

func (b *itemSearchBackend) DeleteItem(context.Context, core.KnowledgeBase, string) error {
	return nil
}

func (b *itemSearchBackend) SupportsSemantic(core.KnowledgeBase) bool { return b.semantic }

func countRunes(s string) int { return len([]rune(s)) }
```

- [ ] **Step 2: Run service tests to verify snippet failures**

Run:

```bash
go test ./internal/service
```

Expected: FAIL because `Search` still returns full content in `Snippet` and `ContentPreview`.

- [ ] **Step 3: Normalize final search hits in `Service.Search`**

In `internal/service/service.go`, change the end of `Search` from:

```go
	sort.Slice(result.Hits, func(i, j int) bool { return result.Hits[i].Score > result.Hits[j].Score })
	if opt.Limit > 0 && len(result.Hits) > opt.Limit {
		result.Hits = result.Hits[:opt.Limit]
	}
	return result, nil
}
```

to:

```go
	sort.Slice(result.Hits, func(i, j int) bool { return result.Hits[i].Score > result.Hits[j].Score })
	if opt.Limit > 0 && len(result.Hits) > opt.Limit {
		result.Hits = result.Hits[:opt.Limit]
	}
	return s.withSearchSnippets(ctx, opt.Query, result, backends), nil
}
```

- [ ] **Step 4: Add snippet helper functions**

Add these helpers near `searchOptionsForKnowledgeBase` in `internal/service/service.go`:

```go
const searchSnippetContextRunes = 120
const searchFallbackSnippetRunes = 240

func (s *Service) withSearchSnippets(ctx context.Context, query string, result SearchResult, backends map[string]core.StoreBackend) SearchResult {
	kbs, _ := s.snapshot()
	kbByID := map[string]core.KnowledgeBase{}
	for _, kb := range kbs {
		kbByID[kb.ID] = kb
	}
	for i := range result.Hits {
		hit := &result.Hits[i]
		kb, ok := kbByID[hit.KBID]
		backend := backends[kb.StoreType]
		if !ok || backend == nil {
			setFallbackSnippet(hit)
			result.Warnings = append(result.Warnings, fmt.Sprintf("%s/%s: could not load full content for snippet", hit.KBID, hit.ItemID))
			continue
		}
		item, err := backend.GetItem(ctx, kb, hit.ItemID)
		if err != nil {
			setFallbackSnippet(hit)
			result.Warnings = append(result.Warnings, fmt.Sprintf("%s/%s: could not load full content for snippet: %v", hit.KBID, hit.ItemID, err))
			continue
		}
		snippet := snippetAroundQuery(item.Content, query)
		hit.Snippet = snippet
		hit.ContentPreview = snippet
	}
	return result
}

func setFallbackSnippet(hit *core.SearchHit) {
	text := hit.Snippet
	if text == "" {
		text = hit.ContentPreview
	}
	snippet := truncateRunes(text, searchFallbackSnippetRunes)
	hit.Snippet = snippet
	hit.ContentPreview = snippet
}

func snippetAroundQuery(content string, query string) string {
	terms := queryTerms(query)
	contentLower := strings.ToLower(content)
	for _, term := range terms {
		termLower := strings.ToLower(term)
		byteIndex := strings.Index(contentLower, termLower)
		if byteIndex < 0 {
			continue
		}
		return snippetAroundByteIndex(content, byteIndex, len([]rune(content[:byteIndex])), len([]rune(content[byteIndex:byteIndex+len(term)])))
	}
	return truncateRunes(content, searchFallbackSnippetRunes)
}

func snippetAroundByteIndex(content string, _ int, matchStartRunes int, matchRunes int) string {
	runes := []rune(content)
	start := matchStartRunes - searchSnippetContextRunes
	if start < 0 {
		start = 0
	}
	end := matchStartRunes + matchRunes + searchSnippetContextRunes
	if end > len(runes) {
		end = len(runes)
	}
	snippet := string(runes[start:end])
	if start > 0 {
		snippet = "…" + snippet
	}
	if end < len(runes) {
		snippet += "…"
	}
	return snippet
}

func truncateRunes(content string, limit int) string {
	runes := []rune(content)
	if len(runes) <= limit {
		return content
	}
	return string(runes[:limit]) + "…"
}

func queryTerms(query string) []string {
	return strings.FieldsFunc(query, func(r rune) bool {
		return unicode.IsSpace(r) || unicode.IsPunct(r) || unicode.IsSymbol(r)
	})
}
```

Update the import block in `internal/service/service.go` to include `unicode`:

```go
import (
	"context"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
	"sync"
	"unicode"

	"github.com/kindbrave/knowledger/internal/config"
	"github.com/kindbrave/knowledger/internal/core"
	"github.com/kindbrave/knowledger/internal/registry"
)
```

- [ ] **Step 5: Simplify the unused helper parameter**

After adding the helpers, remove the unused `_ int` parameter from `snippetAroundByteIndex` if `go test` reports it is awkward or unnecessary. The preferred final signatures are:

```go
func snippetAroundQuery(content string, query string) string {
	terms := queryTerms(query)
	contentLower := strings.ToLower(content)
	for _, term := range terms {
		termLower := strings.ToLower(term)
		byteIndex := strings.Index(contentLower, termLower)
		if byteIndex < 0 {
			continue
		}
		return snippetAroundMatch(content, len([]rune(content[:byteIndex])), len([]rune(content[byteIndex:byteIndex+len(term)])))
	}
	return truncateRunes(content, searchFallbackSnippetRunes)
}

func snippetAroundMatch(content string, matchStartRunes int, matchRunes int) string {
	runes := []rune(content)
	start := matchStartRunes - searchSnippetContextRunes
	if start < 0 {
		start = 0
	}
	end := matchStartRunes + matchRunes + searchSnippetContextRunes
	if end > len(runes) {
		end = len(runes)
	}
	snippet := string(runes[start:end])
	if start > 0 {
		snippet = "…" + snippet
	}
	if end < len(runes) {
		snippet += "…"
	}
	return snippet
}
```

- [ ] **Step 6: Run service tests to verify snippets pass**

Run:

```bash
go test ./internal/service
```

Expected: PASS.

- [ ] **Step 7: Checkpoint**

If commits are authorized for this session, run:

```bash
git add internal/service/service.go internal/service/service_test.go
git commit -m "feat: return bounded search snippets"
```

If commits are not authorized, skip this step and continue without committing.

---

### Task 5: Add CLI `get` command for full content

**Files:**
- Create: `internal/adapters/cli/get.go`
- Create: `internal/adapters/cli/get_test.go`
- Modify: `internal/adapters/cli/root.go:13-19`
- Modify: `internal/adapters/cli/root_test.go:10-24`

- [ ] **Step 1: Write failing CLI `get` tests**

Create `internal/adapters/cli/get_test.go` with:

```go
package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/kindbrave/knowledger/internal/adapters/cli"
	"github.com/kindbrave/knowledger/internal/core"
	"github.com/kindbrave/knowledger/internal/service"
)

type getCommandBackend struct{}

func (getCommandBackend) Add(context.Context, core.KnowledgeBase, core.AddInput) (core.KnowledgeItem, core.IngestionResult, core.IndexStatus, error) {
	return core.KnowledgeItem{}, core.IngestionResult{}, core.IndexStatus{}, nil
}

func (getCommandBackend) Search(context.Context, core.KnowledgeBase, core.SearchOptions) ([]core.SearchHit, error) {
	return nil, nil
}

func (getCommandBackend) GetItem(_ context.Context, kb core.KnowledgeBase, itemID string) (core.KnowledgeItem, error) {
	return core.KnowledgeItem{ID: itemID, KBID: kb.ID, Type: "note", Title: "Stored", Content: "complete content body"}, nil
}

func (getCommandBackend) ListItems(context.Context, core.KnowledgeBase) ([]core.KnowledgeItem, error) {
	return nil, nil
}

func (getCommandBackend) DeleteItem(context.Context, core.KnowledgeBase, string) error {
	return nil
}

func (getCommandBackend) SupportsSemantic(core.KnowledgeBase) bool { return false }

func TestGetCommandOutputsFullKnowledgeItem(t *testing.T) {
	stdout := new(bytes.Buffer)
	svc := service.New(
		[]core.KnowledgeBase{{ID: "notes", StoreType: "sqlite", Enabled: true}},
		map[string]core.StoreBackend{"sqlite": getCommandBackend{}},
	)
	cmd := cli.NewRootCommand(svc)
	cmd.SetOut(stdout)
	cmd.SetErr(new(bytes.Buffer))
	cmd.SetArgs([]string{"get", "--kb", "notes", "--id", "123"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	var item core.KnowledgeItem
	if err := json.Unmarshal(stdout.Bytes(), &item); err != nil {
		t.Fatalf("expected item JSON, got %q: %v", stdout.String(), err)
	}
	if item.ID != "123" || item.KBID != "notes" || item.Content != "complete content body" {
		t.Fatalf("unexpected item: %#v", item)
	}
}

func TestGetCommandRequiresKBAndID(t *testing.T) {
	cmd := cli.NewRootCommand(service.New(nil, nil))
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetErr(new(bytes.Buffer))
	cmd.SetArgs([]string{"get", "--kb", "notes"})

	if err := cmd.Execute(); err == nil {
		t.Fatalf("expected missing id to fail")
	}
}
```

Also update `TestRootCommandShowsSearchSubcommand` in `internal/adapters/cli/root_test.go` to check for `get`:

```go
func TestRootCommandShowsSearchAndGetSubcommands(t *testing.T) {
	buf := new(bytes.Buffer)
	cmd := cli.NewRootCommand(nil)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"--help"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	for _, expected := range []string{"search", "get"} {
		if !bytes.Contains(buf.Bytes(), []byte(expected)) {
			t.Fatalf("expected help output to mention %s subcommand, got %s", expected, buf.String())
		}
	}
}
```

- [ ] **Step 2: Run CLI tests to verify failure**

Run:

```bash
go test ./internal/adapters/cli
```

Expected: FAIL because `get` command is not registered.

- [ ] **Step 3: Implement `newGetCommand`**

Create `internal/adapters/cli/get.go` with:

```go
package cli

import (
	"context"
	"encoding/json"

	"github.com/kindbrave/knowledger/internal/service"
	"github.com/spf13/cobra"
)

func newGetCommand(svc *service.Service) *cobra.Command {
	var kbID string
	var itemID string
	cmd := &cobra.Command{
		Use: "get",
		RunE: func(cmd *cobra.Command, args []string) error {
			item, err := svc.GetKnowledgeItem(context.Background(), kbID, itemID)
			if err != nil {
				return err
			}
			return json.NewEncoder(cmd.OutOrStdout()).Encode(item)
		},
	}
	cmd.Flags().StringVar(&kbID, "kb", "", "knowledge base id")
	cmd.Flags().StringVar(&itemID, "id", "", "knowledge item id")
	return cmd
}
```

- [ ] **Step 4: Register `get` on the root command**

Update `internal/adapters/cli/root.go`:

```go
func NewRootCommandWithAddress(svc *service.Service, address string) *cobra.Command {
	cmd := &cobra.Command{Use: "knowledger"}
	cmd.AddCommand(newSearchCommand(svc))
	cmd.AddCommand(newGetCommand(svc))
	cmd.AddCommand(newAddCommand(svc))
	cmd.AddCommand(newListKBsCommand(svc))
	cmd.AddCommand(newServeCommand(svc, address))
	return cmd
}
```

- [ ] **Step 5: Run CLI tests to verify pass**

Run:

```bash
go test ./internal/adapters/cli
```

Expected: PASS.

- [ ] **Step 6: Checkpoint**

If commits are authorized for this session, run:

```bash
git add internal/adapters/cli/root.go internal/adapters/cli/root_test.go internal/adapters/cli/get.go internal/adapters/cli/get_test.go
git commit -m "feat: add cli knowledge item get command"
```

If commits are not authorized, skip this step and continue without committing.

---

### Task 6: Verify integration and search output behavior

**Files:**
- Modify if needed: `internal/app/app_test.go`
- Modify if needed: `internal/adapters/web/server.go`
- Modify if needed: `internal/adapters/web/server_test.go`

- [ ] **Step 1: Run all internal tests**

Run:

```bash
go test ./internal/...
```

Expected: PASS. If the web server interface needs `GetKnowledgeItem` only because a fake implements `StoreBackend`, do not add a web route; update only fakes or interface requirements needed for compilation.

- [ ] **Step 2: Run focused CLI search/get smoke test with SQLite lexical indexing disabled for semantic side effects**

Build and exercise the CLI against a temp config. Use shell variables directly in one command:

```bash
tmpdir=$(mktemp -d) && cfg="$tmpdir/knowledger.yaml" && db="$tmpdir/knowledge.db" && cat > "$cfg" <<EOF
knowledge_bases:
  - id: notes
    name: Notes
    store_type: sqlite
    enabled: true
    store_config:
      path: $db
    indexing:
      semantic:
        enabled: false
EOF
go run . --config "$cfg" add --kb notes --title "Long" --content "$(python - <<'PY'
print('A' * 150 + 'needle' + 'B' * 150)
PY
)" && go run . --config "$cfg" search --query needle --limit 1 && go run . --config "$cfg" get --kb notes --id 1
```

Expected:

- The `search` JSON contains `needle` and does not print all 150 leading `A` characters plus all 150 trailing `B` characters.
- The `get` JSON contains the complete content with all leading `A` and trailing `B` characters.

- [ ] **Step 3: Run package tests named in project memory**

Run:

```bash
go test ./internal/adapters/cli
```

Expected: PASS.

- [ ] **Step 4: Run status and inspect changed files**

Run:

```bash
git status --short
git diff -- internal/core/backend.go internal/backends/sqlite/backend.go internal/backends/sqlite/backend_test.go internal/backends/text/backend.go internal/backends/text/backend_test.go internal/service/service.go internal/service/service_test.go internal/adapters/cli/root.go internal/adapters/cli/root_test.go internal/adapters/cli/get.go internal/adapters/cli/get_test.go internal/adapters/cli/add_test.go internal/adapters/web/server_test.go
```

Expected: Only planned files changed, plus any pre-existing untracked/modified files from the working tree remain visible.

- [ ] **Step 5: Final checkpoint**

If commits are authorized for this session, run:

```bash
git add internal/core/backend.go internal/backends/sqlite/backend.go internal/backends/sqlite/backend_test.go internal/backends/text/backend.go internal/backends/text/backend_test.go internal/service/service.go internal/service/service_test.go internal/adapters/cli/root.go internal/adapters/cli/root_test.go internal/adapters/cli/get.go internal/adapters/cli/get_test.go internal/adapters/cli/add_test.go internal/adapters/web/server_test.go
git commit -m "feat: add retrieval snippets and full item lookup"
```

If earlier task-level commits were created, do not create this final commit. If commits are not authorized, skip this step and report the uncommitted changes.

---

## Self-Review

### Spec coverage

- Search no longer returns full content: covered by Task 4 tests and implementation.
- Query-centered snippets with 120 runes before and after the term: covered by Task 4.
- Prefix fallback of 240 runes when no literal term matches: covered by Task 4.
- Unicode/rune counting for Chinese content: covered by Task 4 using repeated Chinese runes and `countRunes`.
- Full-content backend interface by ID: covered by Tasks 1 and 2.
- Service `GetKnowledgeItem`: covered by Task 3.
- CLI `get --kb --id`: covered by Task 5.
- SQLite KB isolation and not-found behavior: covered by Task 1.
- Search lookup failure warning and fallback truncation: covered by Task 4.
- Existing semantic/hybrid fallback behavior: preserved by Task 4, verified by existing service tests and Task 6.

### Placeholder scan

No placeholders, no deferred implementation steps, and no unspecified tests are left in this plan.

### Type consistency

The plan uses a single backend method signature throughout:

```go
GetItem(context.Context, KnowledgeBase, string) (KnowledgeItem, error)
```

The service method name is consistently:

```go
GetKnowledgeItem(ctx context.Context, kbID string, itemID string) (core.KnowledgeItem, error)
```
