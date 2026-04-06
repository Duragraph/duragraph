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
    """Execute an LLM node with optional tool calling.

    Args:
        node_name: Name of the node.
        metadata: Node metadata with LLM configuration.
        state: Current state.

    Returns:
        State updates from the LLM, potentially including tool results.
    """
    from duragraph.llm import LLMRequest, get_provider
    from duragraph.tools import resolve_tool_calls

    config = metadata.config
    model = config.get("model", "gpt-4o-mini")
    temperature = config.get("temperature", 0.7)
    max_tokens = config.get("max_tokens")
    system_prompt = config.get("system_prompt")
    tool_schemas = config.get("tool_schemas", [])

    # Build messages from state
    messages = []
    if "messages" in state:
        # State contains conversation messages
        messages = state["messages"].copy()
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
        tools=tool_schemas,  # Pass tool schemas instead of names
    )

    response = await provider.acomplete(request)

    # Update state with response
    result: dict[str, Any] = {}

    # Handle tool calls if present
    if response.tool_calls:
        # Execute tools and get results
        tool_results = resolve_tool_calls(response.tool_calls)

        # Add assistant message with tool calls
        if "messages" in state:
            messages.append(
                {
                    "role": "assistant",
                    "content": response.content,
                    "tool_calls": response.tool_calls,
                }
            )
            # Add tool results
            messages.extend(tool_results)

            # Make another LLM call to process tool results
            follow_up_request = LLMRequest(
                messages=messages,
                model=model,
                temperature=temperature,
                max_tokens=max_tokens,
                system_prompt=system_prompt,
                tools=tool_schemas,
            )

            follow_up_response = await provider.acomplete(follow_up_request)

            # Add final response
            messages.append({"role": "assistant", "content": follow_up_response.content})

            result["messages"] = messages
            result["response"] = follow_up_response.content
        else:
            # For non-message mode, just store tool call info
            result["tool_calls"] = response.tool_calls
            result["tool_results"] = tool_results
            result["response"] = response.content
    else:
        # No tool calls, handle normally
        if "messages" in state:
            messages.append({"role": "assistant", "content": response.content})
            result["messages"] = messages
        else:
            result["response"] = response.content

    return result


async def execute_function_node(
    node_method: Any,
    state: State,
) -> Any:
    """Execute a regular function node (sync or async).

    Args:
        node_method: The node method to execute.
        state: Current state.

    Returns:
        The raw result from the node method. Callers should check if it's
        a dict for state updates, or a string for routing decisions.
    """
    if inspect.iscoroutinefunction(node_method):
        result = await node_method(state)
    else:
        result = node_method(state)

    return result


async def execute_node(
    node_name: str,
    metadata: NodeMetadata,
    node_method: Any,
    state: State,
) -> Any:
    """Execute a node based on its type.

    Args:
        node_name: Name of the node.
        metadata: Node metadata.
        node_method: The node method.
        state: Current state.

    Returns:
        For LLM nodes, always returns a dict of state updates.
        For function/tool/router/human nodes, returns the raw result
        (may be a dict, string, or None).
    """
    node_type = metadata.node_type

    if node_type == "llm":
        return await execute_llm_node(node_name, metadata, state)
    elif node_type in ("function", "tool", "router", "human"):
        return await execute_function_node(node_method, state)
    else:
        raise ValueError(f"Unknown node type: {node_type}")
