"""Cohere embedding provider integration."""

import os
from collections.abc import Sequence
from typing import Any, Literal

from duragraph.vectorstores.base import EmbeddingProvider

try:
    import cohere

    HAS_COHERE = True
except ImportError:
    HAS_COHERE = False
    cohere = None  # type: ignore


# Known dimensions for Cohere embedding models
COHERE_EMBEDDING_DIMENSIONS = {
    "embed-english-v3.0": 1024,
    "embed-multilingual-v3.0": 1024,
    "embed-english-light-v3.0": 384,
    "embed-multilingual-light-v3.0": 384,
    "embed-english-v2.0": 4096,
    "embed-multilingual-v2.0": 768,
}


class CohereEmbeddings(EmbeddingProvider):
    """Cohere embedding provider.

    Uses Cohere's embedding API to generate dense vector representations of text.
    Supports both English and multilingual models.

    Example:
        ```python
        from duragraph.embeddings import CohereEmbeddings

        # Using default model
        embeddings = CohereEmbeddings()

        # Using multilingual model
        embeddings = CohereEmbeddings(model="embed-multilingual-v3.0")

        # Specify input type for better results
        embeddings = CohereEmbeddings(input_type="search_document")

        # Embed texts
        vectors = await embeddings.embed(["Hello world", "Goodbye world"])
        ```
    """

    def __init__(
        self,
        model: str = "embed-english-v3.0",
        *,
        api_key: str | None = None,
        input_type: Literal[
            "search_document", "search_query", "classification", "clustering"
        ] = "search_document",
        truncate: Literal["NONE", "START", "END"] = "END",
        **kwargs: Any,
    ) -> None:
        """Initialize Cohere embeddings.

        Args:
            model: Cohere embedding model name.
            api_key: Cohere API key. Defaults to COHERE_API_KEY env var.
            input_type: Type of input text (affects embedding optimization):
                - "search_document": For documents to be searched
                - "search_query": For search queries
                - "classification": For text classification
                - "clustering": For text clustering
            truncate: How to handle texts exceeding max length.
            **kwargs: Additional configuration.
        """
        if not HAS_COHERE:
            raise ImportError(
                "cohere package not installed. Install with: pip install duragraph-python[cohere]"
            )

        super().__init__(**kwargs)

        self._model = model
        self._api_key = api_key or os.environ.get("COHERE_API_KEY")
        self._input_type = input_type
        self._truncate = truncate

        if not self._api_key:
            raise ValueError("Cohere API key required. Set COHERE_API_KEY or pass api_key.")

        # Initialize client
        self._client = cohere.AsyncClient(api_key=self._api_key)

    async def embed(self, texts: Sequence[str]) -> list[list[float]]:
        """Generate embeddings for texts.

        Args:
            texts: List of texts to embed.

        Returns:
            List of embedding vectors.
        """
        if not texts:
            return []

        # Cohere API call
        response = await self._client.embed(
            texts=list(texts),
            model=self._model,
            input_type=self._input_type,
            truncate=self._truncate,
        )

        return [list(emb) for emb in response.embeddings]

    async def embed_query(self, text: str) -> list[float]:
        """Generate embedding for a single query.

        Uses "search_query" input type for better query-document matching.

        Args:
            text: Query text.

        Returns:
            Embedding vector.
        """
        # Use search_query input type for queries
        response = await self._client.embed(
            texts=[text],
            model=self._model,
            input_type="search_query",
            truncate=self._truncate,
        )

        return list(response.embeddings[0])

    @property
    def dimension(self) -> int | None:
        """Get the embedding dimension.

        Returns:
            Embedding dimension.
        """
        return COHERE_EMBEDDING_DIMENSIONS.get(self._model)

    @property
    def model(self) -> str:
        """Get the model name."""
        return self._model
