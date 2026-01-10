"""Tests for the tool system."""

import pytest

from duragraph import Graph, entrypoint, get_global_registry, llm_node, tool
from duragraph.tools import ToolRegistry, _generate_json_schema, resolve_tool_calls


def test_tool_decorator():
    """Test that @tool decorator correctly registers tools."""
    # Create a separate registry for testing
    test_registry = ToolRegistry()

    @tool(description="Test function", registry=test_registry)
    def test_func(x: int, y: str = "default") -> str:
        return f"{x}: {y}"

    # Check that tool was registered
    assert "test_func" in test_registry.list_tools()

    # Check metadata
    metadata = test_registry.get_tool("test_func")
    assert metadata is not None
    assert metadata.name == "test_func"
    assert metadata.description == "Test function"
    assert metadata.function == test_func


def test_json_schema_generation():
    """Test JSON schema generation for function parameters."""

    def sample_func(
        name: str, age: int, active: bool = True, tags: list[str] | None = None
    ) -> dict:
        return {"name": name, "age": age, "active": active, "tags": tags or []}

    schema = _generate_json_schema(sample_func)

    assert schema["type"] == "object"
    assert "name" in schema["properties"]
    assert "age" in schema["properties"]
    assert "active" in schema["properties"]
    assert "tags" in schema["properties"]

    assert schema["properties"]["name"]["type"] == "string"
    assert schema["properties"]["age"]["type"] == "integer"
    assert schema["properties"]["active"]["type"] == "boolean"
    assert schema["properties"]["tags"]["type"] == "array"

    # Check required fields (no default values)
    assert set(schema["required"]) == {"name", "age"}


def test_tool_execution():
    """Test tool execution through registry."""
    test_registry = ToolRegistry()

    @tool(description="Add two numbers", registry=test_registry)
    def add(x: int, y: int) -> int:
        return x + y

    @tool(description="Greet someone", registry=test_registry)
    def greet(name: str, prefix: str = "Hello") -> str:
        return f"{prefix}, {name}!"

    # Test successful execution
    result = test_registry.execute_tool("add", {"x": 5, "y": 3})
    assert result == 8

    result = test_registry.execute_tool("greet", {"name": "Alice"})
    assert result == "Hello, Alice!"

    result = test_registry.execute_tool("greet", {"name": "Bob", "prefix": "Hi"})
    assert result == "Hi, Bob!"

    # Test error handling
    with pytest.raises(ValueError, match="Tool 'nonexistent' not found"):
        test_registry.execute_tool("nonexistent", {})

    with pytest.raises(ValueError, match="Invalid arguments"):
        test_registry.execute_tool("add", {"x": 5})  # Missing required argument


def test_tool_schemas_for_llm():
    """Test getting tool schemas for LLM function calling."""
    test_registry = ToolRegistry()

    @tool(description="Search the web", registry=test_registry)
    def web_search(query: str, max_results: int = 10) -> str:
        return f"Search results for: {query} (max {max_results})"

    schemas = test_registry.get_tool_schemas()

    assert len(schemas) == 1
    schema = schemas[0]

    assert schema["type"] == "function"
    assert schema["function"]["name"] == "web_search"
    assert schema["function"]["description"] == "Search the web"

    params = schema["function"]["parameters"]
    assert params["type"] == "object"
    assert "query" in params["properties"]
    assert "max_results" in params["properties"]
    assert params["required"] == ["query"]


def test_resolve_tool_calls():
    """Test resolving LLM tool calls."""
    test_registry = ToolRegistry()

    @tool(description="Get weather", registry=test_registry)
    def get_weather(city: str) -> dict:
        return {"city": city, "temperature": 22, "condition": "sunny"}

    # Simulate tool calls from LLM
    tool_calls = [
        {
            "id": "call_123",
            "function": {"name": "get_weather", "arguments": '{"city": "San Francisco"}'},
        }
    ]

    results = resolve_tool_calls(tool_calls, test_registry)

    assert len(results) == 1
    result = results[0]
    assert result["tool_call_id"] == "call_123"
    assert result["role"] == "tool"
    assert '"city": "San Francisco"' in result["content"]


