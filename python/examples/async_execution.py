"""Async execution example with custom async nodes.

This example demonstrates:
- Async def node functions
- Mixing sync and async nodes
- Proper await usage with agent.arun()

Requirements:
    uv add duragraph-python[openai]

Environment:
    export OPENAI_API_KEY=your-api-key-here

Usage:
    uv run python examples/async_execution.py
"""

import asyncio
import os

from duragraph import Graph, entrypoint, llm_node, node


@Graph(id="async_workflow", description="Demonstrates async execution")
class AsyncWorkflow:
    """A workflow with both sync and async nodes."""

    @entrypoint
    @node()
    def prepare_input(self, state):
        """Sync node - prepare the input."""
        print("📝 Preparing input (sync)...")
        return {
            "messages": [
                {"role": "user", "content": state.get("query", "What is async programming?")}
            ]
        }

    @llm_node(model="gpt-4o-mini", temperature=0.7)
    async def analyze(self, state):
        """Async LLM node - analyze the query."""
        print("🤖 Analyzing with LLM (async)...")
        # The LLM call is automatically async
        return state

    @node()
    async def fetch_data(self, state):
        """Async custom node - simulate API call."""
        print("🌐 Fetching additional data (async)...")
        # Simulate async I/O operation
        await asyncio.sleep(0.5)
        return {"data_fetched": True, "timestamp": "2024-01-01"}

    @node()
    def format_output(self, state):
        """Sync node - format the final output."""
        print("✨ Formatting output (sync)...")
        response = state["messages"][-1]["content"]
        return {
            "final_output": {
                "response": response,
                "metadata": {
                    "data_fetched": state.get("data_fetched"),
                    "timestamp": state.get("timestamp"),
                }
            }
        }

    # Define edges
    prepare_input >> analyze >> fetch_data >> format_output


async def main():
    """Run the async workflow example."""
    # Check for API key
    if not os.environ.get("OPENAI_API_KEY"):
        print("Error: OPENAI_API_KEY environment variable not set")
        print("Set it with: export OPENAI_API_KEY=your-key-here")
        return

    print("=" * 60)
    print("Async Execution Example")
    print("=" * 60)
    print()

    # Create workflow instance
    workflow = AsyncWorkflow()

    # Execute asynchronously
    print("🚀 Starting async execution...")
    print()

    result = await workflow.arun(
        {"query": "Explain the benefits of async/await in Python"}
    )

    print()
    print("=" * 60)
    print("✅ Execution Complete!")
    print("=" * 60)
    print()
    print(f"Response: {result.output['final_output']['response'][:200]}...")
    print()
    print(f"Metadata: {result.output['final_output']['metadata']}")
    print()
    print(f"Nodes executed: {result.nodes_executed}")


if __name__ == "__main__":
    asyncio.run(main())
