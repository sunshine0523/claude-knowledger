from dataclasses import dataclass, field
from pathlib import Path
from typing import Any
import os

import yaml


DEFAULT_SERVER_ADDRESS = ":34125"
DEFAULT_STORAGE_PATH = "~/.knowledger/db"
DEFAULT_RUNTIME_REGISTRY_PATH = "~/.knowledger/registry.json"
DEFAULT_KB_ID = "default"
DEFAULT_KB_NAME = "Default"
DEFAULT_CHROMA_PROVIDER = "chroma"
DEFAULT_CHROMA_MODE = "persistent"
DEFAULT_CHROMA_HTTP_MODE = "http"
DEFAULT_CHROMA_BASE_URL = "http://127.0.0.1:8000"
DEFAULT_CHROMA_STORAGE_PATH = "~/.knowledger/chroma"
DEFAULT_CHROMA_SYNC_MODE = "async"


@dataclass
class ServerConfig:
    address: str = DEFAULT_SERVER_ADDRESS


@dataclass
class KnowledgeBaseConfig:
    id: str
    name: str = ""
    store_type: str = ""
    store_config: dict[str, Any] = field(default_factory=dict)
    enabled: bool = True
    indexing: dict[str, Any] = field(default_factory=dict)
    tags: list[str] = field(default_factory=list)


@dataclass
class Config:
    knowledge_bases: list[KnowledgeBaseConfig] = field(default_factory=list)
    server: ServerConfig = field(default_factory=ServerConfig)
    runtime_registry_path: str = ""


def load(path: str) -> Config:
    """Load configuration from YAML file and apply defaults."""
    cfg = Config(
        server=ServerConfig(address=DEFAULT_SERVER_ADDRESS),
    )

    with open(path, 'r') as f:
        data = yaml.safe_load(f)

    if data:
        # Map YAML fields to Config fields
        if "runtime_registry_path" in data:
            cfg.runtime_registry_path = data["runtime_registry_path"]
        if "server" in data:
            server_data = data["server"]
            if isinstance(server_data, dict) and "address" in server_data:
                cfg.server = ServerConfig(address=server_data["address"])
        if "knowledge_bases" in data:
            kb_list = data["knowledge_bases"]
            if isinstance(kb_list, list):
                cfg.knowledge_bases = []
                for kb_data in kb_list:
                    if isinstance(kb_data, dict):
                        kb = KnowledgeBaseConfig(
                            id=kb_data.get("id", ""),
                            name=kb_data.get("name", ""),
                            store_type=kb_data.get("store_type", ""),
                            store_config=kb_data.get("store_config", {}),
                            enabled=kb_data.get("enabled", True),
                            indexing=kb_data.get("indexing", {}),
                            tags=kb_data.get("tags", []),
                        )
                        cfg.knowledge_bases.append(kb)

    apply_defaults(cfg)
    return cfg


def default() -> Config:
    """Create a Config with defaults applied."""
    cfg = Config()
    apply_defaults(cfg)
    return cfg


def apply_defaults(cfg: Config) -> None:
    """Apply defaults to configuration in-place."""
    if not cfg.runtime_registry_path:
        cfg.runtime_registry_path = DEFAULT_RUNTIME_REGISTRY_PATH

    cfg.runtime_registry_path = expand_home_path(cfg.runtime_registry_path)

    if not cfg.server.address:
        cfg.server.address = DEFAULT_SERVER_ADDRESS

    if not cfg.knowledge_bases:
        kb = _default_knowledge_base()
        cfg.knowledge_bases = [kb]
        return

    for kb in cfg.knowledge_bases:
        apply_kb_defaults(kb)


def apply_kb_defaults(kb: KnowledgeBaseConfig) -> None:
    """Apply defaults to a knowledge base configuration."""
    if kb.store_type == "sqlite":
        _apply_sqlite_store_defaults(kb)
    elif kb.store_type == "text":
        # path is user-provided; nothing to default
        pass

    _apply_semantic_indexing_defaults(kb)


