from typing import Any

try:
    import chromadb
    from chromadb import Collection
except ImportError:
    chromadb = None
    Collection = None

from knowledger.core.types import KnowledgeBase


class ChromaClient:
    """Wrapper around chromadb Python client."""

    def __init__(self, kb: KnowledgeBase):
        if chromadb is None:
            raise ImportError(
                "chromadb is not installed. Install with: pip install chromadb"
            )

        semantic = kb.indexing.get("semantic", {})
        mode = semantic.get("mode", "embedded")

        if mode == "http":
            base_url = semantic.get("base_url", "http://localhost:8000")
            self._client = chromadb.HttpClient(host=base_url.split("://")[-1].split(":")[0],
                                              port=int(base_url.split(":")[-1]) if ":" in base_url.split("://")[-1] else 8000)
        else:  # embedded/persistent
            path = semantic.get("path", "")
            if path:
                self._client = chromadb.PersistentClient(path=path)
            else:
                self._client = chromadb.EphemeralClient()

    def get_or_create_collection(self, name: str) -> "Collection":
        """Get or create a collection by name."""
        return self._client.get_or_create_collection(name=name)

    def upsert(
        self, collection: "Collection", item_id: str, text: str, metadata: dict[str, Any]
    ) -> None:
        """Upsert a document into the collection."""
        collection.upsert(
            ids=[item_id],
            documents=[text],
            metadatas=[metadata]
        )

    def delete(self, collection: "Collection", item_id: str) -> None:
        """Delete a document from the collection."""
        collection.delete(ids=[item_id])

    def delete_by_parent(
        self, collection: "Collection", kb_id: str, parent_id: str
    ) -> None:
        """Delete all chunks for a parent item."""
        collection.delete(
            where={
                "$and": [
                    {"kb_id": {"$eq": kb_id}},
                    {"parent_id": {"$eq": parent_id}}
                ]
            }
        )

    def close(self) -> None:
        """Close the client connection."""
        # chromadb Python client doesn't have explicit close
        pass
