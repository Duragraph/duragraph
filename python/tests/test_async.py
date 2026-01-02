"""Tests for async execution support."""

import asyncio

import pytest

from duragraph import Graph, entrypoint, node


@pytest.mark.asyncio
class TestAsyncExecution:
    """Test async execution functionality."""

    async def test_async_node_function(self) -> None:
        """Test that async def node functions work."""

        @Graph(id="test_async")
        class TestGraph:
            @entrypoint
            @node()
            async def async_node(self, state):
                """Async node function."""
                await asyncio.sleep(0.01)  # Simulate async work
                return {"result": "async_completed"}

        graph = TestGraph()
        result = await graph.arun({"input": "test"})

        assert result.status == "completed"
        assert result.output["result"] == "async_completed"
        assert "async_node" in result.nodes_executed

    async def test_sync_node_function(self) -> None:
        """Test that sync node functions still work."""

        @Graph(id="test_sync")
        class TestGraph:
            @entrypoint
            @node()
            def sync_node(self, state):
                """Sync node function."""
                return {"result": "sync_completed"}

        graph = TestGraph()
        result = await graph.arun({"input": "test"})

        assert result.status == "completed"
        assert result.output["result"] == "sync_completed"

    @pytest.mark.skip(reason="Requires >> operator in class body (issue #5)")
    async def test_mixed_sync_async_nodes(self) -> None:
        """Test mixing sync and async nodes."""

        @Graph(id="test_mixed")
        class TestGraph:
            def __init__(self):
                # Define edges in __init__ (>> operator not yet implemented in class body)
                self.sync_start >> self.async_middle >> self.sync_end

            @entrypoint
            @node()
            def sync_start(self, state):
                return {"step1": "sync"}

            @node()
            async def async_middle(self, state):
                await asyncio.sleep(0.01)
                return {"step2": "async"}

            @node()
            def sync_end(self, state):
                return {"step3": "sync"}

        graph = TestGraph()
        result = await graph.arun({"input": "test"})

        assert result.status == "completed"
        assert result.output["step1"] == "sync"
        assert result.output["step2"] == "async"
        assert result.output["step3"] == "sync"
        assert len(result.nodes_executed) == 3

    async def test_async_with_state_updates(self) -> None:
        """Test async nodes properly update state."""

        @Graph(id="test_state")
        class TestGraph:
            @entrypoint
            @node()
            async def process(self, state):
                await asyncio.sleep(0.01)
                count = state.get("count", 0)
                return {"count": count + 1, "processed": True}

        graph = TestGraph()
        result = await graph.arun({"count": 5})

        assert result.output["count"] == 6
        assert result.output["processed"] is True

    async def test_async_error_handling(self) -> None:
        """Test error handling in async nodes."""

        @Graph(id="test_error")
        class TestGraph:
            @entrypoint
            @node()
            async def failing_node(self, state):
                await asyncio.sleep(0.01)
                raise ValueError("Test error")

        graph = TestGraph()

        with pytest.raises(ValueError, match="Test error"):
            await graph.arun({"input": "test"})

    async def test_arun_returns_immediately(self) -> None:
        """Test that arun is truly async and doesn't block."""

        @Graph(id="test_nonblocking")
        class TestGraph:
            @entrypoint
            @node()
            async def slow_node(self, state):
                await asyncio.sleep(0.1)
                return {"done": True}

        graph = TestGraph()

        # Run multiple graphs concurrently
        results = await asyncio.gather(
            graph.arun({"id": 1}),
            graph.arun({"id": 2}),
            graph.arun({"id": 3}),
        )

        assert len(results) == 3
        assert all(r.status == "completed" for r in results)
        assert all(r.output["done"] is True for r in results)
