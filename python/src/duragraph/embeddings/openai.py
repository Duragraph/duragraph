"""OpenAI embedding provider implementation."""

from typing import Any

from duragraph.embeddings.base import (
    EmbeddingData,
    EmbeddingProvider,
    EmbeddingRequest,
    EmbeddingResponse,
    EmbeddingUsage,
)


class OpenAIEmbeddingProvider(EmbeddingProvider):
    """OpenAI embedding provider using the OpenAI API."""

    def __init__(
        self,
        api_key: str | None = None,
        model: str = "text-embedding-3-small",
        base_url: str | None = None,
        **kwargs: Any,
    ):
        """Initialize OpenAI embedding provider.

        Args:
            api_key: OpenAI API key. If None, reads from OPENAI_API_KEY env var.
            model: Default model to use for embeddings.
            base_url: Optional base URL for API requests.
            **kwargs: Additional client configuration.
        """
        self.model = model
        self._client = None
        self._aclient = None
        self._api_key = api_key
        self._base_url = base_url
        self._client_kwargs = kwargs

        # Model dimensions mapping
        self._dimensions = {
            "text-embedding-3-small": 1536,
            "text-embedding-3-large": 3072,
            "text-embedding-ada-002": 1536,
        }

    def _get_client(self):
        """Get or create sync OpenAI client."""
        if self._client is None:
            try:
                from openai import OpenAI
            except ImportError as e:
                raise ImportError(
                    "OpenAI package not found. Install with: pip install openai"
                ) from e

            self._client = OpenAI(
                api_key=self._api_key, base_url=self._base_url, **self._client_kwargs
            )
        return self._client

    def _get_aclient(self):
        """Get or create async OpenAI client."""
        if self._aclient is None:
            try:
                from openai import AsyncOpenAI
            except ImportError as e:
                raise ImportError(
                    "OpenAI package not found. Install with: pip install openai"
                ) from e

            self._aclient = AsyncOpenAI(
                api_key=self._api_key, base_url=self._base_url, **self._client_kwargs
            )
        return self._aclient

    async def aembed_documents(self, texts: list[str], **kwargs: Any) -> list[list[float]]:
        """Embed multiple documents asynchronously."""
        model = kwargs.get("model", self.model)
        encoding_format = kwargs.get("encoding_format", "float")
        dimensions = kwargs.get("dimensions")

        client = self._get_aclient()

        # OpenAI has a limit on batch size, so chunk if needed
        batch_size = 2048  # Conservative batch size
        all_embeddings = []

        for i in range(0, len(texts), batch_size):
            batch = texts[i : i + batch_size]

            response = await client.embeddings.create(
                input=batch,
                model=model,
                encoding_format=encoding_format,
                dimensions=dimensions,
            )

            batch_embeddings = [item.embedding for item in response.data]
            all_embeddings.extend(batch_embeddings)

        return all_embeddings

    async def aembed_query(self, text: str, **kwargs: Any) -> list[float]:
        """Embed a single query text asynchronously."""
        embeddings = await self.aembed_documents([text], **kwargs)
        return embeddings[0]

    @property
    def dimension(self) -> int:
        """Get the dimensionality of embeddings produced by this provider."""
        return self._dimensions.get(self.model, 1536)  # Default to 1536

    def create_request(self, input: str | list[str], **kwargs: Any) -> EmbeddingRequest:
        """Create a request object for the OpenAI API."""
        model = kwargs.get("model", self.model)
        encoding_format = kwargs.get("encoding_format")
        dimensions = kwargs.get("dimensions")
        user = kwargs.get("user")

        return EmbeddingRequest(
            input=input,
            model=model,
            encoding_format=encoding_format,
            dimensions=dimensions,
            user=user,
        )

    async def send_request(self, request: EmbeddingRequest) -> EmbeddingResponse:
        """Send request to OpenAI API and get response."""
        client = self._get_aclient()

        response = await client.embeddings.create(
            input=request.input,
            model=request.model,
            encoding_format=request.encoding_format,
            dimensions=request.dimensions,
            user=request.user,
        )

        # Convert OpenAI response to our format
        data = [
            EmbeddingData(
                embedding=item.embedding,
                index=item.index,
            )
            for item in response.data
        ]

        return EmbeddingResponse(
            data=data,
            model=response.model,
            usage=EmbeddingUsage(
                prompt_tokens=response.usage.prompt_tokens,
                total_tokens=response.usage.total_tokens,
            ),
        )
