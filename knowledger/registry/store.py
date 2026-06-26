from __future__ import annotations

from dataclasses import dataclass, field
from typing import Any, Protocol
import threading


@dataclass
class RuntimeKnowledgeBase:
    id: str
    name: str = ""
    store_type: str = ""
    store_config: dict[str, Any] = field(default_factory=dict)
    enabled: bool = True
    indexing: dict[str, Any] = field(default_factory=dict)
    tags: list[str] = field(default_factory=list)


class Store(Protocol):
    def list(self) -> list[RuntimeKnowledgeBase]: ...
    def save(self, items: list[RuntimeKnowledgeBase]) -> None: ...
    def version(self) -> str: ...


class MemoryStore:
    def __init__(self, items: list[RuntimeKnowledgeBase] | None = None) -> None:
        self._lock = threading.Lock()
        self._items: list[RuntimeKnowledgeBase] = list(items) if items else []
        self._version: int = 0

    def list(self) -> list[RuntimeKnowledgeBase]:
        with self._lock:
            return list(self._items)

    def save(self, items: list[RuntimeKnowledgeBase]) -> None:
        with self._lock:
            self._items = list(items)
            self._version += 1

    def version(self) -> str:
        with self._lock:
            return str(self._version)
