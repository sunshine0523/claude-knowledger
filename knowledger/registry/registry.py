from __future__ import annotations

import hashlib
import os
from dataclasses import dataclass
from enum import StrEnum
from typing import Any

from knowledger.core.types import KnowledgeBase
from knowledger.core.scope import normalize_scope, SCOPE_GLOBAL, SCOPE_PROJECT
from knowledger.config.config import KnowledgeBaseConfig, expand_home_path
from .store import Store, RuntimeKnowledgeBase


class Source(StrEnum):
    STATIC = "static"
    RUNTIME = "runtime"


@dataclass
class KnowledgeBaseRecord:
    knowledge_base: KnowledgeBase
    source: Source
    deletable: bool


class Registry:
    def __init__(
        self,
        static: list[KnowledgeBaseConfig],
        global_store: Store,
        project_store: Store | None,
        project_root: str,
    ) -> None:
        self._static = static
        self._global_store = global_store
        self._project_store = project_store
        self._project_root = project_root

    def has_project_store(self) -> bool:
        return self._project_store is not None

    def project_root(self) -> str:
        return self._project_root

    def signature(self) -> str:
        global_ver = self._global_store.version()
        project_ver = ""
        if self._project_store is not None:
            project_ver = self._project_store.version()
        return f"g={global_ver}|p={project_ver}"

    def list(self) -> list[KnowledgeBase]:
        return [r.knowledge_base for r in self.list_with_sources()]

    def list_with_sources(self) -> list[KnowledgeBaseRecord]:
        merged: dict[tuple[str, str], KnowledgeBaseRecord] = {}

        for item in self._static:
            key = (SCOPE_GLOBAL, item.id)
            merged[key] = KnowledgeBaseRecord(
                knowledge_base=_static_to_core(item),
                source=Source.STATIC,
                deletable=False,
            )

        for item in self._global_store.list():
            key = (SCOPE_GLOBAL, item.id)
            merged[key] = KnowledgeBaseRecord(
                knowledge_base=_runtime_to_core(item, SCOPE_GLOBAL),
                source=Source.RUNTIME,
                deletable=True,
            )

        if self._project_store is not None:
            for item in self._project_store.list():
                resolved = _resolve_project_paths(item, self._project_root)
                key = (SCOPE_PROJECT, resolved.id)
                merged[key] = KnowledgeBaseRecord(
                    knowledge_base=_runtime_to_core(resolved, SCOPE_PROJECT),
                    source=Source.RUNTIME,
                    deletable=True,
                )

        def sort_key(k: tuple[str, str]) -> tuple[int, str]:
            scope, kb_id = k
            return (0 if scope == SCOPE_PROJECT else 1, kb_id)

        keys = sorted(merged.keys(), key=sort_key)
        return [merged[k] for k in keys]

    def store_for_scope(self, scope: str) -> Store:
        scope = normalize_scope(scope)
        if scope == SCOPE_GLOBAL:
            return self._global_store
        if scope == SCOPE_PROJECT:
            if self._project_store is None:
                raise ValueError("not in a project directory; cannot operate on scope=project")
            return self._project_store
        raise ValueError(f"unknown scope {scope!r}")

    def add(self, item: RuntimeKnowledgeBase, scope: str) -> None:
        scope = normalize_scope(scope)
        if not item.id:
            raise ValueError("knowledge base id is required")
        store = self.store_for_scope(scope)

        if scope == SCOPE_GLOBAL:
            for s in self._static:
                if s.id == item.id:
                    raise ValueError(f"knowledge base {item.id!r} already exists")

        existing = store.list()
        for e in existing:
            if e.id == item.id:
                raise ValueError(f"knowledge base {item.id!r} already exists")

        if scope == SCOPE_PROJECT:
            _apply_project_defaults(item, self._project_root)

        existing.append(item)
        store.save(existing)

    def delete(self, scope: str, kb_id: str) -> None:
        scope = normalize_scope(scope)
        store = self.store_for_scope(scope)
        items = store.list()

        for i, item in enumerate(items):
            if item.id == kb_id:
                items.pop(i)
                store.save(items)
                return

        if scope == SCOPE_GLOBAL:
            for s in self._static:
                if s.id == kb_id:
                    raise ValueError(f"knowledge base {kb_id!r} is defined in static config")

        raise ValueError(f"knowledge base {kb_id!r} not found in {scope} runtime registry")


