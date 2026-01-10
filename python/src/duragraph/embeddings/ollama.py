"""Ollama embedding provider integration."""

from collections.abc import Sequence
from typing import Any

from duragraph.vectorstores.base import EmbeddingProvider

try:
    import ollama
    from ollama import AsyncClient

    HAS_OLLAMA = True
except ImportError:
    HAS_OLLAMA = False
    ollama = None  # type: ignore
    AsyncClient = None  # type: ignore


# Known dimensions for popular Ollama embedding models
OLLAMA_EMBEDDING_DIMENSIONS = {
    "nomic-embed-text": 768,
    "mxbai-embed-large": 1024,
    "all-minilm": 384,
    "snowflake-arctic-embed": 1024,
    "bge-m3": 1024,
    "bge-large": 1024,
}


class OllamaEmbeddings(EmbeddingProvider):
    """Ollama embedding provider.

    Uses Ollama to run embedding models locally. No API key required.
    Supports various open-source embedding models.

    Example:
        ```python
        from duragraph.embeddings import OllamaEmbeddings

        # Using default model
        embeddings = OllamaEmbeddings()  # Uses nomic-embed-text

        # Using a specific model
        embeddings = OllamaEmbeddings(model="mxbai-embed-large")

        # Custom Ollama server
        embeddings = OllamaEmbeddings(
            model="nomic-embed-text",
            host="http://localhost:11434"
        )

        # Embed texts
        vectors = await embeddings.embed(["Hello world", "Goodbye world"])
        ```

    Note:
        Make sure the model is pulled before using:
        ```bash
        ollama pull nomic-embed-text
        ```
    """

    def __init__(
        self,
        model: str = "nomic-embed-text",
        *,
        host: str | None = None,
        **kwargs: Any,
    ) -> None:
        """Initialize Ollama embeddings.

        Args:
            model: Ollama embedding model name.
            host: Ollama server URL. Defaults to http://localhost:11434.
            **kwargs: Additional configuration.
        """
        if not HAS_OLLAMA:
            raise ImportError(
                "ollama package not installed. Install with: pip install duragraph-python[ollama]"
            )

        super().__init__(**kwargs)

        self._model = model
        self._host = host or "http://localhost:11434"

        # Initialize client
        self._client = AsyncClient(host=self._host)

    async def embed(self, texts: Sequence[str]) -> list[list[float]]:
        """Generate embeddings for texts.

        Args:
            texts: List of texts to embed.

        Returns:
            List of embedding vectors.
        """
        if not texts:
            return []

        embeddings = []
        for text in texts:
            response = await self._client.embeddings(
                model=self._model,
                prompt=text,
            )
            embeddings.append(response["embedding"])

        return embeddings

    async def embed_query(self, text: str) -> list[float]:
        """Generate embedding for a single query.

        Args:
            text: Query text.

        Returns:
            Embedding vector.
        """
        response = await self._client.embeddings(
            model=self._model,
            prompt=text,
        )
        return response["embedding"]

    @property
    def dimension(self) -> int | None:
        """Get the embedding dimension.

        Returns:
            Embedding dimension, or None if unknown.
        """
        return OLLAMA_EMBEDDING_DIMENSIONS.get(self._model)

    @property
    def model(self) -> str:
        """Get the model name."""
        return self._model

    async def pull_model(self) -> None:
        """Pull the model if not already downloaded.

        Convenience method to ensure the model is available.
        """
        await self._client.pull(self._model)
