from __future__ import annotations

import re
import threading
import unicodedata
from dataclasses import dataclass, field
from typing import Any, Callable

from knowledger.core import (
    AddInput, ErrorKind, IndexMaintainer, IndexOptions, IndexResult,
    IngestionResult, IndexStatus, KnowledgeBase, KnowledgeItem,
    KnowledgerError, SearchHit, SearchOptions, ScopedKBRef, StoreBackend,
    SCOPE_GLOBAL, SCOPE_PROJECT, normalize_scope,
)
from knowledger.registry import KnowledgeBaseRecord, Registry, RuntimeKnowledgeBase, Source

_KB_ID_PATTERN = re.compile(r'^[A-Za-z0-9_.\-]+$')
_SNIPPET_CONTEXT = 120
_FALLBACK_SNIPPET = 240


@dataclass
class SearchResult:
    hits: list[SearchHit] = field(default_factory=list)
    warnings: list[str] = field(default_factory=list)


@dataclass
class KnowledgeBaseSummary:
    record: KnowledgeBaseRecord
    item_count: int = 0


@dataclass
class IndexKnowledgeInput:
    scope: str = ""
    kb_id: str = ""
    rebuild: bool = False
    progress: Any = None


@dataclass
class KnowledgeBaseIndexResult:
    kb_id: str
    scope: str
    store_type: str
    result: IndexResult


@dataclass
class IndexKnowledgeResult:
    results: list[KnowledgeBaseIndexResult] = field(default_factory=list)
    warnings: list[str] = field(default_factory=list)


@dataclass
class CreateKnowledgeBaseInput:
    scope: str = ""
    id: str = ""
    name: str = ""
    store_type: str = ""
    path: str = ""
    enabled: bool | None = None
    semantic_enabled: bool | None = None
    tags: list[str] = field(default_factory=list)


BackendBuilder = Callable[[list[KnowledgeBase]], dict[str, StoreBackend]]