def test_resolve_tool_calls_error_handling():
    """Test error handling in tool call resolution."""
    test_registry = ToolRegistry()

    @tool(description="Divide numbers", registry=test_registry)
    def divide(x: float, y: float) -> float:
        if y == 0:
            raise ValueError("Cannot divide by zero")
        return x / y

    # Test error in tool execution
    tool_calls = [
        {"id": "call_456", "function": {"name": "divide", "arguments": '{"x": 10, "y": 0}'}}
    ]

    results = resolve_tool_calls(tool_calls, test_registry)

    assert len(results) == 1
    result = results[0]
    assert result["tool_call_id"] == "call_456"
    assert result["role"] == "tool"
    assert "Error executing tool" in result["content"]
    assert "Cannot divide by zero" in result["content"]


def test_llm_node_with_tools():
    """Test LLM node integration with tools."""

    # Define some test tools
    @tool(description="Calculate the square of a number")
    def square(x: float) -> float:
        return x**2

    @tool(description="Get user information")
    def get_user_info(user_id: str) -> dict:
        return {"user_id": user_id, "name": f"User {user_id}", "status": "active"}

    @Graph(id="tool_test")
    class ToolTestGraph:
        @entrypoint
        @llm_node(model="gpt-4o-mini", tools=[square, get_user_info])
        def process(self, state):
            """This method would normally interact with LLM, but we'll just check metadata."""
            return state

    graph = ToolTestGraph()
    definition = graph._get_definition()

    # Check that tools are properly registered in the node
    node_meta = definition.nodes["process"]
    assert node_meta.node_type == "llm"
    assert "tool_schemas" in node_meta.config

    tool_schemas = node_meta.config["tool_schemas"]
    assert len(tool_schemas) == 2

    # Check tool schema structure
    square_schema = next(s for s in tool_schemas if s["function"]["name"] == "square")
    assert square_schema["function"]["description"] == "Calculate the square of a number"
    assert "x" in square_schema["function"]["parameters"]["properties"]


def test_global_registry():
    """Test that global registry works correctly."""
    # Clear any existing tools from global registry
    global_registry = get_global_registry()
    original_tools = global_registry._tools.copy()
    global_registry._tools.clear()

    try:

        @tool(description="Global test function")
        def global_func(value: str) -> str:
            return f"Global: {value}"

        # Should be registered in global registry
        assert "global_func" in global_registry.list_tools()

        # Should be able to execute
        result = global_registry.execute_tool("global_func", {"value": "test"})
        assert result == "Global: test"

    finally:
        # Restore original tools
        global_registry._tools = original_tools


def test_complex_tool_types():
    """Test complex parameter types in tools."""
    test_registry = ToolRegistry()

    @tool(description="Process complex data", registry=test_registry)
    def process_data(items: list[str], config: dict, count: int = 5, enabled: bool = True) -> dict:
        return {
            "items": items,
            "config": config,
            "count": count,
            "enabled": enabled,
            "processed": True,
        }

    metadata = test_registry.get_tool("process_data")
    params = metadata.parameters["properties"]

    assert params["items"]["type"] == "array"
    assert params["items"]["items"]["type"] == "string"
    assert params["config"]["type"] == "object"
    assert params["count"]["type"] == "integer"
    assert params["enabled"]["type"] == "boolean"

    # Test execution
    result = test_registry.execute_tool(
        "process_data", {"items": ["a", "b", "c"], "config": {"mode": "fast"}, "count": 10}
    )

    assert result["items"] == ["a", "b", "c"]
    assert result["config"] == {"mode": "fast"}
    assert result["count"] == 10
    assert result["enabled"] is True  # Default value
    assert result["processed"] is True
