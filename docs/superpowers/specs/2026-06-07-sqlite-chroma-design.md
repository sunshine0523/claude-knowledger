# SQLite + Chroma Semantic Search Design

## Goal

Make the default knowledge base a SQLite-backed store that also supports Chroma vector retrieval. Existing default/runtime SQLite configuration should be upgraded by adding missing Chroma semantic indexing settings, without deleting the physical SQLite database file.

## Current State

- Default config already creates a `default` SQLite knowledge base at `~/.knowledger/db`.
- SQLite config defaults already add `indexing.semantic.provider: chroma` metadata.
- SQLite backend currently reports `SupportsSemantic=false`, so `semantic` and `hybrid` search requests fall back to lexical results.
- The Web knowledge-base creation form supports `text` and `sqlite`, but does not expose Chroma semantic indexing choices.

## Backend Behavior

SQLite remains the primary store type. Chroma is a semantic provider configured under the SQLite knowledge base, not a separate `store_type`.

For a SQLite knowledge base:

- Semantic support is enabled when `indexing.semantic.enabled=true` and `indexing.semantic.provider=chroma`.
- `semantic` and `hybrid` searches first query Chroma using the configured `base_url` and `collection`.
- Chroma results are converted to `core.SearchHit` values with semantic/hybrid match metadata.
- If Chroma is unavailable, returns an error, or returns no parseable hits, SQLite lexical/FTS results are returned instead with a warning.
- This fallback applies to both explicit `semantic` searches and `hybrid` searches.

## Ingestion and Indexing

SQLite `Add` writes to the local SQLite database first. If Chroma semantic indexing is enabled for the KB, the backend then attempts to upsert the new item into the configured Chroma collection.

- Successful Chroma upsert returns `IndexStatus{State: "indexed"}`.
- Failed Chroma upsert does not roll back the SQLite write.
- Failed Chroma upsert returns `IndexStatus{State: "failed"}` and adds an ingestion warning.
- Existing SQLite data is not deleted and is not automatically reindexed in this change.

## Defaults and Runtime Configuration

Default creation remains SQLite-first:

- `store_type: sqlite`
- `store_config.path: ~/.knowledger/db`
- `indexing.lexical.enabled: true`
- `indexing.semantic.enabled: true`
- `indexing.semantic.provider: chroma`
- `indexing.semantic.base_url: http://127.0.0.1:8000`
- `indexing.semantic.collection: <kb id>`
- `indexing.semantic.sync_mode: async`

Runtime registry entries loaded through the registry should receive the same SQLite defaults as static config, so older runtime SQLite KBs missing semantic config are upgraded in memory and persisted when created through the API. The physical SQLite file is not deleted.

## Web Management UI and API

The `/kbs` create form defaults to SQLite.

When `store_type=sqlite`, the form shows an "enable Chroma vector search" checkbox that is checked by default. When the checkbox is checked, `POST /api/kbs` creates the SQLite KB with Chroma semantic indexing enabled. When unchecked, it creates a SQLite KB with semantic indexing disabled. The option is disabled or ignored for `text` KBs.

The create API accepts a semantic/Chroma option and passes it into service normalization. The response can keep the existing KB view shape; exposing detailed indexing settings in the table is not required for this change.

## Testing

Add or update tests for:

- SQLite backend reports semantic support only for enabled Chroma semantic config.
- SQLite semantic search queries Chroma and maps returned results to search hits.
- SQLite semantic search falls back to lexical results with warning behavior through service search when Chroma fails.
- SQLite Add attempts Chroma upsert when enabled and reports indexed/failed status without losing the SQLite item.
- Runtime-created SQLite KBs default to Chroma semantic enabled unless the API request disables it.
- Web create API accepts the Chroma option and rejects/ignores it appropriately for non-SQLite KBs.
- The `/kbs` template defaults the type selector to SQLite and includes the Chroma checkbox.

## Out of Scope

- Deleting `~/.knowledger/db` or any other physical database file.
- Full reindexing of existing SQLite rows into Chroma.
- A separate Chroma-only store type.
- Exposing live indexing queue/failure dashboards.
