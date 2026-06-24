import json
import os
import threading
from pathlib import Path

from .store import RuntimeKnowledgeBase

_REGISTRY_FILE_VERSION = 1


class FileStore:
    def __init__(self, path: str) -> None:
        self._path = path
        self._lock = threading.Lock()

    def list(self) -> list[RuntimeKnowledgeBase]:
        with self._lock:
            try:
                data = Path(self._path).read_text(encoding="utf-8")
            except FileNotFoundError:
                return []

            if not data.strip():
                raise ValueError(f"runtime registry {self._path!r} is empty")

            file_data = json.loads(data)
            kbs = file_data.get("knowledge_bases", [])
            return [_dict_to_rkb(kb) for kb in kbs]

    def version(self) -> str:
        with self._lock:
            try:
                stat = os.stat(self._path)
            except FileNotFoundError:
                return ""
            mtime = stat.st_mtime_ns
            sec = mtime // 1_000_000_000
            nsec = mtime % 1_000_000_000
            return f"{sec}.{nsec:09d}-{stat.st_size}"

    def save(self, items: list[RuntimeKnowledgeBase]) -> None:
        with self._lock:
            os.makedirs(os.path.dirname(self._path) or ".", exist_ok=True)
            file_data = {
                "version": _REGISTRY_FILE_VERSION,
                "knowledge_bases": [_rkb_to_dict(item) for item in items],
            }
            data = json.dumps(file_data, indent=2) + "\n"

            dir_path = os.path.dirname(self._path) or "."
            tmp_fd, tmp_path = _make_temp(dir_path)
            try:
                with os.fdopen(tmp_fd, "w", encoding="utf-8") as f:
                    f.write(data)
                os.replace(tmp_path, self._path)
            except Exception:
                try:
                    os.remove(tmp_path)
                except OSError:
                    pass
                raise


def _make_temp(dir_path: str) -> tuple[int, str]:
    import tempfile
    fd, path = tempfile.mkstemp(prefix=".registry-", suffix=".tmp", dir=dir_path)
    return fd, path


def _rkb_to_dict(item: RuntimeKnowledgeBase) -> dict:
    return {
        "id": item.id,
        "name": item.name,
        "store_type": item.store_type,
        "store_config": item.store_config,
        "enabled": item.enabled,
        "default_search_mode": item.default_search_mode,
        "indexing": item.indexing,
        "tags": item.tags,
    }


def _dict_to_rkb(d: dict) -> RuntimeKnowledgeBase:
    return RuntimeKnowledgeBase(
        id=d.get("id", ""),
        name=d.get("name", ""),
        store_type=d.get("store_type", ""),
        store_config=d.get("store_config") or {},
        enabled=d.get("enabled", True),
        default_search_mode=d.get("default_search_mode", ""),
        indexing=d.get("indexing") or {},
        tags=d.get("tags") or [],
    )
