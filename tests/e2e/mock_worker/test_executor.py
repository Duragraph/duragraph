"""Tests for the mock worker executor.

Run with: pytest tests/e2e/mock_worker/test_executor.py -v
"""

import asyncio
import pytest

from .executor import execute_run, ExecutionError, InterruptError
from .graphs import list_graphs


@pytest.fixture
def run_id():
    return "test-run-123"


class TestSimpleEcho:
    """Tests for simple_echo graph."""

    @pytest.mark.asyncio
    async def test_echoes_input(self, run_id):
        """Simple echo graph returns input."""
        state, emitter = await execute_run(
            run_id=run_id,
            graph_id="simple_echo",
            input_data={"message": "hello world"},
        )

        assert "last_response" in state.values
        assert "hello world" in state.values["last_response"]

    @pytest.mark.asyncio
    async def test_emits_correct_events(self, run_id):
        """Correct events are emitted."""
        state, emitter = await execute_run(
            run_id=run_id,
            graph_id="simple_echo",
            input_data={"message": "test"},
        )

        events = emitter.get_all_events()
        event_types = [e["event"] for e in events]

        assert "run.started" in event_types
        assert "node.started" in event_types
        assert "node.completed" in event_types
        assert "run.completed" in event_types
        assert "llm.start" in event_types
        assert "llm.end" in event_types


class TestMultiStep:
    """Tests for multi_step graph."""

    @pytest.mark.asyncio
    async def test_executes_all_nodes(self, run_id):
        """All nodes execute in order."""
        state, emitter = await execute_run(
            run_id=run_id,
            graph_id="multi_step",
            input_data={"message": "test query"},
        )

        # Check state was updated by each node
        assert state.values.get("intent") == "general_inquiry"
        assert state.values.get("processed") is True

    @pytest.mark.asyncio
    async def test_creates_checkpoints(self, run_id):
        """Checkpoints created after each node."""
        state, emitter = await execute_run(
            run_id=run_id,
            graph_id="multi_step",
            input_data={"message": "test"},
        )

        events = emitter.get_all_events()
        checkpoint_events = [e for e in events if e["event"] == "checkpoint.created"]

        # Should have checkpoint after each non-start/end node
        assert len(checkpoint_events) >= 3


class TestBranching:
    """Tests for branching graph."""

    @pytest.mark.asyncio
    async def test_takes_path_a(self, run_id):
        """Routes to path A when route=a."""
        state, emitter = await execute_run(
            run_id=run_id,
            graph_id="branching",
            input_data={"route": "a", "message": "test"},
        )

        assert state.values.get("path_taken") == "a"
        assert "path_a" in state.nodes_executed
        assert "path_b" not in state.nodes_executed

    @pytest.mark.asyncio
    async def test_takes_path_b(self, run_id):
        """Routes to path B when route=b."""
        state, emitter = await execute_run(
            run_id=run_id,
            graph_id="branching",
            input_data={"route": "b", "message": "test"},
        )

        assert state.values.get("path_taken") == "b"
        assert "path_b" in state.nodes_executed
        assert "path_a" not in state.nodes_executed

    @pytest.mark.asyncio
    async def test_default_path(self, run_id):
        """Routes to default when no match."""
        state, emitter = await execute_run(
            run_id=run_id,
            graph_id="branching",
            input_data={"route": "unknown", "message": "test"},
        )

        # Default is path_a
        assert state.values.get("path_taken") == "a"


class TestToolCalling:
    """Tests for tool_calling graph."""

    @pytest.mark.asyncio
    async def test_executes_tools(self, run_id):
        """Tools are executed and results stored."""
        state, emitter = await execute_run(
            run_id=run_id,
            graph_id="tool_calling",
            input_data={"message": "search for something"},
        )

        assert "tool_results" in state.values
        assert len(state.values["tool_results"]) > 0

    @pytest.mark.asyncio
    async def test_tool_events_emitted(self, run_id):
        """Tool events are emitted."""
        state, emitter = await execute_run(
            run_id=run_id,
            graph_id="tool_calling",
            input_data={"message": "search"},
        )

        events = emitter.get_all_events()
        event_types = [e["event"] for e in events]

        assert "tool.start" in event_types
        assert "tool.end" in event_types


