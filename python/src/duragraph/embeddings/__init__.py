"""Embedding providers for DuraGraph."""

from duragraph.embeddings.base import (
    EmbeddingData,
    EmbeddingProvider,
    EmbeddingRequest,
    EmbeddingResponse,
    EmbeddingUsage,
)
from duragraph.embeddings.registry import (
    create_embedding_provider,
    get_provider,
    list_providers,
    register_provider,
)

# Import providers if dependencies are available
try:
    from duragraph.embeddings.openai import OpenAIEmbeddingProvider
except ImportError:
    OpenAIEmbeddingProvider = None  # type: ignore

try:
    from duragraph.embeddings.anthropic import AnthropicEmbeddingProvider
except ImportError:
    AnthropicEmbeddingProvider = None  # type: ignore

__all__ = [
    # Base classes
    "EmbeddingProvider",
    "EmbeddingRequest",
    "EmbeddingResponse",
    "EmbeddingData",
    "EmbeddingUsage",
    # Registry functions
    "get_provider",
    "register_provider",
    "list_providers",
    "create_embedding_provider",
    # Provider classes (if available)
    "OpenAIEmbeddingProvider",
    "AnthropicEmbeddingProvider",
]
