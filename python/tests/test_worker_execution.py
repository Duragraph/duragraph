"""Tests for worker task execution with real graph instances."""

import asyncio
from unittest.mock import AsyncMock, MagicMock

import httpx
import pytest

from duragraph import Graph, entrypoint, llm_node, node, router_node, tool_node
from duragraph.graph import GraphInstance
from duragraph.worker import Worker, WorkerStatus


@pytest.fixture
def mock_httpx_client():
    """Mock httpx client that records sent events."""
    client = AsyncMock(spec=httpx.AsyncClient)
    mock_response = MagicMock()
    mock_response.raise_for_status = MagicMock()
    client.post.return_value = mock_response
    return client


def make_worker(mock_client, graph_instance, graph_def):
    """Create a worker with a registered graph instance."""
    worker = Worker("http://localhost:8080")
    worker._client = mock_client
    worker._worker_id = "worker-123"
    worker._status = WorkerStatus.READY
    worker.register_graph(graph_def, instance=graph_instance)
    return worker


def get_events(mock_client):
    """Extract event_type and data from all _send_event calls."""
    events = []
    for call in mock_client.post.call_args_list:
        url = call[0][0]
        if "/events" in url:
            payload = call[1]["json"]
            events.append((payload["event_type"], payload["data"]))
    return events


class TestWorkerFunctionNodeExecution:
    """Test that the worker executes user-defined function nodes."""

    async def test_single_function_node(self, mock_httpx_client):
        """Test worker calls user-defined function node method."""

        @Graph(id="simple")
        class SimpleGraph:
            @entrypoint
            @node()
            def process(self, state):
                return {"result": state.get("input", "") + "_processed"}

        graph = SimpleGraph()
        definition = graph._get_definition()
        worker = make_worker(mock_httpx_client, graph, definition)

        await worker._execute_run(
            {
                "run_id": "run-1",
                "graph_id": "simple",
                "input": {"input": "hello"},
                "thread_id": "thread-1",
            }
        )

        events = get_events(mock_httpx_client)
        event_types = [e[0] for e in events]

        assert "run_started" in event_types
        assert "node_started" in event_types
        assert "node_completed" in event_types
        assert "run_completed" in event_types
        assert "run_failed" not in event_types

        run_completed = next(e for e in events if e[0] == "run_completed")
        assert run_completed[1]["output"]["result"] == "hello_processed"

    async def test_multi_node_chain(self, mock_httpx_client):
        """Test worker executes a chain of function nodes."""

        @Graph(id="chain")
        class ChainGraph:
            @entrypoint
            @node()
            def step1(self, state):
                return {"value": 1}

            @node()
            def step2(self, state):
                return {"value": state["value"] + 10}

            @node()
            def step3(self, state):
                return {"value": state["value"] * 2}

            _ = step1 >> step2 >> step3

        graph = ChainGraph()
        definition = graph._get_definition()
        worker = make_worker(mock_httpx_client, graph, definition)

        await worker._execute_run(
            {
                "run_id": "run-1",
                "graph_id": "chain",
                "input": {},
                "thread_id": "thread-1",
            }
        )

        events = get_events(mock_httpx_client)
        run_completed = next(e for e in events if e[0] == "run_completed")
        assert run_completed[1]["output"]["value"] == 22

    async def test_async_function_node(self, mock_httpx_client):
        """Test worker calls async user-defined node methods."""

        @Graph(id="async_graph")
        class AsyncGraph:
            @entrypoint
            @node()
            async def process(self, state):
                await asyncio.sleep(0.01)
                return {"async_result": "done"}

        graph = AsyncGraph()
        definition = graph._get_definition()
        worker = make_worker(mock_httpx_client, graph, definition)

        await worker._execute_run(
            {
                "run_id": "run-1",
                "graph_id": "async_graph",
                "input": {},
                "thread_id": "thread-1",
            }
        )

        events = get_events(mock_httpx_client)
        run_completed = next(e for e in events if e[0] == "run_completed")
        assert run_completed[1]["output"]["async_result"] == "done"


