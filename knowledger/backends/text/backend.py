import os
from datetime import datetime, timezone

import yaml

from knowledger.core.backend import AddInput, IndexOptions, IndexResult
from knowledger.core.errors import ErrorKind, KnowledgerError
from knowledger.core.types import (
    IngestionResult, IndexStatus, KnowledgeBase, KnowledgeItem,
)


def _now_iso() -> str:
    return datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")


def _parse_dt(s: str) -> datetime:
    return datetime.fromisoformat(s.replace("Z", "+00:00"))


def _item_id() -> str:
    import time
    return str(int(time.time_ns()))


def _write_file(path: str, item: KnowledgeItem) -> None:
    frontmatter = {
        "id": item.id,
        "kb_id": item.kb_id,
        "title": item.title,
        "tags": item.tags,
        "metadata": item.metadata,
        "created_at": item.created_at.strftime("%Y-%m-%dT%H:%M:%SZ"),
        "updated_at": item.updated_at.strftime("%Y-%m-%dT%H:%M:%SZ"),
    }
    body = f"---\n{yaml.dump(frontmatter, allow_unicode=True, default_flow_style=False)}---\n\n{item.content}\n"
    with open(path, "w", encoding="utf-8") as f:
        f.write(body)


def _read_file(path: str, kb_id: str) -> KnowledgeItem:
    with open(path, encoding="utf-8") as f:
        raw = f.read()
    content = raw
    metadata = {}
    tags = []
    title = os.path.splitext(os.path.basename(path))[0]
    created_at = updated_at = datetime.now(timezone.utc)
    item_id = title

    if raw.startswith("---"):
        end = raw.find("\n---", 3)
        if end != -1:
            fm_text = raw[3:end].strip()
            rest = raw[end + 4:].lstrip("\n")
            try:
                fm = yaml.safe_load(fm_text) or {}
                item_id = str(fm.get("id", item_id))
                title = fm.get("title", title)
                tags = fm.get("tags") or []
                metadata = fm.get("metadata") or {}
                if fm.get("created_at"):
                    created_at = _parse_dt(str(fm["created_at"]))
                if fm.get("updated_at"):
                    updated_at = _parse_dt(str(fm["updated_at"]))
            except yaml.YAMLError:
                pass
            content = rest

    stat = os.stat(path)
    if created_at == updated_at:
        mtime = datetime.fromtimestamp(stat.st_mtime, tz=timezone.utc)
        created_at = mtime
        updated_at = mtime

    return KnowledgeItem(
        id=item_id,
        kb_id=kb_id,
        type="document",
        title=title,
        content=content,
        metadata=metadata,
        tags=tags if isinstance(tags, list) else [tags],
        created_at=created_at,
        updated_at=updated_at,
    )


class Backend:
    def __init__(self, indexer=None):
        self.indexer = indexer

    def _dir(self, kb: KnowledgeBase) -> str:
        path = kb.store_config.get("path", "")
        if not path:
            raise KnowledgerError(ErrorKind.CONFIG, "text backend requires store_config.path")
        return path

    def add(self, kb: KnowledgeBase, inp: AddInput) -> tuple[KnowledgeItem, IngestionResult, IndexStatus]:
        dir_ = self._dir(kb)
        os.makedirs(dir_, exist_ok=True)
        now_str = _now_iso()
        now = _parse_dt(now_str)
        item_id = _item_id()
        item = KnowledgeItem(
            id=item_id,
            kb_id=kb.id,
            type="document",
            title=inp.title,
            content=inp.content,
            metadata=inp.metadata or {},
            tags=inp.tags or [],
            created_at=now,
            updated_at=now,
        )
        file_path = os.path.join(dir_, f"{item_id}.md")
        _write_file(file_path, item)

        if self.indexer and self.indexer.supports_kb(kb):
            stat = os.stat(file_path)
            extra = {"path": f"{item_id}.md", "mtime": int(stat.st_mtime)}
            self.indexer.upsert_item(kb, item, extra)
            return item, IngestionResult(success=True, item_id=item_id), IndexStatus(state="indexed")
        return item, IngestionResult(success=True, item_id=item_id), IndexStatus(state="not_indexed")

    def get_item(self, kb: KnowledgeBase, item_id: str) -> KnowledgeItem:
        dir_ = self._dir(kb)
        fpath = os.path.join(dir_, f"{item_id}.md")
        if not os.path.isfile(fpath):
            raise KnowledgerError(ErrorKind.STORE, "knowledge item not found")
        return _read_file(fpath, kb.id)

    def list_items(self, kb: KnowledgeBase) -> list[KnowledgeItem]:
        dir_ = self._dir(kb)
        if not os.path.isdir(dir_):
            return []
        items = []
        for fname in os.listdir(dir_):
            if not (fname.endswith(".md") or fname.endswith(".txt")):
                continue
            fpath = os.path.join(dir_, fname)
            if not os.path.isfile(fpath):
                continue
            try:
                items.append(_read_file(fpath, kb.id))
            except Exception:
                continue
        return items

    def delete_item(self, kb: KnowledgeBase, item_id: str) -> None:
        dir_ = self._dir(kb)
        fpath = os.path.join(dir_, f"{item_id}.md")
        if not os.path.isfile(fpath):
            raise KnowledgerError(ErrorKind.STORE, "knowledge item not found")
        os.remove(fpath)
        if self.indexer and self.indexer.supports_kb(kb):
            try:
                self.indexer.delete_item(kb, item_id)
            except Exception:
                pass

    def maintain_index(self, kb: KnowledgeBase, opt: IndexOptions) -> IndexResult:
        if not (self.indexer and self.indexer.supports_kb(kb)):
            return IndexResult(skipped=1, warnings=[f"{kb.id}: semantic indexing not enabled"])
        dir_ = kb.store_config.get("path", "")
        return self.indexer.maintain_index(
            kb, opt,
            lambda: self.list_items(kb),
            lambda item: {"path": f"{item.id}.md", "mtime": int(os.stat(os.path.join(dir_, f'{item.id}.md')).st_mtime) if os.path.isfile(os.path.join(dir_, f'{item.id}.md')) else 0},
        )

