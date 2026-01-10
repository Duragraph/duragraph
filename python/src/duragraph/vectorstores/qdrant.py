"""Qdrant vector store integration."""

import uuid
from collections.abc import Sequence
from typing import Any

from duragraph.vectorstores.base import Document, EmbeddingProvider, SearchResult, VectorStore

try:
    from qdrant_client import QdrantClient
    from qdrant_client.http import models

    HAS_QDRANT = True
except ImportError:
    HAS_QDRANT = False
    QdrantClient = None  # type: ignore
    models = None  # type: ignore


class QdrantVectorStore(VectorStore):
    """Qdrant vector store.

    Qdrant is a high-performance vector search engine with advanced filtering.

    Example:
        ```python
        from duragraph.vectorstores import QdrantVectorStore
        from duragraph.embeddings import OpenAIEmbeddings

        # Local in-memory
        store = QdrantVectorStore(
            collection="my_docs",
            embedding_provider=OpenAIEmbeddings(),
        )

        # Remote server
        store = QdrantVectorStore(
            collection="my_docs",
            url="http://localhost:6333",
            embedding_provider=OpenAIEmbeddings(),
        )

        # Qdrant Cloud
        store = QdrantVectorStore(
            collection="my_docs",
            url="https://xxx.qdrant.io",
            api_key="your-api-key",
            embedding_provider=OpenAIEmbeddings(),
        )
        ```
    """

    def __init__(
        self,
        collection: str,
        *,
        url: str | None = None,
        api_key: str | None = None,
        embedding_provider: EmbeddingProvider | None = None,
        dimension: int | None = None,
        distance: str = "Cosine",
        **kwargs: Any,
    ) -> None:
        """Initialize Qdrant vector store.

        Args:
            collection: Name of the collection.
            url: Qdrant server URL. If None, uses in-memory storage.
            api_key: API key for Qdrant Cloud.
            embedding_provider: Embedding provider for automatic embedding.
            dimension: Vector dimension (required for collection creation).
            distance: Distance metric (Cosine, Euclid, Dot).
            **kwargs: Additional configuration.
        """
        if not HAS_QDRANT:
            raise ImportError(
                "qdrant-client package not installed. "
                "Install with: pip install duragraph-python[qdrant]"
            )

        super().__init__(**kwargs)

        self._collection_name = collection
        self._embedding_provider = embedding_provider
        self._dimension = dimension
        self._distance = distance

        # Initialize client
        if url:
            self._client = QdrantClient(url=url, api_key=api_key)
        else:
            # In-memory mode
            self._client = QdrantClient(":memory:")

        # Ensure collection exists
        self._ensure_collection()

    def _ensure_collection(self) -> None:
        """Ensure the collection exists, creating it if necessary."""
        collections = self._client.get_collections().collections
        collection_names = [c.name for c in collections]

        if self._collection_name not in collection_names:
            # Determine dimension
            dim = self._dimension
            if not dim and self._embedding_provider:
                dim = self._embedding_provider.dimension
            if not dim:
                raise ValueError(
                    "dimension required to create collection. "
                    "Provide dimension or use an embedding provider with known dimension."
                )

            # Map distance string to Qdrant distance type
            distance_map = {
                "Cosine": models.Distance.COSINE,
                "Euclid": models.Distance.EUCLID,
                "Dot": models.Distance.DOT,
            }
            distance = distance_map.get(self._distance, models.Distance.COSINE)

            self._client.create_collection(
                collection_name=self._collection_name,
                vectors_config=models.VectorParams(
                    size=dim,
                    distance=distance,
                ),
            )

    async def add(
        self,
        documents: Sequence[Document],
        *,
        embeddings: Sequence[Sequence[float]] | None = None,
        ids: Sequence[str] | None = None,
    ) -> list[str]:
        """Add documents to Qdrant."""
        if not documents:
            return []

        # Generate IDs if not provided
        doc_ids = list(ids) if ids else [doc.id or str(uuid.uuid4()) for doc in documents]

        # Generate embeddings if needed
        if embeddings is None:
            if not self._embedding_provider:
                raise ValueError("embeddings required when no embedding_provider is configured")
            contents = [doc.content for doc in documents]
            embedding_list = await self._embedding_provider.embed(contents)
        else:
            embedding_list = [list(e) for e in embeddings]

        # Prepare points
        points = []
        for doc_id, doc, emb in zip(doc_ids, documents, embedding_list, strict=True):
            payload = dict(doc.metadata)
            payload["_content"] = doc.content

            points.append(
                models.PointStruct(
                    id=doc_id,
                    vector=emb,
                    payload=payload,
                )
            )

        # Upsert points
        self._client.upsert(
            collection_name=self._collection_name,
            points=points,
        )

        return doc_ids

    async def search(
        self,
        query: str | list[float],
        *,
        k: int = 10,
        filter: dict[str, Any] | None = None,
    ) -> list[SearchResult]:
        """Search for similar documents in Qdrant."""
        # Get query embedding
        if isinstance(query, str):
            if not self._embedding_provider:
                raise ValueError("embedding_provider required for text queries")
            query_embedding = await self._embedding_provider.embed_query(query)
        else:
            query_embedding = query

        # Build filter if provided
        qdrant_filter = None
        if filter:
            conditions = []
            for key, value in filter.items():
                if isinstance(value, list):
                    conditions.append(
                        models.FieldCondition(
                            key=key,
                            match=models.MatchAny(any=value),
                        )
                    )
                else:
                    conditions.append(
                        models.FieldCondition(
                            key=key,
                            match=models.MatchValue(value=value),
                        )
                    )
            qdrant_filter = models.Filter(must=conditions)

        # Search
        results = self._client.search(
            collection_name=self._collection_name,
            query_vector=query_embedding,
            limit=k,
            query_filter=qdrant_filter,
        )

        # Convert to SearchResult
        search_results = []
        for point in results:
            payload = dict(point.payload) if point.payload else {}
            content = payload.pop("_content", "")

            search_results.append(
                SearchResult(
                    document=Document(
                        content=content,
                        metadata=payload,
                        id=str(point.id),
                    ),
                    score=point.score,
                    id=str(point.id),
                )
            )

        return search_results

    async def delete(
        self,
        ids: Sequence[str] | None = None,
        *,
        filter: dict[str, Any] | None = None,
    ) -> int:
        """Delete documents from Qdrant."""
        if not ids and not filter:
            return 0

        if ids:
            # Get count before delete
            before_count = await self.count()

            self._client.delete(
                collection_name=self._collection_name,
                points_selector=models.PointIdsList(points=list(ids)),
            )

            after_count = await self.count()
            return before_count - after_count

        elif filter:
            conditions = []
            for key, value in filter.items():
                if isinstance(value, list):
                    conditions.append(
                        models.FieldCondition(
                            key=key,
                            match=models.MatchAny(any=value),
                        )
                    )
                else:
                    conditions.append(
                        models.FieldCondition(
                            key=key,
                            match=models.MatchValue(value=value),
                        )
                    )

            before_count = await self.count()

            self._client.delete(
                collection_name=self._collection_name,
                points_selector=models.FilterSelector(
                    filter=models.Filter(must=conditions),
                ),
            )

            after_count = await self.count()
            return before_count - after_count

        return 0

    async def get(self, ids: Sequence[str]) -> list[Document]:
        """Get documents by ID from Qdrant."""
        if not ids:
            return []

        results = self._client.retrieve(
            collection_name=self._collection_name,
            ids=list(ids),
            with_payload=True,
            with_vectors=True,
        )

        documents = []
        for point in results:
            payload = dict(point.payload) if point.payload else {}
            content = payload.pop("_content", "")

            documents.append(
                Document(
                    content=content,
                    metadata=payload,
                    id=str(point.id),
                    embedding=list(point.vector) if point.vector else None,
                )
            )

        return documents

    async def clear(self) -> int:
        """Clear all documents from Qdrant collection."""
        count = await self.count()

        # Delete and recreate collection
        collection_info = self._client.get_collection(self._collection_name)
        vectors_config = collection_info.config.params.vectors

        self._client.delete_collection(self._collection_name)
        self._client.create_collection(
            collection_name=self._collection_name,
            vectors_config=vectors_config,
        )

        return count

    async def count(self) -> int:
        """Get the number of documents in the collection."""
        info = self._client.get_collection(self._collection_name)
        return info.points_count
