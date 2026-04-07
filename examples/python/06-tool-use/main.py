"""
DuraGraph Tool Use Example

Demonstrates:
- Defining tools with the @tool decorator
- Binding tools to LLM nodes with @llm_node(tools=[...])
- Automatic tool schema generation from type hints
- Tool call resolution and follow-up LLM reasoning
- Combining tool results with graph state
"""

import os
from typing import Any

from duragraph import Graph, entrypoint, node, tool, tool_node, llm_node


@tool(description="Get the current weather for a city")
def get_weather(city: str, unit: str = "celsius") -> str:
    """Return simulated weather data for demonstration."""
    weather_data = {
        "new york": {"temp": 22, "condition": "Partly cloudy"},
        "london": {"temp": 15, "condition": "Rainy"},
        "tokyo": {"temp": 28, "condition": "Sunny"},
        "sydney": {"temp": 19, "condition": "Clear"},
    }
    data = weather_data.get(city.lower(), {"temp": 20, "condition": "Unknown"})
    temp = data["temp"] if unit == "celsius" else int(data["temp"] * 9 / 5 + 32)
    symbol = "°C" if unit == "celsius" else "°F"
    return f"{city}: {data['condition']}, {temp}{symbol}"


@tool(description="Search for documents by keyword")
def search_documents(query: str, max_results: int = 3) -> str:
    """Return simulated search results for demonstration."""
    docs = [
        {"title": "Event Sourcing Guide", "summary": "Patterns for event-driven architectures"},
        {"title": "CQRS in Practice", "summary": "Command-query responsibility segregation"},
        {"title": "AI Orchestration", "summary": "Building reliable AI workflow pipelines"},
        {"title": "Graph Execution", "summary": "DAG-based execution models for agents"},
    ]
    matches = [d for d in docs if query.lower() in d["title"].lower() or query.lower() in d["summary"].lower()]
    if not matches:
        matches = docs[:max_results]
    results = matches[:max_results]
    return "\n".join(f"- {d['title']}: {d['summary']}" for d in results)


@tool(description="Perform a mathematical calculation")
def calculate(expression: str) -> str:
    """Evaluate a math expression safely."""
    allowed = set("0123456789+-*/.(). ")
    if not all(c in allowed for c in expression):
        return f"Error: invalid characters in '{expression}'"
    try:
        result = eval(expression)  # noqa: S307
        return f"{expression} = {result}"
    except Exception as e:
        return f"Error: {e}"


@Graph(id="tool_use_agent", description="Agent that uses tools to answer questions")
class ToolUseAgent:
    """An agent that demonstrates tool calling with LLM nodes."""

    @entrypoint
    @node()
    async def prepare(self, state: dict[str, Any]) -> dict[str, Any]:
        """Prepare the request for tool-augmented LLM processing."""
        user_input = state.get("input", "")
        state["messages"] = [{"role": "user", "content": user_input}]
        state["tools_used"] = []
        print(f"[prepare] Query: {user_input}")
        return state

    @tool_node()
    async def run_tools(self, state: dict[str, Any]) -> dict[str, Any]:
        """Execute tool calls returned by the LLM.

        In production, the executor automatically resolves tool calls from
        the LLM response. This node demonstrates a manual tool execution
        step for custom workflows where you need control over tool routing.
        """
        from duragraph.tools import get_global_registry

        registry = get_global_registry()
        query = state.get("input", "").lower()

        if "weather" in query:
            city = "New York"
            for c in ["london", "tokyo", "sydney", "new york"]:
                if c in query:
                    city = c.title()
                    break
            result = get_weather(city)
            state["tool_results"] = [result]
            state["tools_used"].append("get_weather")

        elif "search" in query or "find" in query or "document" in query:
            result = search_documents(query)
            state["tool_results"] = [result]
            state["tools_used"].append("search_documents")

        elif "calculate" in query or "math" in query:
            expr = query.replace("calculate", "").replace("math", "").strip()
            if not expr:
                expr = "2 + 2"
            result = calculate(expr)
            state["tool_results"] = [result]
            state["tools_used"].append("calculate")

        else:
            available = [t.name for t in registry.list_tools()]
            state["tool_results"] = [f"Available tools: {', '.join(available)}"]

        print(f"[run_tools] Used: {state['tools_used']}")
        return state

    @node()
    async def synthesize(self, state: dict[str, Any]) -> dict[str, Any]:
        """Combine tool results into a final response."""
        tool_results = state.get("tool_results", [])
        tools_used = state.get("tools_used", [])

        if tool_results:
            result_text = "\n".join(tool_results)
            state["response"] = (
                f"Based on {', '.join(tools_used) if tools_used else 'available tools'}:\n"
                f"{result_text}"
            )
        else:
            state["response"] = "No tool results available."

        print(f"[synthesize] Response ready")
        return state

    prepare >> run_tools >> synthesize


def main() -> None:
    agent = ToolUseAgent()

    queries = [
        "What's the weather in Tokyo?",
        "Search for documents about orchestration",
        "Calculate 42 * 17 + 3",
        "What tools are available?",
    ]

    print("=== Tool Use Example ===\n")
    for query in queries:
        print(f"--- Query: {query} ---")
        result = agent.run({"input": query})
        print(f"Response: {result.output.get('response', 'N/A')}")
        print(f"Tools used: {result.output.get('tools_used', [])}")
        print()

    control_plane = os.getenv("DURAGRAPH_URL", "http://localhost:8081")
    print(f"=== Serving on {control_plane} ===")
    print("Press Ctrl+C to stop\n")
    agent.serve(control_plane, worker_name="tool-use-worker")


if __name__ == "__main__":
    main()
