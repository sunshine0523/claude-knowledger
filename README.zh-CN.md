# Knowledger

[English](README.md)

Knowledger 是一个本地优先、面向 agent 的知识聚合工具。它提供统一的核心服务，并通过 CLI、Web 控制台和 MCP 适配层，让 agent 与人都可以添加、浏览和检索多个知识库中的内容。

## 功能特性

- **多后端支持**：SQLite 知识库和文本文件知识库。
- **默认本地存储**：没有配置文件也能运行，默认使用 `~/.knowledger/db`。
- **词法搜索**：SQLite 可使用 FTS5；文件型知识库支持文本搜索。
- **语义搜索**：SQLite 知识库可使用 Chroma 进行 semantic 或 hybrid 检索。
- **内嵌持久化 Chroma**：默认配置不需要外部 Chroma server。
- **CLI 工作流**：在终端中添加、搜索、获取知识条目，并列出知识库。
- **Web 控制台**：管理运行时知识库、查看知识条目、调试搜索行为。
- **MCP 适配基础**：提供面向 agent 集成的高层工具定义。

## 环境要求

- Go 1.24+
- 支持 CGO 的 Go 工具链

## 安装

从源码构建：

```bash
git clone https://github.com/kindbrave/knowledger.git
cd knowledger
go build -o knowledger ./cmd/knowledger
```

不安装直接运行：

```bash
go run ./cmd/knowledger list-kbs
```

安装 CLI：

```bash
go install github.com/kindbrave/knowledger/cmd/knowledger@latest
```

## 快速开始

Knowledger 可以在没有 `knowledger.yaml` 的情况下运行。当前目录没有配置文件时，会创建默认 SQLite 知识库：

- ID：`default`
- SQLite 路径：`~/.knowledger/db`
- 内嵌 Chroma 路径：`~/.knowledger/chroma/default`
- Chroma collection：`default`

添加知识条目：

```bash
go run ./cmd/knowledger add \
  --kb default \
  --title "SQLite notes" \
  --content "Knowledger stores local knowledge in SQLite."
```

搜索知识：

```bash
go run ./cmd/knowledger search --query SQLite --limit 10
```

通过 ID 获取条目：

```bash
go run ./cmd/knowledger get --kb default --id 1
```

列出知识库：

```bash
go run ./cmd/knowledger list-kbs
```

## Web 控制台

启动控制台：

```bash
go run ./cmd/knowledger serve
```

默认地址：

```text
http://127.0.0.1:34125/
```

页面：

- `/` — 仪表盘概览
- `/kbs` — 知识库管理
- `/knowledge` — 知识条目浏览
- `/search-lab` — 搜索调试 UI
- `/debug` — 诊断页面

通过 Web 控制台创建的运行时知识库默认写入 `~/.knowledger/registry.json`。`knowledger.yaml` 中定义的静态知识库在控制台中只读展示。删除运行时知识库只会移除注册表记录，不会删除 SQLite 数据库、文本目录或 Markdown 文件。

## 配置

复制示例配置：

```bash
cp knowledger.example.yaml knowledger.yaml
```

使用指定配置启动：

```bash
go run ./cmd/knowledger --config knowledger.yaml serve
```

最小 SQLite 配置：

```yaml
default_search_mode: auto
runtime_registry_path: ~/.knowledger/registry.json
server:
  address: ":34125"
knowledge_bases:
  - id: default
    name: Default
    store_type: sqlite
    enabled: true
    store_config:
      path: ~/.knowledger/db
    indexing:
      lexical:
        enabled: true
      semantic:
        enabled: true
        provider: chroma
        mode: persistent
        path: ~/.knowledger/chroma/default
        collection: default
        auto_download: true
        sync_mode: async
```

文本知识库配置：

```yaml
knowledge_bases:
  - id: docs
    name: Docs
    store_type: text
    enabled: true
    store_config:
      path: ./data/docs
```

文本知识库会读取和写入 `.md` / `.txt` 文件。

## 搜索模式

支持的搜索模式：

- `auto` — 使用知识库默认行为。
- `lexical` — 关键词 / FTS 搜索。
- `semantic` — 对支持的 SQLite 知识库通过 Chroma 进行向量搜索。
- `hybrid` — 在支持时结合语义检索和词法检索。

示例：

```bash
go run ./cmd/knowledger search --query "agent memory" --search-mode hybrid --limit 5
```

如果查询时语义搜索不可用，支持的路径会回退到词法搜索并返回 warning。

## 开发

运行测试：

```bash
go test ./...
```

运行 SQLite/FTS5 测试：

```bash
CGO_ENABLED=1 go test -tags fts5 ./...
```

## 项目状态

Knowledger 仍处于早期开发阶段。CLI 和 Web 控制台已可用于本地实验。MCP 适配层目前提供工具注册基础，后续可继续扩展面向 agent 的集成能力。
