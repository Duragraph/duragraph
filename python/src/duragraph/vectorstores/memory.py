"""In-memory vector store implementation."""

import math
import uuid
from typing import Any

from duragraph.vectorstores.base import Document, VectorStore


class InMemoryVectorStore(VectorStore):
    """Simple in-memory vector store for development and testing."""

    def __init__(self, embedding_function: Any = None, **kwargs: Any):
        """Initialize in-memory vector store.

        Args:
            embedding_function: Function to generate embeddings from text.
            **kwargs: Additional configuration (ignored for in-memory store).
        """
        super().__init__(embedding_function, **kwargs)
        self._documents: dict[str, Document] = {}
        self._embeddings: dict[str, list[float]] = {}

    async def aadd_documents(
        self, documents: list[Document], ids: list[str] | None = None, **kwargs: Any
    ) -> list[str]:
        """Add documents to the in-memory store asynchronously."""
        if ids is None:
            ids = [str(uuid.uuid4()) for _ in documents]

        if len(ids) != len(documents):
            raise ValueError("Number of ids must match number of documents")

        # Generate embeddings for documents
        embeddings = []
        for doc in documents:
            if self.embedding_function:
                if hasattr(self.embedding_function, "aembed_query"):
                    # Async embedding function
                    embedding = await self.embedding_function.aembed_query(doc.page_content)
                elif hasattr(self.embedding_function, "embed_query"):
                    # Sync embedding function
                    embedding = self.embedding_function.embed_query(doc.page_content)
                else:
                    # Callable embedding function
                    embedding = self.embedding_function(doc.page_content)
            else:
                # Default: simple hash-based embedding for testing
                embedding = self._hash_embedding(doc.page_content)
            embeddings.append(embedding)

        # Store documents and embeddings
        for doc_id, doc, embedding in zip(ids, documents, embeddings, strict=False):
            self._documents[doc_id] = doc
            self._embeddings[doc_id] = embedding

        return ids

    def _hash_embedding(self, text: str, dimension: int = 768) -> list[float]:
        """Generate a deterministic hash-based embedding for testing."""
        import hashlib
        import struct

        # Create deterministic hash
        hash_obj = hashlib.sha256(text.encode("utf-8"))
        hash_bytes = hash_obj.digest()

        # Convert to float vector
        embedding = []
        for i in range(0, min(len(hash_bytes), dimension * 4), 4):
            if i + 4 <= len(hash_bytes):
                float_val = struct.unpack("f", hash_bytes[i : i + 4])[0]
            else:
                # Pad with normalized values
                float_val = (i % 256) / 256.0 - 0.5
            embedding.append(float_val)

        # Pad to target dimension
        while len(embedding) < dimension:
            embedding.append(0.0)

        return embedding[:dimension]

    def _cosine_similarity(self, a: list[float], b: list[float]) -> float:
        """Calculate cosine similarity between two vectors."""
        dot_product = sum(x * y for x, y in zip(a, b, strict=False))
        norm_a = math.sqrt(sum(x * x for x in a))
        norm_b = math.sqrt(sum(x * x for x in b))

        if norm_a == 0 or norm_b == 0:
            return 0.0

        return dot_product / (norm_a * norm_b)

    async def asimilarity_search(
        self, query: str, k: int = 4, filter: dict[str, Any] | None = None, **kwargs: Any
    ) -> list[Document]:
        """Search for similar documents asynchronously."""
        results = await self.asimilarity_search_with_score(query, k, filter, **kwargs)
        return [doc for doc, _ in results]

    async def asimilarity_search_with_score(
        self, query: str, k: int = 4, filter: dict[str, Any] | None = None, **kwargs: Any
    ) -> list[tuple[Document, float]]:
        """Search for similar documents with scores asynchronously."""
        # Generate embedding for query
        if self.embedding_function:
            if hasattr(self.embedding_function, "aembed_query"):
                # Async embedding function
                query_embedding = await self.embedding_function.aembed_query(query)
            elif hasattr(self.embedding_function, "embed_query"):
                # Sync embedding function
                query_embedding = self.embedding_function.embed_query(query)
            else:
                # Callable embedding function
                query_embedding = self.embedding_function(query)
        else:
            # Default: hash-based embedding
            query_embedding = self._hash_embedding(query)

        return await self.asimilarity_search_with_score_by_vector(
            query_embedding, k, filter, **kwargs
        )

    async def asimilarity_search_with_score_by_vector(
        self,
        embedding: list[float],
        k: int = 4,
        filter: dict[str, Any] | None = None,
        **kwargs: Any,
    ) -> list[tuple[Document, float]]:
        """Search for similar documents by embedding with scores asynchronously."""
        candidates = []

        for doc_id, doc in self._documents.items():
            # Apply metadata filter if specified
            if filter:
                if not self._matches_filter(doc.metadata, filter):
                    continue

            # Calculate similarity
            doc_embedding = self._embeddings[doc_id]
            similarity = self._cosine_similarity(embedding, doc_embedding)
            candidates.append((doc, similarity, doc_id))

        # Sort by similarity (highest first) and take top k
        candidates.sort(key=lambda x: x[1], reverse=True)
        results = [(doc, score) for doc, score, _ in candidates[:k]]

        return results

    def _matches_filter(self, metadata: dict[str, Any], filter: dict[str, Any]) -> bool:
        """Check if document metadata matches filter criteria."""
        for key, value in filter.items():
            if key not in metadata:
                return False

            doc_value = metadata[key]

            # Handle different filter types
            if isinstance(value, dict):
                # Complex filter (e.g., {"$eq": "value"}, {"$in": [1, 2, 3]})
                for op, filter_value in value.items():
                    if op == "$eq":
                        if doc_value != filter_value:
                            return False
                    elif op == "$ne":
                        if doc_value == filter_value:
                            return False
                    elif op == "$in":
                        if doc_value not in filter_value:
                            return False
                    elif op == "$nin":
                        if doc_value in filter_value:
                            return False
                    elif op == "$gt":
                        if not (doc_value > filter_value):
                            return False
                    elif op == "$gte":
                        if not (doc_value >= filter_value):
                            return False
                    elif op == "$lt":
                        if not (doc_value < filter_value):
                            return False
                    elif op == "$lte":
                        if not (doc_value <= filter_value):
                            return False
                    else:
                        # Unknown operator, ignore
                        continue
            else:
                # Simple equality filter
                if doc_value != value:
                    return False

        return True

    async def adelete(
        self,
        ids: list[str] | None = None,
        filter: dict[str, Any] | None = None,
        **kwargs: Any,
    ) -> bool:
        """Delete documents from the vector store asynchronously."""
        if ids:
            # Delete by IDs
            deleted_count = 0
            for doc_id in ids:
                if doc_id in self._documents:
                    del self._documents[doc_id]
                    del self._embeddings[doc_id]
                    deleted_count += 1
            return deleted_count > 0
        elif filter:
            # Delete by filter
            to_delete = []
            for doc_id, doc in self._documents.items():
                if self._matches_filter(doc.metadata, filter):
                    to_delete.append(doc_id)

            for doc_id in to_delete:
                del self._documents[doc_id]
                del self._embeddings[doc_id]

            return len(to_delete) > 0
        else:
            # Clear all
            self._documents.clear()
            self._embeddings.clear()
            return True

    def get_document_count(self) -> int:
        """Get the number of documents in the store."""
        return len(self._documents)

    def list_document_ids(self) -> list[str]:
        """List all document IDs in the store."""
        return list(self._documents.keys())

    def get_document_by_id(self, doc_id: str) -> Document | None:
        """Get a document by its ID."""
        return self._documents.get(doc_id)
