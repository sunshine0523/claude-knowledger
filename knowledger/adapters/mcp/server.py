from __future__ import annotations

import sys
from typing import Any

from knowledger.core import (
    AddInput, IndexOptions, KnowledgeBase, SearchOptions, ScopedKBRef,
    normalize_scope, SCOPE_GLOBAL, SCOPE_PROJECT,
)
from knowledger.service import CreateKnowledgeBaseInput, IndexKnowledgeInput, Service

SERVER_INSTRUCTIONS = """\
# Knowledger — local knowledge recall, runs BEFORE answering

Knowledger is the project's persistent knowledge base — decisions,
conventions, library/tool usage notes, debugging recipes, domain
references. It captures knowledge ABOUT the code that grep, file
reads, and codegraph cannot find.

## Recall — call BEFORE answering

Call search_knowledge BEFORE answering ANY of these question shapes,
even when the user does not say "knowledge / 知识库 / 记得":

- "How do I use X" / "X 怎么用"
- "What is X"     / "X 是什么"
- "How does X work" / "X 怎么实现"
- "Why did we do X this way"
- "What's our convention for X"
- "Where do we store/track X"
- Any debugging question that could have a saved recipe.

One cheap query. If hits are weak, list_knowledge_items the relevant
KBs — title/tag scans often surface what semantic search missed.

## Capture — only on explicit user intent

add_knowledge_item when the user says save / capture / remember /
记一下 / 保存到 / 添加到 — and the target KB is unambiguous.
Otherwise list_knowledge_bases and ask which KB to use.

## Skip

Conversational turns, ephemeral state, secrets, or anything fully
derivable from the current diff/file.
"""


def _default_scope(raw: str, svc: Service) -> str:
    raw = raw.strip()
    if not raw:
        return SCOPE_PROJECT if svc.has_project_scope() else SCOPE_GLOBAL
    return normalize_scope(raw)


def _parse_kb_ids(kb_ids: list[str], default_scope: str) -> list[ScopedKBRef]:
    out = []
    for raw in kb_ids:
        raw = raw.strip()
        if not raw:
            continue
        if ":" in raw:
            scope_part, id_part = raw.split(":", 1)
            out.append(ScopedKBRef(scope=normalize_scope(scope_part), id=id_part.strip()))
        else:
            out.append(ScopedKBRef(scope=default_scope, id=raw))
    return out


def _fmt_kb_list(svc: Service, kbs: list[KnowledgeBase]) -> str:
    if not kbs:
        return "no knowledge bases configured"
    lines = []
    for kb in kbs:
        scope = kb.scope or SCOPE_GLOBAL
        header = f"[{scope}:{kb.id}]"
        if kb.name and kb.name != kb.id:
            header += f" {kb.name}"
        header += f" (store={kb.store_type}"
        if not kb.enabled:
            header += ", disabled"
        if kb.tags:
            header += f", tags={','.join(kb.tags)}"
        header += ")"
        lines.append(header)
        try:
            items = svc.list_knowledge_items(scope, kb.id)
            if not items:
                lines.append("  (empty)")
            else:
                for item in items:
                    tag_str = f" [{','.join(item.tags)}]" if item.tags else ""
                    lines.append(f"  - {item.id}\t{item.title}{tag_str}")
        except Exception as e:
            lines.append(f"  (items unavailable: {e})")
    return "\n".join(lines)


