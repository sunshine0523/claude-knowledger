from knowledger.core.types import KnowledgeBase


def semantic_enabled(kb: KnowledgeBase) -> bool:
    semantic = kb.indexing.get("semantic", {})
    return bool(semantic.get("enabled", False))


def get_collection_name(kb: KnowledgeBase) -> str:
    semantic = kb.indexing.get("semantic", {})
    return semantic.get("collection", kb.id)