class Service:
    def __init__(
        self,
        kbs: list[KnowledgeBase] | None = None,
        backends: dict[str, StoreBackend] | None = None,
        registry: Registry | None = None,
        build_backends: BackendBuilder | None = None,
    ):
        self._lock = threading.RLock()
        self._refresh_lock = threading.Lock()
        self._kbs: list[KnowledgeBase] = list(kbs or [])
        self._backends: dict[str, StoreBackend] = dict(backends or {})
        self._registry = registry
        self._build_backends = build_backends
        self._last_signature = ""

        for kb in self._kbs:
            if not kb.scope:
                kb.scope = SCOPE_GLOBAL

    @classmethod
    def new_managed(cls, registry: Registry, build_backends: BackendBuilder) -> "Service":
        if registry is None:
            raise ValueError("registry is required")
        if build_backends is None:
            raise ValueError("backend builder is required")
        svc = cls(registry=registry, build_backends=build_backends)
        svc.reload()
        return svc

    def search(self, opt: SearchOptions) -> SearchResult:
        self._refresh_if_changed_silently()
        kbs, backends = self._snapshot()
        result = SearchResult()
        for kb in kbs:
            if not kb.enabled or not _matches_kb_filter(kb.scope, kb.id, opt.kb_ids):
                continue
            backend = backends.get(kb.store_type)
            if backend is None:
                raise KnowledgerError(ErrorKind.CONFIG, f"backend not registered for store type {kb.store_type}")
            effective_opt, warning = _search_options_for_kb(opt, kb, backend)
            if warning:
                result.warnings.append(warning)
            try:
                kb_hits = backend.search(kb, effective_opt)
            except Exception as e:
                if effective_opt.search_mode in ("semantic", "hybrid") and backend.supports_semantic(kb):
                    fallback = SearchOptions(
                        query=effective_opt.query, limit=effective_opt.limit,
                        kb_ids=effective_opt.kb_ids, search_mode="lexical",
                    )
                    kb_hits = backend.search(kb, fallback)
                    result.warnings.append(f"{kb.id}: semantic path unavailable, lexical fallback used")
                else:
                    raise
            result.hits.extend(_stamp_scope(kb.scope, kb_hits))
        result.hits.sort(key=lambda h: h.score, reverse=True)
        if opt.limit > 0:
            result.hits = result.hits[:opt.limit]
        return self._with_search_snippets(opt.query, result, backends)

    def add(self, inp: AddInput) -> tuple[KnowledgeItem, IngestionResult, IndexStatus]:
        self._refresh_if_changed_silently()
        kb, backend = self._backend_for(inp.scope, inp.kb_id)
        return backend.add(kb, inp)

    def index_knowledge(self, inp: IndexKnowledgeInput) -> IndexKnowledgeResult:
        self._refresh_if_changed_silently()
        kbs, backends = self._snapshot()
        kb_id = inp.kb_id.strip()
        scope_filter = inp.scope.strip()
        if scope_filter:
            scope_filter = normalize_scope(scope_filter)
        result = IndexKnowledgeResult()
        matched = False
        for kb in kbs:
            if scope_filter and kb.scope != scope_filter:
                continue
            if kb_id and kb.id != kb_id:
                continue
            matched = True
            if not kb_id and not kb.enabled:
                continue
            backend = backends.get(kb.store_type)
            if backend is None:
                raise KnowledgerError(ErrorKind.CONFIG, f"backend not registered for store type {kb.store_type}")
            if not isinstance(backend, IndexMaintainer):
                warn = f"{kb.id}: index maintenance is not supported for {kb.store_type} backend"
                ir = IndexResult(skipped=1, warnings=[warn])
                result.results.append(KnowledgeBaseIndexResult(kb.id, kb.scope, kb.store_type, ir))
                result.warnings.append(warn)
                continue
            index_result = backend.maintain_index(kb, IndexOptions(rebuild=inp.rebuild, progress=inp.progress))
            result.results.append(KnowledgeBaseIndexResult(kb.id, kb.scope, kb.store_type, index_result))
            result.warnings.extend(index_result.warnings)
        if kb_id and not matched:
            raise KnowledgerError(ErrorKind.CONFIG, "knowledge base not found")
        return result

    def list_knowledge_bases(self) -> list[KnowledgeBase]:
        self._refresh_if_changed_silently()
        with self._lock:
            return list(self._kbs)

    def list_knowledge_base_records(self) -> list[KnowledgeBaseRecord]:
        self._refresh_if_changed_silently()
        if self._registry is None:
            return [KnowledgeBaseRecord(kb, Source.STATIC, False) for kb in self.list_knowledge_bases()]
        return self._registry.list_with_sources()

    def list_knowledge_base_summaries(self) -> list[KnowledgeBaseSummary]:
        self._refresh_if_changed_silently()
        records = self.list_knowledge_base_records()
        summaries = []
        for record in records:
            try:
                items = self._list_items_for_kb(record.knowledge_base)
                count = len(items)
            except Exception:
                count = 0
            summaries.append(KnowledgeBaseSummary(record, count))
        return summaries

    def list_knowledge_items(self, scope: str, kb_id: str) -> list[KnowledgeItem]:
        self._refresh_if_changed_silently()
        kb_id = kb_id.strip()
        if not kb_id:
            raise KnowledgerError(ErrorKind.CONFIG, "knowledge base id is required")
        kb, backend = self._backend_for(scope, kb_id)
        return backend.list_items(kb)

    def get_knowledge_item(self, scope: str, kb_id: str, item_id: str) -> KnowledgeItem:
        self._refresh_if_changed_silently()
        kb_id, item_id = kb_id.strip(), item_id.strip()
        if not kb_id:
            raise KnowledgerError(ErrorKind.CONFIG, "knowledge base id is required")
        if not item_id:
            raise KnowledgerError(ErrorKind.CONFIG, "knowledge item id is required")
        kb, backend = self._backend_for(scope, kb_id)
        return backend.get_item(kb, item_id)

    def delete_knowledge_item(self, scope: str, kb_id: str, item_id: str) -> None:
        self._refresh_if_changed_silently()
        kb_id, item_id = kb_id.strip(), item_id.strip()
        if not kb_id:
            raise KnowledgerError(ErrorKind.CONFIG, "knowledge base id is required")
        if not item_id:
            raise KnowledgerError(ErrorKind.CONFIG, "knowledge item id is required")
        kb, backend = self._backend_for(scope, kb_id)
        backend.delete_item(kb, item_id)

    def create_knowledge_base(self, inp: CreateKnowledgeBaseInput) -> KnowledgeBaseRecord:
        if self._registry is None or self._build_backends is None:
            raise RuntimeError("runtime registry is not available")
        self._refresh_if_changed_silently()
        scope = normalize_scope(inp.scope)
        if scope == SCOPE_PROJECT and not self._registry.has_project_store():
            raise KnowledgerError(ErrorKind.CONFIG, "not in a project directory; cannot create scope=project knowledge base")
        runtime_kb = _normalize_create_input(inp)
        existing = self._registry.list_with_sources()
        for rec in existing:
            if rec.knowledge_base.id == runtime_kb.id and rec.knowledge_base.scope == scope:
                raise ValueError(f"knowledge base {runtime_kb.id!r} already exists")
        self._registry.create(scope, runtime_kb)
        self.reload()
        for record in self._registry.list_with_sources():
            if record.knowledge_base.id == runtime_kb.id and record.knowledge_base.scope == scope:
                return record
        raise ValueError(f"knowledge base {runtime_kb.id!r} not found after create")

    def delete_knowledge_base(self, scope: str, kb_id: str) -> None:
        if self._registry is None or self._build_backends is None:
            raise RuntimeError("runtime registry is not available")
        self._registry.delete(scope, kb_id)
        self.reload()

    def has_project_scope(self) -> bool:
        return self._registry is not None and self._registry.has_project_store()

    def project_root(self) -> str:
        return self._registry.project_root if self._registry else ""

    def reload(self) -> None:
        if self._registry is None or self._build_backends is None:
            return
        kbs = self._registry.list()
        for kb in kbs:
            if not kb.scope:
                kb.scope = SCOPE_GLOBAL
        backends = self._build_backends(kbs)
        try:
            sig = self._registry.signature()
        except Exception:
            sig = None
        with self._lock:
            self._kbs = list(kbs)
            self._backends = dict(backends)
            if sig is not None:
                self._last_signature = sig

    def refresh_if_changed(self) -> None:
        if self._registry is None or self._build_backends is None:
            return
        try:
            sig = self._registry.signature()
        except Exception:
            return
        with self._lock:
            current = self._last_signature
        if sig == current:
            return
        with self._refresh_lock:
            with self._lock:
                current = self._last_signature
            if sig == current:
                return
            self.reload()

    def _refresh_if_changed_silently(self) -> None:
        try:
            self.refresh_if_changed()
        except Exception:
            pass

    def close(self) -> None:
        _, backends = self._snapshot()
        for backend in backends.values():
            if hasattr(backend, "close"):
                try:
                    backend.close()
                except Exception:
                    pass

    def _snapshot(self) -> tuple[list[KnowledgeBase], dict[str, StoreBackend]]:
        with self._lock:
            return list(self._kbs), dict(self._backends)

    def _list_items_for_kb(self, kb: KnowledgeBase) -> list[KnowledgeItem]:
        _, backend = self._backend_for(kb.scope, kb.id)
        return backend.list_items(kb)

    def _backend_for(self, scope: str, kb_id: str) -> tuple[KnowledgeBase, StoreBackend]:
        scope = normalize_scope(scope)
        kbs, backends = self._snapshot()
        for kb in kbs:
            if kb.id != kb_id or kb.scope != scope:
                continue
            backend = backends.get(kb.store_type)
            if backend is None:
                raise KnowledgerError(ErrorKind.CONFIG, f"backend not registered for store type {kb.store_type}")
            return kb, backend
        raise KnowledgerError(ErrorKind.CONFIG, "knowledge base not found")

    def _with_search_snippets(self, query: str, result: SearchResult, backends: dict[str, StoreBackend]) -> SearchResult:
        kbs, _ = self._snapshot()
        kb_map = {(kb.scope, kb.id): kb for kb in kbs}
        for hit in result.hits:
            kb = kb_map.get((hit.scope, hit.kb_id))
            if kb is None:
                _set_fallback_snippet(hit)
                continue
            backend = backends.get(kb.store_type)
            if backend is None:
                _set_fallback_snippet(hit)
                continue
            try:
                item = backend.get_item(kb, hit.item_id)
                snippet = _snippet_around_query(item.content, query)
                hit.snippet = snippet
                hit.content_preview = snippet
            except Exception:
                _set_fallback_snippet(hit)
        return result


