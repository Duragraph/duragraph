"""Tests for worker lifecycle management."""

import asyncio
import time
from unittest.mock import AsyncMock, MagicMock, patch

import httpx
import pytest

from duragraph import Graph, entrypoint, node
from duragraph.worker import Worker, WorkerStatus


@pytest.fixture
def mock_httpx_client():
    """Mock httpx client for testing."""
    client = AsyncMock(spec=httpx.AsyncClient)
    return client


@pytest.fixture
def simple_graph():
    """Create a simple test graph."""

    @Graph(id="test_graph")
    class TestGraph:
        @entrypoint
        @node()
        def process(self, state):
            return {"processed": True}

    graph = TestGraph()
    return graph._get_definition()


class TestWorkerLifecycle:
    """Test worker lifecycle management."""

    async def test_worker_initialization(self):
        """Test worker initializes with correct defaults."""
        worker = Worker(
            control_plane_url="http://localhost:8080",
            name="test-worker",
            capabilities=["test"],
        )

        assert worker.name == "test-worker"
        assert worker.capabilities == ["test"]
        assert worker.heartbeat_interval == 30.0
        assert worker.max_concurrent_runs == 10
        assert worker.shutdown_timeout == 60.0
        assert worker._status == WorkerStatus.STARTING

    async def test_worker_registration_success(self, mock_httpx_client, simple_graph):
        """Test successful worker registration."""
        # Setup mock responses
        mock_response = MagicMock()
        mock_response.json.return_value = {"worker_id": "worker-123"}
        mock_response.raise_for_status = MagicMock()
        mock_httpx_client.post.return_value = mock_response

        worker = Worker("http://localhost:8080")
        worker._client = mock_httpx_client
        worker.register_graph(simple_graph)

        worker_id = await worker._register_with_control_plane()

        assert worker_id == "worker-123"
        assert worker._health_metrics["registration_attempts"] == 1

        # Verify registration payload
        mock_httpx_client.post.assert_called_once()
        call_args = mock_httpx_client.post.call_args
        assert "/api/v1/workers/register" in call_args[0][0]
        payload = call_args[1]["json"]
        assert payload["name"] == worker.name
        assert payload["status"] == WorkerStatus.STARTING.value

    async def test_worker_registration_retry(self, mock_httpx_client, simple_graph):
        """Test worker registration with retries on failure."""
        # First two attempts fail, third succeeds
        mock_httpx_client.post.side_effect = [
            httpx.ConnectError("Connection failed"),
            httpx.ConnectError("Connection failed"),
            MagicMock(
                json=MagicMock(return_value={"worker_id": "worker-123"}),
                raise_for_status=MagicMock(),
            ),
        ]

        worker = Worker("http://localhost:8080")
        worker._client = mock_httpx_client
        worker.register_graph(simple_graph)

        # Patch sleep to speed up test
        with patch("asyncio.sleep", return_value=None):
            worker_id = await worker._register_with_control_plane()

        assert worker_id == "worker-123"
        assert worker._health_metrics["registration_attempts"] == 3
        assert mock_httpx_client.post.call_count == 3

    async def test_heartbeat_with_metrics(self, mock_httpx_client):
        """Test heartbeat includes health metrics."""
        mock_response = MagicMock()
        mock_response.raise_for_status = MagicMock()
        mock_httpx_client.post.return_value = mock_response

        worker = Worker("http://localhost:8080")
        worker._client = mock_httpx_client
        worker._worker_id = "worker-123"
        worker._status = WorkerStatus.READY
        worker._health_metrics = {
            "runs_completed": 5,
            "runs_failed": 1,
            "uptime_start": time.time() - 100,
            "last_heartbeat": None,
            "registration_attempts": 1,
        }
        worker._active_runs = {"run-1", "run-2"}

        await worker._heartbeat()

        # Verify heartbeat was sent
        mock_httpx_client.post.assert_called_once()
        call_args = mock_httpx_client.post.call_args
        assert "/api/v1/workers/worker-123/heartbeat" in call_args[0][0]

        # Verify payload
        payload = call_args[1]["json"]
        assert payload["status"] == "ready"
        assert payload["metrics"]["active_runs"] == 2
        assert payload["metrics"]["runs_completed"] == 5
        assert payload["metrics"]["runs_failed"] == 1
        assert payload["metrics"]["uptime_seconds"] >= 100

    async def test_heartbeat_reregistration_on_404(self, mock_httpx_client):
        """Test worker re-registers when heartbeat returns 404."""
        # First heartbeat returns 404
        error_response = MagicMock()
        error_response.status_code = 404
        mock_httpx_client.post.side_effect = [
            httpx.HTTPStatusError("Not found", request=MagicMock(), response=error_response),
            # Registration succeeds
            MagicMock(
                json=MagicMock(return_value={"worker_id": "worker-456"}),
                raise_for_status=MagicMock(),
            ),
        ]

        worker = Worker("http://localhost:8080")
        worker._client = mock_httpx_client
        worker._worker_id = "worker-123"
        worker._graphs = {}

        await worker._heartbeat()

        # Should have tried to re-register
        assert mock_httpx_client.post.call_count == 2
        assert worker._worker_id == "worker-456"

    async def test_poll_respects_draining_status(self, mock_httpx_client):
        """Test worker doesn't accept new work when draining."""
        worker = Worker("http://localhost:8080")
        worker._client = mock_httpx_client
        worker._worker_id = "worker-123"
        worker._status = WorkerStatus.DRAINING

        result = await worker._poll_for_work()

        assert result is None
        mock_httpx_client.get.assert_not_called()

    async def test_graceful_shutdown_no_active_runs(self):
        """Test graceful shutdown with no active runs."""
        worker = Worker("http://localhost:8080")
        worker._status = WorkerStatus.READY
        worker._client = AsyncMock()

        await worker._graceful_shutdown()

        assert worker._status == WorkerStatus.STOPPED

    async def test_graceful_shutdown_waits_for_runs(self):
        """Test graceful shutdown waits for active runs to complete."""
        worker = Worker("http://localhost:8080")
        worker._status = WorkerStatus.BUSY
        worker._client = AsyncMock()
        worker._active_runs = {"run-1", "run-2"}
        worker.shutdown_timeout = 2.0  # Short timeout for testing

        # Simulate runs completing after 0.5 seconds
        async def clear_runs():
            await asyncio.sleep(0.5)
            worker._active_runs.clear()

        clear_task = asyncio.create_task(clear_runs())

        start_time = time.time()
        await worker._graceful_shutdown()
        elapsed = time.time() - start_time

        assert worker._status == WorkerStatus.STOPPED
        assert elapsed >= 0.5  # Waited for runs
        assert elapsed < 2.0  # Didn't timeout

        await clear_task

    async def test_graceful_shutdown_timeout(self):
        """Test graceful shutdown times out with stuck runs."""
        worker = Worker("http://localhost:8080")
        worker._status = WorkerStatus.BUSY
        worker._client = AsyncMock()
        worker._active_runs = {"run-1"}
        worker._run_tasks = {"run-1": AsyncMock(done=MagicMock(return_value=False))}
        worker.shutdown_timeout = 1.0  # Very short timeout

        start_time = time.time()
        await worker._graceful_shutdown()
        elapsed = time.time() - start_time

        assert worker._status == WorkerStatus.STOPPED
        assert elapsed >= 1.0  # Hit timeout
        assert elapsed < 1.5  # Not much over

        # Should have tried to cancel the task
        worker._run_tasks["run-1"].cancel.assert_called_once()

    async def test_concurrent_run_execution(self, mock_httpx_client):
        """Test worker can execute multiple runs concurrently."""
        worker = Worker("http://localhost:8080")
        worker._client = mock_httpx_client
        worker._worker_id = "worker-123"
        worker.max_concurrent_runs = 3

        # Track execution order
        execution_order = []

        async def mock_execute(work):
            run_id = work.get("run_id")
            execution_order.append(f"start-{run_id}")
            await asyncio.sleep(0.1)  # Simulate work
            execution_order.append(f"end-{run_id}")

        # Patch execute_run
        with patch.object(worker, "_execute_run", side_effect=mock_execute):
            # Start three runs
            tasks = [
                asyncio.create_task(worker._execute_run({"run_id": "1"})),
                asyncio.create_task(worker._execute_run({"run_id": "2"})),
                asyncio.create_task(worker._execute_run({"run_id": "3"})),
            ]

            await asyncio.gather(*tasks)

        # All should start before any finish (concurrent execution)
        assert "start-1" in execution_order
        assert "start-2" in execution_order
        assert "start-3" in execution_order
        start_indices = [execution_order.index(f"start-{i}") for i in ["1", "2", "3"]]
        end_indices = [execution_order.index(f"end-{i}") for i in ["1", "2", "3"]]
        assert max(start_indices) < min(end_indices)  # All started before any ended

    async def test_status_transitions(self):
        """Test worker status transitions during lifecycle."""
        worker = Worker("http://localhost:8080")

        # Initial state
        assert worker._status == WorkerStatus.STARTING

        # After successful registration (simulated)
        worker._status = WorkerStatus.READY
        assert worker._status == WorkerStatus.READY

        # When processing runs
        worker._active_runs.add("run-1")
        worker._status = WorkerStatus.BUSY
        assert worker._status == WorkerStatus.BUSY

        # When run completes
        worker._active_runs.clear()
        worker._status = WorkerStatus.READY
        assert worker._status == WorkerStatus.READY

        # During shutdown
        worker._status = WorkerStatus.DRAINING
        assert worker._status == WorkerStatus.DRAINING

        # After shutdown
        worker._status = WorkerStatus.STOPPED
        assert worker._status == WorkerStatus.STOPPED
