"""Weaviate vector store integration."""

import uuid
from collections.abc import Sequence
from typing import Any

from duragraph.vectorstores.base import Document, EmbeddingProvider, SearchResult, VectorStore

try:
    import weaviate
    from weaviate.classes.config import Configure, DataType, Property
    from weaviate.classes.query import Filter, MetadataQuery

    HAS_WEAVIATE = True
except ImportError:
    HAS_WEAVIATE = False
    weaviate = None  # type: ignore


class WeaviateVectorStore(VectorStore):
    """Weaviate vector store.

    Weaviate is an open-source vector database with built-in ML models.

    Example:
        ```python
        from duragraph.vectorstores import WeaviateVectorStore
        from duragraph.embeddings import OpenAIEmbeddings

        # Local instance
        store = WeaviateVectorStore(
            collection="MyDocs",
            url="http://localhost:8080",
            embedding_provider=OpenAIEmbeddings(),
        )

        # Weaviate Cloud
        store = WeaviateVectorStore(
            collection="MyDocs",
            url="https://xxx.weaviate.network",
            api_key="your-api-key",
            embedding_provider=OpenAIEmbeddings(),
        )

        # Embedded mode (no server needed)
        store = WeaviateVectorStore(
            collection="MyDocs",
            embedded=True,
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
        embedded: bool = False,
        embedding_provider: EmbeddingProvider | None = None,
        dimension: int | None = None,
        **kwargs: Any,
    ) -> None:
        """Initialize Weaviate vector store.

        Args:
            collection: Name of the collection (class in Weaviate terms).
            url: Weaviate server URL.
            api_key: API key for Weaviate Cloud.
            embedded: Use embedded Weaviate (no server needed).
            embedding_provider: Embedding provider for automatic embedding.
            dimension: Vector dimension for collection schema.
            **kwargs: Additional configuration.
        """
        if not HAS_WEAVIATE:
            raise ImportError(
                "weaviate-client package not installed. "
                "Install with: pip install duragraph-python[weaviate]"
            )

        super().__init__(**kwargs)

        self._collection_name = collection
        self._embedding_provider = embedding_provider
        self._dimension = dimension

        # Initialize client
        if embedded:
            self._client = weaviate.connect_to_embedded()
        elif url:
            if api_key:
                self._client = weaviate.connect_to_wcs(
                    cluster_url=url,
                    auth_credentials=weaviate.auth.AuthApiKey(api_key),
                )
            else:
                self._client = weaviate.connect_to_local(host=url)
        else:
            raise ValueError("Either url or embedded=True required")

        # Ensure collection exists
        self._ensure_collection()

    def _ensure_collection(self) -> None:
        """Ensure the collection exists, creating it if necessary."""
        if not self._client.collections.exists(self._collection_name):
            # Determine dimension
            dim = self._dimension
            if not dim and self._embedding_provider:
                dim = self._embedding_provider.dimension

            # Create collection with vectorizer
            properties = [
                Property(name="content", data_type=DataType.TEXT),
                Property(name="metadata", data_type=DataType.OBJECT),
            ]

            if dim:
                self._client.collections.create(
                    name=self._collection_name,
                    vectorizer_config=Configure.Vectorizer.none(),
                    properties=properties,
                )
            else:
                # Let Weaviate use default vectorizer
                self._client.collections.create(
                    name=self._collection_name,
                    properties=properties,
                )

        self._collection = self._client.collections.get(self._collection_name)

    async def add(
        self,
        documents: Sequence[Document],
        *,
        embeddings: Sequence[Sequence[float]] | None = None,
        ids: Sequence[str] | None = None,
    ) -> list[str]:
        """Add documents to Weaviate."""
        if not documents:
            return []

        # Generate IDs if not provided
        doc_ids = list(ids) if ids else [doc.id or str(uuid.uuid4()) for doc in documents]

        # Generate embeddings if needed
        if embeddings is None and self._embedding_provider:
            contents = [doc.content for doc in documents]
            embedding_list = await self._embedding_provider.embed(contents)
        elif embeddings:
            embedding_list = [list(e) for e in embeddings]
        else:
            embedding_list = None

        # Add objects
        with self._collection.batch.dynamic() as batch:
            for i, (doc_id, doc) in enumerate(zip(doc_ids, documents, strict=True)):
                properties = {
                    "content": doc.content,
                    "metadata": doc.metadata,
                }

                vector = embedding_list[i] if embedding_list else None

                batch.add_object(
                    properties=properties,
                    uuid=doc_id,
                    vector=vector,
                )

        return doc_ids

    async def search(
        self,
        query: str | list[float],
        *,
        k: int = 10,
        filter: dict[str, Any] | None = None,
    ) -> list[SearchResult]:
        """Search for similar documents in Weaviate."""
        # Get query embedding
        if isinstance(query, str):
            if self._embedding_provider:
                query_embedding = await self._embedding_provider.embed_query(query)
            else:
                # Use Weaviate's built-in vectorizer via near_text
                weaviate_filter = self._build_filter(filter) if filter else None
                results = self._collection.query.near_text(
                    query=query,
                    limit=k,
                    filters=weaviate_filter,
                    return_metadata=MetadataQuery(distance=True),
                )
                return self._convert_results(results.objects)
        else:
            query_embedding = query

        # Search by vector
        weaviate_filter = self._build_filter(filter) if filter else None
        results = self._collection.query.near_vector(
            near_vector=query_embedding,
            limit=k,
            filters=weaviate_filter,
            return_metadata=MetadataQuery(distance=True),
        )

        return self._convert_results(results.objects)

    def _build_filter(self, filter_dict: dict[str, Any]) -> Any:
        """Build Weaviate filter from dict."""
        conditions = []
        for key, value in filter_dict.items():
            # Access metadata fields via metadata.key
            field_path = f"metadata.{key}" if key != "content" else key
            conditions.append(Filter.by_property(field_path).equal(value))

        if len(conditions) == 1:
            return conditions[0]
        elif len(conditions) > 1:
            return Filter.all_of(conditions)
        return None

    def _convert_results(self, objects: list[Any]) -> list[SearchResult]:
        """Convert Weaviate objects to SearchResult."""
        search_results = []
        for obj in objects:
            content = obj.properties.get("content", "")
            metadata = obj.properties.get("metadata", {})

            # Convert distance to similarity score
            # Weaviate uses distance, lower is better
            distance = obj.metadata.distance if obj.metadata else 0
            score = 1.0 / (1.0 + distance)

            search_results.append(
                SearchResult(
                    document=Document(
                        content=content,
                        metadata=metadata or {},
                        id=str(obj.uuid),
                    ),
                    score=score,
                    id=str(obj.uuid),
                )
            )

        return search_results

    async def delete(
        self,
        ids: Sequence[str] | None = None,
        *,
        filter: dict[str, Any] | None = None,
    ) -> int:
        """Delete documents from Weaviate."""
        if not ids and not filter:
            return 0

        count = 0

        if ids:
            for doc_id in ids:
                self._collection.data.delete_by_id(doc_id)
                count += 1

        elif filter:
            weaviate_filter = self._build_filter(filter)
            result = self._collection.data.delete_many(where=weaviate_filter)
            count = result.successful if result else 0

        return count

    async def get(self, ids: Sequence[str]) -> list[Document]:
        """Get documents by ID from Weaviate."""
        if not ids:
            return []

        documents = []
        for doc_id in ids:
            obj = self._collection.query.fetch_object_by_id(
                uuid=doc_id,
                include_vector=True,
            )
            if obj:
                content = obj.properties.get("content", "")
                metadata = obj.properties.get("metadata", {})

                documents.append(
                    Document(
                        content=content,
                        metadata=metadata or {},
                        id=str(obj.uuid),
                        embedding=list(obj.vector.get("default", [])) if obj.vector else None,
                    )
                )

        return documents

    async def clear(self) -> int:
        """Clear all documents from Weaviate collection."""
        count = await self.count()

        # Delete all objects
        self._collection.data.delete_many(where=Filter.by_id().not_equal(""))

        return count

    async def count(self) -> int:
        """Get the number of documents in the collection."""
        result = self._collection.aggregate.over_all(total_count=True)
        return result.total_count if result else 0

    def close(self) -> None:
        """Close the Weaviate client connection."""
        self._client.close()

    def __del__(self) -> None:
        """Cleanup on deletion."""
        try:
            self.close()
        except Exception:
            pass
