"""Embedding provider registry and utilities."""

from typing import Any

from duragraph.embeddings.base import EmbeddingProvider

# Global registry of embedding providers
_providers: dict[str, type[EmbeddingProvider]] = {}


def register_provider(name: str, provider_class: type[EmbeddingProvider]) -> None:
    """Register an embedding provider.

    Args:
        name: Provider name (e.g., "openai", "anthropic").
        provider_class: Provider class implementing EmbeddingProvider.
    """
    _providers[name] = provider_class


def get_provider(name: str, **kwargs: Any) -> EmbeddingProvider:
    """Get an embedding provider instance.

    Args:
        name: Provider name.
        **kwargs: Provider-specific initialization arguments.

    Returns:
        Initialized embedding provider.

    Raises:
        ValueError: If provider not found.
    """
    if name not in _providers:
        raise ValueError(
            f"Unknown embedding provider: {name}. Available: {list(_providers.keys())}"
        )

    provider_class = _providers[name]
    return provider_class(**kwargs)


def list_providers() -> list[str]:
    """List all registered embedding providers."""
    return list(_providers.keys())


def create_embedding_provider(
    provider: str, model: str | None = None, **kwargs: Any
) -> EmbeddingProvider:
    """Create an embedding provider with common parameters.

    Args:
        provider: Provider name ("openai", "anthropic", etc.).
        model: Optional model name override.
        **kwargs: Additional provider-specific parameters.

    Returns:
        Configured embedding provider.
    """
    provider_kwargs = kwargs.copy()
    if model:
        provider_kwargs["model"] = model

    return get_provider(provider, **provider_kwargs)


# Register built-in providers
def _register_builtin_providers() -> None:
    """Register built-in embedding providers."""
    try:
        from duragraph.embeddings.openai import OpenAIEmbeddingProvider

        register_provider("openai", OpenAIEmbeddingProvider)
    except ImportError:
        pass  # OpenAI not available

    try:
        from duragraph.embeddings.anthropic import AnthropicEmbeddingProvider

        register_provider("anthropic", AnthropicEmbeddingProvider)
    except ImportError:
        pass  # Anthropic not available


# Auto-register on import
_register_builtin_providers()
