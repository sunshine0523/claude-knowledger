import json
import os
import sqlite3
from datetime import datetime, timezone

from knowledger.core.backend import AddInput, IndexOptions, IndexResult, SearchOptions
from knowledger.core.errors import ErrorKind, KnowledgerError
from knowledger.core.types import (
    IngestionResult, IndexStatus, KnowledgeBase, KnowledgeItem, SearchHit,
)
from knowledger.core.utils import tokenize_query

SCHEMA_SQL = """
CREATE TABLE IF NOT EXISTS knowledge_items (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    kb_id TEXT NOT NULL,
    title TEXT NOT NULL,
    content TEXT NOT NULL,
    tags TEXT NOT NULL DEFAULT '',
    metadata_json TEXT NOT NULL DEFAULT '{}',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
)
"""

FTS5_SQL = """
CREATE VIRTUAL TABLE IF NOT EXISTS knowledge_items_fts USING fts5(
    title,
    content,
    content='knowledge_items',
    content_rowid='id'
);
CREATE TRIGGER IF NOT EXISTS knowledge_items_ai AFTER INSERT ON knowledge_items BEGIN
    INSERT INTO knowledge_items_fts(rowid, title, content) VALUES (new.id, new.title, new.content);
END;
CREATE TRIGGER IF NOT EXISTS knowledge_items_ad AFTER DELETE ON knowledge_items BEGIN
    INSERT INTO knowledge_items_fts(knowledge_items_fts, rowid, title, content) VALUES('delete', old.id, old.title, old.content);
END;
"""


def _now_iso() -> str:
    return datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")


def _parse_dt(s: str) -> datetime:
    return datetime.fromisoformat(s.replace("Z", "+00:00"))


def _split_tags(tags_str: str) -> list[str]:
    if not tags_str:
        return []
    return [t.strip() for t in tags_str.split(",") if t.strip()]


def _scan_item(row: tuple, kb_id: str) -> KnowledgeItem:
    rid, title, content, tags, metadata_json, created_at_raw, updated_at_raw = row
    metadata = json.loads(metadata_json) if metadata_json else {}
    return KnowledgeItem(
        id=str(rid),
        kb_id=kb_id,
        type="note",
        title=title,
        content=content,
        metadata=metadata,
        tags=_split_tags(tags),
        created_at=_parse_dt(created_at_raw),
        updated_at=_parse_dt(updated_at_raw),
    )


def _scan_hit(row: tuple, kb_id: str) -> SearchHit:
    rid, title, content = row
    return SearchHit(
        item_id=str(rid),
        kb_id=kb_id,
        item_type="note",
        title=title,
        snippet=content,
        content_preview=content,
        score=1.0,
        match_mode="lexical",
        source_backend="sqlite",
    )


