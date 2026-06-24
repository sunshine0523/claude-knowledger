import os
from pathlib import Path

from knowledger.config import config as cfg_module
from knowledger.registry import FileStore, Registry
from knowledger.backends.sqlite.backend import MultiBackend as SqliteMultiBackend
from knowledger.backends.text.backend import Backend as TextBackend
from knowledger.indexing.semantic.indexer import Indexer
from knowledger.indexing.chunking.splitter import default_splitter
from knowledger.service.service import Service
from knowledger.core import KnowledgeBase, StoreBackend


def build_backends(kbs: list[KnowledgeBase]) -> dict[str, StoreBackend]:
    indexer = Indexer(default_splitter())
    backends: dict[str, StoreBackend] = {"text": TextBackend(indexer=indexer)}
    if any(kb.store_type == "sqlite" for kb in kbs):
        backends["sqlite"] = SqliteMultiBackend(kbs, indexer=indexer)
    return backends


def build_service(config_path: str, project_root: str = "") -> Service:
    cfg = cfg_module.load(config_path)
    return build_service_from_config(cfg, project_root)


def build_default_service(project_root: str = "") -> Service:
    cfg = cfg_module.default()
    return build_service_from_config(cfg, project_root)


def build_service_from_config(cfg, project_root: str = "") -> Service:
    cfg_module.apply_defaults(cfg)
    global_store = FileStore(cfg.runtime_registry_path)
    project_store = None
    if project_root:
        project_store = FileStore(os.path.join(project_root, ".knowledger", "registry.json"))
    reg = Registry(cfg.knowledge_bases, global_store, project_store, project_root)
    return Service.new_managed(reg, build_backends)


def discover_project_root() -> str:
    cwd = Path.cwd()
    for parent in [cwd, *cwd.parents]:
        if (parent / ".knowledger").is_dir() or (parent / ".git").is_dir():
            return str(parent)
    return ""