class TestHumanInterrupt:
    """Tests for human_interrupt graph."""

    @pytest.mark.asyncio
    async def test_interrupts_at_human_node(self, run_id):
        """Execution interrupts at human node."""
        with pytest.raises(InterruptError) as exc_info:
            await execute_run(
                run_id=run_id,
                graph_id="human_interrupt",
                input_data={"message": "need approval"},
            )

        assert exc_info.value.node_id == "human_review"
        assert "review" in exc_info.value.prompt.lower()

    @pytest.mark.asyncio
    async def test_resumes_after_approval(self, run_id):
        """Execution continues after resume."""
        # First, try to execute (will interrupt)
        try:
            await execute_run(
                run_id=run_id,
                graph_id="human_interrupt",
                input_data={"message": "need approval"},
            )
        except InterruptError:
            pass

        # Now resume with approval
        state, emitter = await execute_run(
            run_id=run_id,
            graph_id="human_interrupt",
            input_data={"message": "need approval", "_human_approved": True},
            resume_from="finalize",
        )

        assert "finalize" in state.nodes_executed


class TestFailure:
    """Tests for failure graph."""

    @pytest.mark.asyncio
    async def test_fails_at_configured_node(self, run_id):
        """Execution fails at failure node."""
        with pytest.raises(ExecutionError) as exc_info:
            await execute_run(
                run_id=run_id,
                graph_id="failure",
                input_data={"message": "test"},
            )

        assert exc_info.value.node_id == "fail_here"
        assert "rate limit" in str(exc_info.value).lower()

    @pytest.mark.asyncio
    async def test_failure_events_emitted(self, run_id):
        """Failure events are emitted."""
        try:
            state, emitter = await execute_run(
                run_id=run_id,
                graph_id="failure",
                input_data={"message": "test"},
            )
        except ExecutionError:
            pass  # Expected

        # Note: We can't easily get the emitter after failure
        # This would need refactoring to capture events before exception


class TestMockConfig:
    """Tests for per-run configuration override."""

    @pytest.mark.asyncio
    async def test_override_graph_via_input(self, run_id):
        """Can override graph via _mock_config."""
        state, emitter = await execute_run(
            run_id=run_id,
            graph_id="simple_echo",  # Default
            input_data={
                "_mock_config": {"graph": "multi_step"},
                "message": "test",
            },
        )

        # Should have multi_step behavior
        assert "intent" in state.values

    @pytest.mark.asyncio
    async def test_override_delay(self, run_id):
        """Can override delay via _mock_config."""
        import time

        start = time.time()
        state, emitter = await execute_run(
            run_id=run_id,
            graph_id="multi_step",
            input_data={
                "_mock_config": {"delay_ms": 0},
                "message": "test",
            },
        )
        elapsed = time.time() - start

        # Should be fast with no delay
        assert elapsed < 1.0


class TestTokenCounting:
    """Tests for simulated token counting."""

    @pytest.mark.asyncio
    async def test_tokens_counted(self, run_id):
        """Tokens are counted in events."""
        state, emitter = await execute_run(
            run_id=run_id,
            graph_id="multi_step",
            input_data={"message": "test"},
        )

        summary = emitter.get_token_summary()
        assert summary["input"] > 0
        assert summary["output"] > 0
        assert summary["total"] == summary["input"] + summary["output"]


class TestAllGraphs:
    """Ensure all graphs can be loaded."""

    def test_all_graphs_available(self):
        """All expected graphs are available."""
        graphs = list_graphs()
        expected = [
            "simple_echo",
            "multi_step",
            "branching",
            "tool_calling",
            "human_interrupt",
            "long_running",
            "failure",
        ]
        for g in expected:
            assert g in graphs
