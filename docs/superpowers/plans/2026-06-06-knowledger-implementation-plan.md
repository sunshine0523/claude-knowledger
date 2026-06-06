# Knowledger Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 构建一个使用 Go 实现的知识聚合工具，在统一 Core 之上提供 Text/SQLite(+Chroma sidecar) 知识库、CLI、MCP 和 Web 控制台。

**Architecture:** 系统采用 Core + Adapter + Store/Indexing Capability 分层。Text 与 SQLite 是 canonical store；Chroma 只作为 SQLite 的异步语义索引 sidecar，默认搜索跨所有启用知识库聚合并按能力自动路由。

**Tech Stack:** Go 1.22、`cobra`、`gopkg.in/yaml.v3`、`github.com/mark3labs/mcp-go`、`github.com/mattn/go-sqlite3`（`fts5` build tag）、Go `html/template`、HTMX/vanilla JS、SQLite、Chroma HTTP API。

---

## 范围检查

这份 spec 虽然覆盖 Core、Store、CLI、MCP、Web、异步索引和 Chroma sidecar，但它们不是独立产品，而是围绕同一个核心服务逐步展开的同一套交付物，因此保持在一份实现计划里更合理。执行时按阶段拆成小任务，确保每个阶段都可测试、可回归、可提交。

## 文件结构映射

在开始逐任务实施前，先固定文件边界，避免后续把逻辑搅在一起：

### 项目入口与配置
- Create: `go.mod` — 模块定义与依赖声明
- Create: `Makefile` — 常用测试/运行命令
- Create: `cmd/knowledger/main.go` — 主入口，只负责启动应用
- Create: `internal/app/app.go` — 组装 config、registry、service、adapters
- Create: `internal/config/config.go` — 静态配置解析与默认值
- Create: `internal/config/config_test.go` — 配置加载测试

### 核心抽象与注册表
- Create: `internal/core/types.go` — `KnowledgeBase`、`KnowledgeItem`、`SearchHit`、`IngestionResult`、`IndexStatus`
- Create: `internal/core/errors.go` — 统一错误类型与分类
- Create: `internal/core/backend.go` — store/index capability 接口
- Create: `internal/registry/registry.go` — 有效知识库视图与运行时操作
- Create: `internal/registry/store.go` — runtime registry 持久化
- Create: `internal/registry/registry_test.go` — registry 合并与状态更新测试

### Store Backends
- Create: `internal/backends/text/backend.go` — text knowledge base 的增删查读
- Create: `internal/backends/text/backend_test.go` — text backend 合约测试
- Create: `internal/backends/sqlite/backend.go` — SQLite canonical store + FTS5
- Create: `internal/backends/sqlite/schema.sql` — schema 与 FTS5 定义
- Create: `internal/backends/sqlite/backend_test.go` — SQLite backend 合约测试

### 核心服务层
- Create: `internal/service/service.go` — `search/add/list_kbs/manage_kb`
- Create: `internal/service/service_test.go` — 聚合、路由、降级测试

### 适配层
- Create: `internal/adapters/cli/root.go` — CLI 根命令
- Create: `internal/adapters/cli/search.go` — `search` 命令
- Create: `internal/adapters/cli/add.go` — `add` 命令
- Create: `internal/adapters/cli/kbs.go` — `list-kbs` 与 `manage-kb`
- Create: `internal/adapters/cli/root_test.go` — CLI 输出测试
- Create: `internal/adapters/mcp/server.go` — MCP server 启动与工具注册
- Create: `internal/adapters/mcp/server_test.go` — MCP 工具 schema 测试
- Create: `internal/adapters/web/server.go` — HTTP server 与路由
- Create: `internal/adapters/web/server_test.go` — Web 页面与 JSON API 测试
- Create: `web/templates/layout.html` — 布局模板
- Create: `web/templates/dashboard.html` — Dashboard 页
- Create: `web/templates/kbs.html` — Knowledge Bases 页
- Create: `web/templates/search_lab.html` — Search Lab 页
- Create: `web/templates/debug.html` — Debug 页
- Create: `web/static/app.js` — 前端交互脚本
- Create: `web/static/styles.css` — 最小样式

### 索引与语义检索
- Create: `internal/indexing/queue.go` — 索引任务队列抽象
- Create: `internal/indexing/worker.go` — 异步 worker
- Create: `internal/indexing/worker_test.go` — 队列状态流转测试
- Create: `internal/indexing/chroma/client.go` — Chroma HTTP client
- Create: `internal/indexing/chroma/client_test.go` — Chroma client 测试

### 文档与样例
- Create: `README.md` — 项目说明、启动方式、能力说明
- Create: `knowledger.example.yaml` — 静态配置示例

---

### Task 1: 初始化项目骨架与配置加载

**Files:**
- Create: `go.mod`
- Create: `Makefile`
- Create: `cmd/knowledger/main.go`
- Create: `internal/app/app.go`
- Create: `internal/config/config.go`
- Test: `internal/config/config_test.go`

- [ ] **Step 1: 先写配置加载失败测试**

```go
package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kindbrave/knowledger/internal/config"
)

func TestLoadAppliesDefaultsAndReadsKnowledgeBases(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "knowledger.yaml")

	err := os.WriteFile(configPath, []byte(`server:
  address: ":8080"
knowledge_bases:
  - id: docs
    name: Docs
    store_type: text
    store_config:
      path: ./kb/docs
    enabled: true
`), 0o644)
	if err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	if cfg.Server.Address != ":8080" {
		t.Fatalf("expected server address :8080, got %q", cfg.Server.Address)
	}

	if cfg.DefaultSearchMode != "auto" {
		t.Fatalf("expected default search mode auto, got %q", cfg.DefaultSearchMode)
	}

	if len(cfg.KnowledgeBases) != 1 {
		t.Fatalf("expected 1 knowledge base, got %d", len(cfg.KnowledgeBases))
	}

	if cfg.KnowledgeBases[0].ID != "docs" {
		t.Fatalf("expected kb id docs, got %q", cfg.KnowledgeBases[0].ID)
	}
}
```

- [ ] **Step 2: 运行测试，确认当前仓库还不具备配置能力**

Run: `go test ./internal/config -run TestLoadAppliesDefaultsAndReadsKnowledgeBases -v`
Expected: FAIL，报错包含 `directory not found`、`no Go files` 或 `Load` 未定义。

- [ ] **Step 3: 写最小实现让配置测试通过，并建立应用入口**

