"""Minimal hello-world graph. Edit this file and `duragraph dev` will
hot-reload the worker automatically.
"""

import os

from duragraph import Graph, entrypoint, node


@Graph(id="hello_world")
class HelloWorld:
    """A graph that greets whatever name you pass in."""

    @entrypoint
    @node()
    async def greet(self, state: dict) -> dict:
        name = state.get("name", "world")
        state["greeting"] = f"Hello, {name}!"
        return state


if __name__ == "__main__":
    agent = HelloWorld()
    control_plane = os.getenv("DURAGRAPH_URL", "http://localhost:8081")
    agent.serve(control_plane, worker_name="hello-world-worker")
