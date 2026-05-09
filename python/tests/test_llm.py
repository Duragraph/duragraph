"""Tests for LLM provider integrations."""

import os
from unittest.mock import AsyncMock, MagicMock, patch

import pytest

from duragraph.llm import LLMRequest, get_provider

# Check if optional dependencies are installed
try:
    import openai  # noqa: F401

    OPENAI_AVAILABLE = True
except ImportError:
    OPENAI_AVAILABLE = False

try:
    import anthropic  # noqa: F401

    ANTHROPIC_AVAILABLE = True
except ImportError:
    ANTHROPIC_AVAILABLE = False


class TestProviderRegistry:
    """Test provider registry functionality."""

    @pytest.mark.skipif(not OPENAI_AVAILABLE, reason="OpenAI not installed")
    def test_get_provider_openai_from_model(self) -> None:
        """Test getting OpenAI provider from model name."""
        with patch.dict(os.environ, {"OPENAI_API_KEY": "test-key"}):
            provider = get_provider("gpt-4o-mini")
            assert provider.__class__.__name__ == "OpenAIProvider"

    @pytest.mark.skipif(not ANTHROPIC_AVAILABLE, reason="Anthropic not installed")
    def test_get_provider_anthropic_from_model(self) -> None:
        """Test getting Anthropic provider from model name."""
        with patch.dict(os.environ, {"ANTHROPIC_API_KEY": "test-key"}):
            provider = get_provider("claude-3-sonnet")
            assert provider.__class__.__name__ == "AnthropicProvider"

    def test_get_provider_unknown_model(self) -> None:
        """Test error for unknown model."""
        with pytest.raises(ValueError, match="Cannot determine provider"):
            get_provider("unknown-model-xyz")


@pytest.mark.skipif(not OPENAI_AVAILABLE, reason="OpenAI not installed")
@pytest.mark.asyncio
class TestOpenAIProvider:
    """Test OpenAI provider."""

    async def test_acomplete(self) -> None:
        """Test async completion."""
        from duragraph.llm.openai import OpenAIProvider

        # Mock the OpenAI client
        mock_response = MagicMock()
        mock_response.model = "gpt-4o-mini"
        mock_response.choices = [
            MagicMock(
                message=MagicMock(content="Test response", tool_calls=None),
                finish_reason="stop",
            )
        ]
        mock_response.usage = MagicMock(
            prompt_tokens=10,
            completion_tokens=20,
            total_tokens=30,
        )

        with patch("duragraph.llm.openai.AsyncOpenAI") as mock_client_cls:
            mock_client = AsyncMock()
            mock_client.chat.completions.create = AsyncMock(return_value=mock_response)
            mock_client_cls.return_value = mock_client

            provider = OpenAIProvider(api_key="test-key")
            request = LLMRequest(
                messages=[{"role": "user", "content": "Hello"}],
                model="gpt-4o-mini",
            )

            response = await provider.acomplete(request)

            assert response.content == "Test response"
            assert response.model == "gpt-4o-mini"
            assert response.usage == {
                "prompt_tokens": 10,
                "completion_tokens": 20,
                "total_tokens": 30,
            }


@pytest.mark.skipif(not ANTHROPIC_AVAILABLE, reason="Anthropic not installed")
@pytest.mark.asyncio
class TestAnthropicProvider:
    """Test Anthropic provider."""

    async def test_acomplete(self) -> None:
        """Test async completion."""
        from duragraph.llm.anthropic import AnthropicProvider

        # Mock the Anthropic client
        mock_response = MagicMock()
        mock_response.model = "claude-3-sonnet"
        mock_response.content = [MagicMock(type="text", text="Test response")]
        mock_response.stop_reason = "end_turn"
        mock_response.usage = MagicMock(input_tokens=10, output_tokens=20)

        with patch("duragraph.llm.anthropic.AsyncAnthropic") as mock_client_cls:
            mock_client = AsyncMock()
            mock_client.messages.create = AsyncMock(return_value=mock_response)
            mock_client_cls.return_value = mock_client

            provider = AnthropicProvider(api_key="test-key")
            request = LLMRequest(
                messages=[{"role": "user", "content": "Hello"}],
                model="claude-3-sonnet",
            )

            response = await provider.acomplete(request)

            assert response.content == "Test response"
            assert response.model == "claude-3-sonnet"
            assert response.usage == {
                "prompt_tokens": 10,
                "completion_tokens": 20,
                "total_tokens": 30,
            }
