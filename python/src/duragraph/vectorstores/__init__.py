"""Vector stores for DuraGraph."""

from duragraph.vectorstores.base import Document, VectorStore, VectorStoreQuery, VectorStoreResult
from duragraph.vectorstores.registry import (
    create_vectorstore,
    get_vectorstore,
    list_vectorstores,
    register_vectorstore,
)

# Import stores if dependencies are available
try:
    from duragraph.vectorstores.memory import InMemoryVectorStore
except ImportError:
    InMemoryVectorStore = None  # type: ignore

try:
    from duragraph.vectorstores.chroma import ChromaVectorStore
except ImportError:
    ChromaVectorStore = None  # type: ignore

try:
    from duragraph.vectorstores.qdrant import QdrantVectorStore
except ImportError:
    QdrantVectorStore = None  # type: ignore

try:
    from duragraph.vectorstores.weaviate import WeaviateVectorStore
except ImportError:
    WeaviateVectorStore = None  # type: ignore

try:
    from duragraph.vectorstores.pgvector import PgVectorStore
except ImportError:
    PgVectorStore = None  # type: ignore

try:
    from duragraph.vectorstores.pinecone import PineconeVectorStore
except ImportError:
    PineconeVectorStore = None  # type: ignore

__all__ = [
    # Base classes
    "VectorStore",
    "Document",
    "VectorStoreQuery",
    "VectorStoreResult",
    # Registry functions
    "get_vectorstore",
    "register_vectorstore",
    "list_vectorstores",
    "create_vectorstore",
    # Store implementations (if available)
    "InMemoryVectorStore",
    "ChromaVectorStore",
    "QdrantVectorStore",
    "WeaviateVectorStore",
    "PgVectorStore",
    "PineconeVectorStore",
]
