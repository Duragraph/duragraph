"""Node decorators for DuraGraph workflows."""

from collections.abc import Callable
from functools import wraps
from typing import Any, TypeVar

F = TypeVar("F", bound=Callable[..., Any])


class NodeMetadata:
    """Metadata attached to node functions."""

    def __init__(
        self,
        node_type: str,
        name: str | None = None,
        config: dict[str, Any] | None = None,
    ):
        self.node_type = node_type
        self.name = name
        self.config = config or {}


def node(
    name: str | None = None,
    *,
    retry_on: list[str] | None = None,
    max_retries: int = 3,
    retry_delay: float = 1.0,
) -> Callable[[F], F]:
    """Basic node decorator for custom logic.

    Args:
        name: Optional name for the node. Defaults to function name.
        retry_on: List of exception types to retry on.
        max_retries: Maximum number of retry attempts.
        retry_delay: Delay between retries in seconds.

    Example:
        @node()
        def my_processor(self, state):
            return {"processed": True}
    """

    def decorator(func: F) -> F:
        @wraps(func)
        def wrapper(*args: Any, **kwargs: Any) -> Any:
            return func(*args, **kwargs)

        wrapper._node_metadata = NodeMetadata(  # type: ignore
            node_type="function",
            name=name or func.__name__,
            config={
                "retry_on": retry_on or [],
                "max_retries": max_retries,
                "retry_delay": retry_delay,
            },
        )
        return wrapper  # type: ignore

    return decorator


def llm_node(
    model: str = "gpt-4o-mini",
    *,
    name: str | None = None,
    temperature: float = 0.7,
    max_tokens: int | None = None,
    system_prompt: str | None = None,
    tools: list[str] | None = None,
    stream: bool = True,
) -> Callable[[F], F]:
    """LLM node decorator for AI-powered processing.

    Args:
        model: LLM model identifier (e.g., "gpt-4o-mini", "claude-3-sonnet").
        name: Optional name for the node. Defaults to function name.
        temperature: Sampling temperature (0.0 to 2.0).
        max_tokens: Maximum tokens in response.
        system_prompt: System prompt for the LLM.
        tools: List of tool names available to the LLM.
        stream: Whether to stream responses.

    Example:
        @llm_node(model="gpt-4o-mini", temperature=0.3)
        def classify_intent(self, state):
            return state
    """

    def decorator(func: F) -> F:
        @wraps(func)
        def wrapper(*args: Any, **kwargs: Any) -> Any:
            return func(*args, **kwargs)

        wrapper._node_metadata = NodeMetadata(  # type: ignore
            node_type="llm",
            name=name or func.__name__,
            config={
                "model": model,
                "temperature": temperature,
                "max_tokens": max_tokens,
                "system_prompt": system_prompt,
                "tools": tools or [],
                "stream": stream,
            },
        )
        return wrapper  # type: ignore

    return decorator


def tool_node(
    name: str | None = None,
    *,
    timeout: float = 30.0,
    retry_on: list[str] | None = None,
    max_retries: int = 3,
) -> Callable[[F], F]:
    """Tool node decorator for external tool execution.

    Args:
        name: Optional name for the node. Defaults to function name.
        timeout: Execution timeout in seconds.
        retry_on: List of exception types to retry on.
        max_retries: Maximum number of retry attempts.

    Example:
        @tool_node()
        def search_database(self, state):
            results = db.search(state["query"])
            return {"results": results}
    """

    def decorator(func: F) -> F:
        @wraps(func)
        def wrapper(*args: Any, **kwargs: Any) -> Any:
            return func(*args, **kwargs)

        wrapper._node_metadata = NodeMetadata(  # type: ignore
            node_type="tool",
            name=name or func.__name__,
            config={
                "timeout": timeout,
                "retry_on": retry_on or [],
                "max_retries": max_retries,
            },
        )
        return wrapper  # type: ignore

    return decorator


def router_node(
    name: str | None = None,
) -> Callable[[F], F]:
    """Router node decorator for conditional branching.

    The decorated function should return the name of the next node to execute.

    Args:
        name: Optional name for the node. Defaults to function name.

    Example:
        @router_node()
        def route_by_intent(self, state):
            if state["intent"] == "billing":
                return "billing_handler"
            return "general_handler"
    """

    def decorator(func: F) -> F:
        @wraps(func)
        def wrapper(*args: Any, **kwargs: Any) -> Any:
            return func(*args, **kwargs)

        wrapper._node_metadata = NodeMetadata(  # type: ignore
            node_type="router",
            name=name or func.__name__,
            config={},
        )
        return wrapper  # type: ignore

    return decorator


def human_node(
    prompt: str = "Please review and continue",
    *,
    name: str | None = None,
    timeout: float | None = None,
    interrupt_before: bool = True,
) -> Callable[[F], F]:
    """Human-in-the-loop node decorator.

    Args:
        prompt: Message to display to the human reviewer.
        name: Optional name for the node. Defaults to function name.
        timeout: Optional timeout for human response in seconds.
        interrupt_before: If True, interrupt before node execution.

    Example:
        @human_node(prompt="Please approve this response")
        def review_response(self, state):
            return state
    """

    def decorator(func: F) -> F:
        @wraps(func)
        def wrapper(*args: Any, **kwargs: Any) -> Any:
            return func(*args, **kwargs)

        wrapper._node_metadata = NodeMetadata(  # type: ignore
            node_type="human",
            name=name or func.__name__,
            config={
                "prompt": prompt,
                "timeout": timeout,
                "interrupt_before": interrupt_before,
            },
        )
        return wrapper  # type: ignore

    return decorator


def entrypoint(func: F) -> F:
    """Mark a node as the graph entry point.

    Example:
        @entrypoint
        @llm_node(model="gpt-4o-mini")
        def start(self, state):
            return state
    """
    # Check if already has node metadata
    if hasattr(func, "_node_metadata"):
        func._node_metadata.config["is_entrypoint"] = True  # type: ignore
    else:
        # Wrap as basic node
        @wraps(func)
        def wrapper(*args: Any, **kwargs: Any) -> Any:
            return func(*args, **kwargs)

        wrapper._node_metadata = NodeMetadata(  # type: ignore
            node_type="function",
            name=func.__name__,
            config={"is_entrypoint": True},
        )
        return wrapper  # type: ignore

    return func
