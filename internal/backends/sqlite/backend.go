package sqlite

import (
	"context"
	"database/sql"
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/kindbrave/knowledger/internal/core"
	_ "github.com/mattn/go-sqlite3"
)

//go:embed schema.sql
var schemaSQL string

//go:embed fts5.sql
var fts5SQL string

type Backend struct {
	db         *sql.DB
	ftsEnabled bool
}

func New(path string) (*Backend, error) {
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
	return &Backend{db: db, ftsEnabled: ftsEnabled}, nil
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

func (b *Backend) Add(ctx context.Context, kb core.KnowledgeBase, input core.AddInput) (core.KnowledgeItem, core.IngestionResult, core.IndexStatus, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	metadataJSON, err := json.Marshal(input.Metadata)
	if err != nil {
		return core.KnowledgeItem{}, core.IngestionResult{}, core.IndexStatus{}, err
	}
	res, err := b.db.ExecContext(ctx, `
			INSERT INTO knowledge_items(kb_id, title, content, tags, metadata_json, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?)
		`, kb.ID, input.Title, input.Content, strings.Join(input.Tags, ","), string(metadataJSON), now, now)
	if err != nil {
		return core.KnowledgeItem{}, core.IngestionResult{}, core.IndexStatus{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return core.KnowledgeItem{}, core.IngestionResult{}, core.IndexStatus{}, err
	}
	item := core.KnowledgeItem{ID: fmt.Sprintf("%d", id), KBID: kb.ID, Type: "note", Title: input.Title, Content: input.Content}
	return item, core.IngestionResult{Success: true, ItemID: item.ID}, core.IndexStatus{State: "not_indexed"}, nil
}

func (b *Backend) Search(ctx context.Context, kb core.KnowledgeBase, opt core.SearchOptions) ([]core.SearchHit, error) {
	limit := opt.Limit
	if limit <= 0 {
		limit = 10
	}
	if !b.ftsEnabled {
		return b.searchLike(ctx, kb, opt.Query, limit)
	}
	hits, err := b.searchFTS(ctx, kb, opt.Query, limit)
	if err != nil {
		return b.searchLike(ctx, kb, opt.Query, limit)
	}
	if len(hits) > 0 {
		return hits, nil
	}
	return b.searchLike(ctx, kb, opt.Query, limit)
}

func (b *Backend) searchFTS(ctx context.Context, kb core.KnowledgeBase, query string, limit int) ([]core.SearchHit, error) {
	rows, err := b.db.QueryContext(ctx, `
			SELECT k.id, k.title, k.content
			FROM knowledge_items_fts f
			JOIN knowledge_items k ON k.id = f.rowid
			WHERE knowledge_items_fts MATCH ? AND k.kb_id = ?
			LIMIT ?
		`, query, kb.ID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanHits(rows, kb.ID)
}

func (b *Backend) searchLike(ctx context.Context, kb core.KnowledgeBase, query string, limit int) ([]core.SearchHit, error) {
	rows, err := b.db.QueryContext(ctx, `
			SELECT id, title, content
			FROM knowledge_items
			WHERE kb_id = ? AND (title LIKE ? OR content LIKE ?)
			ORDER BY id DESC
			LIMIT ?
		`, kb.ID, "%"+query+"%", "%"+query+"%", limit)
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

func (b *Backend) ListItems(ctx context.Context, kb core.KnowledgeBase) ([]core.KnowledgeItem, error) {
	rows, err := b.db.QueryContext(ctx, `SELECT id, title, content FROM knowledge_items WHERE kb_id = ? ORDER BY id DESC`, kb.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []core.KnowledgeItem
	for rows.Next() {
		var id int64
		var title, content string
		if err := rows.Scan(&id, &title, &content); err != nil {
			return nil, err
		}
		items = append(items, core.KnowledgeItem{ID: fmt.Sprintf("%d", id), KBID: kb.ID, Type: "note", Title: title, Content: content})
	}
	return items, rows.Err()
}

func (b *Backend) SupportsSemantic(kb core.KnowledgeBase) bool {
	return false
}
