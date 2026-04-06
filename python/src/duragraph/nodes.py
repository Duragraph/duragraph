"""Node decorators for DuraGraph workflows."""

import inspect
from collections.abc import Callable
from functools import wraps
from typing import Any, TypeVar

F = TypeVar("F", bound=Callable[..., Any])


class NodeDescriptor:
    """A descriptor that enables >> operator at class definition time."""

    # Class-level storage for edges created with >>
    _all_edges: list[tuple[str, str]] = []

    def __init__(self, func: Callable[..., Any], metadata: "NodeMetadata"):
        self.func = func
        self.metadata = metadata
        self.name = func.__name__

    def __rshift__(self, other: "NodeDescriptor") -> "NodeDescriptor":
        """Enable >> operator between node descriptors at class definition time."""
        # Store the edge definition at class level
        NodeDescriptor._all_edges.append((self.name, other.name))
        return other

    def __get__(self, instance: Any, owner: type) -> Any:
        """Descriptor protocol: return bound method when accessed on instance."""
        if instance is None:
            # Class-level access returns self (for >> operator)
            return self
        # Instance-level access returns wrapped bound method with metadata
        bound_method = self.func.__get__(instance, owner)

        # Create a wrapper that preserves both the method and metadata
        if inspect.iscoroutinefunction(self.func):

            @wraps(self.func)
            async def async_wrapper(*args: Any, **kwargs: Any) -> Any:
                return await bound_method(*args, **kwargs)

            async_wrapper._node_metadata = self.metadata  # type: ignore
            return async_wrapper
        else:

            @wraps(self.func)
            def wrapper(*args: Any, **kwargs: Any) -> Any:
                return bound_method(*args, **kwargs)

            wrapper._node_metadata = self.metadata  # type: ignore
            return wrapper

    def __set_name__(self, owner: type, name: str) -> None:
        """Called when descriptor is assigned to a class attribute."""
        self.name = name


class NodeMetadata:
    """Metadata attached to node functions."""

    def __init__(
        self,
        node_type: str,
        name: str | None = None,
        config: dict[str, Any] | None = None,
        is_async: bool = False,
    ):
        self.node_type = node_type
        self.name = name
        self.config = config or {}
        self.is_async = is_async


