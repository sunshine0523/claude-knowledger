# Knowledger

[Simplified Chinese](README.zh-CN.md)

Knowledger is a local-first knowledge aggregation tool for agents. It provides a unified core service with CLI, Web dashboard, and MCP-facing adapters so agents and humans can add, browse, and retrieve knowledge from multiple knowledge bases.

## Features

- **Multiple backends**: SQLite and text-file knowledge bases.
- **Local default storage**: runs without a config file using `~/.knowledger/db`.
- **Lexical search**: SQLite FTS5 when available, plus text search for file-based stores.
- **Semantic search**: SQLite knowledge bases can use Chroma for semantic or hybrid retrieval.
- **Embedded persistent Chroma**: the default setup does not require an external Chroma server.
- **CLI workflow**: add, search, fetch, and list knowledge bases from the terminal.
- **Web dashboard**: manage runtime knowledge bases, inspect items, and test search behavior.
- **MCP adapter foundation**: high-level tool definitions for agent integration.

## Requirements

- Go 1.24+
- CGO-enabled Go toolchain

## Installation

**macOS / Linux** — one-liner:

```bash
curl -fsSL https://raw.githubusercontent.com/kindbrave/claude-knowledger/main/install.sh | sh
```

**Windows** (PowerShell):

```powershell
irm https://raw.githubusercontent.com/kindbrave/claude-knowledger/main/install.ps1 | iex
```

The script detects your OS and architecture, downloads the correct binary from the [latest release](https://github.com/kindbrave/claude-knowledger/releases/latest), and installs it to `/usr/local/bin` (or `~/.local/bin` if `/usr/local/bin` is not writable).

**Build from source** (requires Go 1.24+ with CGO):

```bash
git clone https://github.com/kindbrave/claude-knowledger.git
cd claude-knowledger
go build -o knowledger ./cmd/knowledger
```

## Quick Start

Knowledger can run without `knowledger.yaml`. If no config file is present, it creates a default SQLite knowledge base:

- ID: `default`
- SQLite path: `~/.knowledger/db`
- Embedded Chroma path: `~/.knowledger/chroma/default`
- Chroma collection: `default`

Add a knowledge item:

```bash
go run ./cmd/knowledger add \
  --kb default \
  --title "SQLite notes" \
  --content "Knowledger stores local knowledge in SQLite."
```

Search knowledge:

```bash
go run ./cmd/knowledger search --query SQLite --limit 10
```

Get an item by ID:

```bash
go run ./cmd/knowledger get --kb default --id 1
```

List knowledge bases:

```bash
go run ./cmd/knowledger list-kbs
```

## Web Dashboard

Start the dashboard:

```bash
go run ./cmd/knowledger serve
```

Default address:

```text
http://127.0.0.1:34125/
```

Pages:

- `/` — dashboard overview
- `/kbs` — knowledge base management
- `/knowledge` — knowledge item browsing
- `/search-lab` — search testing UI
- `/debug` — diagnostics

Runtime knowledge bases created from the Web dashboard are stored in `~/.knowledger/registry.json` by default. Static knowledge bases defined in `knowledger.yaml` are read-only in the dashboard. Deleting a runtime knowledge base removes only its registry entry; it does not delete SQLite databases, text directories, or Markdown files.

## MCP Server

Start Knowledger as a local MCP server over stdio:

```bash
knowledger mcp
```

Example MCP client configuration:

```json
{
  "mcpServers": {
    "knowledger": {
      "command": "/absolute/path/to/knowledger",
      "args": ["mcp"]
    }
  }
}
```

With an explicit config file:

```json
{
  "mcpServers": {
    "knowledger": {
      "command": "/absolute/path/to/knowledger",
      "args": ["--config", "/absolute/path/to/knowledger.yaml", "mcp"]
    }
  }
}
```

The MCP server exposes `get_knowledge_item`, `add_knowledge_item`, and `list_knowledge_bases`.

## Configuration

Copy the example config:

```bash
cp knowledger.example.yaml knowledger.yaml
```

Start with an explicit config file:

```bash
go run ./cmd/knowledger --config knowledger.yaml serve
```

Minimal SQLite config:

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

Text knowledge base config:

```yaml
knowledge_bases:
  - id: docs
    name: Docs
    store_type: text
    enabled: true
    store_config:
      path: ./data/docs
```

Text knowledge bases read and write `.md` and `.txt` files.

## Search Modes

Supported search modes:

- `auto` — use the knowledge base default behavior.
- `lexical` — keyword/FTS search.
- `semantic` — vector search through Chroma for supported SQLite knowledge bases.
- `hybrid` — combine semantic and lexical retrieval when supported.

Example:

```bash
go run ./cmd/knowledger search --query "agent memory" --search-mode hybrid --limit 5
```

If semantic search is unavailable at query time, supported paths fall back to lexical search with warnings.

## Development

Run tests:

```bash
go test ./...
```

Run SQLite/FTS5 tests:

```bash
CGO_ENABLED=1 go test -tags fts5 ./...
```

## Project Status

Knowledger is in early development. The CLI and Web dashboard are usable for local experiments. The MCP adapter currently provides foundational tool registration for future agent integration.
