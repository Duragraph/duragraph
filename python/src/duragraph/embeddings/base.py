"""Base classes for embedding providers."""

from abc import ABC, abstractmethod
from typing import Any

from pydantic import BaseModel


class EmbeddingRequest(BaseModel):
    """Request for generating embeddings."""

    input: str | list[str]  # Text or list of texts to embed
    model: str  # Model identifier
    encoding_format: str | None = None  # Optional encoding format (e.g., "float", "base64")
    dimensions: int | None = None  # Optional output dimensionality
    user: str | None = None  # Optional user identifier


class EmbeddingData(BaseModel):
    """Individual embedding data."""

    object: str = "embedding"
    embedding: list[float]
    index: int


class EmbeddingUsage(BaseModel):
    """Token usage information."""

    prompt_tokens: int
    total_tokens: int


class EmbeddingResponse(BaseModel):
    """Response from embedding generation."""

    object: str = "list"
    data: list[EmbeddingData]
    model: str
    usage: EmbeddingUsage


class EmbeddingProvider(ABC):
    """Abstract base class for embedding providers."""

    def __init__(self, **kwargs: Any):
        """Initialize provider with configuration."""
        self.config = kwargs

    @abstractmethod
    async def aembed_documents(self, texts: list[str], **kwargs: Any) -> list[list[float]]:
        """Embed multiple documents asynchronously.

        Args:
            texts: List of texts to embed.
            **kwargs: Additional model-specific parameters.

        Returns:
            List of embeddings (one per text).
        """

    @abstractmethod
    async def aembed_query(self, text: str, **kwargs: Any) -> list[float]:
        """Embed a single query text asynchronously.

        Args:
            text: Text to embed.
            **kwargs: Additional model-specific parameters.

        Returns:
            Single embedding vector.
        """

    def embed_documents(self, texts: list[str], **kwargs: Any) -> list[list[float]]:
        """Embed multiple documents synchronously.

        Args:
            texts: List of texts to embed.
            **kwargs: Additional model-specific parameters.

        Returns:
            List of embeddings (one per text).
        """
        import asyncio

        try:
            # Try to get existing loop
            loop = asyncio.get_event_loop()
            if loop.is_running():
                # If loop is running, we need to use run_in_executor
                import concurrent.futures

                import nest_asyncio

                try:
                    nest_asyncio.apply()
                    return loop.run_until_complete(self.aembed_documents(texts, **kwargs))
                except ImportError:
                    # nest_asyncio not available, run in thread
                    with concurrent.futures.ThreadPoolExecutor() as executor:
                        future = executor.submit(
                            asyncio.run, self.aembed_documents(texts, **kwargs)
                        )
                        return future.result()
            else:
                return loop.run_until_complete(self.aembed_documents(texts, **kwargs))
        except RuntimeError:
            # No event loop exists, create one
            return asyncio.run(self.aembed_documents(texts, **kwargs))

    def embed_query(self, text: str, **kwargs: Any) -> list[float]:
        """Embed a single query text synchronously.

        Args:
            text: Text to embed.
            **kwargs: Additional model-specific parameters.

        Returns:
            Single embedding vector.
        """
        import asyncio

        try:
            # Try to get existing loop
            loop = asyncio.get_event_loop()
            if loop.is_running():
                # If loop is running, we need to use run_in_executor
                import concurrent.futures

                import nest_asyncio

                try:
                    nest_asyncio.apply()
                    return loop.run_until_complete(self.aembed_query(text, **kwargs))
                except ImportError:
                    # nest_asyncio not available, run in thread
                    with concurrent.futures.ThreadPoolExecutor() as executor:
                        future = executor.submit(asyncio.run, self.aembed_query(text, **kwargs))
                        return future.result()
            else:
                return loop.run_until_complete(self.aembed_query(text, **kwargs))
        except RuntimeError:
            # No event loop exists, create one
            return asyncio.run(self.aembed_query(text, **kwargs))

    @property
    @abstractmethod
    def dimension(self) -> int:
        """Get the dimensionality of embeddings produced by this provider."""

    @abstractmethod
    def create_request(self, input: str | list[str], **kwargs: Any) -> EmbeddingRequest:
        """Create a request object for the provider API."""

    @abstractmethod
    async def send_request(self, request: EmbeddingRequest) -> EmbeddingResponse:
        """Send request to provider API and get response."""
