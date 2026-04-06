"""Worker implementation for DuraGraph control plane."""

import asyncio
import json
import signal
import time
from collections.abc import Callable
from enum import Enum
from typing import Any
from uuid import uuid4

import httpx

from duragraph.graph import GraphDefinition

try:
    import nats as nats_client

    NATS_AVAILABLE = True
except ImportError:
    NATS_AVAILABLE = False


class WorkerStatus(Enum):
    """Worker status states."""

    STARTING = "starting"
    READY = "ready"
    BUSY = "busy"
    DRAINING = "draining"
    STOPPED = "stopped"


class Worker:
    """Worker that connects to DuraGraph control plane and executes graphs.

    Supports two task delivery modes:
    - HTTP polling (default, always available)
    - NATS JetStream subscription (optional, for instant task delivery)

    When nats_url is provided and nats-py is installed, the worker subscribes
    to NATS task notifications for instant delivery and uses HTTP polling only
    as a fallback safety net (every 30s instead of 1s).
    """

    def __init__(
        self,
        control_plane_url: str,
        *,
        name: str | None = None,
        capabilities: list[str] | None = None,
        nats_url: str | None = None,
        poll_interval: float = 1.0,
        heartbeat_interval: float = 30.0,
        max_concurrent_runs: int = 10,
        shutdown_timeout: float = 60.0,
    ):
        self.control_plane_url = control_plane_url.rstrip("/")
        self.name = name or f"worker-{uuid4().hex[:8]}"
        self.capabilities = capabilities or []
        self.nats_url = nats_url
        self.poll_interval = poll_interval
        self.heartbeat_interval = heartbeat_interval
        self.max_concurrent_runs = max_concurrent_runs
        self.shutdown_timeout = shutdown_timeout

        self._worker_id: str | None = None
        self._graphs: dict[str, GraphDefinition] = {}
        self._graph_instances: dict[str, Any] = {}
        self._executors: dict[str, Callable[..., Any]] = {}
        self._status = WorkerStatus.STARTING
        self._client: httpx.AsyncClient | None = None

        # NATS client (optional)
        self._nc: Any | None = None
        self._js: Any | None = None
        self._nats_subscriptions: list[Any] = []
        self._use_nats = bool(nats_url and NATS_AVAILABLE)

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

    async def _connect_nats(self) -> None:
        """Connect to NATS and subscribe to task assignment subjects."""
        if not self._use_nats or not self.nats_url:
            return

        try:
            self._nc = await nats_client.connect(self.nats_url)
            self._js = self._nc.jetstream()
            print(f"✅ NATS connected: {self.nats_url}")

            graph_ids = list(self._graphs.keys())
            for graph_id in graph_ids:
                subject = f"duragraph.tasks.assign.{graph_id}"
                durable = f"worker-{self.name}-{graph_id}"

                try:
                    sub = await self._js.subscribe(
                        subject,
                        durable=durable,
                        manual_ack=True,
                    )
                    self._nats_subscriptions.append(sub)
                    print(f"  📡 Subscribed: {subject}")

                    asyncio.create_task(self._nats_message_loop(sub))
                except Exception as e:
                    print(f"  ⚠️  Failed to subscribe to {subject}: {e}")

        except Exception as e:
            print(f"⚠️  NATS connection failed, falling back to HTTP polling: {e}")
            self._use_nats = False
            self._nc = None
            self._js = None

    async def _nats_message_loop(self, sub: Any) -> None:
        """Process messages from a NATS subscription."""
        try:
            async for msg in sub.messages:
                if self._status in (WorkerStatus.DRAINING, WorkerStatus.STOPPED):
                    await msg.nak()
                    continue

                if len(self._active_runs) >= self.max_concurrent_runs:
                    await msg.nak(delay=5)
                    continue

                try:
                    task_data = json.loads(msg.data.decode())
                    run_id = task_data.get("run_id", "unknown")
                    print(f"📥 NATS task received: {run_id}")

                    work = await self._claim_task_via_http(task_data)
                    if work:
                        task = asyncio.create_task(self._execute_run(work))
                        self._run_tasks[run_id] = task

                        if self._status == WorkerStatus.READY:
                            self._status = WorkerStatus.BUSY

                    await msg.ack()
                except Exception as e:
                    print(f"  ⚠️  Error processing NATS message: {e}")
                    await msg.nak()
        except Exception:
            pass

    async def _claim_task_via_http(self, task_data: dict[str, Any]) -> dict[str, Any] | None:
        """Claim a task via HTTP polling after NATS notification."""
        if self._client is None or self._worker_id is None:
            return None

        try:
            response = await self._client.post(
                f"{self.control_plane_url}/api/v1/workers/{self._worker_id}/poll",
                json={"max_tasks": 1},
            )
            response.raise_for_status()
            data = response.json()
            tasks = data.get("tasks", [])
            if tasks:
                return tasks[0]
        except Exception:
            pass

        return task_data

    async def _register_with_control_plane(self, retry_count: int = 0) -> str:
        """Register this worker with the control plane."""
        if self._client is None:
            self._client = httpx.AsyncClient(timeout=30.0)

        self._health_metrics["registration_attempts"] += 1
        max_retries = 5

        graphs = [{"graph_id": g.graph_id, "definition": g.to_ir()} for g in self._graphs.values()]

        payload = {
            "worker_id": self.name,
            "name": self.name,
            "capabilities": {
                "graphs": list(self._graphs.keys()),
                "max_concurrent_runs": self.max_concurrent_runs,
            },
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

    async def _poll_for_work(self) -> dict[str, Any] | None:
        """Poll the control plane for work via HTTP."""
        if self._client is None or self._worker_id is None:
            return None

        if self._status == WorkerStatus.DRAINING:
            return None

        try:
            response = await self._client.post(
                f"{self.control_plane_url}/api/v1/workers/{self._worker_id}/poll",
                json={"max_tasks": 1},
            )
            if response.status_code == 204:
                return None
            response.raise_for_status()
            data = response.json()
            tasks = data.get("tasks", [])
            if tasks:
                return tasks[0]
            return None
        except httpx.HTTPStatusError as e:
            if e.response.status_code == 404:
                print("Worker not found on control plane, re-registering...")
                self._worker_id = await self._register_with_control_plane()
            return None
        except (httpx.ConnectError, httpx.TimeoutException):
            return None
        except Exception as e:
            print(f"Error polling for work: {e}")
            return None

    async def _execute_run(self, work: dict[str, Any]) -> None:
        """Execute a run from the control plane.

        Uses executor.execute_node() with the graph class instance to call
        user-defined node methods, matching the same execution path as
        GraphInstance.arun() for local execution.
        """
        from duragraph.executor import execute_node

        run_id = work.get("run_id")
        graph_id = work.get("graph_id")
        input_data = work.get("input", {})
        thread_id = work.get("thread_id")

        if not run_id or not graph_id:
            return

        self._active_runs.add(run_id)

        try:
            graph_def = self._graphs.get(graph_id)
            if not graph_def:
                await self._send_event(
                    run_id,
                    "run_failed",
                    {
                        "error": f"Graph '{graph_id}' not registered with this worker",
                    },
                )
                return

            instance = self._graph_instances.get(graph_id)

            await self._send_event(run_id, "run_started", {"thread_id": thread_id})

            try:
                state = input_data.copy()
                current_node = graph_def.entrypoint

                while current_node:
                    if self._status == WorkerStatus.DRAINING:
                        print(f"Worker draining, but completing run {run_id}")

                    await self._send_event(
                        run_id,
                        "node_started",
                        {
                            "node_id": current_node,
                        },
                    )

                    node_meta = graph_def.nodes.get(current_node)
                    if not node_meta:
                        raise ValueError(f"Node '{current_node}' not found")

                    if node_meta.node_type == "human":
                        result = await self._handle_human_node(run_id, node_meta, state)
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
                        "node_completed",
                        {
                            "node_id": current_node,
                            "output": result,
                        },
                    )

                    next_node = self._resolve_next_node(graph_def, current_node, result)
                    current_node = next_node

                await self._send_event(
                    run_id,
                    "run_completed",
                    {
                        "output": state,
                        "thread_id": thread_id,
                    },
                )
                self._health_metrics["runs_completed"] += 1

            except Exception as e:
                await self._send_event(
                    run_id,
                    "run_failed",
                    {
                        "error": str(e),
                        "thread_id": thread_id,
                    },
                )
                self._health_metrics["runs_failed"] += 1

        finally:
            self._active_runs.discard(run_id)
            self._run_tasks.pop(run_id, None)

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
        node_meta: Any,
        state: dict[str, Any],
    ) -> dict[str, Any] | None:
        """Handle a human-in-the-loop node by suspending the run."""
        config = node_meta.config
        prompt = config.get("prompt", "Please review")

        await self._send_event(
            run_id,
            "run_requires_action",
            {
                "action_type": "human_review",
                "prompt": prompt,
                "state": state,
            },
        )

        return None

    async def _send_event(
        self,
        run_id: str,
        event_type: str,
        data: dict[str, Any],
    ) -> None:
        """Send an event to the control plane via HTTP."""
        if self._client is None or self._worker_id is None:
            return

        payload = {
            "run_id": run_id,
            "event_type": event_type,
            "data": data,
        }

        try:
            response = await self._client.post(
                f"{self.control_plane_url}/api/v1/workers/{self._worker_id}/events",
                json=payload,
            )
            response.raise_for_status()
        except Exception:
            pass  # Best effort

    async def _heartbeat(self) -> None:
        """Send heartbeat to control plane."""
        if self._client is None or self._worker_id is None:
            return

        self._health_metrics["last_heartbeat"] = time.time()

        payload = {
            "status": self._status.value,
            "active_runs": len(self._active_runs),
            "total_runs": self._health_metrics["runs_completed"]
            + self._health_metrics["runs_failed"],
            "failed_runs": self._health_metrics["runs_failed"],
        }

        try:
            response = await self._client.post(
                f"{self.control_plane_url}/api/v1/workers/{self._worker_id}/heartbeat",
                json=payload,
            )
            response.raise_for_status()
        except httpx.HTTPStatusError as e:
            if e.response.status_code == 404:
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

        if self._use_nats:
            await self._connect_nats()

        heartbeat_task = asyncio.create_task(self._heartbeat_loop())
        poll_task = asyncio.create_task(self._poll_loop())

        try:
            await asyncio.gather(heartbeat_task, poll_task)
        except asyncio.CancelledError:
            pass

    async def _heartbeat_loop(self) -> None:
        """Heartbeat loop."""
        while self._status not in (WorkerStatus.STOPPED,):
            await self._heartbeat()
            await asyncio.sleep(self.heartbeat_interval)

    async def _poll_loop(self) -> None:
        """Poll loop - reduced frequency when NATS is active."""
        effective_interval = 30.0 if self._use_nats else self.poll_interval

        while self._status not in (WorkerStatus.STOPPED,):
            if (
                len(self._active_runs) < self.max_concurrent_runs
                and self._status != WorkerStatus.DRAINING
            ):
                work = await self._poll_for_work()
                if work:
                    run_id = work.get("run_id", "unknown")
                    print(f"📥 Received work (HTTP poll): {run_id}")

                    if self._status == WorkerStatus.READY:
                        self._status = WorkerStatus.BUSY

                    task = asyncio.create_task(self._execute_run(work))
                    self._run_tasks[run_id] = task

                    completed_tasks = [rid for rid, t in self._run_tasks.items() if t.done()]
                    for rid in completed_tasks:
                        del self._run_tasks[rid]

            if self._status == WorkerStatus.BUSY and len(self._active_runs) == 0:
                self._status = WorkerStatus.READY

            await asyncio.sleep(effective_interval)

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
        """Cleanup NATS and HTTP connections."""
        for sub in self._nats_subscriptions:
            try:
                await sub.unsubscribe()
            except Exception:
                pass
        self._nats_subscriptions.clear()

        if self._nc:
            try:
                await self._nc.close()
            except Exception:
                pass
            self._nc = None

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
        await self._cleanup()
        print("✓ Worker shut down gracefully")

    def _shutdown(self) -> None:
        """Legacy shutdown method for compatibility."""
        asyncio.create_task(self._graceful_shutdown())

    def stop(self) -> None:
        """Stop the worker."""
        asyncio.create_task(self._graceful_shutdown())