class Server:
    def __init__(self, svc: Service | None):
        self.svc = svc
        self._mcp = self._build()

    def _build(self):
        from mcp.server.fastmcp import FastMCP
        mcp = FastMCP("knowledger", instructions=SERVER_INSTRUCTIONS)
        svc = self.svc

        @mcp.tool()
        def search_knowledge(
            query: str,
            scope: str = "",
            kb_ids: list[str] | None = None,
            limit: int = 10,
            search_mode: str = "auto",
        ) -> dict:
            """PRIMARY recall tool — call FIRST before answering any project/domain/library/tool question."""
            if svc is None:
                return {"error": "service is not configured"}
            try:
                ds = _default_scope(scope, svc)
                refs = _parse_kb_ids(kb_ids or [], ds)
                result = svc.search(SearchOptions(query=query, kb_ids=refs, limit=limit or 10, search_mode=search_mode))
                return {
                    "hits": [
                        {
                            "item_id": h.item_id, "kb_id": h.kb_id, "scope": h.scope,
                            "item_type": h.item_type, "title": h.title, "snippet": h.snippet,
                            "score": h.score, "match_mode": h.match_mode,
                            "source_backend": h.source_backend, "locator": h.locator,
                            "metadata": h.metadata,
                        }
                        for h in result.hits
                    ],
                    "warnings": result.warnings,
                }
            except Exception as e:
                return {"error": str(e)}

        @mcp.tool()
        def get_knowledge_item(kb_id: str, item_id: str, scope: str = "") -> dict:
            """Fetch the full content and metadata of a single knowledge item."""
            if svc is None:
                return {"error": "service is not configured"}
            try:
                sc = _default_scope(scope, svc)
                item = svc.get_knowledge_item(sc, kb_id, item_id)
                return {
                    "id": item.id, "kb_id": item.kb_id, "type": item.type,
                    "title": item.title, "content": item.content, "summary": item.summary,
                    "tags": item.tags, "metadata": item.metadata,
                    "created_at": item.created_at.isoformat() if item.created_at else "",
                    "updated_at": item.updated_at.isoformat() if item.updated_at else "",
                }
            except Exception as e:
                return {"error": str(e)}

        @mcp.tool()
        def list_knowledge_items(kb_id: str, scope: str = "", limit: int = 0, offset: int = 0) -> dict:
            """Browse a knowledge base as a lightweight directory (id/title/tags, no content)."""
            if svc is None:
                return {"error": "service is not configured"}
            try:
                sc = _default_scope(scope, svc)
                items = svc.list_knowledge_items(sc, kb_id)
                total = len(items)
                off = max(0, min(offset, total))
                end = total if limit <= 0 else min(total, off + limit)
                page = items[off:end]
                return {
                    "items": [
                        {
                            "id": i.id, "kb_id": i.kb_id, "scope": sc,
                            "type": i.type, "title": i.title, "summary": i.summary,
                            "tags": i.tags,
                            "updated_at": i.updated_at.isoformat() if i.updated_at else "",
                        }
                        for i in page
                    ],
                    "total": total, "offset": off, "limit": limit,
                }
            except Exception as e:
                return {"error": str(e)}

        @mcp.tool()
        def add_knowledge_item(
            kb_id: str, title: str, content: str,
            scope: str = "", tags: list[str] | None = None,
            metadata: dict | None = None,
        ) -> dict:
            """Add a knowledge item to a knowledge base."""
            if svc is None:
                return {"error": "service is not configured"}
            try:
                sc = _default_scope(scope, svc)
                item, ingestion, index_status = svc.add(AddInput(
                    kb_id=kb_id, scope=sc, title=title, content=content,
                    tags=tags or [], metadata=metadata or {},
                ))
                return {
                    "item": {"id": item.id, "kb_id": item.kb_id, "title": item.title},
                    "ingestion_result": {"success": ingestion.success, "item_id": ingestion.item_id},
                    "index_status": {"state": index_status.state},
                }
            except Exception as e:
                return {"error": str(e)}

        @mcp.tool()
        def delete_knowledge_item(kb_id: str, item_id: str, scope: str = "") -> dict:
            """Delete a knowledge item from a knowledge base."""
            if svc is None:
                return {"error": "service is not configured"}
            try:
                sc = _default_scope(scope, svc)
                svc.delete_knowledge_item(sc, kb_id, item_id)
                return {"deleted": True, "scope": sc, "kb_id": kb_id.strip(), "item_id": item_id.strip()}
            except Exception as e:
                return {"error": str(e)}

        @mcp.tool()
        def list_knowledge_bases() -> str:
            """List all configured knowledge bases AND every item id/title/tags."""
            if svc is None:
                return "service is not configured"
            return _fmt_kb_list(svc, svc.list_knowledge_bases())

        @mcp.tool()
        def create_knowledge_base(
            id: str, store_type: str,
            scope: str = "", name: str = "", path: str = "",
            enabled: bool | None = None, semantic_enabled: bool | None = None,
            tags: list[str] | None = None,
        ) -> dict:
            """Create a new knowledge base."""
            if svc is None:
                return {"error": "service is not configured"}
            try:
                sc = _default_scope(scope, svc)
                record = svc.create_knowledge_base(CreateKnowledgeBaseInput(
                    scope=sc, id=id.strip(), name=name.strip(), store_type=store_type.strip(),
                    path=path, enabled=enabled, semantic_enabled=semantic_enabled,
                    tags=tags or [],
                ))
                kb = record.knowledge_base
                return {"knowledge_base": {"id": kb.id, "scope": kb.scope, "store_type": kb.store_type, "enabled": kb.enabled}}
            except Exception as e:
                return {"error": str(e)}

        @mcp.tool()
        def delete_knowledge_base(id: str, scope: str = "") -> dict:
            """Delete a runtime-managed knowledge base."""
            if svc is None:
                return {"error": "service is not configured"}
            try:
                sc = _default_scope(scope, svc)
                kb_id = id.strip()
                if not kb_id:
                    return {"error": "knowledge base id is required"}
                svc.delete_knowledge_base(sc, kb_id)
                return {"deleted": True, "scope": sc, "id": kb_id}
            except Exception as e:
                return {"error": str(e)}

        @mcp.tool()
        def index_knowledge(scope: str = "", kb_id: str = "", rebuild: bool = False) -> dict:
            """Backfill or rebuild semantic indexes for one knowledge base or all enabled knowledge bases."""
            if svc is None:
                return {"error": "service is not configured"}
            try:
                sc = scope.strip()
                kid = kb_id.strip()
                if sc:
                    sc = normalize_scope(sc)
                elif kid:
                    sc = _default_scope("", svc)
                result = svc.index_knowledge(IndexKnowledgeInput(scope=sc, kb_id=kid, rebuild=rebuild))
                return {
                    "results": [
                        {"kb_id": r.kb_id, "scope": r.scope, "store_type": r.store_type,
                         "indexed": r.result.indexed, "deleted": r.result.deleted, "skipped": r.result.skipped}
                        for r in result.results
                    ],
                    "warnings": result.warnings,
                }
            except Exception as e:
                return {"error": str(e)}

        return mcp

    def serve_stdio(self) -> None:
        if self.svc and self.svc.has_project_scope():
            print(f"knowledger: project mode (root={self.svc.project_root()})", file=sys.stderr)
        else:
            print("knowledger: global mode", file=sys.stderr)
        self._mcp.run(transport="stdio")