class TestWorkerToolNodeExecution:
    """Test that tool nodes execute user-defined functions (not stubs)."""

    async def test_tool_node_calls_user_function(self, mock_httpx_client):
        """Test tool node runs the user's method, not a no-op stub."""

        @Graph(id="tool_graph")
        class ToolGraph:
            @entrypoint
            @tool_node()
            def search(self, state):
                return {"search_result": f"found: {state.get('query', 'nothing')}"}

        graph = ToolGraph()
        definition = graph._get_definition()
        worker = make_worker(mock_httpx_client, graph, definition)

        await worker._execute_run(
            {
                "run_id": "run-1",
                "graph_id": "tool_graph",
                "input": {"query": "duragraph"},
                "thread_id": "thread-1",
            }
        )

        events = get_events(mock_httpx_client)
        run_completed = next(e for e in events if e[0] == "run_completed")
        assert run_completed[1]["output"]["search_result"] == "found: duragraph"


class TestWorkerRouterNodeExecution:
    """Test that router nodes execute and determine next node."""

    async def test_router_node_selects_branch(self, mock_httpx_client):
        """Test router node result determines conditional edge target."""

        @Graph(id="router_graph")
        class RouterGraph:
            @entrypoint
            @router_node()
            def classify(self, state):
                if state.get("urgent"):
                    return "fast_path"
                return "slow_path"

        graph = RouterGraph()
        definition = graph._get_definition()
        from duragraph.edges import Edge

        definition.edges.append(
            Edge("classify", {"fast_path": "fast_path", "slow_path": "slow_path"})
        )

        from duragraph.nodes import NodeMetadata

        definition.nodes["fast_path"] = NodeMetadata(node_type="function", name="fast_path")
        definition.nodes["slow_path"] = NodeMetadata(node_type="function", name="slow_path")

        graph.fast_path = lambda state: {"route": "fast"}
        graph.slow_path = lambda state: {"route": "slow"}

        worker = make_worker(mock_httpx_client, graph, definition)

        await worker._execute_run(
            {
                "run_id": "run-1",
                "graph_id": "router_graph",
                "input": {"urgent": True},
                "thread_id": "thread-1",
            }
        )

        events = get_events(mock_httpx_client)
        event_types = [e[0] for e in events]
        assert "run_completed" in event_types

        node_started_events = [e for e in events if e[0] == "node_started"]
        node_ids = [e[1]["node_id"] for e in node_started_events]
        assert "classify" in node_ids
        assert "fast_path" in node_ids
        assert "slow_path" not in node_ids


class TestWorkerErrorHandling:
    """Test error handling during task execution."""

    async def test_node_exception_sends_run_failed(self, mock_httpx_client):
        """Test that a node raising an exception results in run_failed event."""

        @Graph(id="error_graph")
        class ErrorGraph:
            @entrypoint
            @node()
            def explode(self, state):
                raise RuntimeError("something went wrong")

        graph = ErrorGraph()
        definition = graph._get_definition()
        worker = make_worker(mock_httpx_client, graph, definition)

        await worker._execute_run(
            {
                "run_id": "run-1",
                "graph_id": "error_graph",
                "input": {},
                "thread_id": "thread-1",
            }
        )

        events = get_events(mock_httpx_client)
        event_types = [e[0] for e in events]
        assert "run_failed" in event_types
        assert "run_completed" not in event_types

        run_failed = next(e for e in events if e[0] == "run_failed")
        assert "something went wrong" in run_failed[1]["error"]

    async def test_missing_graph_sends_run_failed(self, mock_httpx_client):
        """Test that referencing an unregistered graph fails gracefully."""
        worker = Worker("http://localhost:8080")
        worker._client = mock_httpx_client
        worker._worker_id = "worker-123"

        await worker._execute_run(
            {
                "run_id": "run-1",
                "graph_id": "nonexistent",
                "input": {},
            }
        )

        events = get_events(mock_httpx_client)
        event_types = [e[0] for e in events]
        assert "run_failed" in event_types

    async def test_no_instance_sends_run_failed(self, mock_httpx_client):
        """Test that a graph registered without instance fails with clear error."""

        @Graph(id="no_instance")
        class NoInstanceGraph:
            @entrypoint
            @node()
            def process(self, state):
                return state

        graph = NoInstanceGraph()
        definition = graph._get_definition()

        worker = Worker("http://localhost:8080")
        worker._client = mock_httpx_client
        worker._worker_id = "worker-123"
        worker.register_graph(definition)

        await worker._execute_run(
            {
                "run_id": "run-1",
                "graph_id": "no_instance",
                "input": {},
            }
        )

        events = get_events(mock_httpx_client)
        run_failed = next(e for e in events if e[0] == "run_failed")
        assert "No graph instance" in run_failed[1]["error"]


