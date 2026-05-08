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


import httpx


@tool(description="Get the current weather for a city")
def get_weather(city: str, unit: str = "celsius") -> str:
    """Fetch live weather from wttr.in (no API key required)."""
    fmt = "3" if unit == "celsius" else "u"
    try:
        r = httpx.get(
            f"https://wttr.in/{city}",
            params={"format": fmt} if unit == "celsius" else {"format": "3", "u": ""},
            headers={"User-Agent": "duragraph-tool-use-demo/1.0"},
            timeout=10.0,
        )
        r.raise_for_status()
        # wttr.in `format=3` returns one line: "City: 🌫 +12°C"
        return r.text.strip()
    except Exception as exc:  # noqa: BLE001
        return f"weather lookup failed for {city!r}: {exc}"


@tool(description="Search Wikipedia and return a one-sentence summary")
def search_documents(query: str, max_results: int = 3) -> str:
    """Hit the Wikipedia REST API (no key) and summarize the top hit."""
    try:
        # Step 1 — search
        sr = httpx.get(
            "https://en.wikipedia.org/w/api.php",
            params={
                "action": "opensearch",
                "search": query,
                "limit": max_results,
                "format": "json",
            },
            headers={"User-Agent": "duragraph-tool-use-demo/1.0"},
            timeout=10.0,
        )
        sr.raise_for_status()
        data = sr.json()
        titles, descs, urls = data[1], data[2], data[3]
        if not titles:
            return f"no Wikipedia results for {query!r}"
        lines = [f"- {t}: {d or '(no summary)'}\n  {u}" for t, d, u in zip(titles, descs, urls)]
        return "\n".join(lines[:max_results])
    except Exception as exc:  # noqa: BLE001
        return f"wikipedia search failed for {query!r}: {exc}"


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
        """Prepare the request for tool-augmented LLM processing.

        Accepts both input shapes:
          - LangChain / Studio:  {"messages": [{"role": "user", "content": "..."}]}
          - Legacy / curl demo:  {"input": "..."}
        """
        # Studio sends `messages`; pull the latest user message text out
        incoming = state.get("messages") or []
        user_input = state.get("input", "") or ""
        if not user_input and incoming:
            for m in reversed(incoming):
                if m.get("role") == "user":
                    user_input = m.get("content", "")
                    break

        state["input"] = user_input
        state["messages"] = (
            list(incoming)
            if incoming
            else [{"role": "user", "content": user_input}]
        )
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
            # registry.list_tools() returns list[str] (tool names)
            available = list(registry.list_tools())
            state["tool_results"] = [
                "Hi! I'm a tool-using agent. Try asking me about the weather "
                "in a city, searching Wikipedia for a topic, or doing some "
                f"math. Available tools: {', '.join(available)}."
            ]

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
