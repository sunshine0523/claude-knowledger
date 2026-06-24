from abc import ABC, abstractmethod
from collections.abc import Callable
from dataclasses import dataclass, field
from typing import Any

from knowledger.core.types import (
    IngestionResult, IndexStatus, KnowledgeBase, KnowledgeItem, SearchHit
)


@dataclass
class ScopedKBRef:
    scope: str
    id: str


@dataclass
class SearchOptions:
    query: str = ""
    limit: int = 0
    kb_ids: list[ScopedKBRef] = field(default_factory=list)
    search_mode: str = ""


@dataclass
class AddInput:
    kb_id: str
    scope: str
    title: str
    content: str
    tags: list[str] = field(default_factory=list)
    metadata: dict[str, Any] = field(default_factory=dict)


class IndexProgressPhase:
    START = "start"
    REBUILD_RESET = "rebuild_reset"
    INDEX = "index"
    SKIP = "skip"
    DELETE_ORPHAN = "delete_orphan"
    DONE = "done"


@dataclass
class IndexProgressEvent:
    kb_id: str
    phase: str
    item: str = ""
    done: int = 0
    total: int = 0
    message: str = ""


IndexProgress = Callable[[IndexProgressEvent], None]


@dataclass
class IndexOptions:
    rebuild: bool = False
    progress: IndexProgress | None = None


@dataclass
class IndexResult:
    indexed: int = 0
    deleted: int = 0
    skipped: int = 0
    warnings: list[str] = field(default_factory=list)
    errors: list[str] = field(default_factory=list)


class StoreBackend(ABC):
    @abstractmethod
    def add(self, kb: KnowledgeBase, inp: AddInput) -> tuple[KnowledgeItem, IngestionResult, IndexStatus]:
        ...

    @abstractmethod
    def search(self, kb: KnowledgeBase, opt: SearchOptions) -> list[SearchHit]:
        ...

    @abstractmethod
    def get_item(self, kb: KnowledgeBase, item_id: str) -> KnowledgeItem:
        ...

    @abstractmethod
    def list_items(self, kb: KnowledgeBase) -> list[KnowledgeItem]:
        ...

    @abstractmethod
    def delete_item(self, kb: KnowledgeBase, item_id: str) -> None:
        ...

    @abstractmethod
    def supports_semantic(self, kb: KnowledgeBase) -> bool:
        ...


class IndexMaintainer(ABC):
    @abstractmethod
    def maintain_index(self, kb: KnowledgeBase, opt: IndexOptions) -> IndexResult:
        ...
