"""Anthropic LLM provider implementation."""

import os
from collections.abc import AsyncIterator
from typing import Any

from duragraph.llm.base import LLMProvider, LLMRequest, LLMResponse, StreamChunk


class AnthropicProvider(LLMProvider):
    """Anthropic LLM provider using the official SDK."""

    def __init__(
        self,
        api_key: str | None = None,
        base_url: str | None = None,
        **kwargs: Any,
    ):
        """Initialize Anthropic provider.

        Args:
            api_key: Anthropic API key. Defaults to ANTHROPIC_API_KEY env var.
            base_url: Optional base URL for API requests.
            **kwargs: Additional arguments passed to Anthropic client.
        """
        super().__init__(api_key=api_key, base_url=base_url, **kwargs)

        try:
            from anthropic import AsyncAnthropic
        except ImportError as e:
            raise ImportError(
                "Anthropic SDK not installed. Install with: uv add duragraph-python[anthropic]"
            ) from e

        self.client = AsyncAnthropic(
            api_key=api_key or os.environ.get("ANTHROPIC_API_KEY"),
            base_url=base_url,
            **kwargs,
        )

    def _convert_messages(
        self, messages: list[dict[str, Any]], system_prompt: str | None = None
    ) -> tuple[list[dict[str, Any]], str | None]:
        """Convert messages to Anthropic format.

        Anthropic requires system messages to be separate from conversation messages.

        Args:
            messages: List of message dicts.
            system_prompt: Optional system prompt.

        Returns:
            Tuple of (converted_messages, system_content).
        """
        converted = []
        system_content = system_prompt

        for msg in messages:
            role = msg.get("role", "user")
            content = msg.get("content", "")

            if role == "system":
                # Extract system messages
                if system_content:
                    system_content += "\n\n" + content
                else:
                    system_content = content
            else:
                # Map roles
                if role == "assistant":
                    converted.append({"role": "assistant", "content": content})
                else:
                    converted.append({"role": "user", "content": content})

        return converted, system_content

    async def acomplete(self, request: LLMRequest) -> LLMResponse:
        """Generate a completion using Anthropic.

        Args:
            request: The LLM request.

        Returns:
            The LLM response.
        """
        messages, system = self._convert_messages(request.messages, request.system_prompt)

        # Build API request
        api_request: dict[str, Any] = {
            "model": request.model,
            "messages": messages,
            "temperature": request.temperature,
            "max_tokens": request.max_tokens or 4096,  # Required by Anthropic
        }

        if system:
            api_request["system"] = system

        if request.tools:
            api_request["tools"] = request.tools

        # Add extra params
        api_request.update(request.extra_params)

        # Call Anthropic API
        response = await self.client.messages.create(**api_request)

        # Extract response data
        content = ""
        tool_calls = None

        for block in response.content:
            if block.type == "text":
                content += block.text
            elif block.type == "tool_use":
                if tool_calls is None:
                    tool_calls = []
                tool_calls.append(
                    {
                        "id": block.id,
                        "type": "function",
                        "function": {
                            "name": block.name,
                            "arguments": str(block.input),  # Anthropic provides dict
                        },
                    }
                )

        usage = {
            "prompt_tokens": response.usage.input_tokens,
            "completion_tokens": response.usage.output_tokens,
            "total_tokens": response.usage.input_tokens + response.usage.output_tokens,
        }

        return LLMResponse(
            content=content,
            model=response.model,
            usage=usage,
            tool_calls=tool_calls,
            finish_reason=response.stop_reason,
            raw_response=response,
        )

    async def astream(self, request: LLMRequest) -> AsyncIterator[StreamChunk]:
        """Stream a completion using Anthropic.

        Args:
            request: The LLM request with stream=True.

        Yields:
            Stream chunks from Anthropic.
        """
        messages, system = self._convert_messages(request.messages, request.system_prompt)

        api_request: dict[str, Any] = {
            "model": request.model,
            "messages": messages,
            "temperature": request.temperature,
            "max_tokens": request.max_tokens or 4096,
        }

        if system:
            api_request["system"] = system

        if request.tools:
            api_request["tools"] = request.tools

        api_request.update(request.extra_params)

        # Stream from Anthropic
        current_tool_call: dict[str, Any] | None = None
        model_name = request.model

        async with self.client.messages.stream(**api_request) as stream:
            async for event in stream:
                if event.type == "message_start":
                    model_name = event.message.model
                    continue

                elif event.type == "content_block_start":
                    if event.content_block.type == "tool_use":
                        current_tool_call = {
                            "id": event.content_block.id,
                            "type": "function",
                            "function": {
                                "name": event.content_block.name,
                                "arguments": "",
                            },
                        }
                    continue

                elif event.type == "content_block_delta":
                    if event.delta.type == "text_delta":
                        yield StreamChunk(
                            content=event.delta.text,
                            model=model_name,
                        )
                    elif event.delta.type == "input_json_delta":
                        if current_tool_call:
                            current_tool_call["function"]["arguments"] += event.delta.partial_json
                    continue

                elif event.type == "content_block_stop":
                    if current_tool_call:
                        yield StreamChunk(
                            content="",
                            model=model_name,
                            tool_calls=[current_tool_call],
                        )
                        current_tool_call = None
                    continue

                elif event.type == "message_delta":
                    finish_reason = event.delta.stop_reason
                    if finish_reason:
                        usage = None
                        if event.usage:
                            usage = {
                                "completion_tokens": event.usage.output_tokens,
                            }
                        yield StreamChunk(
                            content="",
                            model=model_name,
                            finish_reason=finish_reason,
                            usage=usage,
                        )
                    continue

    @property
    def supported_models(self) -> list[str]:
        """List of Anthropic models supported."""
        return [
            # Claude 3.5 models
            "claude-3-5-sonnet-20241022",
            "claude-3-5-sonnet-20240620",
            "claude-3-5-haiku-20241022",
            # Claude 3 models
            "claude-3-opus-20240229",
            "claude-3-sonnet-20240229",
            "claude-3-haiku-20240307",
            # Aliases
            "claude-3-opus",
            "claude-3-sonnet",
            "claude-3-haiku",
            "claude-3.5-sonnet",
            "claude-3.5-haiku",
        ]

    @classmethod
    def from_env(cls) -> "AnthropicProvider":
        """Create Anthropic provider from environment variables.

        Environment variables:
            ANTHROPIC_API_KEY: Required API key
            ANTHROPIC_BASE_URL: Optional base URL

        Returns:
            Configured Anthropic provider.
        """
        api_key = os.environ.get("ANTHROPIC_API_KEY")
        if not api_key:
            raise ValueError("ANTHROPIC_API_KEY environment variable not set")

        return cls(
            api_key=api_key,
            base_url=os.environ.get("ANTHROPIC_BASE_URL"),
        )