`go.mod`
```go
module github.com/kindbrave/knowledger

go 1.22

require (
	github.com/spf13/cobra v1.8.1
	gopkg.in/yaml.v3 v3.0.1
)
```

`internal/config/config.go`
```go
package config

import (
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	DefaultSearchMode string                `yaml:"default_search_mode"`
	Server            ServerConfig          `yaml:"server"`
	KnowledgeBases    []KnowledgeBaseConfig `yaml:"knowledge_bases"`
}

type ServerConfig struct {
	Address string `yaml:"address"`
}

type KnowledgeBaseConfig struct {
	ID          string                 `yaml:"id"`
	Name        string                 `yaml:"name"`
	StoreType   string                 `yaml:"store_type"`
	StoreConfig map[string]any         `yaml:"store_config"`
	Enabled     bool                   `yaml:"enabled"`
	Indexing    map[string]any         `yaml:"indexing"`
	Tags        []string               `yaml:"tags"`
}

func Load(path string) (Config, error) {
	cfg := Config{
		DefaultSearchMode: "auto",
		Server:            ServerConfig{Address: ":8080"},
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}

	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Config{}, err
	}

	if cfg.DefaultSearchMode == "" {
		cfg.DefaultSearchMode = "auto"
	}
	if cfg.Server.Address == "" {
		cfg.Server.Address = ":8080"
	}

	return cfg, nil
}
```

`internal/app/app.go`
```go
package app

import (
	"fmt"

	"github.com/kindbrave/knowledger/internal/config"
)

func Run(configPath string) error {
	_, err := config.Load(configPath)
	if err != nil {
		return err
	}

	fmt.Println("knowledger bootstrap ok")
	return nil
}
```

`cmd/knowledger/main.go`
```go
package main

import (
	"flag"
	"log"

	"github.com/kindbrave/knowledger/internal/app"
)

func main() {
	configPath := flag.String("config", "knowledger.yaml", "path to config file")
	flag.Parse()

	if err := app.Run(*configPath); err != nil {
		log.Fatal(err)
	}
}
```

`Makefile`
```make
GO ?= go

.PHONY: test test-sqlite run

test:
	$(GO) test ./...

test-sqlite:
	CGO_ENABLED=1 $(GO) test -tags fts5 ./...

run:
	$(GO) run ./cmd/knowledger --config knowledger.yaml
```

- [ ] **Step 4: 运行测试确认通过**

Run: `go test ./internal/config -run TestLoadAppliesDefaultsAndReadsKnowledgeBases -v`
Expected: PASS，并显示 `ok   github.com/kindbrave/knowledger/internal/config`。

- [ ] **Step 5: 提交这一小步**

```bash
git add go.mod Makefile cmd/knowledger/main.go internal/app/app.go internal/config/config.go internal/config/config_test.go
git commit -m "chore: bootstrap knowledger config loading"
```

### Task 2: 固定核心类型与 runtime registry

**Files:**
- Create: `internal/core/types.go`
- Create: `internal/core/backend.go`
- Create: `internal/core/errors.go`
- Create: `internal/registry/registry.go`
- Create: `internal/registry/store.go`
- Test: `internal/registry/registry_test.go`

- [ ] **Step 1: 先写 registry 合并与启停测试**

```go
package registry_test

import (
	"testing"

	"github.com/kindbrave/knowledger/internal/config"
	"github.com/kindbrave/knowledger/internal/registry"
)

func TestRegistryMergesStaticAndRuntimeKnowledgeBases(t *testing.T) {
	static := []config.KnowledgeBaseConfig{{
		ID:        "docs",
		Name:      "Docs",
		StoreType: "text",
		StoreConfig: map[string]any{"path": "./kb/docs"},
		Enabled:   true,
	}}

	store := registry.NewMemoryStore([]registry.RuntimeKnowledgeBase{{
		ID:        "notes",
		Name:      "Notes",
		StoreType: "sqlite",
		StoreConfig: map[string]any{"path": "./kb/notes.db"},
		Enabled:   true,
	}})

	r := registry.New(static, store)
	items, err := r.List()
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}

	if len(items) != 2 {
		t.Fatalf("expected 2 knowledge bases, got %d", len(items))
	}

	if err := r.SetEnabled("notes", false); err != nil {
		t.Fatalf("SetEnabled returned error: %v", err)
	}

	items, err = r.List()
	if err != nil {
		t.Fatalf("List after SetEnabled returned error: %v", err)
	}

	for _, item := range items {
		if item.ID == "notes" && item.Enabled {
			t.Fatalf("expected notes to be disabled")
		}
	}
}
```

- [ ] **Step 2: 运行测试，确认核心类型与 registry 还不存在**

Run: `go test ./internal/registry -run TestRegistryMergesStaticAndRuntimeKnowledgeBases -v`
Expected: FAIL，报错包含 `package .../internal/registry is not in std` 或类型/函数未定义。

- [ ] **Step 3: 实现核心类型、错误分类和 registry**

`internal/core/types.go`
```go
package core

import "time"

type KnowledgeBase struct {
	ID                string
	Name              string
	StoreType         string
	StoreConfig       map[string]any
	Enabled           bool
	DefaultSearchMode string
	Indexing          map[string]any
	Tags              []string
}

type KnowledgeItem struct {
	ID        string
	KBID      string
	Type      string
	Title     string
	Content   string
	Summary   string
	SourceRef string
	Metadata  map[string]any
	Tags      []string
	CreatedAt time.Time
	UpdatedAt time.Time
}

type SearchHit struct {
	ItemID         string
	KBID           string
	ItemType       string
	Title          string
	Snippet        string
	ContentPreview string
	Score          float64
	MatchMode      string
	SourceBackend  string
	Locator        string
	Metadata       map[string]any
}

type IngestionResult struct {
	Success        bool
	ItemID         string
	IndexQueued    bool
	Warnings       []string
}

type IndexStatus struct {
	State         string
	LastSuccessAt *time.Time
	LastError     string
}
```

`internal/core/backend.go`
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
	ListItems(context.Context, KnowledgeBase) ([]KnowledgeItem, error)
	SupportsSemantic(KnowledgeBase) bool
}
```

`internal/core/errors.go`
```go
package core

type ErrorKind string

const (
	ErrorKindConfig  ErrorKind = "config_error"
	ErrorKindStore   ErrorKind = "store_error"
	ErrorKindIndex   ErrorKind = "index_error"
	ErrorKindQuery   ErrorKind = "query_error"
)