class TestWorkerHumanNode:
    """Test human-in-the-loop node handling."""

    async def test_human_node_suspends_run(self, mock_httpx_client):
        """Test human node sends requires_action and suspends."""

        from duragraph import human_node

        @Graph(id="human_graph")
        class HumanGraph:
            @entrypoint
            @human_node(prompt="Please approve")
            def review(self, state):
                return state

        graph = HumanGraph()
        definition = graph._get_definition()
        worker = make_worker(mock_httpx_client, graph, definition)

        await worker._execute_run(
            {
                "run_id": "run-1",
                "graph_id": "human_graph",
                "input": {"data": "review this"},
                "thread_id": "thread-1",
            }
        )

        events = get_events(mock_httpx_client)
        event_types = [e[0] for e in events]
        assert "run_requires_action" in event_types
        assert "run_completed" not in event_types

        action = next(e for e in events if e[0] == "run_requires_action")
        assert action[1]["prompt"] == "Please approve"


class TestWorkerGraphInstanceIntegration:
    """Test GraphInstance.serve passes instance to worker."""

    async def test_register_graph_stores_instance(self):
        """Test that register_graph with instance stores it."""

        @Graph(id="test")
        class TestGraph:
            @entrypoint
            @node()
            def process(self, state):
                return state

        graph = TestGraph()
        definition = graph._get_definition()

        worker = Worker("http://localhost:8080")
        worker.register_graph(definition, instance=graph)

        assert "test" in worker._graph_instances
        assert worker._graph_instances["test"] is graph

    async def test_register_graph_without_instance_backward_compat(self):
        """Test that register_graph without instance is backward-compatible."""

        @Graph(id="test")
        class TestGraph:
            @entrypoint
            @node()
            def process(self, state):
                return state

        graph = TestGraph()
        definition = graph._get_definition()

        worker = Worker("http://localhost:8080")
        worker.register_graph(definition)

        assert "test" not in worker._graph_instances
        assert "test" in worker._graphs


class TestResolveNextNode:
    """Test edge resolution logic."""

    async def test_simple_edge(self):
        """Test resolving a simple string edge."""
        from duragraph.edges import Edge

        @Graph(id="test")
        class TestGraph:
            @entrypoint
            @node()
            def a(self, state):
                return state

            @node()
            def b(self, state):
                return state

            _ = a >> b

        graph = TestGraph()
        definition = graph._get_definition()
        worker = Worker("http://localhost:8080")

        next_node = worker._resolve_next_node(definition, "a", {})
        assert next_node == "b"

    async def test_conditional_edge(self):
        """Test resolving a conditional edge based on result string."""
        from duragraph.edges import Edge
        from duragraph.graph import GraphDefinition
        from duragraph.nodes import NodeMetadata

        definition = GraphDefinition(
            graph_id="test",
            nodes={
                "router": NodeMetadata(node_type="router", name="router"),
                "yes": NodeMetadata(node_type="function", name="yes"),
                "no": NodeMetadata(node_type="function", name="no"),
            },
            edges=[Edge("router", {"yes": "yes", "no": "no"})],
            entrypoint="router",
        )

        worker = Worker("http://localhost:8080")

        assert worker._resolve_next_node(definition, "router", "yes") == "yes"
        assert worker._resolve_next_node(definition, "router", "no") == "no"
        assert worker._resolve_next_node(definition, "router", "maybe") is None

    async def test_no_matching_edge(self):
        """Test resolving when no edge matches current node."""
        from duragraph.graph import GraphDefinition
        from duragraph.nodes import NodeMetadata

        definition = GraphDefinition(
            graph_id="test",
            nodes={"a": NodeMetadata(node_type="function", name="a")},
            edges=[],
            entrypoint="a",
        )

        worker = Worker("http://localhost:8080")
        assert worker._resolve_next_node(definition, "a", {}) is None
