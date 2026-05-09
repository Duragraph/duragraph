"""Tests for embedding providers."""

from unittest.mock import AsyncMock, Mock, patch

import pytest

from duragraph.embeddings import (
    EmbeddingProvider,
    EmbeddingRequest,
    EmbeddingResponse,
    create_embedding_provider,
    get_provider,
    list_providers,
    register_provider,
)


class MockEmbeddingProvider(EmbeddingProvider):
    """Mock embedding provider for testing."""

    def __init__(self, **kwargs):
        self.model = kwargs.get("model", "mock-model")
        self._dimension = kwargs.get("dimension", 768)

    async def aembed_documents(self, texts, **kwargs):
        """Return mock embeddings."""
        # Create embeddings of the right dimension
        base_values = [0.1, 0.2, 0.3]
        embeddings = []
        for _ in texts:
            emb = []
            for i in range(self._dimension):
                emb.append(base_values[i % len(base_values)])
            embeddings.append(emb)
        return embeddings

    async def aembed_query(self, text, **kwargs):
        """Return mock embedding."""
        # Create embedding of the right dimension
        base_values = [0.1, 0.2, 0.3]
        emb = []
        for i in range(self._dimension):
            emb.append(base_values[i % len(base_values)])
        return emb

    @property
    def dimension(self):
        return self._dimension

    def create_request(self, input, **kwargs):
        model = kwargs.get("model", self.model)
        return EmbeddingRequest(input=input, model=model)

    async def send_request(self, request):
        from duragraph.embeddings.base import EmbeddingData, EmbeddingUsage

        input_texts = request.input if isinstance(request.input, list) else [request.input]
        embeddings = await self.aembed_documents(input_texts)

        data = [EmbeddingData(embedding=emb, index=i) for i, emb in enumerate(embeddings)]

        return EmbeddingResponse(
            data=data, model=request.model, usage=EmbeddingUsage(prompt_tokens=10, total_tokens=10)
        )


def test_provider_registry():
    """Test provider registration and retrieval."""
    # Register mock provider
    register_provider("mock", MockEmbeddingProvider)

    # Check it's listed
    assert "mock" in list_providers()

    # Get provider instance
    provider = get_provider("mock", dimension=512)
    assert isinstance(provider, MockEmbeddingProvider)
    assert provider.dimension == 512


def test_create_embedding_provider():
    """Test provider creation with common parameters."""
    register_provider("test", MockEmbeddingProvider)

    provider = create_embedding_provider("test", model="custom-model", dimension=1024)
    assert provider.model == "custom-model"
    assert provider.dimension == 1024


def test_unknown_provider():
    """Test error handling for unknown provider."""
    with pytest.raises(ValueError, match="Unknown embedding provider"):
        get_provider("nonexistent")


@pytest.mark.asyncio
async def test_mock_provider_embed_documents():
    """Test embedding multiple documents."""
    provider = MockEmbeddingProvider(dimension=768)

    texts = ["Hello world", "How are you?", "Testing embeddings"]
    embeddings = await provider.aembed_documents(texts)

    assert len(embeddings) == 3
    assert all(len(emb) == 768 for emb in embeddings)
    assert all(isinstance(val, float) for emb in embeddings for val in emb)


@pytest.mark.asyncio
async def test_mock_provider_embed_query():
    """Test embedding a single query."""
    provider = MockEmbeddingProvider(dimension=384)

    embedding = await provider.aembed_query("Test query")

    assert len(embedding) == 384
    assert all(isinstance(val, float) for val in embedding)


def test_sync_embedding_methods():
    """Test synchronous embedding methods."""
    provider = MockEmbeddingProvider(dimension=256)

    # Test sync document embedding
    texts = ["Document 1", "Document 2"]
    embeddings = provider.embed_documents(texts)
    assert len(embeddings) == 2
    assert all(len(emb) == 256 for emb in embeddings)

    # Test sync query embedding
    embedding = provider.embed_query("Query")
    assert len(embedding) == 256


@pytest.mark.asyncio
async def test_request_response_flow():
    """Test the request/response flow."""
    provider = MockEmbeddingProvider()

    # Create request
    request = provider.create_request(["Test input"], model="test-model")
    assert request.model == "test-model"
    assert request.input == ["Test input"]

    # Send request
    response = await provider.send_request(request)
    assert response.model == "test-model"
    assert len(response.data) == 1
    assert response.data[0].index == 0
    assert len(response.data[0].embedding) > 0


