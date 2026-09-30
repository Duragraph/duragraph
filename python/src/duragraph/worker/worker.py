"""Worker implementation for DuraGraph control plane."""

import asyncio
import signal
import time
from collections.abc import Callable
from enum import Enum
from typing import Any
from uuid import NAMESPACE_DNS, uuid4, uuid5

import httpx

from duragraph.graph import GraphDefinition


class WorkerStatus(Enum):
    """Worker status states."""

    STARTING = "starting"
    READY = "ready"
    BUSY = "busy"
    DRAINING = "draining"
    STOPPED = "stopped"


# The SDK's decorator names are richer than execution_history.node_type's
# CHECK (start/end/llm/tool/conditional/human). Preserve the original graph
# definition at registration; only normalize the persisted execution event.
_EVENT_NODE_TYPES = {
    "function": "tool",
    "dspy": "tool",
    "router": "conditional",
    "llm": "llm",
    "tool": "tool",
    "human": "human",
}


class Worker:
    """Worker that connects to DuraGraph control plane and executes graphs.

    Claims queued runs over HTTP and pushes epoch-fenced events to the server.
    """

    def __init__(
        self,
        control_plane_url: str,
        *,
        name: str | None = None,
        capabilities: list[str] | None = None,
        nats_url: str | None = None,
        claim_interval: float = 1.0,
        heartbeat_interval: float = 30.0,
        max_concurrent_runs: int = 10,
        shutdown_timeout: float = 60.0,
    ):
        self.control_plane_url = control_plane_url.rstrip("/")
        self.name = name or f"worker-{uuid4().hex[:8]}"
        if nats_url is not None:
            raise ValueError("NATS worker delivery is not supported by the v2 claim protocol")
        self._identity = str(uuid5(NAMESPACE_DNS, f"duragraph:{self.name}"))
        self.capabilities = capabilities or []
        self.claim_interval = claim_interval
        self.heartbeat_interval = heartbeat_interval
        self.max_concurrent_runs = max_concurrent_runs
        self.shutdown_timeout = shutdown_timeout

        self._worker_id: str | None = None
        self._graphs: dict[str, GraphDefinition] = {}
        self._graph_instances: dict[str, Any] = {}
        self._executors: dict[str, Callable[..., Any]] = {}
        self._status = WorkerStatus.STARTING
        self._client: httpx.AsyncClient | None = None

        # Track in-progress runs for graceful shutdown
        self._active_runs: set[str] = set()
        self._run_tasks: dict[str, asyncio.Task[None]] = {}

        # Health metrics
        self._health_metrics: dict[str, Any] = {
            "runs_completed": 0,
            "runs_failed": 0,
            "last_heartbeat": None,
            "uptime_start": None,
            "registration_attempts": 0,
        }

    def register_graph(
        self,
        definition: GraphDefinition,
        executor: Callable[..., Any] | None = None,
        instance: Any | None = None,
    ) -> None:
        """Register a graph definition with this worker.

        Args:
            definition: The graph definition (IR metadata + edges).
            executor: Optional custom executor callback.
            instance: The graph class instance containing user-defined node methods.
                      Required for the worker to call user-defined node functions.
        """
        self._graphs[definition.graph_id] = definition
        if instance is not None:
            self._graph_instances[definition.graph_id] = instance
        if executor:
            self._executors[definition.graph_id] = executor

    async def _register_with_control_plane(self, retry_count: int = 0) -> str:
        """Register this worker with the control plane."""
        if self._client is None:
            self._client = httpx.AsyncClient(timeout=30.0)

        self._health_metrics["registration_attempts"] += 1
        max_retries = 5

        # controlplane/endpoints/worker_types.go: GraphDefinitionInput.
        graphs = [
            {
                "name": getattr(g, "name", "") or g.graph_id,
                "description": getattr(g, "description", "") or "",
                "nodes": [
                    {"id": name, "type": meta.node_type, "config": meta.config}
                    for name, meta in g.nodes.items()
                ],
                "edges": [e.to_dict() for e in g.edges],
                "config": {"entry_point": g.entrypoint},
            }
            for g in self._graphs.values()
        ]

        payload = {
            "worker_id": self._identity,
            "graphs": list(self._graphs.keys()),
            "capacity": self.max_concurrent_runs,
            "graph_definitions": graphs,
        }

        try:
            response = await self._client.post(
                f"{self.control_plane_url}/api/v1/workers/register",
                json=payload,
            )
            response.raise_for_status()
            data = response.json()
            print(f"✓ Worker registered successfully (attempt {retry_count + 1})")
            return data["worker_id"]

        except (httpx.HTTPError, httpx.ConnectError) as e:
            if retry_count < max_retries:
                wait_time = 2 ** (retry_count + 1)
                print(
                    f"✗ Registration failed (attempt {retry_count + 1}/{max_retries}), "
                    f"retrying in {wait_time}s: {e}"
                )
                await asyncio.sleep(wait_time)
                return await self._register_with_control_plane(retry_count + 1)
            else:
                print(f"✗ Registration failed after {max_retries} attempts")
                raise

    async def _claim_work(self, max_runs: int) -> list[dict[str, Any]]:
        """Lease up to max_runs queued runs; the claim itself emits run.started."""
        if self._client is None or self._worker_id is None:
            return []

        if self._status == WorkerStatus.DRAINING:
            return []

        try:
            response = await self._client.post(
                f"{self.control_plane_url}/api/v1/workers/{self._worker_id}/runs/claim",
                json={"max_runs": max_runs},
            )
            response.raise_for_status()
            return response.json()["runs"]
        except httpx.HTTPStatusError as e:
            if e.response.status_code == 409:
                self._worker_id = await self._register_with_control_plane()
                return []
            raise
        except (httpx.ConnectError, httpx.TimeoutException):
            return []

    async def _execute_run(self, work: dict[str, Any]) -> None:
        """Execute a run from the control plane.

        Uses executor.execute_node() with the graph class instance to call
        user-defined node methods, matching the same execution path as
        GraphInstance.arun() for local execution.
        """
        from duragraph.executor import execute_node

        run = work["run"]
        run_id = run["run_id"]
        graph_id = work.get("graph_id") or run.get("graph_id")
        input_data = work.get("input") or run.get("input") or {}
        thread_id = run.get("thread_id")
        epoch = work["lease_epoch"]

        if not run_id or not graph_id:
            return

        self._active_runs.add(run_id)

        try:
            graph_def = self._graphs.get(graph_id)
            if not graph_def:
                await self._send_event(
                    run_id,
                    epoch,
                    "run.failed",
                    error=f"Graph '{graph_id}' not registered with this worker",
                )
                return

            instance = self._graph_instances.get(graph_id)

            try:
                state = input_data.copy()
                current_node = graph_def.entrypoint
                version = 0
                if work.get("checkpoint_id") is not None:
                    checkpoint = await self._read_checkpoint(thread_id, work["checkpoint_id"])
                    state = checkpoint["state"]["channels"]
                    current_node = checkpoint["state"]["next_node"]
                    version = checkpoint["version"]

                while current_node:
                    if self._status == WorkerStatus.DRAINING:
                        print(f"Worker draining, but completing run {run_id}")

                    node_meta = graph_def.nodes.get(current_node)
                    if not node_meta:
                        raise ValueError(f"Node '{current_node}' not found")
                    node_type = _EVENT_NODE_TYPES[node_meta.node_type]
                    await self._send_event(
                        run_id,
                        epoch,
                        "execution.node_started",
                        node_id=current_node,
                        node_type=node_type,
                        node_status="started",
                        input=state,
                    )

                    if node_meta.node_type == "human":
                        result = await self._handle_human_node(
                            run_id, epoch, current_node, node_meta, state
                        )
                        if result is None:
                            return
                    elif instance is not None:
                        node_method = getattr(instance, current_node, None)
                        if node_method is None:
                            raise ValueError(
                                f"Node method '{current_node}' not found on graph instance"
                            )
                        result = await execute_node(current_node, node_meta, node_method, state)
                    else:
                        raise ValueError(
                            f"No graph instance registered for '{graph_id}'. "
                            f"Pass instance= to register_graph() or use serve()."
                        )

                    if isinstance(result, dict):
                        state.update(result)

                    await self._send_event(
                        run_id,
                        epoch,
                        "execution.node_completed",
                        node_id=current_node,
                        node_type=node_type,
                        node_status="completed",
                        output=result,
                    )

                    next_node = self._resolve_next_node(graph_def, current_node, result)
                    version += 1
                    if thread_id and thread_id != "00000000-0000-0000-0000-000000000000":
                        await self._write_checkpoint(
                            thread_id, run_id, epoch, version, state, next_node
                        )
                    current_node = next_node

                await self._send_event(run_id, epoch, "run.completed", output=state)
                self._health_metrics["runs_completed"] += 1

            except Exception as e:
                if isinstance(e, httpx.HTTPStatusError) and e.response.status_code == 409:
                    # Fenced by a newer lease: never report failure on its behalf.
                    return
                await self._send_event(run_id, epoch, "run.failed", error=str(e))
                self._health_metrics["runs_failed"] += 1

        finally:
            self._active_runs.discard(run_id)

    def _resolve_next_node(
        self,
        graph_def: GraphDefinition,
        current_node: str,
        result: Any,
    ) -> str | None:
        """Resolve the next node to execute based on edges and result."""
        for edge in graph_def.edges:
            if edge.source == current_node:
                if isinstance(edge.target, str):
                    return edge.target
                elif isinstance(edge.target, dict):
                    if isinstance(result, str) and result in edge.target:
                        return edge.target[result]
                break
        return None

    async def _handle_human_node(
        self,
        run_id: str,
        epoch: int,
        node_id: str,
        node_meta: Any,
        state: dict[str, Any],
    ) -> dict[str, Any] | None:
        """Handle a human-in-the-loop node by suspending the run."""
        config = node_meta.config
        prompt = config.get("prompt", "Please review")

        await self._send_event(
            run_id,
            epoch,
            "run.requires_action",
            node_id=node_id,
            reason="approval_required",
            state={**state, "prompt": prompt},
        )

        return None

    async def _send_event(
        self,
        run_id: str,
        epoch: int,
        event_type: str,
        **data: Any,
    ) -> None:
        """Send an event to the control plane via HTTP."""
        if self._client is None or self._worker_id is None:
            raise RuntimeError("Worker not registered")
        response = await self._client.post(
            f"{self.control_plane_url}/api/v1/workers/{self._worker_id}/runs/{run_id}/events",
            json={"events": [{"type": event_type, "lease_epoch": epoch, **data}]},
        )
        response.raise_for_status()

    async def _read_checkpoint(self, thread_id: str, checkpoint_id: int) -> dict[str, Any]:
        assert self._client is not None
        response = await self._client.get(
            f"{self.control_plane_url}/api/v1/threads/{thread_id}/checkpoints/{checkpoint_id}"
        )
        response.raise_for_status()
        return response.json()

    async def _write_checkpoint(
        self,
        thread_id: str,
        run_id: str,
        epoch: int,
        version: int,
        state: dict[str, Any],
        next_node: str | None,
    ) -> None:
        assert self._client is not None
        response = await self._client.post(
            f"{self.control_plane_url}/api/v1/threads/{thread_id}/checkpoints",
            json={
                "run_id": run_id,
                "lease_epoch": epoch,
                "version": version,
                "state": {"channels": state, "next_node": next_node},
            },
        )
        response.raise_for_status()

    async def _heartbeat(self) -> None:
        """Send heartbeat to control plane."""
        if self._client is None or self._worker_id is None:
            return

        self._health_metrics["last_heartbeat"] = time.time()

        payload = {
            "status": "draining" if self._status == WorkerStatus.DRAINING else "online",
            "active_runs": len(self._active_runs),
        }

        try:
            response = await self._client.post(
                f"{self.control_plane_url}/api/v1/workers/{self._worker_id}/heartbeat",
                json=payload,
            )
            response.raise_for_status()
        except httpx.HTTPStatusError as e:
            if e.response.status_code == 409:
                print("Worker not found during heartbeat, re-registering...")
                self._worker_id = await self._register_with_control_plane()
        except (httpx.ConnectError, httpx.TimeoutException):
            print("Failed to send heartbeat (connection issue)")
        except Exception as e:
            print(f"Failed to send heartbeat: {e}")

    async def _run_loop(self) -> None:
        """Main worker loop."""
        self._status = WorkerStatus.STARTING
        self._health_metrics["uptime_start"] = time.time()

        print(f"🚀 Starting worker '{self.name}'...")
        try:
            self._worker_id = await self._register_with_control_plane()
            print(f"✓ Registered with worker_id: {self._worker_id}")
            self._status = WorkerStatus.READY
        except Exception as e:
            print(f"✗ Failed to register worker: {e}")
            raise

        heartbeat_task = asyncio.create_task(self._heartbeat_loop())
        claim_task = asyncio.create_task(self._claim_loop())

        try:
            await asyncio.gather(heartbeat_task, claim_task)
        except asyncio.CancelledError:
            pass
        finally:
            heartbeat_task.cancel()
            claim_task.cancel()
            await asyncio.gather(heartbeat_task, claim_task, return_exceptions=True)

    async def _heartbeat_loop(self) -> None:
        """Heartbeat loop."""
        while self._status not in (WorkerStatus.STOPPED,):
            await self._heartbeat()
            await asyncio.sleep(self.heartbeat_interval)

    async def _claim_loop(self) -> None:
        """Claim only available capacity; every claim already owns a lease."""
        while self._status not in (WorkerStatus.STOPPED,):
            available = self.max_concurrent_runs - len(self._run_tasks)
            if available > 0 and self._status != WorkerStatus.DRAINING:
                for work in await self._claim_work(available):
                    run_id = work["run"]["run_id"]
                    task = asyncio.create_task(self._execute_run(work))
                    self._run_tasks[run_id] = task
                    self._status = WorkerStatus.BUSY
                    task.add_done_callback(lambda t, rid=run_id: self._finish_run(rid, t))

            if self._status == WorkerStatus.BUSY and not self._run_tasks:
                self._status = WorkerStatus.READY

            await asyncio.sleep(self.claim_interval)

    def _finish_run(self, run_id: str, task: asyncio.Task[None]) -> None:
        self._run_tasks.pop(run_id, None)
        if not task.cancelled() and task.exception() is not None:
            print(f"Run {run_id} could not report its result: {task.exception()}")

    def run(self) -> None:
        """Run the worker (blocking)."""
        loop = asyncio.new_event_loop()
        asyncio.set_event_loop(loop)

        for sig in (signal.SIGTERM, signal.SIGINT):
            loop.add_signal_handler(sig, lambda: asyncio.create_task(self._graceful_shutdown()))

        try:
            loop.run_until_complete(self.arun())
        except KeyboardInterrupt:
            print("\n⚠️  Interrupt received, shutting down gracefully...")
            loop.run_until_complete(self._graceful_shutdown())
        finally:
            if self._client:
                loop.run_until_complete(self._client.aclose())
            loop.close()

    async def arun(self) -> None:
        """Run the worker asynchronously."""
        try:
            await self._run_loop()
        finally:
            await self._cleanup()

    async def _cleanup(self) -> None:
        """Close the HTTP connection."""
        if self._client:
            await self._client.aclose()
            self._client = None

    async def _graceful_shutdown(self) -> None:
        """Gracefully shutdown the worker."""
        if self._status == WorkerStatus.STOPPED:
            return

        print("\n🛑 Initiating graceful shutdown...")
        self._status = WorkerStatus.DRAINING

        await self._heartbeat()

        if self._active_runs:
            print(f"⏳ Waiting for {len(self._active_runs)} active run(s) to complete...")
            print(f"   Active runs: {', '.join(self._active_runs)}")

            start_time = time.time()
            while self._active_runs and (time.time() - start_time) < self.shutdown_timeout:
                await asyncio.sleep(1)
                remaining = len(self._active_runs)
                if remaining > 0:
                    elapsed = int(time.time() - start_time)
                    print(f"   Still waiting for {remaining} run(s) - {elapsed}s elapsed...")

            if self._active_runs:
                print(
                    f"⚠️  Timeout reached, forcing shutdown with "
                    f"{len(self._active_runs)} run(s) still active"
                )
                for task in self._run_tasks.values():
                    if not task.done():
                        task.cancel()
            else:
                print("✓ All runs completed successfully")

        self._status = WorkerStatus.STOPPED
        if self._client is not None and self._worker_id is not None:
            try:
                response = await self._client.post(
                    f"{self.control_plane_url}/api/v1/workers/{self._worker_id}/deregister"
                )
                response.raise_for_status()
            except httpx.HTTPError as e:
                print(f"Failed to deregister worker: {e}")
        await self._cleanup()
        print("✓ Worker shut down gracefully")

    def _shutdown(self) -> None:
        """Legacy shutdown method for compatibility."""
        asyncio.create_task(self._graceful_shutdown())

    def stop(self) -> None:
        """Stop the worker."""
        asyncio.create_task(self._graceful_shutdown())