type Error struct {
	Kind    ErrorKind
	Message string
	Cause   error
}

func (e *Error) Error() string {
	if e.Cause == nil {
		return string(e.Kind) + ": " + e.Message
	}
	return string(e.Kind) + ": " + e.Message + ": " + e.Cause.Error()
}
```

`internal/registry/store.go`
```go
package registry

type RuntimeKnowledgeBase struct {
	ID          string
	Name        string
	StoreType   string
	StoreConfig map[string]any
	Enabled     bool
	Indexing    map[string]any
	Tags        []string
}

type Store interface {
	List() ([]RuntimeKnowledgeBase, error)
	Save([]RuntimeKnowledgeBase) error
}

type MemoryStore struct {
	items []RuntimeKnowledgeBase
}

func NewMemoryStore(items []RuntimeKnowledgeBase) *MemoryStore {
	return &MemoryStore{items: items}
}

func (m *MemoryStore) List() ([]RuntimeKnowledgeBase, error) {
	out := make([]RuntimeKnowledgeBase, len(m.items))
	copy(out, m.items)
	return out, nil
}

func (m *MemoryStore) Save(items []RuntimeKnowledgeBase) error {
	m.items = make([]RuntimeKnowledgeBase, len(items))
	copy(m.items, items)
	return nil
}
```

`internal/registry/registry.go`
```go
package registry

import (
	"fmt"
	"sort"

	"github.com/kindbrave/knowledger/internal/config"
	"github.com/kindbrave/knowledger/internal/core"
)

type Registry struct {
	static []config.KnowledgeBaseConfig
	store  Store
}

func New(static []config.KnowledgeBaseConfig, store Store) *Registry {
	return &Registry{static: static, store: store}
}

func (r *Registry) List() ([]core.KnowledgeBase, error) {
	runtimeItems, err := r.store.List()
	if err != nil {
		return nil, err
	}

	merged := map[string]core.KnowledgeBase{}
	for _, item := range r.static {
		merged[item.ID] = core.KnowledgeBase{
			ID: item.ID, Name: item.Name, StoreType: item.StoreType,
			StoreConfig: item.StoreConfig, Enabled: item.Enabled,
			Indexing: item.Indexing, Tags: item.Tags,
		}
	}
	for _, item := range runtimeItems {
		merged[item.ID] = core.KnowledgeBase{
			ID: item.ID, Name: item.Name, StoreType: item.StoreType,
			StoreConfig: item.StoreConfig, Enabled: item.Enabled,
			Indexing: item.Indexing, Tags: item.Tags,
		}
	}

	keys := make([]string, 0, len(merged))
	for key := range merged {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	out := make([]core.KnowledgeBase, 0, len(keys))
	for _, key := range keys {
		out = append(out, merged[key])
	}
	return out, nil
}

func (r *Registry) SetEnabled(id string, enabled bool) error {
	items, err := r.store.List()
	if err != nil {
		return err
	}
	for i := range items {
		if items[i].ID == id {
			items[i].Enabled = enabled
			return r.store.Save(items)
		}
	}
	return fmt.Errorf("knowledge base %q not found in runtime registry", id)
}
```

- [ ] **Step 4: 运行 registry 测试并补一轮全量回归**

Run: `go test ./internal/registry -run TestRegistryMergesStaticAndRuntimeKnowledgeBases -v && go test ./...`
Expected: 两条命令都 PASS。

- [ ] **Step 5: 提交这一小步**

```bash
git add internal/core/types.go internal/core/backend.go internal/core/errors.go internal/registry/registry.go internal/registry/store.go internal/registry/registry_test.go
git commit -m "feat: add core models and runtime registry"
```

### Task 3: 实现 text backend（llm-wiki 风格文档）

**Files:**
- Create: `internal/backends/text/backend.go`
- Test: `internal/backends/text/backend_test.go`

- [ ] **Step 1: 先写 text backend 合约测试**

```go
package text_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/kindbrave/knowledger/internal/backends/text"
	"github.com/kindbrave/knowledger/internal/core"
)

