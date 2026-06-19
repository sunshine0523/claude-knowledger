package sqlite

import (
	"context"
	"database/sql"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/kindbrave/knowledger/internal/core"
	"github.com/kindbrave/knowledger/internal/indexing/semantic"
	_ "github.com/mattn/go-sqlite3"
)

//go:embed schema.sql
var schemaSQL string

//go:embed fts5.sql
var fts5SQL string

type Option func(*Backend)

type Backend struct {
	db         *sql.DB
	ftsEnabled bool
	indexer    *semantic.Indexer
}

type MultiBackend struct {
	backends map[string]*Backend
}

func WithIndexer(idx *semantic.Indexer) Option {
	return func(b *Backend) {
		if idx != nil {
			b.indexer = idx
		}
	}
}

func New(path string, opts ...Option) (*Backend, error) {
	if path == "" {
		return nil, fmt.Errorf("sqlite path is required")
	}
	if err := prepareDatabasePath(path); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		return nil, err
	}
	if _, err := db.Exec(schemaSQL); err != nil {
		_ = db.Close()
		return nil, err
	}
	ftsEnabled := true
	if _, err := db.Exec(fts5SQL); err != nil {
		ftsEnabled = false
	}
	backend := &Backend{db: db, ftsEnabled: ftsEnabled}
	for _, opt := range opts {
		if opt != nil {
			opt(backend)
		}
	}
	if backend.indexer == nil {
		backend.indexer = semantic.NewIndexer(nil, nil)
	}
	return backend, nil
}

func NewMulti(kbs []core.KnowledgeBase, opts ...Option) (*MultiBackend, error) {
	multi := &MultiBackend{backends: map[string]*Backend{}}
	for _, kb := range kbs {
		if kb.StoreType != "sqlite" {
			continue
		}
		path, err := databasePath(kb)
		if err != nil {
			return nil, errors.Join(err, multi.Close())
		}
		if multi.backends[path] != nil {
			continue
		}
		backend, err := New(path, opts...)
		if err != nil {
			return nil, errors.Join(err, multi.Close())
		}
		multi.backends[path] = backend
	}
	return multi, nil
}

func (m *MultiBackend) Add(ctx context.Context, kb core.KnowledgeBase, input core.AddInput) (core.KnowledgeItem, core.IngestionResult, core.IndexStatus, error) {
	backend, err := m.backend(kb)
	if err != nil {
		return core.KnowledgeItem{}, core.IngestionResult{}, core.IndexStatus{}, err
	}
	return backend.Add(ctx, kb, input)
}

func (m *MultiBackend) Search(ctx context.Context, kb core.KnowledgeBase, opt core.SearchOptions) ([]core.SearchHit, error) {
	backend, err := m.backend(kb)
	if err != nil {
		return nil, err
	}
	return backend.Search(ctx, kb, opt)
}

func (m *MultiBackend) GetItem(ctx context.Context, kb core.KnowledgeBase, itemID string) (core.KnowledgeItem, error) {
	backend, err := m.backend(kb)
	if err != nil {
		return core.KnowledgeItem{}, err
	}
	return backend.GetItem(ctx, kb, itemID)
}

func (m *MultiBackend) ListItems(ctx context.Context, kb core.KnowledgeBase) ([]core.KnowledgeItem, error) {
	backend, err := m.backend(kb)
	if err != nil {
		return nil, err
	}
	return backend.ListItems(ctx, kb)
}

func (m *MultiBackend) DeleteItem(ctx context.Context, kb core.KnowledgeBase, itemID string) error {
	backend, err := m.backend(kb)
	if err != nil {
		return err
	}
	return backend.DeleteItem(ctx, kb, itemID)
}

func (m *MultiBackend) MaintainIndex(ctx context.Context, kb core.KnowledgeBase, opt core.IndexOptions) (core.IndexResult, error) {
	backend, err := m.backend(kb)
	if err != nil {
		return core.IndexResult{}, err
	}
	return backend.MaintainIndex(ctx, kb, opt)
}

func (m *MultiBackend) SupportsSemantic(kb core.KnowledgeBase) bool {
	backend, err := m.backend(kb)
	if err != nil {
		return false
	}
	return backend.SupportsSemantic(kb)
}

