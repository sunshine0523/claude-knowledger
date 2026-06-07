# Knowledger

Knowledger 是一个面向 agent 的知识聚合工具，提供统一 Core、CLI、MCP 和 Web 控制台。

## 当前能力

- Text knowledge base
- SQLite knowledge base + FTS5
- 默认 SQLite 存储位置：`~/.knowledger/db`
- SQLite + Chroma hybrid search 降级基础
- CLI / MCP / Web 三个入口的基础适配
- 异步索引 worker

## 默认存储

当没有指定配置文件且当前目录不存在 `knowledger.yaml` 时，Knowledger 会使用默认 knowledge base：

- `id: default`
- `store_type: sqlite`
- `store_config.path: ~/.knowledger/db`
- lexical indexing enabled
- semantic indexing uses embedded persistent Chroma at `~/.knowledger/chroma/default`, collection `default`

在配置文件中省略 `knowledge_bases` 或设置 `knowledge_bases: []` 也会生成同样的默认 SQLite + embedded persistent Chroma 配置。对于显式的 SQLite knowledge base，如果省略 `store_config.path`，同样会使用 `~/.knowledger/db`。默认模式不需要运行外部 Chroma server。

## Web Dashboard

Web 控制台包含知识库管理页面 `/kbs`。静态 `knowledger.yaml` 中的 knowledge base 会以只读方式展示；通过 Web 新增/删除的 knowledge base 写入运行时注册表，默认位置为 `~/.knowledger/registry.json`。Web 删除只移除运行时配置，不删除文本目录、Markdown 文件或 SQLite 数据。

当前 SQLite 后端仍限制所有 SQLite knowledge base 共享同一个物理 `store_config.path`；可以创建多个逻辑 SQLite knowledge base，但不能在同一进程内混用多个 SQLite 路径。

启动 Web：

```bash
go run ./cmd/knowledger serve
```

默认监听：

```text
http://127.0.0.1:34125/
```

使用指定配置启动：

```bash
go run ./cmd/knowledger --config knowledger.example.yaml serve
```

## 开发

本项目要求 Go 1.24+。Ubuntu LTS 的 apt 源可能落后，如果 `go version` 低于 1.24，优先使用官方 Go tarball、asdf、mise 或 snap 安装新版 Go；不要假设 `apt install golang-go` 足够。

```bash
go test ./...
CGO_ENABLED=1 go test -tags fts5 ./...
go run ./cmd/knowledger list-kbs
go run ./cmd/knowledger serve
go run ./cmd/knowledger --config knowledger.example.yaml list-kbs
```

## Smoke Test

1. 运行 `go test ./...`。
2. 运行 `CGO_ENABLED=1 go test -tags fts5 ./...`。
3. 不指定配置执行 `go run ./cmd/knowledger list-kbs`，确认出现 `default` knowledge base。
4. 复用同一个临时 HOME 执行 add/search：

   ```bash
   TMP_HOME="$(mktemp -d)"
   HOME="$TMP_HOME" go run ./cmd/knowledger add --kb default --title "Default DB" --content "SQLite default storage"
   HOME="$TMP_HOME" go run ./cmd/knowledger search --query SQLite
   ls "$TMP_HOME/.knowledger/db"
   ```

5. 打开 `http://127.0.0.1:34125/` 检查 Dashboard / Search Lab / Debug 页面。
