"""
DuraGraph Hello World Example

This minimal example demonstrates:
- Creating a simple graph
- Registering a worker with the control plane
- Executing a basic workflow
"""

import os

from duragraph import Graph, node
from duragraph.worker import Worker


# Define a simple graph with two nodes
@Graph
class HelloWorld:
    """A simple graph that greets the user."""

    @node
    async def greet(self, state: dict) -> dict:
        """First node: Generate greeting."""
        name = state.get("name", "World")
        state["greeting"] = f"Hello, {name}!"
        print(f"[greet] Generated: {state['greeting']}")
        return state

    @node
    async def farewell(self, state: dict) -> dict:
        """Second node: Add farewell message."""
        state["farewell"] = "Goodbye! Thanks for using DuraGraph."
        print(f"[farewell] Generated: {state['farewell']}")
        return state


def main():
    # Get control plane URL from environment
    control_plane_url = os.getenv("DURAGRAPH_URL", "http://localhost:8081")

    print(f"Connecting to DuraGraph at {control_plane_url}")

    # Create worker
    worker = Worker(
        control_plane_url=control_plane_url,
        name="hello-world-worker",
    )

    # Register our graph
    worker.register_graph(HelloWorld)

    print("Worker registered. Waiting for runs...")
    print("Press Ctrl+C to stop")

    # Run the worker (blocks until stopped)
    worker.run()


if __name__ == "__main__":
    main()
