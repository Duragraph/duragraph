"""E2E tests for worker protocol.

Tests the full flow: worker registration, heartbeat, polling, task execution.
"""

import os
import time
import uuid

import httpx
import pytest

API_BASE_URL = os.getenv("API_BASE_URL", "http://localhost:8081/api/v1")
TIMEOUT = 30.0


@pytest.fixture
def api_client():
    """Create HTTP client for API requests."""
    with httpx.Client(base_url=API_BASE_URL, timeout=TIMEOUT) as client:
        yield client


@pytest.fixture
def worker_id():
    """Generate a unique worker ID."""
    return f"test-worker-{uuid.uuid4().hex[:8]}"


class TestWorkerRegistration:
    """Tests for worker registration endpoint."""

    def test_register_worker(self, api_client, worker_id):
        """Test that a worker can register with the control plane."""
        response = api_client.post(
            "/workers/register",
            json={
                "worker_id": worker_id,
                "name": "Test Worker",
                "capabilities": {
                    "graphs": ["simple_echo", "multi_step"],
                    "max_concurrent_runs": 5,
                },
                "graph_definitions": [
                    {
                        "graph_id": "simple_echo",
                        "name": "Simple Echo",
                        "description": "Echoes input back",
                        "nodes": [
                            {"id": "start", "type": "input"},
                            {"id": "echo", "type": "llm"},
                            {"id": "end", "type": "output"},
                        ],
                        "edges": [
                            {"source": "start", "target": "echo"},
                            {"source": "echo", "target": "end"},
                        ],
                        "entry_point": "start",
                    }
                ],
            },
        )

        assert response.status_code == 200
        data = response.json()
        assert data["worker_id"] == worker_id
        assert data["registered"] is True
        assert "heartbeat_url" in data
        assert "poll_url" in data
        assert "deregister_url" in data

    def test_register_worker_without_id_fails(self, api_client):
        """Test that registration without worker_id fails."""
        response = api_client.post(
            "/workers/register",
            json={
                "name": "Test Worker",
                "capabilities": {
                    "graphs": ["simple_echo"],
                    "max_concurrent_runs": 1,
                },
            },
        )

        assert response.status_code == 400


class TestWorkerHeartbeat:
    """Tests for worker heartbeat endpoint."""

    def test_heartbeat_success(self, api_client, worker_id):
        """Test that a registered worker can send heartbeats."""
        # First register
        api_client.post(
            "/workers/register",
            json={
                "worker_id": worker_id,
                "name": "Test Worker",
                "capabilities": {
                    "graphs": ["simple_echo"],
                    "max_concurrent_runs": 5,
                },
            },
        )

        # Send heartbeat
        response = api_client.post(
            f"/workers/{worker_id}/heartbeat",
            json={
                "status": "ready",
                "active_runs": 0,
                "total_runs": 10,
                "failed_runs": 1,
            },
        )

        assert response.status_code == 200
        data = response.json()
        assert data["acknowledged"] is True

    def test_heartbeat_unknown_worker(self, api_client):
        """Test that heartbeat for unknown worker returns 404."""
        response = api_client.post(
            "/workers/unknown-worker-123/heartbeat",
            json={
                "status": "ready",
                "active_runs": 0,
                "total_runs": 0,
                "failed_runs": 0,
            },
        )

        assert response.status_code == 404


class TestWorkerPolling:
    """Tests for worker polling endpoint."""

    def test_poll_empty(self, api_client, worker_id):
        """Test that polling returns empty when no tasks available."""
        # First register
        api_client.post(
            "/workers/register",
            json={
                "worker_id": worker_id,
                "name": "Test Worker",
                "capabilities": {
                    "graphs": ["simple_echo"],
                    "max_concurrent_runs": 5,
                },
            },
        )

        # Poll for tasks
        response = api_client.post(
            f"/workers/{worker_id}/poll",
            json={"max_tasks": 5},
        )

        assert response.status_code == 200
        data = response.json()
        assert data["tasks"] == []

    def test_poll_unknown_worker(self, api_client):
        """Test that polling for unknown worker returns 404."""
        response = api_client.post(
            "/workers/unknown-worker-123/poll",
            json={"max_tasks": 1},
        )

        assert response.status_code == 404


class TestWorkerDeregistration:
    """Tests for worker deregistration endpoint."""

    def test_deregister_worker(self, api_client, worker_id):
        """Test that a worker can deregister."""
        # First register
        api_client.post(
            "/workers/register",
            json={
                "worker_id": worker_id,
                "name": "Test Worker",
                "capabilities": {
                    "graphs": ["simple_echo"],
                    "max_concurrent_runs": 5,
                },
            },
        )

        # Deregister
        response = api_client.post(f"/workers/{worker_id}/deregister")

        assert response.status_code == 200
        data = response.json()
        assert data["worker_id"] == worker_id
        assert data["deregistered"] is True

        # Verify worker is gone
        response = api_client.get(f"/workers/{worker_id}")
        assert response.status_code == 404

    def test_deregister_unknown_worker(self, api_client):
        """Test that deregistering unknown worker still returns OK."""
        response = api_client.post("/workers/unknown-worker-123/deregister")

        assert response.status_code == 200
        data = response.json()
        assert data["deregistered"] is False