class Backend:
    def __init__(self, path: str, indexer=None):
        if not path:
            raise KnowledgerError(ErrorKind.CONFIG, "sqlite path is required")
        if path not in (":memory:",) and not path.startswith("file:"):
            dir_ = os.path.dirname(path)
            if dir_:
                os.makedirs(dir_, exist_ok=True)
        self.conn = sqlite3.connect(path, check_same_thread=False)
        self.conn.execute(SCHEMA_SQL)
        self.fts_enabled = True
        try:
            for stmt in FTS5_SQL.strip().split(";"):
                stmt = stmt.strip()
                if stmt:
                    self.conn.execute(stmt)
            self.conn.commit()
        except sqlite3.OperationalError:
            self.fts_enabled = False
        self.indexer = indexer

    def add(self, kb: KnowledgeBase, inp: AddInput) -> tuple[KnowledgeItem, IngestionResult, IndexStatus]:
        now = _now_iso()
        metadata_json = json.dumps(inp.metadata or {})
        tags_str = ",".join(inp.tags) if inp.tags else ""
        cur = self.conn.execute(
            "INSERT INTO knowledge_items(kb_id, title, content, tags, metadata_json, created_at, updated_at)"
            " VALUES (?, ?, ?, ?, ?, ?, ?)",
            (kb.id, inp.title, inp.content, tags_str, metadata_json, now, now),
        )
        self.conn.commit()
        item = KnowledgeItem(
            id=str(cur.lastrowid),
            kb_id=kb.id,
            type="note",
            title=inp.title,
            content=inp.content,
            metadata=inp.metadata or {},
            tags=inp.tags or [],
            created_at=_parse_dt(now),
            updated_at=_parse_dt(now),
        )
        if self.indexer and self.indexer.supports_kb(kb):
            self.indexer.upsert_item(kb, item, {"mtime": item.updated_at.timestamp()})
            return item, IngestionResult(success=True, item_id=item.id), IndexStatus(state="indexed")
        return item, IngestionResult(success=True, item_id=item.id), IndexStatus(state="not_indexed")

    def search(self, kb: KnowledgeBase, opt: SearchOptions) -> list[SearchHit]:
        limit = opt.limit if opt.limit > 0 else 10
        mode = opt.search_mode
        if mode == "semantic":
            if self.indexer and self.indexer.supports_kb(kb):
                hits = self.indexer.search(kb, opt.query, limit, "semantic")
                for h in hits:
                    h.item_type = "note"
                return hits
            return self._search_lexical(kb, opt.query, limit)
        if mode == "hybrid":
            if self.indexer and self.indexer.supports_kb(kb):
                sem = self.indexer.search(kb, opt.query, limit, "hybrid")
                for h in sem:
                    h.item_type = "note"
                lex = self._search_lexical(kb, opt.query, limit)
                return _merge_hits(sem, lex, limit)
            return self._search_lexical(kb, opt.query, limit)
        return self._search_lexical(kb, opt.query, limit)

    def _search_lexical(self, kb: KnowledgeBase, query: str, limit: int) -> list[SearchHit]:
        if self.fts_enabled:
            hits = self._search_fts(kb, query, limit)
            if hits:
                return hits
        return self._search_like(kb, query, limit)

    def _search_fts(self, kb: KnowledgeBase, query: str, limit: int) -> list[SearchHit]:
        tokens = tokenize_query(query)
        if not tokens:
            return []
        escaped = ['"' + t.replace('"', '""') + '"' for t in tokens]
        match_expr = " OR ".join(escaped)
        try:
            rows = self.conn.execute(
                "SELECT k.id, k.title, k.content"
                " FROM knowledge_items_fts f"
                " JOIN knowledge_items k ON k.id = f.rowid"
                " WHERE knowledge_items_fts MATCH ? AND k.kb_id = ?"
                " LIMIT ?",
                (match_expr, kb.id, limit),
            ).fetchall()
        except sqlite3.OperationalError:
            return []
        return [_scan_hit(r, kb.id) for r in rows]

    def _search_like(self, kb: KnowledgeBase, query: str, limit: int) -> list[SearchHit]:
        tokens = tokenize_query(query)
        if not tokens:
            return []
        clauses = ["(title LIKE ? OR content LIKE ?)"] * len(tokens)
        args: list = [kb.id]
        for tok in tokens:
            pattern = f"%{tok}%"
            args += [pattern, pattern]
        args.append(limit)
        stmt = (
            "SELECT id, title, content FROM knowledge_items"
            " WHERE kb_id = ? AND (" + " OR ".join(clauses) + ")"
            " ORDER BY id DESC LIMIT ?"
        )
        rows = self.conn.execute(stmt, args).fetchall()
        return [_scan_hit(r, kb.id) for r in rows]

    def get_item(self, kb: KnowledgeBase, item_id: str) -> KnowledgeItem:
        row = self.conn.execute(
            "SELECT id, title, content, tags, metadata_json, created_at, updated_at"
            " FROM knowledge_items WHERE kb_id = ? AND id = ?",
            (kb.id, item_id),
        ).fetchone()
        if row is None:
            raise KnowledgerError(ErrorKind.STORE, "knowledge item not found")
        return _scan_item(row, kb.id)

    def list_items(self, kb: KnowledgeBase) -> list[KnowledgeItem]:
        rows = self.conn.execute(
            "SELECT id, title, content, tags, metadata_json, created_at, updated_at"
            " FROM knowledge_items WHERE kb_id = ? ORDER BY id DESC",
            (kb.id,),
        ).fetchall()
        return [_scan_item(r, kb.id) for r in rows]

    def delete_item(self, kb: KnowledgeBase, item_id: str) -> None:
        cur = self.conn.execute(
            "DELETE FROM knowledge_items WHERE kb_id = ? AND id = ?",
            (kb.id, item_id),
        )
        self.conn.commit()
        if cur.rowcount == 0:
            raise KnowledgerError(ErrorKind.STORE, "knowledge item not found")
        if self.indexer and self.indexer.supports_kb(kb):
            self.indexer.delete_item(kb, item_id)

    def maintain_index(self, kb: KnowledgeBase, opt: IndexOptions) -> IndexResult:
        if self.indexer and self.indexer.supports_kb(kb):
            return self.indexer.maintain_index(
                kb, opt,
                lambda: self.list_items(kb),
                lambda item: {"mtime": item.updated_at.timestamp()},
            )
        return IndexResult(skipped=1, warnings=[f"{kb.id}: semantic indexing not enabled"])

    def supports_semantic(self, kb: KnowledgeBase) -> bool:
        return self.indexer is not None and self.indexer.supports_kb(kb)

    def close(self) -> None:
        self.conn.close()


def _merge_hits(primary: list[SearchHit], secondary: list[SearchHit], limit: int) -> list[SearchHit]:
    seen = {h.item_id for h in primary}
    merged = list(primary)
    for h in secondary:
        if h.item_id not in seen:
            merged.append(h)
            seen.add(h.item_id)
    return merged[:limit] if limit > 0 else merged


class MultiBackend:
    def __init__(self, kbs: list[KnowledgeBase], indexer=None):
        self._backends: dict[str, Backend] = {}
        for kb in kbs:
            if kb.store_type != "sqlite":
                continue
            path = kb.store_config.get("path", "")
            if not path:
                raise KnowledgerError(ErrorKind.CONFIG, f"knowledge base {kb.id!r} sqlite store_config.path is required")
            if path not in self._backends:
                self._backends[path] = Backend(path, indexer)

    def _backend(self, kb: KnowledgeBase) -> Backend:
        path = kb.store_config.get("path", "")
        if not path:
            raise KnowledgerError(ErrorKind.CONFIG, f"knowledge base {kb.id!r} sqlite store_config.path is required")
        b = self._backends.get(path)
        if b is None:
            raise KnowledgerError(ErrorKind.CONFIG, f"sqlite backend not registered for {kb.id!r} path {path!r}")
        return b

    def add(self, kb, inp): return self._backend(kb).add(kb, inp)
    def search(self, kb, opt): return self._backend(kb).search(kb, opt)
    def get_item(self, kb, item_id): return self._backend(kb).get_item(kb, item_id)
    def list_items(self, kb): return self._backend(kb).list_items(kb)
    def delete_item(self, kb, item_id): return self._backend(kb).delete_item(kb, item_id)
    def maintain_index(self, kb, opt): return self._backend(kb).maintain_index(kb, opt)
    def supports_semantic(self, kb): return self._backend(kb).supports_semantic(kb)

    def close(self) -> None:
        for b in self._backends.values():
            b.close()
