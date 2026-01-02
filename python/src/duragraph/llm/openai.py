"""OpenAI LLM provider implementation."""

import os
from collections.abc import AsyncIterator
from typing import Any

from duragraph.llm.base import LLMProvider, LLMRequest, LLMResponse, StreamChunk


class OpenAIProvider(LLMProvider):
    """OpenAI LLM provider using the official SDK."""

    def __init__(
        self,
        api_key: str | None = None,
        base_url: str | None = None,
        organization: str | None = None,
        **kwargs: Any,
    ):
        """Initialize OpenAI provider.

        Args:
            api_key: OpenAI API key. Defaults to OPENAI_API_KEY env var.
            base_url: Optional base URL for API requests.
            organization: Optional organization ID.
            **kwargs: Additional arguments passed to OpenAI client.
        """
        super().__init__(api_key=api_key, base_url=base_url, **kwargs)

        try:
            from openai import AsyncOpenAI
        except ImportError as e:
            raise ImportError(
                "OpenAI SDK not installed. Install with: uv add duragraph-python[openai]"
            ) from e

        self.client = AsyncOpenAI(
            api_key=api_key or os.environ.get("OPENAI_API_KEY"),
            base_url=base_url,
            organization=organization,
            **kwargs,
        )

    async def acomplete(self, request: LLMRequest) -> LLMResponse:
        """Generate a completion using OpenAI.

        Args:
            request: The LLM request.

        Returns:
            The LLM response.
        """
        messages = request.messages.copy()

        # Add system prompt if provided
        if request.system_prompt:
            messages.insert(0, {"role": "system", "content": request.system_prompt})

        # Build API request
        api_request: dict[str, Any] = {
            "model": request.model,
            "messages": messages,
            "temperature": request.temperature,
        }

        if request.max_tokens:
            api_request["max_tokens"] = request.max_tokens

        if request.tools:
            api_request["tools"] = request.tools
            api_request["tool_choice"] = "auto"

        # Add extra params
        api_request.update(request.extra_params)

        # Call OpenAI API
        response = await self.client.chat.completions.create(**api_request)

        # Extract response data
        choice = response.choices[0]
        content = choice.message.content or ""
        tool_calls = None

        if choice.message.tool_calls:
            tool_calls = [
                {
                    "id": tc.id,
                    "type": "function",
                    "function": {
                        "name": tc.function.name,
                        "arguments": tc.function.arguments,
                    },
                }
                for tc in choice.message.tool_calls
            ]

        usage = None
        if response.usage:
            usage = {
                "prompt_tokens": response.usage.prompt_tokens,
                "completion_tokens": response.usage.completion_tokens,
                "total_tokens": response.usage.total_tokens,
            }

        return LLMResponse(
            content=content,
            model=response.model,
            usage=usage,
            tool_calls=tool_calls,
            finish_reason=choice.finish_reason,
            raw_response=response,
        )

    async def astream(self, request: LLMRequest) -> AsyncIterator[StreamChunk]:
        """Stream a completion using OpenAI.

        Args:
            request: The LLM request with stream=True.

        Yields:
            Stream chunks from OpenAI.
        """
        messages = request.messages.copy()

        if request.system_prompt:
            messages.insert(0, {"role": "system", "content": request.system_prompt})

        api_request: dict[str, Any] = {
            "model": request.model,
            "messages": messages,
            "temperature": request.temperature,
            "stream": True,
        }

        if request.max_tokens:
            api_request["max_tokens"] = request.max_tokens

        if request.tools:
            api_request["tools"] = request.tools
            api_request["tool_choice"] = "auto"

        api_request.update(request.extra_params)

        # Stream from OpenAI
        stream = await self.client.chat.completions.create(**api_request)

        async for chunk in stream:
            if not chunk.choices:
                continue

            choice = chunk.choices[0]
            delta = choice.delta

            content = delta.content or ""
            finish_reason = choice.finish_reason

            tool_calls = None
            if delta.tool_calls:
                tool_calls = [
                    {
                        "index": tc.index,
                        "id": tc.id,
                        "type": "function",
                        "function": {
                            "name": tc.function.name if tc.function else None,
                            "arguments": tc.function.arguments if tc.function else None,
                        },
                    }
                    for tc in delta.tool_calls
                ]

            usage = None
            if hasattr(chunk, "usage") and chunk.usage:
                usage = {
                    "prompt_tokens": chunk.usage.prompt_tokens,
                    "completion_tokens": chunk.usage.completion_tokens,
                    "total_tokens": chunk.usage.total_tokens,
                }

            yield StreamChunk(
                content=content,
                model=chunk.model,
                finish_reason=finish_reason,
                tool_calls=tool_calls,
                usage=usage,
            )

    @property
    def supported_models(self) -> list[str]:
        """List of OpenAI models supported."""
        return [
            # GPT-4o models
            "gpt-4o",
            "gpt-4o-mini",
            "gpt-4o-2024-11-20",
            "gpt-4o-2024-08-06",
            "gpt-4o-2024-05-13",
            "gpt-4o-mini-2024-07-18",
            # GPT-4 Turbo models
            "gpt-4-turbo",
            "gpt-4-turbo-2024-04-09",
            "gpt-4-turbo-preview",
            "gpt-4-0125-preview",
            "gpt-4-1106-preview",
            # GPT-4 models
            "gpt-4",
            "gpt-4-0613",
            "gpt-4-0314",
            # GPT-3.5 models
            "gpt-3.5-turbo",
            "gpt-3.5-turbo-0125",
            "gpt-3.5-turbo-1106",
            # o1 reasoning models
            "o1-preview",
            "o1-mini",
        ]

    @classmethod
    def from_env(cls) -> "OpenAIProvider":
        """Create OpenAI provider from environment variables.

        Environment variables:
            OPENAI_API_KEY: Required API key
            OPENAI_BASE_URL: Optional base URL
            OPENAI_ORGANIZATION: Optional organization ID

        Returns:
            Configured OpenAI provider.
        """
        api_key = os.environ.get("OPENAI_API_KEY")
        if not api_key:
            raise ValueError("OPENAI_API_KEY environment variable not set")

        return cls(
            api_key=api_key,
            base_url=os.environ.get("OPENAI_BASE_URL"),
            organization=os.environ.get("OPENAI_ORGANIZATION"),
        )