func (m *MultiBackend) Close() error {
	if m == nil {
		return nil
	}
	var err error
	for _, backend := range m.backends {
		err = errors.Join(err, backend.Close())
	}
	return err
}

func (m *MultiBackend) backend(kb core.KnowledgeBase) (*Backend, error) {
	path, err := databasePath(kb)
	if err != nil {
		return nil, err
	}
	backend := m.backends[path]
	if backend == nil {
		return nil, &core.Error{Kind: core.ErrorKindConfig, Message: fmt.Sprintf("sqlite backend not registered for knowledge base %q path %q", kb.ID, path)}
	}
	return backend, nil
}

func databasePath(kb core.KnowledgeBase) (string, error) {
	path, ok := kb.StoreConfig["path"].(string)
	if !ok || path == "" {
		return "", &core.Error{Kind: core.ErrorKindConfig, Message: fmt.Sprintf("knowledge base %q sqlite store_config.path is required", kb.ID)}
	}
	return path, nil
}

func prepareDatabasePath(path string) error {
	if path == ":memory:" || strings.HasPrefix(path, "file:") {
		return nil
	}
	dir := filepath.Dir(path)
	if dir == "." || dir == "" {
		return nil
	}
	return os.MkdirAll(dir, 0o755)
}

func cleanupContext(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
}

func sqliteMeta(item core.KnowledgeItem) map[string]any {
	return map[string]any{"mtime": item.UpdatedAt.Unix()}
}

