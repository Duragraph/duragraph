"""
DuraGraph Hello World Example

Demonstrates:
- Defining a graph with @Graph(id=...) and @node() decorators
- Connecting nodes with the >> operator
- Running locally and serving on a control plane
"""

import os

from duragraph import Graph, node, entrypoint


@Graph(id="hello_world")
class HelloWorld:
    """A simple graph that greets the user."""

    @entrypoint
    @node()
    async def greet(self, state: dict) -> dict:
        """Generate a greeting."""
        name = state.get("name", "World")
        state["greeting"] = f"Hello, {name}!"
        print(f"[greet] {state['greeting']}")
        return state

    @node()
    async def farewell(self, state: dict) -> dict:
        """Add a farewell message."""
        state["farewell"] = "Goodbye! Thanks for using DuraGraph."
        print(f"[farewell] {state['farewell']}")
        return state

    greet >> farewell


def main():
    agent = HelloWorld()

    # --- Local execution ---
    print("=== Local Execution ===")
    result = agent.run({"name": "DuraGraph"})
    print(f"Output: {result.output}\n")

    # --- Serve on control plane ---
    control_plane = os.getenv("DURAGRAPH_URL", "http://localhost:8081")
    print(f"=== Serving on {control_plane} ===")
    print("Press Ctrl+C to stop\n")
    agent.serve(control_plane, worker_name="hello-world-worker")


if __name__ == "__main__":
    main()