class TestWorkerListing:
    """Tests for worker listing endpoint."""

    def test_list_workers(self, api_client, worker_id):
        """Test that registered workers appear in listing."""
        # Register a worker
        api_client.post(
            "/workers/register",
            json={
                "worker_id": worker_id,
                "name": "Test Worker",
                "capabilities": {
                    "graphs": ["simple_echo"],
                    "max_concurrent_runs": 5,
                },
            },
        )

        # List workers
        response = api_client.get("/workers")

        assert response.status_code == 200
        data = response.json()
        assert data["total"] >= 1

        worker_ids = [w["worker_id"] for w in data["workers"]]
        assert worker_id in worker_ids

    def test_list_healthy_workers(self, api_client, worker_id):
        """Test filtering by healthy workers."""
        # Register a worker
        api_client.post(
            "/workers/register",
            json={
                "worker_id": worker_id,
                "name": "Test Worker",
                "capabilities": {
                    "graphs": ["simple_echo"],
                    "max_concurrent_runs": 5,
                },
            },
        )

        # List healthy workers (worker just registered, should be healthy)
        response = api_client.get("/workers", params={"healthy": "true"})

        assert response.status_code == 200
        data = response.json()

        worker_ids = [w["worker_id"] for w in data["workers"]]
        assert worker_id in worker_ids


class TestGraphDefinitions:
    """Tests for graph definition retrieval."""

    def test_get_graph_definition(self, api_client, worker_id):
        """Test that graph definitions can be retrieved."""
        # Register worker with graph definition
        api_client.post(
            "/workers/register",
            json={
                "worker_id": worker_id,
                "name": "Test Worker",
                "capabilities": {
                    "graphs": ["test_graph"],
                    "max_concurrent_runs": 5,
                },
                "graph_definitions": [
                    {
                        "graph_id": "test_graph",
                        "name": "Test Graph",
                        "description": "A test graph",
                        "nodes": [
                            {"id": "start", "type": "input"},
                            {"id": "end", "type": "output"},
                        ],
                        "edges": [
                            {"source": "start", "target": "end"},
                        ],
                        "entry_point": "start",
                    }
                ],
            },
        )

        # Get graph definition
        response = api_client.get("/workers/graphs/test_graph")

        assert response.status_code == 200
        data = response.json()
        assert data["graph_id"] == "test_graph"
        assert data["name"] == "Test Graph"
        assert len(data["nodes"]) == 2
        assert len(data["edges"]) == 1

    def test_get_unknown_graph(self, api_client):
        """Test that unknown graph returns 404."""
        response = api_client.get("/workers/graphs/unknown_graph")

        assert response.status_code == 404


class TestWorkerEvents:
    """Tests for worker event streaming."""

    def test_send_event(self, api_client, worker_id):
        """Test that workers can send events."""
        # Register worker
        api_client.post(
            "/workers/register",
            json={
                "worker_id": worker_id,
                "name": "Test Worker",
                "capabilities": {
                    "graphs": ["simple_echo"],
                    "max_concurrent_runs": 5,
                },
            },
        )

        # Send event
        response = api_client.post(
            f"/workers/{worker_id}/events",
            json={
                "run_id": str(uuid.uuid4()),
                "event_type": "node_started",
                "node_id": "echo",
                "data": {"message": "Starting echo node"},
                "timestamp": "2025-01-15T10:00:00Z",
            },
        )

        assert response.status_code == 200
        data = response.json()
        assert data["received"] is True


class TestFullWorkflow:
    """Integration tests for full worker workflow."""

    def test_worker_lifecycle(self, api_client, worker_id):
        """Test complete worker lifecycle: register -> heartbeat -> poll -> deregister."""
        # 1. Register
        response = api_client.post(
            "/workers/register",
            json={
                "worker_id": worker_id,
                "name": "Lifecycle Test Worker",
                "capabilities": {
                    "graphs": ["simple_echo"],
                    "max_concurrent_runs": 5,
                },
            },
        )
        assert response.status_code == 200

        # 2. Send heartbeat
        response = api_client.post(
            f"/workers/{worker_id}/heartbeat",
            json={
                "status": "ready",
                "active_runs": 0,
                "total_runs": 0,
                "failed_runs": 0,
            },
        )
        assert response.status_code == 200

        # 3. Poll for tasks
        response = api_client.post(
            f"/workers/{worker_id}/poll",
            json={"max_tasks": 5},
        )
        assert response.status_code == 200
        assert response.json()["tasks"] == []

        # 4. Update status via heartbeat
        response = api_client.post(
            f"/workers/{worker_id}/heartbeat",
            json={
                "status": "running",
                "active_runs": 1,
                "total_runs": 1,
                "failed_runs": 0,
            },
        )
        assert response.status_code == 200

        # 5. Verify worker status
        response = api_client.get(f"/workers/{worker_id}")
        assert response.status_code == 200
        data = response.json()
        assert data["status"] == "running"
        assert data["active_runs"] == 1
        assert data["total_runs"] == 1

        # 6. Deregister
        response = api_client.post(f"/workers/{worker_id}/deregister")
        assert response.status_code == 200

        # 7. Verify worker is gone
        response = api_client.get(f"/workers/{worker_id}")
        assert response.status_code == 404