@pytest.mark.skipif(
    not pytest.importorskip("openai", reason="OpenAI not available"),
    reason="OpenAI package not installed",
)
class TestOpenAIProvider:
    """Tests for OpenAI embedding provider."""

    def test_openai_provider_creation(self):
        """Test creating OpenAI provider."""
        from duragraph.embeddings.openai import OpenAIEmbeddingProvider

        provider = OpenAIEmbeddingProvider(api_key="test-key")
        assert provider.model == "text-embedding-3-small"
        assert provider.dimension == 1536

    @patch("openai.AsyncOpenAI")
    @pytest.mark.asyncio
    async def test_openai_embed_documents(self, mock_client_class):
        """Test OpenAI document embedding."""
        from duragraph.embeddings.openai import OpenAIEmbeddingProvider

        # Mock the response
        mock_response = Mock()
        mock_response.data = [
            Mock(embedding=[0.1, 0.2, 0.3], index=0),
            Mock(embedding=[0.4, 0.5, 0.6], index=1),
        ]

        mock_client = Mock()
        mock_client.embeddings.create = AsyncMock(return_value=mock_response)
        mock_client_class.return_value = mock_client

        # Test embedding
        provider = OpenAIEmbeddingProvider(api_key="test-key")
        embeddings = await provider.aembed_documents(["text1", "text2"])

        assert len(embeddings) == 2
        assert embeddings[0] == [0.1, 0.2, 0.3]
        assert embeddings[1] == [0.4, 0.5, 0.6]

    def test_openai_model_dimensions(self):
        """Test model dimension mapping."""
        from duragraph.embeddings.openai import OpenAIEmbeddingProvider

        # Test different models
        provider_small = OpenAIEmbeddingProvider(model="text-embedding-3-small")
        assert provider_small.dimension == 1536

        provider_large = OpenAIEmbeddingProvider(model="text-embedding-3-large")
        assert provider_large.dimension == 3072

        provider_ada = OpenAIEmbeddingProvider(model="text-embedding-ada-002")
        assert provider_ada.dimension == 1536


class TestAnthropicProvider:
    """Tests for Anthropic embedding provider."""

    def test_anthropic_provider_creation(self):
        """Test creating Anthropic provider."""
        from duragraph.embeddings.anthropic import AnthropicEmbeddingProvider

        provider = AnthropicEmbeddingProvider(api_key="test-key")
        assert provider.model == "claude-3-haiku-20240307"
        assert provider.dimension == 768

    @pytest.mark.asyncio
    async def test_anthropic_placeholder_embeddings(self):
        """Test Anthropic placeholder embedding generation."""
        from duragraph.embeddings.anthropic import AnthropicEmbeddingProvider

        provider = AnthropicEmbeddingProvider()

        # Test with consistent input for deterministic output
        text = "Hello world"
        embedding1 = await provider.aembed_query(text)
        embedding2 = await provider.aembed_query(text)

        # Should be deterministic
        assert embedding1 == embedding2
        assert len(embedding1) == 768
        assert all(isinstance(val, float) for val in embedding1)

    @pytest.mark.asyncio
    async def test_anthropic_multiple_documents(self):
        """Test Anthropic multiple document embedding."""
        from duragraph.embeddings.anthropic import AnthropicEmbeddingProvider

        provider = AnthropicEmbeddingProvider()
        texts = ["Document 1", "Document 2", "Different content"]

        embeddings = await provider.aembed_documents(texts)

        assert len(embeddings) == 3
        assert all(len(emb) == 768 for emb in embeddings)

        # Different texts should produce different embeddings
        assert embeddings[0] != embeddings[1]
        assert embeddings[1] != embeddings[2]


def test_embedding_request_validation():
    """Test EmbeddingRequest validation."""
    # Valid request
    request = EmbeddingRequest(input="test", model="test-model")
    assert request.input == "test"
    assert request.model == "test-model"

    # Request with list input
    request2 = EmbeddingRequest(input=["text1", "text2"], model="test-model")
    assert request2.input == ["text1", "text2"]


def test_embedding_response_structure():
    """Test EmbeddingResponse structure."""
    from duragraph.embeddings.base import EmbeddingData, EmbeddingUsage

    data = [
        EmbeddingData(embedding=[0.1, 0.2, 0.3], index=0),
        EmbeddingData(embedding=[0.4, 0.5, 0.6], index=1),
    ]

    usage = EmbeddingUsage(prompt_tokens=10, total_tokens=10)

    response = EmbeddingResponse(data=data, model="test-model", usage=usage)

    assert response.object == "list"
    assert len(response.data) == 2
    assert response.model == "test-model"
    assert response.usage.prompt_tokens == 10
