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

    def query(
        self, collection: "Collection", text: str, n_results: int
    ) -> list[dict[str, Any]]:
        """Query the collection and return results.

        Returns list of {id, distance, metadata, document}.
        """
        if n_results <= 0:
            n_results = 10

        results = collection.query(
            query_texts=[text],
            n_results=n_results,
            include=["metadatas", "documents", "distances"]
        )

        # Flatten chromadb nested structure
        hits = []
        if results and results.get("ids"):
            ids = results["ids"][0] if results["ids"] else []
            distances = results["distances"][0] if results.get("distances") else []
            metadatas = results["metadatas"][0] if results.get("metadatas") else []
            documents = results["documents"][0] if results.get("documents") else []

            for i, doc_id in enumerate(ids):
                hit = {
                    "id": doc_id,
                    "distance": distances[i] if i < len(distances) else 0.0,
                    "metadata": metadatas[i] if i < len(metadatas) else {},
                    "document": documents[i] if i < len(documents) else "",
                }
                hits.append(hit)

        return hits

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