def _apply_sqlite_store_defaults(kb: KnowledgeBaseConfig) -> None:
    """Apply SQLite store defaults."""
    if kb.store_config is None:
        kb.store_config = {}

    path = kb.store_config.get("path")
    if not path:
        default_path = expand_home_path(DEFAULT_STORAGE_PATH)
        kb.store_config["path"] = default_path
        return

    if not isinstance(path, str):
        raise ValueError(f"knowledge base {kb.id!r} sqlite store_config.path must be a string")

    kb.store_config["path"] = expand_home_path(path)


def _apply_semantic_indexing_defaults(kb: KnowledgeBaseConfig) -> None:
    """Apply semantic indexing defaults."""
    if kb.indexing is None:
        kb.indexing = {}

    lexical = _ensure_map(kb.indexing, "lexical")
    _set_default(lexical, "enabled", True)

    if kb.store_type == "text":
        # For text, only fill in defaults if the user actually specified a semantic block
        if "semantic" not in kb.indexing:
            return
        semantic = kb.indexing.get("semantic")
        if not isinstance(semantic, dict):
            return
        _fill_chroma_defaults(kb, semantic)
        return

    semantic = _ensure_map(kb.indexing, "semantic")
    _set_default(semantic, "enabled", True)
    _fill_chroma_defaults(kb, semantic)


def _fill_chroma_defaults(kb: KnowledgeBaseConfig, semantic: dict[str, Any]) -> None:
    """Fill in Chroma-specific defaults."""
    _set_default(semantic, "provider", DEFAULT_CHROMA_PROVIDER)

    collection = kb.id if kb.id else DEFAULT_KB_ID
    _set_default(semantic, "collection", collection)
    _set_default(semantic, "sync_mode", DEFAULT_CHROMA_SYNC_MODE)
    _set_default(semantic, "auto_download", True)

    mode = semantic.get("mode", "")
    if isinstance(mode, str) and mode:
        pass  # mode already set
    else:
        if "base_url" in semantic:
            mode = DEFAULT_CHROMA_HTTP_MODE
        else:
            mode = DEFAULT_CHROMA_MODE
        semantic["mode"] = mode

    if not isinstance(mode, str):
        raise ValueError(f"knowledge base {kb.id!r} chroma semantic mode must be a string")

    if mode == DEFAULT_CHROMA_HTTP_MODE:
        _set_default(semantic, "base_url", DEFAULT_CHROMA_BASE_URL)
        return

    if mode != DEFAULT_CHROMA_MODE:
        return

    path = semantic.get("path")
    if not path:
        base_path = expand_home_path(DEFAULT_CHROMA_STORAGE_PATH)
        semantic["path"] = os.path.join(base_path, collection)
        return

    if not isinstance(path, str) or not path:
        raise ValueError(f"knowledge base {kb.id!r} chroma semantic path must be a string")

    semantic["path"] = expand_home_path(path)


def _default_knowledge_base() -> KnowledgeBaseConfig:
    """Create default knowledge base configuration."""
    path = expand_home_path(DEFAULT_STORAGE_PATH)
    kb = KnowledgeBaseConfig(
        id=DEFAULT_KB_ID,
        name=DEFAULT_KB_NAME,
        store_type="sqlite",
        enabled=True,
        store_config={"path": path},
    )
    _apply_semantic_indexing_defaults(kb)
    return kb


def _ensure_map(parent: dict[str, Any], key: str) -> dict[str, Any]:
    """Ensure a key exists in parent and is a dict."""
    value = parent.get(key)
    if value is None:
        child: dict[str, Any] = {}
        parent[key] = child
        return child

    if isinstance(value, dict):
        return value

    child = {}
    parent[key] = child
    return child


def _set_default(values: dict[str, Any], key: str, value: Any) -> None:
    """Set a default value if key doesn't exist."""
    if key not in values:
        values[key] = value


def expand_home_path(path: str) -> str:
    """Expand ~ to user home directory."""
    if not path:
        return path

    if path == "~" or path.startswith("~/"):
        home = str(Path.home())
        if path == "~":
            return home
        return os.path.join(home, path[2:])

    return path