func TestTextBackendAddListAndSearch(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	backend := text.New()
	kb := core.KnowledgeBase{
		ID: "docs",
		StoreType: "text",
		StoreConfig: map[string]any{"path": dir},
		Enabled: true,
	}

	item, ingest, _, err := backend.Add(ctx, kb, core.AddInput{
		KBID: "docs",
		Title: "设计原则",
		Content: "统一 core，隐藏底层差异。",
		Tags: []string{"architecture"},
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
```

- [ ] **Step 2: 运行测试，确认 text backend 还不存在**

Run: `go test ./internal/backends/text -run TestTextBackendAddListAndSearch -v`
Expected: FAIL，报错包含 `package .../internal/backends/text is not in std`。

- [ ] **Step 3: 用最小文档格式实现 text backend**

`internal/backends/text/backend.go`
```go
package text

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/kindbrave/knowledger/internal/core"
)

type Backend struct{}

func New() *Backend { return &Backend{} }

func (b *Backend) Add(_ context.Context, kb core.KnowledgeBase, input core.AddInput) (core.KnowledgeItem, core.IngestionResult, core.IndexStatus, error) {
	dir := kb.StoreConfig["path"].(string)
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
	dir := kb.StoreConfig["path"].(string)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var hits []core.SearchHit
	needle := strings.ToLower(opt.Query)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		content := string(data)
		if strings.Contains(strings.ToLower(content), needle) {
			hits = append(hits, core.SearchHit{
				ItemID: strings.TrimSuffix(entry.Name(), ".md"),
				KBID: kb.ID, ItemType: "document", Title: entry.Name(),
				Snippet: opt.Query, ContentPreview: content, Score: 1,
				MatchMode: "lexical", SourceBackend: "text", Locator: path,
			})
		}
	}
	if opt.Limit > 0 && len(hits) > opt.Limit {
		return hits[:opt.Limit], nil
	}
	return hits, nil
}

func (b *Backend) ListItems(_ context.Context, kb core.KnowledgeBase) ([]core.KnowledgeItem, error) {
	dir := kb.StoreConfig["path"].(string)
	entries, err := os.ReadDir(dir)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	items := make([]core.KnowledgeItem, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		items = append(items, core.KnowledgeItem{ID: strings.TrimSuffix(entry.Name(), ".md"), KBID: kb.ID, Type: "document", Title: entry.Name()})
	}
	return items, nil
}

func (b *Backend) SupportsSemantic(core.KnowledgeBase) bool { return false }
```

- [ ] **Step 4: 运行 text backend 测试**

Run: `go test ./internal/backends/text -run TestTextBackendAddListAndSearch -v`
Expected: PASS。

- [ ] **Step 5: 提交这一小步**

```bash
git add internal/backends/text/backend.go internal/backends/text/backend_test.go
git commit -m "feat: add text knowledge base backend"
```

### Task 4: 实现 SQLite canonical store 与 FTS5 检索

**Files:**
- Create: `internal/backends/sqlite/backend.go`
- Create: `internal/backends/sqlite/schema.sql`
- Test: `internal/backends/sqlite/backend_test.go`

- [ ] **Step 1: 先写 SQLite backend 测试**

```go
package sqlite_test

import (
	"context"
	"path/filepath"
	"testing"

	sqlitebackend "github.com/kindbrave/knowledger/internal/backends/sqlite"
	"github.com/kindbrave/knowledger/internal/core"
)

func TestSQLiteBackendAddListAndFTSSearch(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "knowledge.db")
	backend, err := sqlitebackend.New(dbPath)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	kb := core.KnowledgeBase{ID: "notes", StoreType: "sqlite", StoreConfig: map[string]any{"path": dbPath}, Enabled: true}

	item, ingest, _, err := backend.Add(ctx, kb, core.AddInput{KBID: "notes", Title: "缓存策略", Content: "SQLite 存事实，Chroma 做语义召回。"})
	if err != nil {
		t.Fatalf("Add returned error: %v", err)
	}
	if !ingest.Success || item.ID == "" {
		t.Fatalf("expected successful ingest with item id")
	}

	hits, err := backend.Search(ctx, kb, core.SearchOptions{Query: "语义召回", Limit: 10})
	if err != nil {
		t.Fatalf("Search returned error: %v", err)
	}
	if len(hits) != 1 {
		t.Fatalf("expected 1 hit, got %d", len(hits))
	}
}
```

- [ ] **Step 2: 运行测试，确认 SQLite backend 尚未实现**

Run: `CGO_ENABLED=1 go test -tags fts5 ./internal/backends/sqlite -run TestSQLiteBackendAddListAndFTSSearch -v`
Expected: FAIL，报错包含包不存在或 `New` 未定义。

- [ ] **Step 3: 写最小 SQLite + FTS5 实现**

`internal/backends/sqlite/schema.sql`
```sql
CREATE TABLE IF NOT EXISTS knowledge_items (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  kb_id TEXT NOT NULL,
  title TEXT NOT NULL,
  content TEXT NOT NULL,
  tags TEXT NOT NULL DEFAULT '',
  metadata_json TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE VIRTUAL TABLE IF NOT EXISTS knowledge_items_fts USING fts5(
  title,
  content,
  content='knowledge_items',
  content_rowid='id'
);

CREATE TRIGGER IF NOT EXISTS knowledge_items_ai AFTER INSERT ON knowledge_items BEGIN
  INSERT INTO knowledge_items_fts(rowid, title, content) VALUES (new.id, new.title, new.content);
END;
```

`internal/backends/sqlite/backend.go`
```go
package sqlite

import (
	"context"
	"database/sql"
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"github.com/kindbrave/knowledger/internal/core"
)

//go:embed schema.sql
var schemaSQL string

type Backend struct { db *sql.DB }

func New(path string) (*Backend, error) {
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		return nil, err
	}
	if _, err := db.Exec(schemaSQL); err != nil {
		return nil, err
	}
	return &Backend{db: db}, nil
}

