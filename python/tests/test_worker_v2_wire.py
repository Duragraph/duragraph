"""Protocol checks against controlplane/endpoints/worker_types.go and workers_claim.go."""

import json
from uuid import UUID

import httpx
import pytest

from duragraph import Graph, entrypoint, node
from duragraph.worker import Worker, WorkerStatus


@pytest.fixture
def graph():
    @Graph(id="example")
    class Example:
        @entrypoint
        @node()
        def apply(self, state):
            return {"answer": state["question"] + "!"}

    return Example()


async def test_claim_event_checkpoint_roundtrip(graph):
    calls = []
    run_id = "11111111-1111-1111-1111-111111111111"
    thread_id = "22222222-2222-2222-2222-222222222222"
    claimed = False

    def server(request):
        nonlocal claimed
        payload = json.loads(request.content) if request.content else None
        calls.append((request.method, request.url.path, payload))
        if request.url.path.endswith("/register"):
            assert UUID(payload["worker_id"])
            assert payload["graphs"] == ["example"]
            assert payload["capacity"] == 10
            assert payload["graph_definitions"][0]["config"]["entry_point"] == "apply"
            return httpx.Response(200, json={"worker_id": payload["worker_id"]})
        if request.url.path.endswith("/runs/claim"):
            assert payload == {"max_runs": 2}
            if claimed:
                return httpx.Response(200, json={"runs": []})
            claimed = True
            return httpx.Response(
                200,
                json={
                    "runs": [
                        {
                            "run": {"run_id": run_id, "thread_id": thread_id},
                            "graph_id": "example",
                            "input": {"question": "yes"},
                            "lease_epoch": 7,
                            "checkpoint_id": None,
                        }
                    ]
                },
            )
        if request.url.path.endswith("/events"):
            event = payload["events"][0]
            assert event["lease_epoch"] == 7
            assert event["type"] != "run.started"
            if event["type"].startswith("execution.node_"):
                assert event["node_type"] == "tool"
            return httpx.Response(200)
        if request.url.path.endswith("/checkpoints"):
            assert payload["run_id"] == run_id
            assert payload["lease_epoch"] == 7
            assert payload["state"]["channels"]["answer"] == "yes!"
            return httpx.Response(200, json={"checkpoint_id": 5})
        raise AssertionError(request.url.path)

    worker = Worker("http://server", name="example-worker")
    worker.register_graph(graph._get_definition(), instance=graph)
    worker._client = httpx.AsyncClient(transport=httpx.MockTransport(server))
    worker._worker_id = await worker._register_with_control_plane()
    work = await worker._claim_work(2)
    assert len(work) == 1
    assert await worker._claim_work(2) == []
    await worker._execute_run(work[0])
    events = [
        payload["events"][0]["type"] for _, path, payload in calls if path.endswith("/events")
    ]
    assert events == ["execution.node_started", "execution.node_completed", "run.completed"]
    assert worker._health_metrics["runs_completed"] == 1
    await worker._client.aclose()


async def test_stale_epoch_stops_without_terminal_event(graph):
    events = []

    def server(request):
        if request.url.path.endswith("/events"):
            events.append(json.loads(request.content)["events"][0]["type"])
            return httpx.Response(409, json={"message": "stale lease_epoch"})
        raise AssertionError(request.url.path)

    worker = Worker("http://server")
    worker.register_graph(graph._get_definition(), instance=graph)
    worker._client = httpx.AsyncClient(transport=httpx.MockTransport(server))
    worker._worker_id = worker._identity
    await worker._execute_run(
        {
            "run": {"run_id": "run", "thread_id": None},
            "graph_id": "example",
            "input": {"question": "yes"},
            "lease_epoch": 4,
            "checkpoint_id": None,
        }
    )
    assert events == ["execution.node_started"]
    await worker._client.aclose()


async def test_claim_conflict_reregisters_without_using_retired_poll():
    paths = []

    def server(request):
        paths.append(request.url.path)
        if request.url.path.endswith("/runs/claim"):
            return httpx.Response(409)
        if request.url.path.endswith("/register"):
            return httpx.Response(200, json={"worker_id": worker._identity})
        raise AssertionError(request.url.path)

    worker = Worker("http://server")
    worker._client = httpx.AsyncClient(transport=httpx.MockTransport(server))
    worker._worker_id = worker._identity
    worker._status = WorkerStatus.READY
    assert await worker._claim_work(1) == []
    assert paths == [f"/api/v1/workers/{worker._identity}/runs/claim", "/api/v1/workers/register"]
    await worker._client.aclose()


def test_nats_delivery_is_explicitly_unsupported():
    with pytest.raises(ValueError, match="not supported"):
        Worker("http://server", nats_url="nats://server:4222")


async def test_claimed_checkpoint_resumes_at_next_node():
    @Graph(id="resume")
    class Resume:
        @entrypoint
        @node()
        def first(self, state):
            raise AssertionError("already checkpointed; do not repeat")

        @node()
        def second(self, state):
            return {"done": state["prior"]}

        _ = first >> second

    graph = Resume()
    paths = []

    def server(request):
        paths.append(request.url.path)
        if request.method == "GET":
            return httpx.Response(
                200,
                json={
                    "checkpoint_id": 8,
                    "version": 1,
                    "state": {"channels": {"prior": "yes"}, "next_node": "second"},
                },
            )
        if request.url.path.endswith("/events"):
            return httpx.Response(200)
        if request.url.path.endswith("/checkpoints"):
            assert json.loads(request.content)["version"] == 2
            return httpx.Response(200, json={"checkpoint_id": 9})
        raise AssertionError(request.url.path)

    worker = Worker("http://server")
    worker.register_graph(graph._get_definition(), instance=graph)
    worker._client = httpx.AsyncClient(transport=httpx.MockTransport(server))
    worker._worker_id = worker._identity
    await worker._execute_run(
        {
            "run": {"run_id": "run", "thread_id": "thread"},
            "graph_id": "resume",
            "input": {},
            "lease_epoch": 2,
            "checkpoint_id": 8,
        }
    )
    assert "/api/v1/threads/thread/checkpoints/8" in paths
    assert worker._health_metrics["runs_completed"] == 1
    await worker._client.aclose()
