"""Pinecone vector store integration."""

import os
import uuid
from collections.abc import Sequence
from typing import Any

from duragraph.vectorstores.base import Document, EmbeddingProvider, SearchResult, VectorStore

try:
    from pinecone import Pinecone, ServerlessSpec

    HAS_PINECONE = True
except ImportError:
    HAS_PINECONE = False
    Pinecone = None  # type: ignore
    ServerlessSpec = None  # type: ignore


class PineconeVectorStore(VectorStore):
    """Pinecone vector store.

    Pinecone is a managed vector database for production workloads.

    Example:
        ```python
        from duragraph.vectorstores import PineconeVectorStore
        from duragraph.embeddings import OpenAIEmbeddings

        # With embedding provider
        embeddings = OpenAIEmbeddings()
        store = PineconeVectorStore(
            index="my-index",
            embedding_provider=embeddings,
        )

        # Add documents
        docs = [Document(content="Hello world", metadata={"type": "greeting"})]
        await store.add(docs)

        # Search
        results = await store.search("greeting", k=5)
        ```
    """

    def __init__(
        self,
        index: str,
        *,
        api_key: str | None = None,
        namespace: str = "",
        embedding_provider: EmbeddingProvider | None = None,
        dimension: int | None = None,
        metric: str = "cosine",
        cloud: str = "aws",
        region: str = "us-east-1",
        **kwargs: Any,
    ) -> None:
        """Initialize Pinecone vector store.

        Args:
            index: Name of the Pinecone index.
            api_key: Pinecone API key. Defaults to PINECONE_API_KEY env var.
            namespace: Optional namespace for multi-tenancy.
            embedding_provider: Embedding provider for automatic embedding.
                Required if not passing pre-computed embeddings.
            dimension: Vector dimension (required for index creation).
            metric: Distance metric (cosine, euclidean, dotproduct).
            cloud: Cloud provider for serverless spec (aws, gcp, azure).
            region: Cloud region for serverless spec.
            **kwargs: Additional configuration.
        """
        if not HAS_PINECONE:
            raise ImportError(
                "pinecone package not installed. "
                "Install with: pip install duragraph-python[pinecone]"
            )

        super().__init__(**kwargs)

        self._api_key = api_key or os.environ.get("PINECONE_API_KEY")
        if not self._api_key:
            raise ValueError("Pinecone API key required. Set PINECONE_API_KEY or pass api_key.")

        self._index_name = index
        self._namespace = namespace
        self._embedding_provider = embedding_provider
        self._dimension = dimension
        self._metric = metric
        self._cloud = cloud
        self._region = region

        # Initialize Pinecone client
        self._client = Pinecone(api_key=self._api_key)

        # Get or create index
        self._ensure_index()
        self._index = self._client.Index(index)

    def _ensure_index(self) -> None:
        """Ensure the index exists, creating it if necessary."""
        existing_indexes = [idx.name for idx in self._client.list_indexes()]

        if self._index_name not in existing_indexes:
            # Determine dimension
            dim = self._dimension
            if not dim and self._embedding_provider:
                dim = self._embedding_provider.dimension
            if not dim:
                raise ValueError(
                    "dimension required to create index. "
                    "Provide dimension or use an embedding provider with known dimension."
                )

            self._client.create_index(
                name=self._index_name,
                dimension=dim,
                metric=self._metric,
                spec=ServerlessSpec(cloud=self._cloud, region=self._region),
            )

    async def add(
        self,
        documents: Sequence[Document],
        *,
        embeddings: Sequence[Sequence[float]] | None = None,
        ids: Sequence[str] | None = None,
    ) -> list[str]:
        """Add documents to Pinecone."""
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

        # Prepare vectors for upsert
        vectors = []
        for doc_id, doc, emb in zip(doc_ids, documents, embedding_list, strict=True):
            metadata = dict(doc.metadata)
            metadata["_content"] = doc.content  # Store content in metadata

            vectors.append(
                {
                    "id": doc_id,
                    "values": emb,
                    "metadata": metadata,
                }
            )

        # Upsert in batches (Pinecone limit is 100)
        batch_size = 100
        for i in range(0, len(vectors), batch_size):
            batch = vectors[i : i + batch_size]
            self._index.upsert(vectors=batch, namespace=self._namespace)

        return doc_ids

    async def search(
        self,
        query: str | list[float],
        *,
        k: int = 10,
        filter: dict[str, Any] | None = None,
    ) -> list[SearchResult]:
        """Search for similar documents in Pinecone."""
        # Get query embedding
        if isinstance(query, str):
            if not self._embedding_provider:
                raise ValueError("embedding_provider required for text queries")
            query_embedding = await self._embedding_provider.embed_query(query)
        else:
            query_embedding = query

        # Query Pinecone
        query_kwargs: dict[str, Any] = {
            "vector": query_embedding,
            "top_k": k,
            "include_metadata": True,
            "namespace": self._namespace,
        }
        if filter:
            query_kwargs["filter"] = filter

        results = self._index.query(**query_kwargs)

        # Convert to SearchResult
        search_results = []
        for match in results.matches:
            metadata = dict(match.metadata) if match.metadata else {}
            content = metadata.pop("_content", "")

            search_results.append(
                SearchResult(
                    document=Document(
                        content=content,
                        metadata=metadata,
                        id=match.id,
                    ),
                    score=match.score,
                    id=match.id,
                )
            )

        return search_results

    async def delete(
        self,
        ids: Sequence[str] | None = None,
        *,
        filter: dict[str, Any] | None = None,
    ) -> int:
        """Delete documents from Pinecone."""
        if not ids and not filter:
            return 0

        if ids:
            id_list = list(ids)
            self._index.delete(ids=id_list, namespace=self._namespace)
            return len(id_list)
        elif filter:
            # Pinecone requires IDs for deletion, so we need to query first
            # This is a limitation of Pinecone
            raise NotImplementedError(
                "Filter-based deletion not supported by Pinecone. "
                "Query for IDs first, then delete by ID."
            )

        return 0

    async def get(self, ids: Sequence[str]) -> list[Document]:
        """Get documents by ID from Pinecone."""
        if not ids:
            return []

        results = self._index.fetch(ids=list(ids), namespace=self._namespace)

        documents = []
        for doc_id, vector in results.vectors.items():
            metadata = dict(vector.metadata) if vector.metadata else {}
            content = metadata.pop("_content", "")

            documents.append(
                Document(
                    content=content,
                    metadata=metadata,
                    id=doc_id,
                    embedding=list(vector.values) if vector.values else None,
                )
            )

        return documents

    async def clear(self) -> int:
        """Clear all documents from Pinecone namespace."""
        # Get stats to know how many vectors
        stats = self._index.describe_index_stats()
        ns_stats = stats.namespaces.get(self._namespace or "")
        count = ns_stats.vector_count if ns_stats else 0

        # Delete all in namespace
        self._index.delete(delete_all=True, namespace=self._namespace)

        return count

    async def count(self) -> int:
        """Get the number of documents in the namespace."""
        stats = self._index.describe_index_stats()
        ns_stats = stats.namespaces.get(self._namespace or "")
        return ns_stats.vector_count if ns_stats else 0