def node(
    name: str | None = None,
    *,
    retry_on: list[str] | None = None,
    max_retries: int = 3,
    retry_delay: float = 1.0,
) -> Callable[[F], NodeDescriptor]:
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

        @node()
        async def my_async_processor(self, state):
            await some_async_operation()
            return {"processed": True}
    """

    def decorator(func: F) -> NodeDescriptor:
        # Check if the original function is async
        is_async = inspect.iscoroutinefunction(func)

        metadata = NodeMetadata(
            node_type="function",
            name=name or func.__name__,
            config={
                "retry_on": retry_on or [],
                "max_retries": max_retries,
                "retry_delay": retry_delay,
            },
            is_async=is_async,
        )

        # Return a NodeDescriptor instead of wrapped function
        return NodeDescriptor(func, metadata)

    return decorator


def llm_node(
    model: str = "gpt-4o-mini",
    *,
    name: str | None = None,
    temperature: float = 0.7,
    max_tokens: int | None = None,
    system_prompt: str | None = None,
    tools: list[Callable[..., Any]] | None = None,
    stream: bool = True,
) -> Callable[[F], NodeDescriptor]:
    """LLM node decorator for AI-powered processing.

    Args:
        model: LLM model identifier (e.g., "gpt-4o-mini", "claude-3-sonnet").
        name: Optional name for the node. Defaults to function name.
        temperature: Sampling temperature (0.0 to 2.0).
        max_tokens: Maximum tokens in response.
        system_prompt: System prompt for the LLM.
        tools: List of tool functions decorated with @tool.
        stream: Whether to stream responses.

    Example:
        @tool(description="Search the web")
        def web_search(query: str) -> str:
            return search_api(query)

        @llm_node(model="gpt-4o-mini", tools=[web_search])
        def process_with_tools(self, state):
            return state
    """

    def decorator(func: F) -> NodeDescriptor:
        # Check if the original function is async
        is_async = inspect.iscoroutinefunction(func)

        # Process tools and extract their metadata
        tool_schemas = []
        tool_names = []
        if tools:
            from duragraph.tools import get_tool_metadata

            for tool_func in tools:
                tool_meta = get_tool_metadata(tool_func)
                if tool_meta:
                    tool_schemas.append(
                        {
                            "type": "function",
                            "function": {
                                "name": tool_meta.name,
                                "description": tool_meta.description,
                                "parameters": tool_meta.parameters,
                            },
                        }
                    )
                    tool_names.append(tool_meta.name)

        metadata = NodeMetadata(
            node_type="llm",
            name=name or func.__name__,
            config={
                "model": model,
                "temperature": temperature,
                "max_tokens": max_tokens,
                "system_prompt": system_prompt,
                "tools": tool_names,  # Store tool names for reference
                "tool_schemas": tool_schemas,  # Store full schemas for LLM
                "stream": stream,
            },
            is_async=is_async,
        )

        return NodeDescriptor(func, metadata)

    return decorator


def tool_node(
    name: str | None = None,
    *,
    timeout: float = 30.0,
    retry_on: list[str] | None = None,
    max_retries: int = 3,
) -> Callable[[F], NodeDescriptor]:
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

    def decorator(func: F) -> NodeDescriptor:
        # Check if the original function is async
        is_async = inspect.iscoroutinefunction(func)

        metadata = NodeMetadata(
            node_type="tool",
            name=name or func.__name__,
            config={
                "timeout": timeout,
                "retry_on": retry_on or [],
                "max_retries": max_retries,
            },
            is_async=is_async,
        )

        return NodeDescriptor(func, metadata)

    return decorator


def router_node(
    name: str | None = None,
) -> Callable[[F], NodeDescriptor]:
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

    def decorator(func: F) -> NodeDescriptor:
        # Check if the original function is async
        is_async = inspect.iscoroutinefunction(func)

        metadata = NodeMetadata(
            node_type="router",
            name=name or func.__name__,
            config={},
            is_async=is_async,
        )

        return NodeDescriptor(func, metadata)

    return decorator


def human_node(
    prompt: str = "Please review and continue",
    *,
    name: str | None = None,
    timeout: float | None = None,
    interrupt_before: bool = True,
) -> Callable[[F], NodeDescriptor]:
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

    def decorator(func: F) -> NodeDescriptor:
        # Check if the original function is async
        is_async = inspect.iscoroutinefunction(func)

        metadata = NodeMetadata(
            node_type="human",
            name=name or func.__name__,
            config={
                "prompt": prompt,
                "timeout": timeout,
                "interrupt_before": interrupt_before,
            },
            is_async=is_async,
        )

        return NodeDescriptor(func, metadata)

    return decorator


def dspy_node(
    signature: str,
    *,
    module: str = "ChainOfThought",
    name: str | None = None,
    model: str | None = None,
    temperature: float = 0.7,
    max_tokens: int | None = None,
    tools: list[Callable[..., Any]] | None = None,
    input_map: dict[str, str] | None = None,
    output_map: dict[str, str] | None = None,
    optimized_path: str | None = None,
) -> Callable[[F], NodeDescriptor]:
    """DSPy module node decorator.

    Wraps a DSPy module (Predict, ChainOfThought, ReAct, etc.) as a
    DuraGraph graph node. Input fields are read from state; output fields
    are written back to state.

    The decorated method body is called *after* the DSPy module runs,
    receiving the state with DSPy outputs already merged in. Return
    the (possibly modified) state dict.

    Args:
        signature: DSPy signature string (e.g. ``"question -> answer"``).
        module: DSPy module class name.
        name: Optional node name. Defaults to function name.
        model: LM model identifier. Uses globally-configured LM if None.
        temperature: Sampling temperature.
        max_tokens: Maximum generation tokens.
        tools: Tool callables (for ReAct modules).
        input_map: State key → signature field mapping.
        output_map: Signature field → state key mapping.
        optimized_path: Path to saved optimized module state.

    Example::

        @dspy_node("question -> answer", module="ChainOfThought")
        async def answer_question(self, state: dict) -> dict:
            return state

        @dspy_node(
            "question -> answer",
            module="ReAct",
            tools=[search_web],
        )
        async def research(self, state: dict) -> dict:
            return state
    """

    def decorator(func: F) -> NodeDescriptor:
        is_async = inspect.iscoroutinefunction(func)

        metadata = NodeMetadata(
            node_type="dspy",
            name=name or func.__name__,
            config={
                "signature": signature,
                "module": module,
                "model": model,
                "temperature": temperature,
                "max_tokens": max_tokens,
                "tools": tools or [],
                "input_map": input_map,
                "output_map": output_map,
                "optimized_path": optimized_path,
            },
            is_async=is_async,
        )

        return NodeDescriptor(func, metadata)

    return decorator


def entrypoint(func: F | NodeDescriptor) -> F | NodeDescriptor:
    """Mark a node as the graph entry point.

    Example:
        @entrypoint
        @llm_node(model="gpt-4o-mini")
        def start(self, state):
            return state
    """
    # Check if it's a NodeDescriptor
    if isinstance(func, NodeDescriptor):
        func.metadata.config["is_entrypoint"] = True
        return func
    # Check if already has node metadata (old-style decorator)
    elif hasattr(func, "_node_metadata"):
        func._node_metadata.config["is_entrypoint"] = True  # type: ignore
        return func
    else:
        # Create a NodeDescriptor for the function
        is_async = inspect.iscoroutinefunction(func)
        metadata = NodeMetadata(
            node_type="function",
            name=func.__name__,  # type: ignore
            config={"is_entrypoint": True},
            is_async=is_async,
        )
        return NodeDescriptor(func, metadata)  # type: ignore
