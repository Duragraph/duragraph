"""Bridge between DuraGraph LLM providers and DSPy language models."""

from __future__ import annotations

from typing import Any


class DuraGraphLM:
    """Wraps a DuraGraph LLMProvider as a DSPy-compatible language model.

    This allows DuraGraph's provider registry to drive DSPy modules so
    users only configure their LLM credentials once.

    Example::

        from duragraph.dspy.bridge import DuraGraphLM, configure_from_provider

        configure_from_provider("gpt-4o-mini")
        # Now all dspy.Predict / ChainOfThought calls use DuraGraph's OpenAI provider.
    """

    def __init__(
        self,
        model: str = "gpt-4o-mini",
        temperature: float = 0.7,
        max_tokens: int | None = None,
        **kwargs: Any,
    ) -> None:
        self.model = model
        self.temperature = temperature
        self.max_tokens = max_tokens
        self.kwargs = kwargs
        self.history: list[dict[str, Any]] = []

    def __call__(self, prompt: str | None = None, **kwargs: Any) -> list[str]:
        import asyncio

        return asyncio.run(self._acall(prompt, **kwargs))

    async def _acall(self, prompt: str | None = None, **kwargs: Any) -> list[str]:
        from duragraph.llm import LLMRequest, get_provider

        messages: list[dict[str, str]] = kwargs.get("messages", [])
        if not messages and prompt:
            messages = [{"role": "user", "content": prompt}]

        provider = get_provider(self.model)
        request = LLMRequest(
            messages=messages,
            model=self.model,
            temperature=kwargs.get("temperature", self.temperature),
            max_tokens=kwargs.get("max_tokens", self.max_tokens),
        )
        response = await provider.acomplete(request)

        self.history.append(
            {
                "prompt": prompt,
                "messages": messages,
                "response": response.content,
                "model": self.model,
                "usage": response.usage,
            }
        )

        return [response.content or ""]


def configure_from_provider(
    model: str = "gpt-4o-mini",
    temperature: float = 0.7,
    max_tokens: int | None = None,
) -> None:
    """Configure DSPy to use a DuraGraph LLM provider.

    Calls ``dspy.configure(lm=...)`` with a :class:`DuraGraphLM` wrapper
    so that DSPy modules automatically route through DuraGraph's provider
    registry.

    Args:
        model: Model identifier recognised by DuraGraph's LLM registry.
        temperature: Default sampling temperature.
        max_tokens: Default max tokens.
    """
    try:
        import dspy
    except ImportError as exc:
        raise ImportError(
            "dspy is required for this feature. "
            "Install it with: pip install duragraph[dspy]"
        ) from exc

    lm = dspy.LM(
        f"openai/{model}" if "/" not in model else model,
        temperature=temperature,
        max_tokens=max_tokens or 4096,
    )
    dspy.configure(lm=lm)
