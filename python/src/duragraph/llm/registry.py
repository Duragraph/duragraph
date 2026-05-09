"""LLM provider registry for automatic provider selection."""

from typing import Any

from duragraph.llm.base import LLMProvider

# Global provider registry
_PROVIDERS: dict[str, type[LLMProvider]] = {}
_MODEL_REGISTRY: dict[str, str] = {}


def register_provider(name: str, provider_cls: type[LLMProvider]) -> None:
    """Register an LLM provider.

    Args:
        name: Provider name (e.g., "openai", "anthropic").
        provider_cls: The provider class.
    """
    _PROVIDERS[name] = provider_cls

    # Register all supported models
    try:
        # Try to get models from class property
        models = provider_cls.supported_models.fget(None)  # type: ignore
        if models:
            for model in models:
                _MODEL_REGISTRY[model] = name
    except (AttributeError, TypeError):
        pass


def get_provider(model: str, **kwargs: Any) -> LLMProvider:
    """Get an LLM provider for the specified model.

    Args:
        model: Model identifier (e.g., "gpt-4o-mini", "claude-3-sonnet").
        **kwargs: Additional arguments to pass to the provider.

    Returns:
        Initialized LLM provider instance.

    Raises:
        ValueError: If no provider is found for the model.
    """
    # Check if provider is explicitly specified
    if "provider" in kwargs:
        provider_name = kwargs.pop("provider")
        if provider_name not in _PROVIDERS:
            raise ValueError(f"Unknown provider: {provider_name}")
        return _PROVIDERS[provider_name](**kwargs)

    # Try to find provider from model name
    provider_name = _MODEL_REGISTRY.get(model)
    if provider_name:
        return _PROVIDERS[provider_name](**kwargs)

    # Try to detect from model prefix
    if model.startswith("gpt-") or model.startswith("o1-"):
        provider_name = "openai"
    elif model.startswith("claude-"):
        provider_name = "anthropic"
    elif model.startswith("gemini-"):
        provider_name = "gemini"
    else:
        raise ValueError(
            f"Cannot determine provider for model: {model}. "
            f"Specify provider explicitly with provider='name'"
        )

    if provider_name not in _PROVIDERS:
        raise ValueError(
            f"Provider '{provider_name}' not available. "
            f"Install with: uv add duragraph-python[{provider_name}]"
        )

    return _PROVIDERS[provider_name](**kwargs)


def get_available_providers() -> list[str]:
    """Get list of available LLM providers.

    Returns:
        List of provider names.
    """
    return list(_PROVIDERS.keys())


def get_supported_models() -> dict[str, list[str]]:
    """Get all supported models by provider.

    Returns:
        Dictionary mapping provider names to their supported models.
    """
    models_by_provider: dict[str, list[str]] = {}
    for provider_name, provider_cls in _PROVIDERS.items():
        try:
            models = provider_cls.supported_models.fget(None)  # type: ignore
            if models:
                models_by_provider[provider_name] = models
        except (AttributeError, TypeError):
            models_by_provider[provider_name] = []
    return models_by_provider


# Auto-register providers that are installed
def _auto_register_providers() -> None:
    """Automatically register available providers."""
    # Try OpenAI
    try:
        from duragraph.llm.openai import OpenAIProvider

        register_provider("openai", OpenAIProvider)
    except ImportError:
        pass

    # Try Anthropic
    try:
        from duragraph.llm.anthropic import AnthropicProvider

        register_provider("anthropic", AnthropicProvider)
    except ImportError:
        pass

    # Try Gemini
    try:
        from duragraph.llm.gemini import GeminiProvider

        register_provider("gemini", GeminiProvider)
    except ImportError:
        pass


# Auto-register on import
_auto_register_providers()
