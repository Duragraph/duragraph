"""Base classes for LLM providers."""

from abc import ABC, abstractmethod
from collections.abc import AsyncIterator
from dataclasses import dataclass, field
from typing import Any


@dataclass
class LLMRequest:
    """Request to an LLM provider."""

    messages: list[dict[str, Any]]
    model: str
    temperature: float = 0.7
    max_tokens: int | None = None
    system_prompt: str | None = None
    tools: list[dict[str, Any]] | None = None
    stream: bool = False
    extra_params: dict[str, Any] = field(default_factory=dict)


@dataclass
class LLMResponse:
    """Response from an LLM provider."""

    content: str
    model: str
    usage: dict[str, int] | None = None
    tool_calls: list[dict[str, Any]] | None = None
    finish_reason: str | None = None
    raw_response: Any = None


@dataclass
class StreamChunk:
    """Streaming chunk from an LLM provider."""

    content: str
    model: str
    finish_reason: str | None = None
    tool_calls: list[dict[str, Any]] | None = None
    usage: dict[str, int] | None = None


class LLMProvider(ABC):
    """Abstract base class for LLM providers."""

    def __init__(
        self,
        api_key: str | None = None,
        base_url: str | None = None,
        **kwargs: Any,
    ):
        """Initialize the LLM provider.

        Args:
            api_key: API key for authentication.
            base_url: Optional base URL for the API.
            **kwargs: Additional provider-specific arguments.
        """
        self.api_key = api_key
        self.base_url = base_url
        self.extra_config = kwargs

    @abstractmethod
    async def acomplete(self, request: LLMRequest) -> LLMResponse:
        """Generate a completion asynchronously.

        Args:
            request: The LLM request.

        Returns:
            The LLM response.
        """
        ...

    @abstractmethod
    async def astream(self, request: LLMRequest) -> AsyncIterator[StreamChunk]:
        """Stream a completion asynchronously.

        Args:
            request: The LLM request with stream=True.

        Yields:
            Stream chunks from the LLM.
        """
        ...

    def complete(self, request: LLMRequest) -> LLMResponse:
        """Generate a completion synchronously.

        Args:
            request: The LLM request.

        Returns:
            The LLM response.
        """
        import asyncio

        return asyncio.run(self.acomplete(request))

    @property
    @abstractmethod
    def supported_models(self) -> list[str]:
        """List of model identifiers supported by this provider."""
        ...

    @classmethod
    @abstractmethod
    def from_env(cls) -> "LLMProvider":
        """Create provider instance from environment variables."""
        ...
