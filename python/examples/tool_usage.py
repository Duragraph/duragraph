"""Example demonstrating tool usage with LLM nodes."""

import asyncio
import json
from typing import Any

from duragraph import Graph, entrypoint, llm_node, tool


# Define tools that the AI agent can use
@tool(description="Search for information on the web")
def web_search(query: str, max_results: int = 5) -> dict[str, Any]:
    """Simulate a web search API."""
    # In a real implementation, this would call a search API
    results = [
        {
            "title": f"Result {i + 1} for '{query}'",
            "url": f"https://example.com/result-{i + 1}",
            "snippet": f"This is snippet {i + 1} about {query}. Contains relevant information.",
        }
        for i in range(min(max_results, 3))  # Simulate 3 results max
    ]

    return {"query": query, "results": results, "total_count": len(results)}


@tool(description="Get current weather information for a city")
def get_weather(city: str, units: str = "celsius") -> dict[str, Any]:
    """Simulate a weather API."""
    # In a real implementation, this would call a weather API
    weather_data = {
        "city": city,
        "temperature": 22.5,
        "condition": "partly cloudy",
        "humidity": 65,
        "wind_speed": 10,
        "units": units,
    }

    if units == "fahrenheit":
        weather_data["temperature"] = 72.5

    return weather_data


@tool(description="Calculate mathematical expressions")
def calculate(expression: str) -> dict[str, Any]:
    """Safely evaluate mathematical expressions."""
    try:
        # Simple whitelist of allowed operations for safety
        allowed_chars = set("0123456789+-*/.()")
        if not all(c in allowed_chars or c.isspace() for c in expression):
            raise ValueError("Invalid characters in expression")

        result = eval(expression)
        return {"expression": expression, "result": result, "success": True}
    except Exception as e:
        return {"expression": expression, "error": str(e), "success": False}


@tool(description="Store information in memory for later use")
def store_memory(key: str, value: str, category: str = "general") -> dict[str, Any]:
    """Store information in a simple memory system."""
    # In a real implementation, this might use a database
    memory_store = getattr(store_memory, "_memory", {})

    memory_item = {
        "key": key,
        "value": value,
        "category": category,
        "timestamp": "2024-01-01T12:00:00Z",  # Would use real timestamp
    }

    memory_store[key] = memory_item
    store_memory._memory = memory_store

    return {"stored": True, "key": key, "category": category}


@tool(description="Retrieve information from memory")
def recall_memory(key: str = None, category: str = None) -> dict[str, Any]:
    """Retrieve stored information."""
    memory_store = getattr(store_memory, "_memory", {})

    if key:
        item = memory_store.get(key)
        return {"found": item is not None, "item": item}

    if category:
        items = [item for item in memory_store.values() if item.get("category") == category]
        return {"category": category, "items": items, "count": len(items)}

    return {"all_items": list(memory_store.values()), "count": len(memory_store)}


@Graph(id="tool_agent", description="AI agent with various tools")
class ToolAgent:
    """An AI agent that can use multiple tools to help users."""

    @entrypoint
    @llm_node(
        model="gpt-4o-mini",
        temperature=0.7,
        tools=[web_search, get_weather, calculate, store_memory, recall_memory],
        system_prompt="""You are a helpful AI assistant with access to several tools:

        1. web_search - Search the internet for information
        2. get_weather - Get current weather for any city
        3. calculate - Perform mathematical calculations
        4. store_memory - Remember information for later
        5. recall_memory - Retrieve previously stored information

        Use these tools to help answer user questions accurately. Always explain what you're doing and why you're using specific tools.""",
    )
    def process_request(self, state: dict[str, Any]) -> dict[str, Any]:
        """Process user requests using available tools."""
        return state


def mock_llm_response_with_tools():
    """Mock an LLM response that includes tool calls for testing."""
    return {
        "content": "I'll help you with that. Let me search for information and check the weather.",
        "tool_calls": [
            {
                "id": "call_1",
                "function": {
                    "name": "web_search",
                    "arguments": json.dumps({"query": "Python programming tips", "max_results": 3}),
                },
            },
            {
                "id": "call_2",
                "function": {
                    "name": "get_weather",
                    "arguments": json.dumps({"city": "San Francisco"}),
                },
            },
        ],
    }


async def demo_tool_execution():
    """Demonstrate tool execution without requiring real LLM calls."""
    print("🔧 DuraGraph Tool System Demo")
    print("=" * 50)

    # Test individual tools
    print("\n1. Testing individual tools:")

    print("\n🔍 Web Search:")
    search_result = web_search("Python programming", 2)
    print(f"Query: {search_result['query']}")
    print(f"Found {search_result['total_count']} results")
    for result in search_result["results"]:
        print(f"  - {result['title']}")

    print("\n🌤️  Weather:")
    weather = get_weather("New York", "fahrenheit")
    print(f"Weather in {weather['city']}: {weather['temperature']}°F, {weather['condition']}")

    print("\n🧮 Calculator:")
    calc_result = calculate("15 + 27 * 2")
    if calc_result["success"]:
        print(f"{calc_result['expression']} = {calc_result['result']}")

    print("\n💾 Memory Storage:")
    store_result = store_memory("user_preference", "likes Python programming", "preferences")
    print(f"Stored: {store_result['key']} in category {store_result['category']}")

    recall_result = recall_memory("user_preference")
    if recall_result["found"]:
        item = recall_result["item"]
        print(f"Recalled: {item['key']} = {item['value']}")

    # Test tool call resolution (simulating LLM interaction)
    print("\n2. Testing tool call resolution:")
    mock_tool_calls = [
        {
            "id": "call_1",
            "function": {"name": "calculate", "arguments": '{"expression": "100 / 4 + 25"}'},
        },
        {
            "id": "call_2",
            "function": {
                "name": "get_weather",
                "arguments": '{"city": "Tokyo", "units": "celsius"}',
            },
        },
    ]

    from duragraph.tools import resolve_tool_calls

    print("\n🤖 Resolving mock LLM tool calls...")
    results = resolve_tool_calls(mock_tool_calls)

    for result in results:
        print(f"Tool call {result['tool_call_id']} result:")
        print(f"  {result['content']}")

    print("\n3. Testing graph with tools:")
    graph = ToolAgent()
    definition = graph._get_definition()

    # Check that tools are properly configured
    process_node = definition.nodes["process_request"]
    tool_schemas = process_node.config.get("tool_schemas", [])

    print(f"\n📋 Graph has {len(tool_schemas)} tools configured:")
    for schema in tool_schemas:
        func_info = schema["function"]
        print(f"  - {func_info['name']}: {func_info['description']}")

    print("\n✅ Tool system demo complete!")


def main():
    """Run the tool demonstration."""
    asyncio.run(demo_tool_execution())


if __name__ == "__main__":
    main()
