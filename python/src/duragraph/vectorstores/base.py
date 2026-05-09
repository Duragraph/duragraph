"""Base classes for vector stores."""

from abc import ABC, abstractmethod
from typing import Any

from pydantic import BaseModel


class Document(BaseModel):
    """A document with content and metadata."""

    page_content: str
    metadata: dict[str, Any] = {}

    def __str__(self) -> str:
        return self.page_content


class VectorStoreQuery(BaseModel):
    """Query for vector store search."""

    query_embedding: list[float] | None = None
    query_text: str | None = None
    k: int = 4  # Number of results to return
    filter: dict[str, Any] | None = None  # Metadata filter
    include_metadata: bool = True
    include_distances: bool = False


class VectorStoreResult(BaseModel):
    """Result from vector store search."""

    documents: list[Document]
    distances: list[float] | None = None
    metadatas: list[dict[str, Any]] | None = None
    ids: list[str] | None = None


class VectorStore(ABC):
    """Abstract base class for vector stores."""

    def __init__(self, embedding_function: Any = None, **kwargs: Any):
        """Initialize vector store.

        Args:
            embedding_function: Function to generate embeddings from text.
            **kwargs: Additional store-specific configuration.
        """
        self.embedding_function = embedding_function

    @abstractmethod
    async def aadd_documents(
        self, documents: list[Document], ids: list[str] | None = None, **kwargs: Any
    ) -> list[str]:
        """Add documents to the vector store asynchronously.

        Args:
            documents: Documents to add.
            ids: Optional document IDs. Generated if not provided.
            **kwargs: Additional store-specific parameters.

        Returns:
            List of document IDs.
        """

    @abstractmethod
    async def asimilarity_search(
        self, query: str, k: int = 4, filter: dict[str, Any] | None = None, **kwargs: Any
    ) -> list[Document]:
        """Search for similar documents asynchronously.

        Args:
            query: Query text.
            k: Number of results to return.
            filter: Optional metadata filter.
            **kwargs: Additional search parameters.

        Returns:
            List of similar documents.
        """

    @abstractmethod
    async def asimilarity_search_with_score(
        self, query: str, k: int = 4, filter: dict[str, Any] | None = None, **kwargs: Any
    ) -> list[tuple[Document, float]]:
        """Search for similar documents with similarity scores asynchronously.

        Args:
            query: Query text.
            k: Number of results to return.
            filter: Optional metadata filter.
            **kwargs: Additional search parameters.

        Returns:
            List of (document, score) tuples.
        """

    async def asimilarity_search_by_vector(
        self,
        embedding: list[float],
        k: int = 4,
        filter: dict[str, Any] | None = None,
        **kwargs: Any,
    ) -> list[Document]:
        """Search for similar documents by embedding vector asynchronously.

        Args:
            embedding: Query embedding vector.
            k: Number of results to return.
            filter: Optional metadata filter.
            **kwargs: Additional search parameters.

        Returns:
            List of similar documents.
        """
        # Default implementation calls similarity_search_with_score_by_vector
        results = await self.asimilarity_search_with_score_by_vector(embedding, k, filter, **kwargs)
        return [doc for doc, _ in results]

    @abstractmethod
    async def asimilarity_search_with_score_by_vector(
        self,
        embedding: list[float],
        k: int = 4,
        filter: dict[str, Any] | None = None,
        **kwargs: Any,
    ) -> list[tuple[Document, float]]:
        """Search for similar documents by embedding with scores asynchronously.

        Args:
            embedding: Query embedding vector.
            k: Number of results to return.
            filter: Optional metadata filter.
            **kwargs: Additional search parameters.

        Returns:
            List of (document, score) tuples.
        """

    async def adelete(
        self,
        ids: list[str] | None = None,
        filter: dict[str, Any] | None = None,
        **kwargs: Any,
    ) -> bool | None:
        """Delete documents from the vector store asynchronously.

        Args:
            ids: Document IDs to delete.
            filter: Metadata filter for documents to delete.
            **kwargs: Additional parameters.

        Returns:
            True if successful, None if not implemented.
        """
        return None  # Default: not implemented

    def add_documents(
        self, documents: list[Document], ids: list[str] | None = None, **kwargs: Any
    ) -> list[str]:
        """Add documents to the vector store synchronously."""
        import asyncio

        return asyncio.run(self.aadd_documents(documents, ids, **kwargs))

    def similarity_search(
        self, query: str, k: int = 4, filter: dict[str, Any] | None = None, **kwargs: Any
    ) -> list[Document]:
        """Search for similar documents synchronously."""
        import asyncio

        return asyncio.run(self.asimilarity_search(query, k, filter, **kwargs))

    def similarity_search_with_score(
        self, query: str, k: int = 4, filter: dict[str, Any] | None = None, **kwargs: Any
    ) -> list[tuple[Document, float]]:
        """Search for similar documents with scores synchronously."""
        import asyncio

        return asyncio.run(self.asimilarity_search_with_score(query, k, filter, **kwargs))

    def similarity_search_by_vector(
        self,
        embedding: list[float],
        k: int = 4,
        filter: dict[str, Any] | None = None,
        **kwargs: Any,
    ) -> list[Document]:
        """Search for similar documents by vector synchronously."""
        import asyncio

        return asyncio.run(self.asimilarity_search_by_vector(embedding, k, filter, **kwargs))

    def similarity_search_with_score_by_vector(
        self,
        embedding: list[float],
        k: int = 4,
        filter: dict[str, Any] | None = None,
        **kwargs: Any,
    ) -> list[tuple[Document, float]]:
        """Search for similar documents by vector with scores synchronously."""
        import asyncio

        return asyncio.run(
            self.asimilarity_search_with_score_by_vector(embedding, k, filter, **kwargs)
        )

    def delete(
        self,
        ids: list[str] | None = None,
        filter: dict[str, Any] | None = None,
        **kwargs: Any,
    ) -> bool | None:
        """Delete documents from the vector store synchronously."""
        import asyncio

        return asyncio.run(self.adelete(ids, filter, **kwargs))

    @classmethod
    def from_documents(
        cls, documents: list[Document], embedding_function: Any, **kwargs: Any
    ) -> "VectorStore":
        """Create vector store from documents.

        Args:
            documents: Initial documents to add.
            embedding_function: Function to generate embeddings.
            **kwargs: Additional store configuration.

        Returns:
            Initialized vector store.
        """
        store = cls(embedding_function=embedding_function, **kwargs)
        store.add_documents(documents)
        return store

    @classmethod
    def from_texts(
        cls,
        texts: list[str],
        embedding_function: Any,
        metadatas: list[dict[str, Any]] | None = None,
        **kwargs: Any,
    ) -> "VectorStore":
        """Create vector store from texts.

        Args:
            texts: List of text strings.
            embedding_function: Function to generate embeddings.
            metadatas: Optional metadata for each text.
            **kwargs: Additional store configuration.

        Returns:
            Initialized vector store.
        """
        documents = []
        for i, text in enumerate(texts):
            metadata = metadatas[i] if metadatas and i < len(metadatas) else {}
            documents.append(Document(page_content=text, metadata=metadata))

        return cls.from_documents(documents, embedding_function, **kwargs)
