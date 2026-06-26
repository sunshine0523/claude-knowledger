import json
import os
import sqlite3
from datetime import datetime, timezone

from knowledger.core.backend import AddInput, IndexOptions, IndexResult
from knowledger.core.errors import ErrorKind, KnowledgerError
from knowledger.core.types import (
    IngestionResult, IndexStatus, KnowledgeBase, KnowledgeItem,
)

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
        self.conn.commit()
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

    def close(self) -> None:
        self.conn.close()


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
    def get_item(self, kb, item_id): return self._backend(kb).get_item(kb, item_id)
    def list_items(self, kb): return self._backend(kb).list_items(kb)
    def delete_item(self, kb, item_id): return self._backend(kb).delete_item(kb, item_id)
    def maintain_index(self, kb, opt): return self._backend(kb).maintain_index(kb, opt)

    def close(self) -> None:
        for b in self._backends.values():
            b.close()