func (b *Backend) Add(ctx context.Context, kb core.KnowledgeBase, input core.AddInput) (core.KnowledgeItem, core.IngestionResult, core.IndexStatus, error) {
	nowTime := time.Now().UTC()
	now := nowTime.Format(time.RFC3339)
	metadataJSON, err := json.Marshal(input.Metadata)
	if err != nil {
		return core.KnowledgeItem{}, core.IngestionResult{}, core.IndexStatus{}, err
	}
	semanticEnabled := b.indexer.SupportsKB(kb)

	tx, err := b.db.BeginTx(ctx, nil)
	if err != nil {
		return core.KnowledgeItem{}, core.IngestionResult{}, core.IndexStatus{}, err
	}
	res, err := tx.ExecContext(ctx, `
				INSERT INTO knowledge_items(kb_id, title, content, tags, metadata_json, created_at, updated_at)
				VALUES (?, ?, ?, ?, ?, ?, ?)
			`, kb.ID, input.Title, input.Content, strings.Join(input.Tags, ","), string(metadataJSON), now, now)
	if err != nil {
		_ = tx.Rollback()
		return core.KnowledgeItem{}, core.IngestionResult{}, core.IndexStatus{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		_ = tx.Rollback()
		return core.KnowledgeItem{}, core.IngestionResult{}, core.IndexStatus{}, err
	}
	item := core.KnowledgeItem{
		ID:        fmt.Sprintf("%d", id),
		KBID:      kb.ID,
		Type:      "note",
		Title:     input.Title,
		Content:   input.Content,
		Metadata:  input.Metadata,
		Tags:      input.Tags,
		CreatedAt: nowTime,
		UpdatedAt: nowTime,
	}
	if !semanticEnabled {
		if err := tx.Commit(); err != nil {
			return core.KnowledgeItem{}, core.IngestionResult{}, core.IndexStatus{}, err
		}
		return item, core.IngestionResult{Success: true, ItemID: item.ID}, core.IndexStatus{State: "not_indexed"}, nil
	}

	if err := b.indexer.UpsertItem(ctx, kb, item, sqliteMeta(item)); err != nil {
		_ = tx.Rollback()
		return core.KnowledgeItem{}, core.IngestionResult{}, core.IndexStatus{}, fmt.Errorf("semantic index failed; sqlite item rolled back: %w", err)
	}
	if err := tx.Commit(); err != nil {
		cleanupCtx, cancel := cleanupContext(ctx)
		defer cancel()
		cleanupErr := b.indexer.DeleteItem(cleanupCtx, kb, item.ID)
		commitErr := fmt.Errorf("sqlite commit failed after semantic index success: %w", err)
		if cleanupErr != nil {
			return core.KnowledgeItem{}, core.IngestionResult{}, core.IndexStatus{}, errors.Join(commitErr, fmt.Errorf("semantic cleanup failed: %w", cleanupErr))
		}
		return core.KnowledgeItem{}, core.IngestionResult{}, core.IndexStatus{}, commitErr
	}
	return item, core.IngestionResult{Success: true, ItemID: item.ID}, core.IndexStatus{State: "indexed"}, nil
}

func (b *Backend) Search(ctx context.Context, kb core.KnowledgeBase, opt core.SearchOptions) ([]core.SearchHit, error) {
	limit := opt.Limit
	if limit <= 0 {
		limit = 10
	}
	switch opt.SearchMode {
	case "semantic":
		if !b.indexer.SupportsKB(kb) {
			return b.searchLexical(ctx, kb, opt.Query, limit)
		}
		return b.semanticHitsWithType(b.indexer.Search(ctx, kb, opt.Query, limit, "semantic"))
	case "hybrid":
		if !b.indexer.SupportsKB(kb) {
			return b.searchLexical(ctx, kb, opt.Query, limit)
		}
		semanticHits, err := b.semanticHitsWithType(b.indexer.Search(ctx, kb, opt.Query, limit, "hybrid"))
		if err != nil {
			return nil, err
		}
		lexicalHits, err := b.searchLexical(ctx, kb, opt.Query, limit)
		if err != nil {
			return nil, err
		}
		return mergeHybridHits(semanticHits, lexicalHits, limit), nil
	default:
		return b.searchLexical(ctx, kb, opt.Query, limit)
	}
}

func (b *Backend) semanticHitsWithType(hits []core.SearchHit, err error) ([]core.SearchHit, error) {
	if err != nil {
		return nil, err
	}
	for i := range hits {
		hits[i].ItemType = "note"
	}
	return hits, nil
}

func mergeHybridHits(semanticHits, lexicalHits []core.SearchHit, limit int) []core.SearchHit {
	merged := make([]core.SearchHit, 0, len(semanticHits)+len(lexicalHits))
	seen := map[string]bool{}
	for _, h := range semanticHits {
		merged = append(merged, h)
		seen[h.ItemID] = true
	}
	for _, h := range lexicalHits {
		if seen[h.ItemID] {
			continue
		}
		merged = append(merged, h)
		seen[h.ItemID] = true
	}
	if limit > 0 && len(merged) > limit {
		merged = merged[:limit]
	}
	return merged
}

func (b *Backend) searchLexical(ctx context.Context, kb core.KnowledgeBase, query string, limit int) ([]core.SearchHit, error) {
	if !b.ftsEnabled {
		return b.searchLike(ctx, kb, query, limit)
	}
	hits, err := b.searchFTS(ctx, kb, query, limit)
	if err != nil {
		return b.searchLike(ctx, kb, query, limit)
	}
	if len(hits) > 0 {
		return hits, nil
	}
	return b.searchLike(ctx, kb, query, limit)
}

func (b *Backend) searchFTS(ctx context.Context, kb core.KnowledgeBase, query string, limit int) ([]core.SearchHit, error) {
	tokens := core.TokenizeQuery(query)
	if len(tokens) == 0 {
		return nil, nil
	}
	parts := make([]string, len(tokens))
	for i, tok := range tokens {
		parts[i] = `"` + strings.ReplaceAll(tok, `"`, `""`) + `"`
	}
	matchExpr := strings.Join(parts, " OR ")
	rows, err := b.db.QueryContext(ctx, `
				SELECT k.id, k.title, k.content
				FROM knowledge_items_fts f
				JOIN knowledge_items k ON k.id = f.rowid
				WHERE knowledge_items_fts MATCH ? AND k.kb_id = ?
				LIMIT ?
			`, matchExpr, kb.ID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanHits(rows, kb.ID)
}

func (b *Backend) searchLike(ctx context.Context, kb core.KnowledgeBase, query string, limit int) ([]core.SearchHit, error) {
	tokens := core.TokenizeQuery(query)
	if len(tokens) == 0 {
		return nil, nil
	}
	clauses := make([]string, len(tokens))
	args := make([]any, 0, 1+2*len(tokens)+1)
	args = append(args, kb.ID)
	for i, tok := range tokens {
		clauses[i] = "(title LIKE ? OR content LIKE ?)"
		pattern := "%" + tok + "%"
		args = append(args, pattern, pattern)
	}
	args = append(args, limit)
	stmt := `
				SELECT id, title, content
				FROM knowledge_items
				WHERE kb_id = ? AND (` + strings.Join(clauses, " OR ") + `)
				ORDER BY id DESC
				LIMIT ?
			`
	rows, err := b.db.QueryContext(ctx, stmt, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanHits(rows, kb.ID)
}

func scanHits(rows *sql.Rows, kbID string) ([]core.SearchHit, error) {
	var hits []core.SearchHit
	for rows.Next() {
		var id int64
		var title, content string
		if err := rows.Scan(&id, &title, &content); err != nil {
			return nil, err
		}
		hits = append(hits, core.SearchHit{ItemID: fmt.Sprintf("%d", id), KBID: kbID, ItemType: "note", Title: title, Snippet: content, ContentPreview: content, Score: 1, MatchMode: "lexical", SourceBackend: "sqlite"})
	}
	return hits, rows.Err()
}

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

func (b *Backend) DeleteItem(ctx context.Context, kb core.KnowledgeBase, itemID string) error {
	tx, err := b.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}

	var id int64
	var title, content, tags, metadataJSON, createdAtRaw, updatedAtRaw string
	err = tx.QueryRowContext(ctx, `
			SELECT id, title, content, tags, metadata_json, created_at, updated_at
			FROM knowledge_items
			WHERE kb_id = ? AND id = ?
		`, kb.ID, itemID).Scan(&id, &title, &content, &tags, &metadataJSON, &createdAtRaw, &updatedAtRaw)
	if err != nil {
		_ = tx.Rollback()
		if errors.Is(err, sql.ErrNoRows) {
			return &core.Error{Kind: core.ErrorKindStore, Message: "knowledge item not found"}
		}
		return err
	}

	res, err := tx.ExecContext(ctx, `DELETE FROM knowledge_items WHERE kb_id = ? AND id = ?`, kb.ID, itemID)
	if err != nil {
		_ = tx.Rollback()
		return err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		_ = tx.Rollback()
		return err
	}
	if affected == 0 {
		_ = tx.Rollback()
		return &core.Error{Kind: core.ErrorKindStore, Message: "knowledge item not found"}
	}

	if !b.indexer.SupportsKB(kb) {
		return tx.Commit()
	}
	if err := b.indexer.DeleteItem(ctx, kb, itemID); err != nil {
		_ = tx.Rollback()
		return err
	}
	if err := tx.Commit(); err != nil {
		metadata := map[string]any{}
		_ = json.Unmarshal([]byte(metadataJSON), &metadata)
		updatedAt, parseErr := time.Parse(time.RFC3339, updatedAtRaw)
		if parseErr != nil {
			updatedAt = time.Now().UTC()
		}
		createdAt, parseErr := time.Parse(time.RFC3339, createdAtRaw)
		if parseErr != nil {
			createdAt = updatedAt
		}
		restored := core.KnowledgeItem{
			ID:        fmt.Sprintf("%d", id),
			KBID:      kb.ID,
			Title:     title,
			Content:   content,
			Tags:      splitTags(tags),
			Metadata:  metadata,
			CreatedAt: createdAt,
			UpdatedAt: updatedAt,
		}
		cleanupCtx, cancel := cleanupContext(ctx)
		defer cancel()
		restoreErr := b.indexer.UpsertItem(cleanupCtx, kb, restored, sqliteMeta(restored))
		commitErr := fmt.Errorf("sqlite commit failed after semantic delete success: %w", err)
		if restoreErr != nil {
			return errors.Join(commitErr, fmt.Errorf("semantic restore failed: %w", restoreErr))
		}
		return commitErr
	}
	return nil
}

func (b *Backend) MaintainIndex(ctx context.Context, kb core.KnowledgeBase, opt core.IndexOptions) (core.IndexResult, error) {
	return b.indexer.MaintainIndex(ctx, kb, opt,
		func(c context.Context) ([]core.KnowledgeItem, error) { return b.ListItems(c, kb) },
		func(item core.KnowledgeItem) map[string]any { return sqliteMeta(item) })
}

func splitTags(tags string) []string {
	if tags == "" {
		return nil
	}
	parts := strings.Split(tags, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func (b *Backend) SupportsSemantic(kb core.KnowledgeBase) bool {
	return b.indexer != nil && b.indexer.SupportsKB(kb)
}

func (b *Backend) Close() error {
	if b.db != nil {
		return b.db.Close()
	}
	return nil
}
