"""Node execution logic for different node types."""

import inspect
from typing import Any

from duragraph.nodes import NodeMetadata
from duragraph.types import State


async def execute_llm_node(
    node_name: str,
    metadata: NodeMetadata,
    state: State,
) -> dict[str, Any]:
    """Execute an LLM node.

    Args:
        node_name: Name of the node.
        metadata: Node metadata with LLM configuration.
        state: Current state.

    Returns:
        State updates from the LLM.
    """
    from duragraph.llm import LLMRequest, get_provider

    config = metadata.config
    model = config.get("model", "gpt-4o-mini")
    temperature = config.get("temperature", 0.7)
    max_tokens = config.get("max_tokens")
    system_prompt = config.get("system_prompt")
    tools = config.get("tools", [])

    # Build messages from state
    messages = []
    if "messages" in state:
        # State contains conversation messages
        messages = state["messages"]
    elif "input" in state:
        # Simple input
        messages = [{"role": "user", "content": str(state["input"])}]
    else:
        # Use entire state as context
        messages = [{"role": "user", "content": str(state)}]

    # Get provider and make request
    provider = get_provider(model)
    request = LLMRequest(
        messages=messages,
        model=model,
        temperature=temperature,
        max_tokens=max_tokens,
        system_prompt=system_prompt,
        tools=tools,
    )

    response = await provider.acomplete(request)

    # Update state with response
    result: dict[str, Any] = {}

    # Add AI message to messages list
    if "messages" in state:
        updated_messages = state["messages"].copy()
        updated_messages.append({"role": "assistant", "content": response.content})
        result["messages"] = updated_messages
    else:
        result["response"] = response.content

    # Add tool calls if present
    if response.tool_calls:
        result["tool_calls"] = response.tool_calls

    return result


async def execute_function_node(
    node_method: Any,
    state: State,
) -> dict[str, Any]:
    """Execute a regular function node (sync or async).

    Args:
        node_method: The node method to execute.
        state: Current state.

    Returns:
        State updates from the node.
    """
    # The node_method is already the wrapped function from the decorator
    # which preserves the async nature
    if inspect.iscoroutinefunction(node_method):
        result = await node_method(state)
    else:
        result = node_method(state)

    if isinstance(result, dict):
        return result
    return {}


async def execute_node(
    node_name: str,
    metadata: NodeMetadata,
    node_method: Any,
    state: State,
) -> dict[str, Any]:
    """Execute a node based on its type.

    Args:
        node_name: Name of the node.
        metadata: Node metadata.
        node_method: The node method.
        state: Current state.

    Returns:
        State updates from the node.
    """
    node_type = metadata.node_type

    if node_type == "llm":
        return await execute_llm_node(node_name, metadata, state)
    elif node_type in ("function", "tool", "router", "human"):
        return await execute_function_node(node_method, state)
    else:
        raise ValueError(f"Unknown node type: {node_type}")
