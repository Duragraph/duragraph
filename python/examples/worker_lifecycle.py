"""v2 worker lifecycle: claim queued runs, push fenced events, drain on shutdown.

Run against a v2 control plane with: uv run python examples/worker_lifecycle.py
"""

import os

from duragraph import Graph, entrypoint, node
from duragraph.worker import Worker


@Graph(id="example")
class Example:
    @entrypoint
    @node()
    def greet(self, state):
        return {"greeting": f"Hello, {state['name']}!"}


if __name__ == "__main__":
    graph = Example()
    worker = Worker(os.getenv("DURAGRAPH_URL", "http://localhost:8081"), name="example-worker")
    worker.register_graph(graph._get_definition(), instance=graph)
    worker.run()
