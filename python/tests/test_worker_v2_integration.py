"""Opt-in smoke against the shipped v2 binary and a real tenant database.

Start `go build -o duragraph ./cmd/duragraph` then `duragraph serve
--control-plane=v2` with isolated Postgres and NATS. Run this test with
`DURAGRAPH_V2_URL=http://127.0.0.1:39081 uv run --extra dev pytest -q
tests/test_worker_v2_integration.py`. Without the URL it is skipped.
"""

import os
from uuid import uuid4

import httpx
import pytest

from duragraph import Graph, entrypoint, node
from duragraph.worker import Worker, WorkerStatus


@pytest.mark.skipif(not os.getenv("DURAGRAPH_V2_URL"), reason="needs running v2 control plane")
async def test_shipped_v2_claim_execute_and_checkpoint():
    url = os.environ["DURAGRAPH_V2_URL"].rstrip("/")
    graph_id = f"python_smoke_{uuid4().hex[:12]}"

    @Graph(id=graph_id)
    class Smoke:
        @entrypoint
        @node()
        def respond(self, state):
            return {"answer": f"hello {state['name']}"}

    graph = Smoke()
    worker = Worker(url, name=f"worker-{uuid4().hex}")
    worker.register_graph(graph._get_definition(), instance=graph)

    async with httpx.AsyncClient(timeout=15.0) as client:
        worker._client = client
        worker._worker_id = await worker._register_with_control_plane()
        worker._status = WorkerStatus.READY
        # A real heartbeat also verifies the DB status CHECK accepts the wire value.
        await worker._heartbeat()

        assistant = await client.post(f"{url}/api/v1/assistants", json={"graph_id": graph_id})
        assistant.raise_for_status()
        thread = await client.post(f"{url}/api/v1/threads", json={})
        thread.raise_for_status()
        thread_id = thread.json()["thread_id"]
        run = await client.post(
            f"{url}/api/v1/threads/{thread_id}/runs",
            json={"assistant_id": assistant.json()["assistant_id"], "input": {"name": "v2"}},
        )
        run.raise_for_status()
        run_id = run.json()["run_id"]

        # No legacy /poll; the v2 claim atomically emits run.started and yields an epoch.
        claims = await worker._claim_work(1)
        assert len(claims) == 1
        assert claims[0]["run"]["run_id"] == run_id
        assert claims[0]["lease_epoch"] > 0
        assert claims[0]["checkpoint_id"] is None
        await worker._execute_run(claims[0])

        recorded = await client.get(f"{url}/api/v1/threads/{thread_id}/runs/{run_id}")
        recorded.raise_for_status()
        assert recorded.json()["status"] == "success"

        checkpoint = await client.get(
            f"{url}/api/v1/threads/{thread_id}/checkpoints/latest",
            params={"run_id": run_id},
        )
        checkpoint.raise_for_status()
        assert checkpoint.json()["state"] == {
            "channels": {"name": "v2", "answer": "hello v2"},
            "next_node": None,
        }
        print(f"v2 smoke: run_id={run_id} checkpoint_id={checkpoint.json()['checkpoint_id']}")

        deregister = await client.post(f"{url}/api/v1/workers/{worker._worker_id}/deregister")
        deregister.raise_for_status()


@pytest.mark.skipif(not os.getenv("DURAGRAPH_V2_URL"), reason="needs running v2 control plane")
async def test_shipped_v2_requeue_fences_stale_worker():
    url = os.environ["DURAGRAPH_V2_URL"].rstrip("/")
    graph_id = f"python_requeue_{uuid4().hex[:12]}"

    @Graph(id=graph_id)
    class Requeue:
        @entrypoint
        @node()
        def respond(self, state):
            return {"answer": f"retry {state['name']}"}

    graph = Requeue()
    workers = [Worker(url, name=f"worker-{uuid4().hex}") for _ in range(2)]
    async with httpx.AsyncClient(timeout=15.0) as client:
        for worker in workers:
            worker._client = client
            worker.register_graph(graph._get_definition(), instance=graph)
            worker._worker_id = await worker._register_with_control_plane()

        assistant = await client.post(f"{url}/api/v1/assistants", json={"graph_id": graph_id})
        assistant.raise_for_status()
        thread = await client.post(f"{url}/api/v1/threads", json={})
        thread.raise_for_status()
        thread_id = thread.json()["thread_id"]
        run = await client.post(
            f"{url}/api/v1/threads/{thread_id}/runs",
            json={"assistant_id": assistant.json()["assistant_id"], "input": {"name": "v2"}},
        )
        run.raise_for_status()
        run_id = run.json()["run_id"]

        first = (await workers[0]._claim_work(1))[0]
        deregister = await client.post(f"{url}/api/v1/workers/{workers[0]._worker_id}/deregister")
        deregister.raise_for_status()
        second = (await workers[1]._claim_work(1))[0]
        assert second["run"]["run_id"] == first["run"]["run_id"] == run_id
        assert second["lease_epoch"] > first["lease_epoch"]

        # Old worker cannot write a node or a terminal event on the new lease.
        await workers[0]._execute_run(first)
        assert workers[0]._health_metrics["runs_completed"] == 0
        await workers[1]._execute_run(second)
        recorded = await client.get(f"{url}/api/v1/threads/{thread_id}/runs/{run_id}")
        recorded.raise_for_status()
        assert recorded.json()["status"] == "success"
        latest = await client.get(
            f"{url}/api/v1/threads/{thread_id}/checkpoints/latest", params={"run_id": run_id}
        )
        latest.raise_for_status()
        assert latest.json()["state"]["channels"]["answer"] == "retry v2"
        deregister = await client.post(f"{url}/api/v1/workers/{workers[1]._worker_id}/deregister")
        deregister.raise_for_status()


@pytest.mark.skipif(not os.getenv("DURAGRAPH_V2_URL"), reason="needs running v2 control plane")
async def test_shipped_v2_node_error_reports_failed_run():
    url = os.environ["DURAGRAPH_V2_URL"].rstrip("/")
    graph_id = f"python_failure_{uuid4().hex[:12]}"

    @Graph(id=graph_id)
    class Failure:
        @entrypoint
        @node()
        def break_run(self, state):
            raise ValueError("expected smoke failure")

    graph = Failure()
    worker = Worker(url, name=f"worker-{uuid4().hex}")
    worker.register_graph(graph._get_definition(), instance=graph)
    async with httpx.AsyncClient(timeout=15.0) as client:
        worker._client = client
        worker._worker_id = await worker._register_with_control_plane()
        assistant = await client.post(f"{url}/api/v1/assistants", json={"graph_id": graph_id})
        assistant.raise_for_status()
        thread = await client.post(f"{url}/api/v1/threads", json={})
        thread.raise_for_status()
        thread_id = thread.json()["thread_id"]
        run = await client.post(
            f"{url}/api/v1/threads/{thread_id}/runs",
            json={"assistant_id": assistant.json()["assistant_id"], "input": {}},
        )
        run.raise_for_status()
        run_id = run.json()["run_id"]

        await worker._execute_run((await worker._claim_work(1))[0])
        recorded = await client.get(f"{url}/api/v1/threads/{thread_id}/runs/{run_id}")
        recorded.raise_for_status()
        assert recorded.json()["status"] == "error"
        assert worker._health_metrics["runs_failed"] == 1
        deregister = await client.post(f"{url}/api/v1/workers/{worker._worker_id}/deregister")
        deregister.raise_for_status()
