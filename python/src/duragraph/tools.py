"""Tool system for DuraGraph - function calling and execution."""

import inspect
import json
import types
from collections.abc import Callable
from typing import Any, Union, get_args, get_origin, get_type_hints

from pydantic import BaseModel


class ToolMetadata(BaseModel):
    """Metadata for a tool function."""

    name: str
    description: str
    parameters: dict[str, Any]  # JSON Schema for parameters
    function: Callable[..., Any]


class ToolRegistry:
    """Registry for managing tools and their metadata."""

    def __init__(self):
        self._tools: dict[str, ToolMetadata] = {}

    def register(self, tool_metadata: ToolMetadata) -> None:
        """Register a tool with the registry."""
        self._tools[tool_metadata.name] = tool_metadata

    def get_tool(self, name: str) -> ToolMetadata | None:
        """Get a tool by name."""
        return self._tools.get(name)

    def list_tools(self) -> list[str]:
        """List all registered tool names."""
        return list(self._tools.keys())

    def get_tool_schemas(self) -> list[dict[str, Any]]:
        """Get JSON schemas for all tools suitable for LLM function calling."""
        schemas = []
        for tool in self._tools.values():
            schema = {
                "type": "function",
                "function": {
                    "name": tool.name,
                    "description": tool.description,
                    "parameters": tool.parameters,
                },
            }
            schemas.append(schema)
        return schemas

    def execute_tool(self, name: str, arguments: dict[str, Any]) -> Any:
        """Execute a tool with the given arguments."""
        tool = self.get_tool(name)
        if not tool:
            raise ValueError(f"Tool '{name}' not found in registry")

        try:
            return tool.function(**arguments)
        except TypeError as e:
            raise ValueError(f"Invalid arguments for tool '{name}': {e}") from e


# Global tool registry
_global_registry = ToolRegistry()


def get_global_registry() -> ToolRegistry:
    """Get the global tool registry."""
    return _global_registry


def _generate_json_schema(func: Callable[..., Any]) -> dict[str, Any]:
    """Generate JSON Schema for function parameters."""
    sig = inspect.signature(func)
    type_hints = get_type_hints(func)

    properties = {}
    required = []

    for param_name, param in sig.parameters.items():
        if param_name == "self":
            continue

        param_type = type_hints.get(param_name, type(None))
        param_schema = _python_type_to_json_schema(param_type)

        # Get parameter description from docstring if available
        param_schema["description"] = f"Parameter {param_name}"

        properties[param_name] = param_schema

        # Check if parameter is required (no default value)
        if param.default is param.empty:
            required.append(param_name)

    return {
        "type": "object",
        "properties": properties,
        "required": required,
    }


def _python_type_to_json_schema(python_type: type) -> dict[str, Any]:
    """Convert Python type to JSON Schema."""
    # Handle None type
    if python_type is type(None):
        return {"type": "null"}

    # Handle basic types
    if python_type is str:
        return {"type": "string"}
    elif python_type is int:
        return {"type": "integer"}
    elif python_type is float:
        return {"type": "number"}
    elif python_type is bool:
        return {"type": "boolean"}
    elif python_type is list:
        return {"type": "array"}
    elif python_type is dict:
        return {"type": "object"}

    # Get origin and args using typing module functions (works across Python versions)
    origin = get_origin(python_type)
    args = get_args(python_type)

    # Handle Union types (including T | None)
    # `T | None` has origin as types.UnionType
    # typing.Union[T, None] has origin as typing.Union
    is_union = origin is Union or origin is types.UnionType

    if is_union and args:
        # Filter out None types
        non_none_types = [arg for arg in args if arg is not type(None)]
        if len(non_none_types) == 1:
            return _python_type_to_json_schema(non_none_types[0])
        elif len(non_none_types) > 1:
            # Multiple non-None types, use the first one as fallback
            return _python_type_to_json_schema(non_none_types[0])

    # Handle list types (list[T])
    if origin is list:
        item_type = args[0] if args else str
        return {
            "type": "array",
            "items": _python_type_to_json_schema(item_type),
        }

    # Handle dict types
    if origin is dict:
        return {"type": "object"}

    # Default to string for unknown types
    return {"type": "string"}


def tool(
    description: str,
    *,
    name: str | None = None,
    registry: ToolRegistry | None = None,
) -> Callable[[Callable[..., Any]], Callable[..., Any]]:
    """Decorator to register a function as a tool.

    Args:
        description: Description of what the tool does.
        name: Optional name for the tool (defaults to function name).
        registry: Optional specific registry (defaults to global).

    Example:
        @tool(description="Search the web for information")
        def web_search(query: str) -> str:
            return search_api(query)
    """

    def decorator(func: Callable[..., Any]) -> Callable[..., Any]:
        tool_name = name or func.__name__
        tool_registry = registry or get_global_registry()

        # Generate JSON schema for parameters
        parameters = _generate_json_schema(func)

        # Create tool metadata
        metadata = ToolMetadata(
            name=tool_name,
            description=description,
            parameters=parameters,
            function=func,
        )

        # Register the tool
        tool_registry.register(metadata)

        # Add metadata to function for introspection
        func._tool_metadata = metadata  # type: ignore

        return func

    return decorator


def get_tool_metadata(func: Callable[..., Any]) -> ToolMetadata | None:
    """Get tool metadata from a decorated function."""
    return getattr(func, "_tool_metadata", None)


def resolve_tool_calls(
    tool_calls: list[dict[str, Any]], registry: ToolRegistry | None = None
) -> list[dict[str, Any]]:
    """Execute tool calls and return results.

    Args:
        tool_calls: List of tool call objects from LLM.
        registry: Tool registry to use (defaults to global).

    Returns:
        List of tool call results.
    """
    tool_registry = registry or get_global_registry()
    results = []

    for tool_call in tool_calls:
        call_id = tool_call.get("id")
        function = tool_call.get("function", {})
        function_name = function.get("name")

        try:
            # Parse arguments from JSON string
            arguments_str = function.get("arguments", "{}")
            arguments = (
                json.loads(arguments_str) if isinstance(arguments_str, str) else arguments_str
            )

            # Execute the tool
            result = tool_registry.execute_tool(function_name, arguments)

            # Convert result to string if necessary
            if not isinstance(result, str):
                result = json.dumps(result)

            results.append(
                {
                    "tool_call_id": call_id,
                    "role": "tool",
                    "content": result,
                }
            )

        except Exception as e:
            # Return error as tool result
            results.append(
                {
                    "tool_call_id": call_id,
                    "role": "tool",
                    "content": f"Error executing tool '{function_name}': {str(e)}",
                }
            )

    return results
