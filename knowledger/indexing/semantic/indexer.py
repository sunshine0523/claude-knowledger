import threading
from collections.abc import Callable
from typing import Any

from knowledger.core.backend import IndexOptions, IndexProgressEvent, IndexProgressPhase, IndexResult
from knowledger.core.types import KnowledgeBase, KnowledgeItem
from knowledger.indexing.chunking.splitter import Splitter, default_splitter
from knowledger.indexing.chroma.client import ChromaClient
from knowledger.indexing.semantic.config import semantic_enabled, get_collection_name


class Indexer:
    def __init__(self, splitter: Splitter | None = None):
        self._clients: dict[str, ChromaClient] = {}
        self._lock = threading.Lock()
        self.splitter = splitter or default_splitter()

    def _client_key(self, kb: KnowledgeBase) -> str:
        return f"{kb.scope}/{kb.id}"

    def _get_client(self, kb: KnowledgeBase) -> ChromaClient:
        key = self._client_key(kb)
        with self._lock:
            if key not in self._clients:
                self._clients[key] = ChromaClient(kb)
            return self._clients[key]

    def supports_kb(self, kb: KnowledgeBase) -> bool:
        return semantic_enabled(kb)

    def index_item(self, kb: KnowledgeBase, item: KnowledgeItem) -> None:
        client = self._get_client(kb)
        collection_name = get_collection_name(kb)
        collection = client.get_or_create_collection(collection_name)

        # Remove existing chunks for this item first
        client.delete_by_parent(collection, kb.id, item.id)

        chunks = self.splitter.split(item.content)
        for chunk in chunks:
            chunk_id = f"{item.id}#chunk-{chunk.index}"
            metadata: dict[str, Any] = {
                "kb_id": kb.id,
                "parent_id": item.id,
                "title": item.title,
                "chunk_index": chunk.index,
                "chunk_total": chunk.total,
            }
            if item.tags:
                metadata["tags"] = item.tags
            client.upsert(collection, chunk_id, chunk.text, metadata)

    def delete_item(self, kb: KnowledgeBase, item_id: str) -> None:
        client = self._get_client(kb)
        collection_name = get_collection_name(kb)
        collection = client.get_or_create_collection(collection_name)
        client.delete_by_parent(collection, kb.id, item_id)

    def maintain_index(
        self,
        kb: KnowledgeBase,
        opt: IndexOptions,
        get_items: Callable[[], list[KnowledgeItem]],
    ) -> IndexResult:
        if not semantic_enabled(kb):
            return IndexResult(skipped=1, warnings=[f"{kb.id}: semantic indexing is not enabled"])

        notify = opt.progress or (lambda _: None)
        result = IndexResult()

        items = get_items()
        notify(IndexProgressEvent(kb_id=kb.id, phase=IndexProgressPhase.START, total=len(items)))

        if opt.rebuild:
            notify(IndexProgressEvent(kb_id=kb.id, phase=IndexProgressPhase.REBUILD_RESET))
            # Re-create client to reset state
            key = self._client_key(kb)
            with self._lock:
                if key in self._clients:
                    self._clients[key].close()
                    del self._clients[key]

        for i, item in enumerate(items):
            notify(IndexProgressEvent(kb_id=kb.id, phase=IndexProgressPhase.INDEX, item=item.id, done=i + 1, total=len(items)))
            try:
                self.index_item(kb, item)
                result.indexed += 1
            except Exception as e:
                result.errors.append(f"index {item.id}: {e}")

        notify(IndexProgressEvent(kb_id=kb.id, phase=IndexProgressPhase.DONE, done=result.indexed + result.skipped, total=len(items)))
        return result

    def close(self) -> None:
        with self._lock:
            for client in self._clients.values():
                client.close()
            self._clients.clear()
