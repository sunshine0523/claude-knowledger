from dataclasses import dataclass, field
from datetime import datetime
from typing import Any


@dataclass
class KnowledgeBase:
    id: str
    scope: str
    name: str = ""
    store_type: str = ""
    store_config: dict[str, Any] = field(default_factory=dict)
    enabled: bool = True
    default_search_mode: str = ""
    indexing: dict[str, Any] = field(default_factory=dict)
    tags: list[str] = field(default_factory=list)


@dataclass
class KnowledgeItem:
    id: str
    kb_id: str
    type: str = ""
    title: str = ""
    content: str = ""
    summary: str = ""
    source_ref: str = ""
    metadata: dict[str, Any] = field(default_factory=dict)
    tags: list[str] = field(default_factory=list)
    created_at: datetime = field(default_factory=datetime.utcnow)
    updated_at: datetime = field(default_factory=datetime.utcnow)


@dataclass
class SearchHit:
    item_id: str
    kb_id: str
    scope: str = ""
    item_type: str = ""
    title: str = ""
    snippet: str = ""
    content_preview: str = ""
    score: float = 0.0
    match_mode: str = ""
    source_backend: str = ""
    locator: str = ""
    metadata: dict[str, Any] = field(default_factory=dict)


@dataclass
class IngestionResult:
    success: bool = True
    item_id: str = ""
    index_queued: bool = False
    warnings: list[str] = field(default_factory=list)


@dataclass
class IndexStatus:
    state: str = ""
    last_success_at: datetime | None = None
    last_error: str = ""