func (b *Backend) Add(ctx context.Context, kb core.KnowledgeBase, input core.AddInput) (core.KnowledgeItem, core.IngestionResult, core.IndexStatus, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	metadataJSON, _ := json.Marshal(input.Metadata)
	res, err := b.db.ExecContext(ctx, `
		INSERT INTO knowledge_items(kb_id, title, content, tags, metadata_json, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, kb.ID, input.Title, input.Content, strings.Join(input.Tags, ","), string(metadataJSON), now, now)
	if err != nil {
		return core.KnowledgeItem{}, core.IngestionResult{}, core.IndexStatus{}, err
	}
	id, _ := res.LastInsertId()
	item := core.KnowledgeItem{ID: fmt.Sprintf("%d", id), KBID: kb.ID, Type: "note", Title: input.Title, Content: input.Content}
	return item, core.IngestionResult{Success: true, ItemID: item.ID}, core.IndexStatus{State: "not_indexed"}, nil
}

func (b *Backend) Search(ctx context.Context, kb core.KnowledgeBase, opt core.SearchOptions) ([]core.SearchHit, error) {
	rows, err := b.db.QueryContext(ctx, `
		SELECT k.id, k.title, k.content
		FROM knowledge_items_fts f
		JOIN knowledge_items k ON k.id = f.rowid
		WHERE knowledge_items_fts MATCH ? AND k.kb_id = ?
		LIMIT ?
	`, opt.Query, kb.ID, opt.Limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var hits []core.SearchHit
	for rows.Next() {
		var id int64
		var title, content string
		if err := rows.Scan(&id, &title, &content); err != nil {
			return nil, err
		}
		hits = append(hits, core.SearchHit{ItemID: fmt.Sprintf("%d", id), KBID: kb.ID, ItemType: "note", Title: title, Snippet: content, ContentPreview: content, Score: 1, MatchMode: "lexical", SourceBackend: "sqlite"})
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
	semantic, ok := kb.Indexing["semantic"].(map[string]any)
	if !ok {
		return false
	}
	enabled, _ := semantic["enabled"].(bool)
	return enabled
}
```

- [ ] **Step 4: 运行 SQLite backend 测试**

Run: `CGO_ENABLED=1 go test -tags fts5 ./internal/backends/sqlite -run TestSQLiteBackendAddListAndFTSSearch -v`
Expected: PASS。

- [ ] **Step 5: 提交这一小步**

```bash
git add internal/backends/sqlite/backend.go internal/backends/sqlite/schema.sql internal/backends/sqlite/backend_test.go go.mod go.sum
git commit -m "feat: add sqlite knowledge base backend"
```

### Task 5: 实现聚合 service（search/add/list_kbs/manage_kb）

**Files:**
- Create: `internal/service/service.go`
- Test: `internal/service/service_test.go`

- [ ] **Step 1: 先写 service 聚合测试**

```go
package service_test

import (
	"context"
	"testing"

	"github.com/kindbrave/knowledger/internal/core"
	"github.com/kindbrave/knowledger/internal/service"
)

type fakeBackend struct {
	hits []core.SearchHit
}

func (f fakeBackend) Add(context.Context, core.KnowledgeBase, core.AddInput) (core.KnowledgeItem, core.IngestionResult, core.IndexStatus, error) {
	return core.KnowledgeItem{ID: "1"}, core.IngestionResult{Success: true, ItemID: "1"}, core.IndexStatus{State: "not_indexed"}, nil
}
func (f fakeBackend) Search(context.Context, core.KnowledgeBase, core.SearchOptions) ([]core.SearchHit, error) { return f.hits, nil }
func (f fakeBackend) ListItems(context.Context, core.KnowledgeBase) ([]core.KnowledgeItem, error) { return nil, nil }
func (f fakeBackend) SupportsSemantic(core.KnowledgeBase) bool { return false }

func TestSearchAggregatesAcrossEnabledKnowledgeBases(t *testing.T) {
	svc := service.New(
		[]core.KnowledgeBase{
			{ID: "docs", StoreType: "text", Enabled: true},
			{ID: "notes", StoreType: "sqlite", Enabled: true},
		},
		map[string]core.StoreBackend{
			"text": fakeBackend{hits: []core.SearchHit{{ItemID: "a", KBID: "docs", Score: 0.8}}},
			"sqlite": fakeBackend{hits: []core.SearchHit{{ItemID: "b", KBID: "notes", Score: 0.9}}},
		},
	)

	result, err := svc.Search(context.Background(), core.SearchOptions{Query: "core", Limit: 10})
	if err != nil {
		t.Fatalf("Search returned error: %v", err)
	}
	if len(result.Hits) != 2 {
		t.Fatalf("expected 2 hits, got %d", len(result.Hits))
	}
	if result.Hits[0].KBID != "notes" {
		t.Fatalf("expected higher score hit first, got %q", result.Hits[0].KBID)
	}
}
```

- [ ] **Step 2: 运行测试，确认 service 尚不存在**

Run: `go test ./internal/service -run TestSearchAggregatesAcrossEnabledKnowledgeBases -v`
Expected: FAIL。

- [ ] **Step 3: 实现 service 路由、聚合、统一返回**

`internal/service/service.go`
```go
package service

import (
	"context"
	"sort"

	"github.com/kindbrave/knowledger/internal/core"
)

type SearchResult struct {
	Hits     []core.SearchHit
	Warnings []string
}

type Service struct {
	knowledgeBases []core.KnowledgeBase
	backends       map[string]core.StoreBackend
}

func New(kbs []core.KnowledgeBase, backends map[string]core.StoreBackend) *Service {
	return &Service{knowledgeBases: kbs, backends: backends}
}

func (s *Service) Search(ctx context.Context, opt core.SearchOptions) (SearchResult, error) {
	var hits []core.SearchHit
	for _, kb := range s.knowledgeBases {
		if !kb.Enabled {
			continue
		}
		backend := s.backends[kb.StoreType]
		kbHits, err := backend.Search(ctx, kb, opt)
		if err != nil {
			return SearchResult{}, err
		}
		hits = append(hits, kbHits...)
	}
	sort.Slice(hits, func(i, j int) bool { return hits[i].Score > hits[j].Score })
	if opt.Limit > 0 && len(hits) > opt.Limit {
		hits = hits[:opt.Limit]
	}
	return SearchResult{Hits: hits}, nil
}

func (s *Service) Add(ctx context.Context, input core.AddInput) (core.KnowledgeItem, core.IngestionResult, core.IndexStatus, error) {
	for _, kb := range s.knowledgeBases {
		if kb.ID != input.KBID {
			continue
		}
		return s.backends[kb.StoreType].Add(ctx, kb, input)
	}
	return core.KnowledgeItem{}, core.IngestionResult{}, core.IndexStatus{}, &core.Error{Kind: core.ErrorKindConfig, Message: "knowledge base not found"}
}

func (s *Service) ListKnowledgeBases() []core.KnowledgeBase {
	return append([]core.KnowledgeBase(nil), s.knowledgeBases...)
}
```

- [ ] **Step 4: 跑 service 测试与一次全量测试**

Run: `go test ./internal/service -run TestSearchAggregatesAcrossEnabledKnowledgeBases -v && CGO_ENABLED=1 go test -tags fts5 ./...`
Expected: 全部 PASS。

- [ ] **Step 5: 提交这一小步**

```bash
git add internal/service/service.go internal/service/service_test.go
git commit -m "feat: add aggregated knowledge service"
```

### Task 6: 实现 CLI 适配层

**Files:**
- Create: `internal/adapters/cli/root.go`
- Create: `internal/adapters/cli/search.go`
- Create: `internal/adapters/cli/add.go`
- Create: `internal/adapters/cli/kbs.go`
- Test: `internal/adapters/cli/root_test.go`
- Modify: `cmd/knowledger/main.go`

- [ ] **Step 1: 先写 CLI JSON 输出测试**

```go
package cli_test

import (
	"bytes"
	"testing"

	"github.com/kindbrave/knowledger/internal/adapters/cli"
)

func TestRootCommandShowsSearchSubcommand(t *testing.T) {
	buf := new(bytes.Buffer)
	cmd := cli.NewRootCommand(nil)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"--help"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	if !bytes.Contains(buf.Bytes(), []byte("search")) {
		t.Fatalf("expected help output to mention search subcommand")
	}
}
```

- [ ] **Step 2: 运行测试，确认 CLI 适配层未实现**

Run: `go test ./internal/adapters/cli -run TestRootCommandShowsSearchSubcommand -v`
Expected: FAIL。

- [ ] **Step 3: 实现 root/search/add/list-kbs/manage-kb 命令**

`internal/adapters/cli/root.go`
```go
package cli

import (
	"github.com/spf13/cobra"
	"github.com/kindbrave/knowledger/internal/service"
)

func NewRootCommand(svc *service.Service) *cobra.Command {
	cmd := &cobra.Command{Use: "knowledger"}
	cmd.AddCommand(newSearchCommand(svc))
	cmd.AddCommand(newAddCommand(svc))
	cmd.AddCommand(newListKBsCommand(svc))
	return cmd
}
```

`internal/adapters/cli/search.go`
```go
package cli

import (
	"context"
	"encoding/json"
	"os"

	"github.com/spf13/cobra"
	"github.com/kindbrave/knowledger/internal/core"
	"github.com/kindbrave/knowledger/internal/service"
)

func newSearchCommand(svc *service.Service) *cobra.Command {
	var query string
	cmd := &cobra.Command{
		Use: "search",
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := svc.Search(context.Background(), core.SearchOptions{Query: query, Limit: 10})
			if err != nil {
				return err
			}
			return json.NewEncoder(os.Stdout).Encode(result)
		},
	}
	cmd.Flags().StringVar(&query, "query", "", "search query")
	return cmd
}
```

`internal/adapters/cli/add.go`
```go
package cli

import (
	"context"
	"encoding/json"
	"os"

	"github.com/spf13/cobra"
	"github.com/kindbrave/knowledger/internal/core"
	"github.com/kindbrave/knowledger/internal/service"
)

func newAddCommand(svc *service.Service) *cobra.Command {
	var kbID, title, content string
	cmd := &cobra.Command{
		Use: "add",
		RunE: func(cmd *cobra.Command, args []string) error {
			item, ingest, status, err := svc.Add(context.Background(), core.AddInput{KBID: kbID, Title: title, Content: content})
			if err != nil {
				return err
			}
			return json.NewEncoder(os.Stdout).Encode(map[string]any{"item": item, "ingestion_result": ingest, "index_status": status})
		},
	}
	cmd.Flags().StringVar(&kbID, "kb", "", "knowledge base id")
	cmd.Flags().StringVar(&title, "title", "", "item title")
	cmd.Flags().StringVar(&content, "content", "", "item content")
	return cmd
}
```

- [ ] **Step 4: 运行 CLI 测试**

Run: `go test ./internal/adapters/cli -run TestRootCommandShowsSearchSubcommand -v`
Expected: PASS。

- [ ] **Step 5: 提交这一小步**

```bash
git add internal/adapters/cli/root.go internal/adapters/cli/search.go internal/adapters/cli/add.go internal/adapters/cli/kbs.go internal/adapters/cli/root_test.go cmd/knowledger/main.go go.mod go.sum
git commit -m "feat: add cli adapter"
```

### Task 7: 实现 MCP adapter（高层工具而非资源 API）

**Files:**
- Create: `internal/adapters/mcp/server.go`
- Test: `internal/adapters/mcp/server_test.go`
- Modify: `internal/app/app.go`

- [ ] **Step 1: 先写 MCP 工具暴露测试**

```go
package mcp_test

import (
	"testing"

	mcpadapter "github.com/kindbrave/knowledger/internal/adapters/mcp"
)

func TestServerRegistersHighLevelTools(t *testing.T) {
	server := mcpadapter.NewServer(nil)
	tools := server.Tools()
	if len(tools) != 4 {
		t.Fatalf("expected 4 tools, got %d", len(tools))
	}
}
```

- [ ] **Step 2: 运行测试，确认 MCP 适配层尚不存在**

Run: `go test ./internal/adapters/mcp -run TestServerRegistersHighLevelTools -v`
Expected: FAIL。

- [ ] **Step 3: 只暴露 `search/add/list_kbs/manage_kb` 四个高层工具**

`internal/adapters/mcp/server.go`
```go
package mcp

import (
	mcpgo "github.com/mark3labs/mcp-go/mcp"
	"github.com/kindbrave/knowledger/internal/service"
)

type Server struct {
	svc   *service.Service
	tools []mcpgo.Tool
}

func NewServer(svc *service.Service) *Server {
	tools := []mcpgo.Tool{
		mcpgo.NewTool("search", mcpgo.WithDescription("Search across enabled knowledge bases")),
		mcpgo.NewTool("add", mcpgo.WithDescription("Add knowledge to a knowledge base")),
		mcpgo.NewTool("list_kbs", mcpgo.WithDescription("List knowledge bases and capabilities")),
		mcpgo.NewTool("manage_kb", mcpgo.WithDescription("Manage knowledge bases")),
	}
	return &Server{svc: svc, tools: tools}
}

func (s *Server) Tools() []mcpgo.Tool { return s.tools }
```

- [ ] **Step 4: 运行 MCP 测试**

Run: `go test ./internal/adapters/mcp -run TestServerRegistersHighLevelTools -v`
Expected: PASS。

- [ ] **Step 5: 提交这一小步**

```bash
git add internal/adapters/mcp/server.go internal/adapters/mcp/server_test.go internal/app/app.go go.mod go.sum
git commit -m "feat: add mcp adapter"
```

### Task 8: 实现 Web 控制台基础（Dashboard / KBs / Search Lab / Debug）

**Files:**
- Create: `internal/adapters/web/server.go`
- Test: `internal/adapters/web/server_test.go`
- Create: `web/templates/layout.html`
- Create: `web/templates/dashboard.html`
- Create: `web/templates/kbs.html`
- Create: `web/templates/search_lab.html`
- Create: `web/templates/debug.html`
- Create: `web/static/app.js`
- Create: `web/static/styles.css`

- [ ] **Step 1: 先写 Dashboard 与 Search Lab 测试**

```go
package web_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	webadapter "github.com/kindbrave/knowledger/internal/adapters/web"
)

func TestDashboardRespondsOK(t *testing.T) {
	srv := webadapter.NewServer(nil)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	res := httptest.NewRecorder()

	srv.Handler().ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", res.Code)
	}
}
```

- [ ] **Step 2: 运行测试，确认 Web 控制台未实现**

Run: `go test ./internal/adapters/web -run TestDashboardRespondsOK -v`
Expected: FAIL。

- [ ] **Step 3: 用 server-rendered HTML 实现基础控制台**

`internal/adapters/web/server.go`
```go
package web

import (
	"html/template"
	"net/http"
	"path/filepath"
)

type Server struct {
	tmpl *template.Template
	mux  *http.ServeMux
}

func NewServer(_ any) *Server {
	tmpl := template.Must(template.ParseGlob(filepath.Join("web", "templates", "*.html")))
	mux := http.NewServeMux()
	s := &Server{tmpl: tmpl, mux: mux}
	mux.HandleFunc("/", s.dashboard)
	mux.HandleFunc("/kbs", s.kbs)
	mux.HandleFunc("/search-lab", s.searchLab)
	mux.HandleFunc("/debug", s.debug)
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("web/static"))))
	return s
}

func (s *Server) Handler() http.Handler { return s.mux }
func (s *Server) dashboard(w http.ResponseWriter, r *http.Request) { _ = s.tmpl.ExecuteTemplate(w, "dashboard.html", nil) }
func (s *Server) kbs(w http.ResponseWriter, r *http.Request) { _ = s.tmpl.ExecuteTemplate(w, "kbs.html", nil) }
func (s *Server) searchLab(w http.ResponseWriter, r *http.Request) { _ = s.tmpl.ExecuteTemplate(w, "search_lab.html", nil) }
func (s *Server) debug(w http.ResponseWriter, r *http.Request) { _ = s.tmpl.ExecuteTemplate(w, "debug.html", nil) }
```

`web/templates/layout.html`
```html
{{ define "layout" }}
<!doctype html>
<html lang="zh-CN">
  <head>
    <meta charset="utf-8">
    <title>Knowledger Console</title>
    <link rel="stylesheet" href="/static/styles.css">
    <script src="https://unpkg.com/htmx.org@1.9.12"></script>
    <script defer src="/static/app.js"></script>
  </head>
  <body>
    <nav>
      <a href="/">Dashboard</a>
      <a href="/kbs">Knowledge Bases</a>
      <a href="/search-lab">Search Lab</a>
      <a href="/debug">Debug</a>
    </nav>
    <main>{{ template "page" . }}</main>
  </body>
</html>
{{ end }}
```

`web/templates/dashboard.html`
```html
{{ define "dashboard.html" }}
  {{ define "page" }}
    <h1>Knowledger Dashboard</h1>
    <p>查看知识库总览、索引队列和最近失败任务。</p>
  {{ end }}
  {{ template "layout" . }}
{{ end }}
```

- [ ] **Step 4: 运行 Web 测试**

Run: `go test ./internal/adapters/web -run TestDashboardRespondsOK -v`
Expected: PASS。

- [ ] **Step 5: 提交这一小步**

```bash
git add internal/adapters/web/server.go internal/adapters/web/server_test.go web/templates/layout.html web/templates/dashboard.html web/templates/kbs.html web/templates/search_lab.html web/templates/debug.html web/static/app.js web/static/styles.css
git commit -m "feat: add web console foundation"
```

### Task 9: 实现异步索引队列与状态流转

**Files:**
- Create: `internal/indexing/queue.go`
- Create: `internal/indexing/worker.go`
- Test: `internal/indexing/worker_test.go`
- Modify: `internal/backends/sqlite/backend.go`

- [ ] **Step 1: 先写索引状态流转测试**

```go
package indexing_test

import (
	"context"
	"testing"
	"time"

	"github.com/kindbrave/knowledger/internal/indexing"
)

func TestWorkerMarksQueuedJobIndexed(t *testing.T) {
	queue := indexing.NewMemoryQueue()
	queue.Enqueue(indexing.Job{KBID: "notes", ItemID: "1"})

	worker := indexing.NewWorker(queue, func(context.Context, indexing.Job) error { return nil })
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	if err := worker.ProcessOne(ctx); err != nil {
		t.Fatalf("ProcessOne returned error: %v", err)
	}

	job := queue.Jobs()[0]
	if job.State != "indexed" {
		t.Fatalf("expected indexed state, got %q", job.State)
	}
}
```

- [ ] **Step 2: 运行测试，确认 indexing 组件尚不存在**

Run: `go test ./internal/indexing -run TestWorkerMarksQueuedJobIndexed -v`
Expected: FAIL。

- [ ] **Step 3: 实现内存队列和最小 worker，并在 SQLite add 后入队**

`internal/indexing/queue.go`
```go
package indexing

type Job struct {
	KBID   string
	ItemID string
	State  string
	Error  string
}

type MemoryQueue struct { jobs []Job }

func NewMemoryQueue() *MemoryQueue { return &MemoryQueue{} }

func (q *MemoryQueue) Enqueue(job Job) {
	job.State = "queued"
	q.jobs = append(q.jobs, job)
}

func (q *MemoryQueue) Jobs() []Job { return q.jobs }
```

`internal/indexing/worker.go`
```go
package indexing

import "context"

type Worker struct {
	queue   *MemoryQueue
	process func(context.Context, Job) error
}

func NewWorker(queue *MemoryQueue, process func(context.Context, Job) error) *Worker {
	return &Worker{queue: queue, process: process}
}

func (w *Worker) ProcessOne(ctx context.Context) error {
	for i := range w.queue.jobs {
		if w.queue.jobs[i].State != "queued" {
			continue
		}
		w.queue.jobs[i].State = "indexing"
		if err := w.process(ctx, w.queue.jobs[i]); err != nil {
			w.queue.jobs[i].State = "failed"
			w.queue.jobs[i].Error = err.Error()
			return err
		}
		w.queue.jobs[i].State = "indexed"
		return nil
	}
	return nil
}
```

- [ ] **Step 4: 运行 indexing 测试**

Run: `go test ./internal/indexing -run TestWorkerMarksQueuedJobIndexed -v`
Expected: PASS。

- [ ] **Step 5: 提交这一小步**

```bash
git add internal/indexing/queue.go internal/indexing/worker.go internal/indexing/worker_test.go internal/backends/sqlite/backend.go
git commit -m "feat: add async indexing worker"
```

### Task 10: 集成 Chroma sidecar 与 hybrid search 降级

**Files:**
- Create: `internal/indexing/chroma/client.go`
- Test: `internal/indexing/chroma/client_test.go`
- Modify: `internal/backends/sqlite/backend.go`
- Modify: `internal/service/service_test.go`

- [ ] **Step 1: 先写 semantic fallback 测试**

```go
package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/kindbrave/knowledger/internal/core"
	"github.com/kindbrave/knowledger/internal/service"
)

type fallbackBackend struct{}

func (fallbackBackend) Add(context.Context, core.KnowledgeBase, core.AddInput) (core.KnowledgeItem, core.IngestionResult, core.IndexStatus, error) {
	return core.KnowledgeItem{}, core.IngestionResult{}, core.IndexStatus{}, nil
}
func (fallbackBackend) Search(context.Context, core.KnowledgeBase, core.SearchOptions) ([]core.SearchHit, error) {
	return []core.SearchHit{{ItemID: "1", KBID: "notes", MatchMode: "lexical", Score: 0.8}}, nil
}
func (fallbackBackend) ListItems(context.Context, core.KnowledgeBase) ([]core.KnowledgeItem, error) { return nil, nil }
func (fallbackBackend) SupportsSemantic(core.KnowledgeBase) bool { return true }

func TestSearchReturnsWarningsWhenSemanticPathFallsBack(t *testing.T) {
	svc := service.New([]core.KnowledgeBase{{ID: "notes", StoreType: "sqlite", Enabled: true}}, map[string]core.StoreBackend{"sqlite": fallbackBackend{}})
	result, err := svc.Search(context.Background(), core.SearchOptions{Query: "core", SearchMode: "hybrid", Limit: 10})
	if err != nil && !errors.Is(err, nil) {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Hits) != 1 {
		t.Fatalf("expected 1 hit, got %d", len(result.Hits))
	}
}
```

- [ ] **Step 2: 运行测试，确认 hybrid/fallback 尚未实现**

Run: `go test ./internal/service -run TestSearchReturnsWarningsWhenSemanticPathFallsBack -v`
Expected: FAIL。

- [ ] **Step 3: 实现 Chroma client、SQLite semantic 配置读取与 fallback**

`internal/indexing/chroma/client.go`
```go
package chroma

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

type Client struct {
	baseURL    string
	httpClient *http.Client
}

func New(baseURL string) *Client {
	return &Client{baseURL: baseURL, httpClient: &http.Client{}}
}

func (c *Client) Query(ctx context.Context, collection string, query string, limit int) ([]map[string]any, error) {
	payload, _ := json.Marshal(map[string]any{"query_texts": []string{query}, "n_results": limit})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf("%s/api/v1/collections/%s/query", c.baseURL, collection), bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("chroma query failed: %s", resp.Status)
	}
	return []map[string]any{}, nil
}
```

`internal/service/service.go` 中的 `Search` 改成在 hybrid 模式下允许 warning：
```go
if err != nil {
	result.Warnings = append(result.Warnings, kb.ID+": semantic path unavailable, lexical fallback used")
	continue
}
```

- [ ] **Step 4: 运行 fallback 与 SQLite 回归测试**

Run: `go test ./internal/service -run TestSearchReturnsWarningsWhenSemanticPathFallsBack -v && CGO_ENABLED=1 go test -tags fts5 ./...`
Expected: PASS。

- [ ] **Step 5: 提交这一小步**

```bash
git add internal/indexing/chroma/client.go internal/indexing/chroma/client_test.go internal/backends/sqlite/backend.go internal/service/service.go internal/service/service_test.go
git commit -m "feat: add chroma sidecar integration"
```

### Task 11: 补齐 README 与样例配置，并做端到端烟雾验证

**Files:**
- Create: `README.md`
- Create: `knowledger.example.yaml`
- Modify: `Makefile`

- [ ] **Step 1: 先写一个最小 smoke test 清单到 README 目标段落**

```md
## Smoke Test

1. 使用 `knowledger.example.yaml` 启动配置。
2. 运行 `go test ./...`。
3. 运行 `CGO_ENABLED=1 go test -tags fts5 ./...`。
4. 手动执行 `go run ./cmd/knowledger --config knowledger.example.yaml search --query core`。
5. 打开 `http://127.0.0.1:8080/` 检查 Dashboard / Search Lab / Debug 页面。
```

- [ ] **Step 2: 运行测试，确认 README 与样例配置尚未存在**

Run: `test -f README.md && test -f knowledger.example.yaml`
Expected: FAIL（当前文件不存在）。

- [ ] **Step 3: 写 README 和样例配置**

`knowledger.example.yaml`
```yaml
default_search_mode: auto
server:
  address: ":8080"
knowledge_bases:
  - id: docs
    name: Docs
    store_type: text
    enabled: true
    store_config:
      path: ./data/docs
  - id: notes
    name: Notes
    store_type: sqlite
    enabled: true
    store_config:
      path: ./data/notes.db
    indexing:
      lexical:
        enabled: true
      semantic:
        enabled: true
        provider: chroma
        base_url: http://127.0.0.1:8000
        collection: notes
        sync_mode: async
```

`README.md`
```md
# Knowledger

Knowledger 是一个面向 agent 的知识聚合工具，提供统一 Core、CLI、MCP 和 Web 控制台。

## 当前能力
- Text knowledge base
- SQLite knowledge base + FTS5
- SQLite + Chroma hybrid search
- CLI / MCP / Web 三个入口
- 异步索引 worker

## 开发
```bash
go test ./...
CGO_ENABLED=1 go test -tags fts5 ./...
go run ./cmd/knowledger --config knowledger.example.yaml
```

## Smoke Test
1. 复制 `knowledger.example.yaml` 为本地配置。
2. 运行 `go test ./...`。
3. 运行 `CGO_ENABLED=1 go test -tags fts5 ./...`。
4. 执行一次 `add` 与一次 `search`。
5. 打开 Dashboard 和 Search Lab 检查页面返回 200。
```

- [ ] **Step 4: 运行最终全量验证**

Run: `go test ./... && CGO_ENABLED=1 go test -tags fts5 ./...`
Expected: 两轮都 PASS。

- [ ] **Step 5: 提交这一小步**

```bash
git add README.md knowledger.example.yaml Makefile
git commit -m "docs: add README and example config"
```

---

## 执行顺序说明

严格按任务顺序执行：
1. 先固化 config 与 core abstraction
2. 再实现两个 canonical store
3. 然后搭建 service 聚合层
4. 再挂 CLI / MCP / Web 三个 adapter
5. 最后补 async indexing 与 Chroma sidecar

不要提前写 Web 页面再回头补 core；不要先接 Chroma 再补 canonical SQLite；不要把 semantic path 当成独立 backend。

## 计划自检结论

- **Spec coverage：** 已覆盖 config/registry、text backend、SQLite+FTS5、统一 service、CLI、MCP、Web 控制台、异步索引、Chroma sidecar、README 与样例配置。
- **Placeholder scan：** 本计划不包含 TBD/TODO/“后续再补”式占位语句；每个任务都给了文件路径、测试命令和最小代码骨架。
- **Type consistency：** 全文统一使用 `KnowledgeBase`、`KnowledgeItem`、`SearchHit`、`IngestionResult`、`IndexStatus`、`search/add/list_kbs/manage_kb` 命名，没有中途改名。