def _normalize_create_input(inp: CreateKnowledgeBaseInput) -> RuntimeKnowledgeBase:
    import os
    from knowledger.config import expand_home_path, apply_kb_defaults, KnowledgeBaseConfig, DEFAULT_SEARCH_MODE

    if not inp.id:
        raise ValueError("knowledge base id is required")
    if len(inp.id) > 64 or not _KB_ID_PATTERN.match(inp.id):
        raise ValueError("knowledge base id may contain only letters, digits, underscore, dash, and dot")
    if inp.store_type not in ("text", "sqlite"):
        raise ValueError(f"unsupported knowledge base store type {inp.store_type!r}")
    scope = normalize_scope(inp.scope)
    path = inp.path.strip()
    enabled = inp.enabled if inp.enabled is not None else True
    store_config: dict[str, Any] = {}
    if path:
        path = expand_home_path(path)
        if inp.store_type == "text" and enabled and (scope == SCOPE_GLOBAL or os.path.isabs(path)):
            info = os.stat(path)
            if not os.path.isdir(path):
                raise ValueError(f"text knowledge base path {path!r} is not a directory")
        store_config["path"] = path
    elif scope == SCOPE_GLOBAL:
        raise ValueError("knowledge base path is required")
    name = inp.name or inp.id
    cfg = KnowledgeBaseConfig(
        id=inp.id, name=name, store_type=inp.store_type,
        store_config=store_config, enabled=enabled,
        default_search_mode=DEFAULT_SEARCH_MODE, tags=list(inp.tags),
    )
    if scope == SCOPE_GLOBAL:
        apply_kb_defaults(cfg)
    if inp.semantic_enabled is not None and inp.store_type in ("sqlite", "text"):
        if cfg.indexing is None:
            cfg.indexing = {}
        semantic = cfg.indexing.get("semantic")
        if not isinstance(semantic, dict):
            semantic = {"provider": "chroma"}
            cfg.indexing["semantic"] = semantic
        semantic["enabled"] = inp.semantic_enabled
        if scope == SCOPE_GLOBAL and inp.store_type == "text" and inp.semantic_enabled:
            apply_kb_defaults(cfg)
    return RuntimeKnowledgeBase(
        id=cfg.id, name=cfg.name, store_type=cfg.store_type,
        store_config=cfg.store_config, enabled=cfg.enabled,
        default_search_mode=cfg.default_search_mode,
        indexing=cfg.indexing, tags=cfg.tags,
    )


