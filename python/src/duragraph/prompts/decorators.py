"""Prompt decorators for DuraGraph."""

from collections.abc import Callable
from functools import wraps
from typing import Any, TypeVar

F = TypeVar("F", bound=Callable[..., Any])


def prompt(
    prompt_id: str,
    *,
    version: str | None = None,
    variant: str | None = None,
) -> Callable[[F], F]:
    """Decorator to attach a prompt from the prompt store to a node.

    Args:
        prompt_id: Identifier for the prompt (e.g., "support/classify_intent").
        version: Optional specific version (e.g., "2.1.0"). Defaults to latest.
        variant: Optional A/B test variant.

    Example:
        @llm_node(model="gpt-4o-mini")
        @prompt("support/classify_intent", version="2.1.0")
        def classify(self, state):
            return state
    """

    def decorator(func: F) -> F:
        @wraps(func)
        def wrapper(*args: Any, **kwargs: Any) -> Any:
            return func(*args, **kwargs)

        # Attach prompt metadata
        wrapper._prompt_metadata = {  # type: ignore
            "prompt_id": prompt_id,
            "version": version,
            "variant": variant,
        }
        return wrapper  # type: ignore

    return decorator