def _static_to_core(item: KnowledgeBaseConfig) -> KnowledgeBase:
    return KnowledgeBase(
        id=item.id,
        scope=SCOPE_GLOBAL,
        name=item.name,
        store_type=item.store_type,
        store_config=item.store_config,
        enabled=item.enabled,
        indexing=item.indexing,
        tags=item.tags,
    )


def _runtime_to_core(item: RuntimeKnowledgeBase, scope: str) -> KnowledgeBase:
    return KnowledgeBase(
        id=item.id,
        scope=scope,
        name=item.name,
        store_type=item.store_type,
        store_config=item.store_config,
        enabled=item.enabled,
        indexing=item.indexing,
        tags=item.tags,
    )


def _project_hash(project_root: str) -> str:
    cleaned = os.path.normpath(project_root)
    digest = hashlib.sha256(cleaned.encode()).hexdigest()
    return digest[:8]


def _apply_project_defaults(item: RuntimeKnowledgeBase, project_root: str) -> None:
    if item.store_config is None:
        item.store_config = {}

    raw_path = item.store_config.get("path", "")
    if not isinstance(raw_path, str) or not raw_path.strip():
        if item.store_type == "sqlite":
            item.store_config["path"] = ".knowledger/db"
        elif item.store_type == "text":
            item.store_config["path"] = os.path.join(".knowledger", "data", item.id)

    if item.store_type != "sqlite":
        return

    if item.indexing is None:
        item.indexing = {}

    if "semantic" not in item.indexing:
        item.indexing["semantic"] = {}

    semantic = item.indexing["semantic"]
    if not isinstance(semantic, dict):
        semantic = {}
        item.indexing["semantic"] = semantic

    if "collection" not in semantic:
        semantic["collection"] = f"proj-{_project_hash(project_root)}-{item.id}"
    if "path" not in semantic:
        coll = semantic.get("collection", "")
        semantic["path"] = os.path.join(".knowledger", "chroma", coll)

    # Apply config defaults for provider/mode/etc, then re-pin relative paths
    from knowledger.config.config import KnowledgeBaseConfig, apply_kb_defaults
    import copy
    cfg = KnowledgeBaseConfig(
        id=item.id,
        store_type=item.store_type,
        store_config=copy.deepcopy(item.store_config),
        indexing=copy.deepcopy(item.indexing),
    )
    apply_kb_defaults(cfg)

    # Re-pin relative paths that the helper may have expanded
    store_path = item.store_config.get("path", "")
    if isinstance(store_path, str) and store_path and not os.path.isabs(store_path):
        cfg.store_config["path"] = store_path

    if isinstance(cfg.indexing.get("semantic"), dict):
        sem_out = cfg.indexing["semantic"]
        orig_sem_path = semantic.get("path", "")
        if isinstance(orig_sem_path, str) and orig_sem_path and not os.path.isabs(orig_sem_path):
            sem_out["path"] = orig_sem_path

    item.store_config = cfg.store_config
    item.indexing = cfg.indexing


def _resolve_project_paths(item: RuntimeKnowledgeBase, project_root: str) -> RuntimeKnowledgeBase:
    import copy
    out = copy.copy(item)
    out.store_config = _clone_map(item.store_config)
    out.indexing = _clone_map(item.indexing)

    if isinstance(out.store_config.get("path"), str):
        out.store_config["path"] = _expand_project_path(out.store_config["path"], project_root)

    semantic = out.indexing.get("semantic")
    if isinstance(semantic, dict):
        sem_copy = _clone_map(semantic)
        if isinstance(sem_copy.get("path"), str):
            sem_copy["path"] = _expand_project_path(sem_copy["path"], project_root)
        out.indexing["semantic"] = sem_copy

    return out


def _expand_project_path(p: str, project_root: str) -> str:
    if not p:
        return p
    p = expand_home_path(p)
    if os.path.isabs(p):
        return p
    return os.path.join(project_root, p)


def _clone_map(m: dict[str, Any] | None) -> dict[str, Any]:
    if m is None:
        return {}
    return dict(m)