def _search_options_for_kb(opt: SearchOptions, kb: KnowledgeBase, backend: StoreBackend) -> tuple[SearchOptions, str]:
    from dataclasses import replace
    requested = opt.search_mode
    if not requested or requested == "auto":
        requested = kb.default_search_mode
    if not requested or requested == "auto":
        requested = "lexical"
    effective = SearchOptions(query=opt.query, limit=opt.limit, kb_ids=opt.kb_ids, search_mode=requested)
    if requested in ("semantic", "hybrid") and not backend.supports_semantic(kb):
        effective.search_mode = "lexical"
        return effective, f"{kb.id}: {requested} search is not implemented for {kb.store_type} backend yet; lexical results returned"
    return effective, ""


def _matches_kb_filter(scope: str, kb_id: str, refs: list[ScopedKBRef]) -> bool:
    if not refs:
        return True
    return any(r.id == kb_id and (not r.scope or r.scope == scope) for r in refs)


def _stamp_scope(scope: str, hits: list[SearchHit]) -> list[SearchHit]:
    for h in hits:
        h.scope = scope
    return hits


def _set_fallback_snippet(hit: SearchHit) -> None:
    text = hit.content_preview or hit.snippet
    hit.snippet = _truncate_runes(text, _FALLBACK_SNIPPET)
    hit.content_preview = hit.snippet


def _snippet_around_query(content: str, query: str) -> str:
    terms = [t for t in _query_terms(query) if t]
    if not terms:
        return _truncate_runes(content, _FALLBACK_SNIPPET)
    first = terms[0]
    lower_content = content.lower()
    lower_term = first.lower()
    idx = lower_content.find(lower_term)
    if idx >= 0:
        runes = list(content)
        start = max(0, idx - _SNIPPET_CONTEXT)
        end = min(len(runes), idx + len(first) + _SNIPPET_CONTEXT)
        snippet = "".join(runes[start:end])
        if start > 0:
            snippet = "…" + snippet
        if end < len(runes):
            snippet += "…"
        return snippet
    return _truncate_runes(content, _FALLBACK_SNIPPET)


def _truncate_runes(content: str, limit: int) -> str:
    runes = list(content)
    if len(runes) <= limit:
        return content
    return "".join(runes[:limit]) + "…"


def _query_terms(query: str) -> list[str]:
    def is_sep(c: str) -> bool:
        cat = unicodedata.category(c)
        return cat.startswith("Z") or cat.startswith("P") or cat.startswith("S")
    parts = []
    current: list[str] = []
    for ch in query:
        if is_sep(ch):
            if current:
                parts.append("".join(current))
                current = []
        else:
            current.append(ch)
    if current:
        parts.append("".join(current))
    return parts
