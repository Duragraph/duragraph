"""Vector store registry and utilities."""

from typing import Any

from duragraph.vectorstores.base import VectorStore

# Global registry of vector store implementations
_stores: dict[str, type[VectorStore]] = {}


def register_vectorstore(name: str, store_class: type[VectorStore]) -> None:
    """Register a vector store implementation.

    Args:
        name: Store name (e.g., "memory", "chroma", "pinecone").
        store_class: Store class implementing VectorStore.
    """
    _stores[name] = store_class


def get_vectorstore(name: str, embedding_function: Any = None, **kwargs: Any) -> VectorStore:
    """Get a vector store instance.

    Args:
        name: Store name.
        embedding_function: Function to generate embeddings.
        **kwargs: Store-specific initialization arguments.

    Returns:
        Initialized vector store.

    Raises:
        ValueError: If store not found.
    """
    if name not in _stores:
        raise ValueError(f"Unknown vector store: {name}. Available: {list(_stores.keys())}")

    store_class = _stores[name]
    return store_class(embedding_function=embedding_function, **kwargs)


def list_vectorstores() -> list[str]:
    """List all registered vector stores."""
    return list(_stores.keys())


def create_vectorstore(
    store_type: str, embedding_function: Any = None, **kwargs: Any
) -> VectorStore:
    """Create a vector store with common parameters.

    Args:
        store_type: Type of store ("memory", "chroma", etc.).
        embedding_function: Function to generate embeddings.
        **kwargs: Additional store-specific parameters.

    Returns:
        Configured vector store.
    """
    return get_vectorstore(store_type, embedding_function, **kwargs)


# Register built-in vector stores
def _register_builtin_stores() -> None:
    """Register built-in vector store implementations."""
    # Always available in-memory store
    from duragraph.vectorstores.memory import InMemoryVectorStore

    register_vectorstore("memory", InMemoryVectorStore)

    # ChromaDB store (if available)
    try:
        from duragraph.vectorstores.chroma import ChromaVectorStore

        register_vectorstore("chroma", ChromaVectorStore)
    except ImportError:
        pass  # ChromaDB not available


# Auto-register on import
_register_builtin_stores()
