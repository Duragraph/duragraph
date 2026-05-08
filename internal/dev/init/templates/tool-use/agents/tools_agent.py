"""Minimal tool-use graph. Demonstrates @tool registration and a
prepare → run_tools → synthesize pipeline.

Extend by adding more @tool functions and routing on the user query.
"""

import os

from duragraph import Graph, entrypoint, node, tool, tool_node


@tool(description="Add two numbers.")
def add(a: float, b: float) -> str:
    return f"{a} + {b} = {a + b}"


@tool(description="Echo a string back, uppercased.")
def shout(text: str) -> str:
    return text.upper()


@Graph(id="tools_agent", description="Tiny agent that picks a tool by keyword")
class ToolsAgent:
    @entrypoint
    @node()
    async def prepare(self, state: dict) -> dict:
        msgs = state.get("messages") or []
        user = state.get("input") or (msgs[-1].get("content", "") if msgs else "")
        state["input"] = user
        state["tools_used"] = []
        return state

    @tool_node()
    async def run_tools(self, state: dict) -> dict:
        q = state.get("input", "").lower()
        if "add" in q or "+" in q:
            state["tool_results"] = [add(1, 2)]
            state["tools_used"].append("add")
        else:
            state["tool_results"] = [shout(state.get("input", ""))]
            state["tools_used"].append("shout")
        return state

    @node()
    async def synthesize(self, state: dict) -> dict:
        state["response"] = "\n".join(state.get("tool_results", []))
        return state

    prepare >> run_tools >> synthesize


if __name__ == "__main__":
    agent = ToolsAgent()
    control_plane = os.getenv("DURAGRAPH_URL", "http://localhost:8081")
    agent.serve(control_plane, worker_name="tools-worker")
