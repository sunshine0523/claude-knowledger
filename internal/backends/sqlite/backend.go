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
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/kindbrave/knowledger/internal/core"
	"github.com/kindbrave/knowledger/internal/indexing/chroma"
	_ "github.com/mattn/go-sqlite3"
)

//go:embed schema.sql
var schemaSQL string

//go:embed fts5.sql
var fts5SQL string

type Option func(*Backend)

type Backend struct {
	db              *sql.DB
	ftsEnabled      bool
	semanticFactory chroma.Factory
	semanticMu      sync.Mutex
	semanticClients map[string]chroma.Client
}

type MultiBackend struct {
	backends map[string]*Backend
}

func WithSemanticClientFactory(factory chroma.Factory) Option {
	return func(b *Backend) {
		if factory != nil {
			b.semanticFactory = factory
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
	backend := &Backend{
		db:              db,
		ftsEnabled:      ftsEnabled,
		semanticFactory: chroma.NewClient,
		semanticClients: map[string]chroma.Client{},
	}
	for _, opt := range opts {
		if opt != nil {
			opt(backend)
		}
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

func (b *Backend) Add(ctx context.Context, kb core.KnowledgeBase, input core.AddInput) (core.KnowledgeItem, core.IngestionResult, core.IndexStatus, error) {
	nowTime := time.Now().UTC()
	now := nowTime.Format(time.RFC3339)
	metadataJSON, err := json.Marshal(input.Metadata)
	if err != nil {
		return core.KnowledgeItem{}, core.IngestionResult{}, core.IndexStatus{}, err
	}
	semanticCfg, semanticEnabled := semanticConfig(kb)

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

	client, err := b.semanticClient(semanticCfg)
	if err != nil {
		_ = tx.Rollback()
		return core.KnowledgeItem{}, core.IngestionResult{}, core.IndexStatus{}, fmt.Errorf("semantic index failed; sqlite item rolled back: %w", err)
	}
	if err := client.Upsert(ctx, semanticCfg.Collection, chromaItem(item)); err != nil {
		_ = tx.Rollback()
		return core.KnowledgeItem{}, core.IngestionResult{}, core.IndexStatus{}, fmt.Errorf("semantic index failed; sqlite item rolled back: %w", err)
	}
	if err := tx.Commit(); err != nil {
		cleanupCtx, cancel := cleanupContext(ctx)
		defer cancel()
		cleanupErr := client.Delete(cleanupCtx, semanticCfg.Collection, item.ID)
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
		if _, ok := semanticConfig(kb); !ok {
			return b.searchLexical(ctx, kb, opt.Query, limit)
		}
		return b.searchSemantic(ctx, kb, opt.Query, limit, "semantic")
	case "hybrid":
		if _, ok := semanticConfig(kb); !ok {
			return b.searchLexical(ctx, kb, opt.Query, limit)
		}
		return b.searchHybrid(ctx, kb, opt.Query, limit)
	default:
		return b.searchLexical(ctx, kb, opt.Query, limit)
	}
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

func (b *Backend) searchSemantic(ctx context.Context, kb core.KnowledgeBase, query string, limit int, matchMode string) ([]core.SearchHit, error) {
	cfg, ok := semanticConfig(kb)
	if !ok {
		return nil, nil
	}
	tokens := core.TokenizeQuery(query)
	if len(tokens) == 0 {
		return nil, nil
	}
	client, err := b.semanticClient(cfg)
	if err != nil {
		return nil, err
	}
	merged := map[string]chroma.Hit{}
	for _, tok := range tokens {
		raw, err := client.Query(ctx, cfg.Collection, tok, limit)
		if err != nil {
			return nil, err
		}
		for _, hit := range filterSemanticHits(kb.ID, raw) {
			if prev, exists := merged[hit.ItemID]; !exists || hit.Score > prev.Score {
				merged[hit.ItemID] = hit
			}
		}
	}
	deduped := make([]chroma.Hit, 0, len(merged))
	for _, hit := range merged {
		deduped = append(deduped, hit)
	}
	sort.Slice(deduped, func(i, j int) bool { return deduped[i].Score > deduped[j].Score })
	if limit > 0 && len(deduped) > limit {
		deduped = deduped[:limit]
	}
	return semanticSearchHits(kb.ID, matchMode, deduped), nil
}

func (b *Backend) searchHybrid(ctx context.Context, kb core.KnowledgeBase, query string, limit int) ([]core.SearchHit, error) {
	semanticHits, err := b.searchSemantic(ctx, kb, query, limit, "hybrid")
	if err != nil {
		return nil, err
	}
	lexicalHits, err := b.searchLexical(ctx, kb, query, limit)
	if err != nil {
		return nil, err
	}
	merged := make([]core.SearchHit, 0, len(semanticHits)+len(lexicalHits))
	seen := map[string]bool{}
	for _, hit := range semanticHits {
		merged = append(merged, hit)
		seen[hit.ItemID] = true
	}
	for _, hit := range lexicalHits {
		if seen[hit.ItemID] {
			continue
		}
		merged = append(merged, hit)
		seen[hit.ItemID] = true
	}
	if len(merged) > limit {
		merged = merged[:limit]
	}
	return merged, nil
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

func filterSemanticHits(kbID string, hits []chroma.Hit) []chroma.Hit {
	out := make([]chroma.Hit, 0, len(hits))
	for _, hit := range hits {
		hitKBID, ok := hit.Metadata["kb_id"].(string)
		if !ok || hitKBID != kbID {
			continue
		}
		out = append(out, hit)
	}
	return out
}

func semanticSearchHits(kbID, matchMode string, hits []chroma.Hit) []core.SearchHit {
	out := make([]core.SearchHit, 0, len(hits))
	for _, hit := range hits {
		out = append(out, core.SearchHit{
			ItemID:         hit.ItemID,
			KBID:           kbID,
			ItemType:       "note",
			Title:          hit.Title(),
			Snippet:        hit.Content,
			ContentPreview: hit.Content,
			Score:          hit.Score,
			MatchMode:      matchMode,
			SourceBackend:  "chroma",
			Metadata:       hit.Metadata,
		})
	}
	return out
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
	var title, content, tags, metadataJSON string
	err = tx.QueryRowContext(ctx, `
		SELECT id, title, content, tags, metadata_json
		FROM knowledge_items
		WHERE kb_id = ? AND id = ?
	`, kb.ID, itemID).Scan(&id, &title, &content, &tags, &metadataJSON)
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

	cfg, ok := semanticConfig(kb)
	if !ok {
		return tx.Commit()
	}
	client, err := b.semanticClient(cfg)
	if err != nil {
		_ = tx.Rollback()
		return err
	}
	if err := client.Delete(ctx, cfg.Collection, itemID); err != nil {
		_ = tx.Rollback()
		return err
	}
	if err := tx.Commit(); err != nil {
		metadata := map[string]any{}
		_ = json.Unmarshal([]byte(metadataJSON), &metadata)
		cleanupCtx, cancel := cleanupContext(ctx)
		defer cancel()
		restoreErr := client.Upsert(cleanupCtx, cfg.Collection, chromaItem(core.KnowledgeItem{ID: fmt.Sprintf("%d", id), KBID: kb.ID, Title: title, Content: content, Tags: splitTags(tags), Metadata: metadata}))
		commitErr := fmt.Errorf("sqlite commit failed after semantic delete success: %w", err)
		if restoreErr != nil {
			return errors.Join(commitErr, fmt.Errorf("semantic restore failed: %w", restoreErr))
		}
		return commitErr
	}
	return nil
}

func (b *Backend) MaintainIndex(ctx context.Context, kb core.KnowledgeBase, opt core.IndexOptions) (core.IndexResult, error) {
	cfg, ok := semanticConfig(kb)
	if !ok {
		return core.IndexResult{Skipped: 1, Warnings: []string{fmt.Sprintf("%s: semantic indexing is not enabled", kb.ID)}}, nil
	}
	client, err := b.semanticClient(cfg)
	if err != nil {
		return core.IndexResult{}, &core.Error{Kind: core.ErrorKindIndex, Message: "semantic index client unavailable", Cause: err}
	}
	items, err := b.ListItems(ctx, kb)
	if err != nil {
		return core.IndexResult{}, err
	}

	result := core.IndexResult{}
	if opt.Rebuild {
		deleted, err := deleteSemanticItems(ctx, client, cfg.Collection, kb.ID, items)
		if err != nil {
			return result, &core.Error{Kind: core.ErrorKindIndex, Message: "semantic index rebuild delete failed", Cause: err}
		}
		result.Deleted = deleted
	}
	for _, item := range items {
		if err := client.Upsert(ctx, cfg.Collection, chromaItem(item)); err != nil {
			return result, &core.Error{Kind: core.ErrorKindIndex, Message: fmt.Sprintf("semantic index upsert failed for item %s", item.ID), Cause: err}
		}
		result.Indexed++
	}
	return result, nil
}

type knowledgeBaseSemanticDeleter interface {
	DeleteForKnowledgeBase(context.Context, string, string) error
}

func deleteSemanticItems(ctx context.Context, client chroma.Client, collection string, kbID string, items []core.KnowledgeItem) (int, error) {
	if deleter, ok := client.(knowledgeBaseSemanticDeleter); ok {
		if err := deleter.DeleteForKnowledgeBase(ctx, collection, kbID); err != nil {
			return 0, err
		}
		return len(items), nil
	}
	for _, item := range items {
		if err := client.Delete(ctx, collection, item.ID); err != nil {
			return 0, err
		}
	}
	return len(items), nil
}

func chromaItem(item core.KnowledgeItem) chroma.Item {
	return chroma.Item{ID: item.ID, KBID: item.KBID, Title: item.Title, Content: item.Content, Tags: item.Tags, Metadata: item.Metadata}
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
	_, ok := semanticConfig(kb)
	return ok
}

func (b *Backend) Close() error {
	b.semanticMu.Lock()
	clients := make([]chroma.Client, 0, len(b.semanticClients))
	for _, client := range b.semanticClients {
		clients = append(clients, client)
	}
	b.semanticClients = nil
	b.semanticMu.Unlock()

	var err error
	for _, client := range clients {
		err = errors.Join(err, client.Close())
	}
	if b.db != nil {
		err = errors.Join(err, b.db.Close())
	}
	return err
}

func (b *Backend) semanticClient(cfg chroma.Config) (chroma.Client, error) {
	b.semanticMu.Lock()
	defer b.semanticMu.Unlock()

	if b.semanticClients == nil {
		b.semanticClients = map[string]chroma.Client{}
	}
	key := semanticClientKey(cfg)
	if client := b.semanticClients[key]; client != nil {
		return client, nil
	}
	factory := b.semanticFactory
	if factory == nil {
		factory = chroma.NewClient
	}
	client, err := factory(cfg)
	if err != nil {
		return nil, err
	}
	b.semanticClients[key] = client
	return client, nil
}

func semanticClientKey(cfg chroma.Config) string {
	return strings.Join([]string{cfg.EffectiveMode(), cfg.BaseURL, cfg.Path, strconv.FormatBool(cfg.AutoDownload)}, "\x00")
}

func semanticConfig(kb core.KnowledgeBase) (chroma.Config, bool) {
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
	cfg.Mode = stringValue(semantic["mode"])
	cfg.BaseURL = stringValue(semantic["base_url"])
	cfg.Path = stringValue(semantic["path"])
	if collection := stringValue(semantic["collection"]); collection != "" {
		cfg.Collection = collection
	}
	if autoDownload, ok := semantic["auto_download"].(bool); ok {
		cfg.AutoDownload = autoDownload
	}
	return cfg, true
}

func stringValue(value any) string {
	out, _ := value.(string)
	return out
}
