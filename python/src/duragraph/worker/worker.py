"""Worker implementation for DuraGraph control plane."""

import asyncio
import signal
import time
from collections.abc import Callable
from enum import Enum
from typing import Any
from uuid import uuid4

import httpx

from duragraph.graph import GraphDefinition


class WorkerStatus(Enum):
    """Worker status states."""

    STARTING = "starting"
    READY = "ready"
    BUSY = "busy"
    DRAINING = "draining"
    STOPPED = "stopped"


class Worker:
    """Worker that connects to DuraGraph control plane and executes graphs."""

    def __init__(
        self,
        control_plane_url: str,
        *,
        name: str | None = None,
        capabilities: list[str] | None = None,
        poll_interval: float = 1.0,
        heartbeat_interval: float = 30.0,
        max_concurrent_runs: int = 10,
        shutdown_timeout: float = 60.0,
    ):
        """Initialize worker.

        Args:
            control_plane_url: URL of the DuraGraph control plane.
            name: Optional name for this worker.
            capabilities: Optional list of capabilities (e.g., ["openai", "tools"]).
            poll_interval: Interval in seconds between polling for work.
            heartbeat_interval: Interval in seconds between heartbeats (default 30s).
            max_concurrent_runs: Maximum number of concurrent runs (default 10).
            shutdown_timeout: Maximum time to wait for runs to complete during shutdown (default 60s).
        """
        self.control_plane_url = control_plane_url.rstrip("/")
        self.name = name or f"worker-{uuid4().hex[:8]}"
        self.capabilities = capabilities or []
        self.poll_interval = poll_interval
        self.heartbeat_interval = heartbeat_interval
        self.max_concurrent_runs = max_concurrent_runs
        self.shutdown_timeout = shutdown_timeout

        self._worker_id: str | None = None
        self._graphs: dict[str, GraphDefinition] = {}
        self._executors: dict[str, Callable[..., Any]] = {}
        self._status = WorkerStatus.STARTING
        self._client: httpx.AsyncClient | None = None

        # Track in-progress runs for graceful shutdown
        self._active_runs: set[str] = set()
        self._run_tasks: dict[str, asyncio.Task] = {}

        # Health metrics
        self._health_metrics = {
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
    ) -> None:
        """Register a graph definition with this worker.

        Args:
            definition: The graph definition to register.
            executor: Optional custom executor function.
        """
        self._graphs[definition.graph_id] = definition
        if executor:
            self._executors[definition.graph_id] = executor

    async def _register_with_control_plane(self, retry_count: int = 0) -> str:
        """Register this worker with the control plane.

        Args:
            retry_count: Current retry attempt number.

        Returns:
            Worker ID from control plane.

        Raises:
            Exception: If registration fails after max retries.
        """
        if self._client is None:
            self._client = httpx.AsyncClient(timeout=30.0)

        self._health_metrics["registration_attempts"] += 1
        max_retries = 5

        # Prepare graph definitions
        graphs = [{"graph_id": g.graph_id, "definition": g.to_ir()} for g in self._graphs.values()]

        payload = {
            "name": self.name,
            "capabilities": self.capabilities,
            "graphs": graphs,
            "status": self._status.value,
            "metrics": {
                "max_concurrent_runs": self.max_concurrent_runs,
                "active_runs": len(self._active_runs),
            },
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
                # Exponential backoff: 2, 4, 8, 16, 32 seconds
                wait_time = 2 ** (retry_count + 1)
                print(
                    f"✗ Registration failed (attempt {retry_count + 1}/{max_retries}), retrying in {wait_time}s: {e}"
                )
                await asyncio.sleep(wait_time)
                return await self._register_with_control_plane(retry_count + 1)
            else:
                print(f"✗ Registration failed after {max_retries} attempts")
                raise

    async def _poll_for_work(self) -> dict[str, Any] | None:
        """Poll the control plane for work."""
        if self._client is None or self._worker_id is None:
            return None

        # Don't accept new work if draining
        if self._status == WorkerStatus.DRAINING:
            return None

        try:
            response = await self._client.get(
                f"{self.control_plane_url}/api/v1/workers/{self._worker_id}/poll",
            )
            if response.status_code == 204:
                return None
            response.raise_for_status()
            return response.json()
        except httpx.HTTPStatusError as e:
            if e.response.status_code == 404:
                # Worker not found, re-register
                print("Worker not found on control plane, re-registering...")
                self._worker_id = await self._register_with_control_plane()
            return None
        except (httpx.ConnectError, httpx.TimeoutException):
            # Connection issues, will retry on next poll
            return None
        except Exception as e:
            print(f"Error polling for work: {e}")
            return None

    async def _execute_run(self, work: dict[str, Any]) -> None:
        """Execute a run from the control plane."""
        run_id = work.get("run_id")
        graph_id = work.get("graph_id")
        input_data = work.get("input", {})
        thread_id = work.get("thread_id")

        if not run_id or not graph_id:
            return

        # Track this run as active
        self._active_runs.add(run_id)

        try:
            # Find the graph definition
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

            # Start the run
            await self._send_event(run_id, "run_started", {"thread_id": thread_id})

            try:
                # Execute nodes
                state = input_data.copy()
                current_node = graph_def.entrypoint

                while current_node:
                    # Check if we're shutting down
                    if self._status == WorkerStatus.DRAINING:
                        print(f"Worker draining, but completing run {run_id}")

                    await self._send_event(
                        run_id,
                        "node_started",
                        {
                            "node_id": current_node,
                        },
                    )

                    # Get node metadata
                    node_meta = graph_def.nodes.get(current_node)
                    if not node_meta:
                        raise ValueError(f"Node '{current_node}' not found")

                    # Execute based on node type
                    if node_meta.node_type == "llm":
                        result = await self._execute_llm_node(node_meta, state)
                    elif node_meta.node_type == "tool":
                        result = await self._execute_tool_node(node_meta, state)
                    elif node_meta.node_type == "human":
                        result = await self._execute_human_node(run_id, node_meta, state)
                        if result is None:
                            # Interrupted, waiting for human input
                            return
                    else:
                        # Default function node - just pass through
                        result = state

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

                    # Find next node
                    next_node = None
                    for edge in graph_def.edges:
                        if edge.source == current_node:
                            if isinstance(edge.target, str):
                                next_node = edge.target
                            elif isinstance(edge.target, dict):
                                if isinstance(result, str) and result in edge.target:
                                    next_node = edge.target[result]
                            break

                    current_node = next_node

                # Run completed
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
            # Remove from active runs
            self._active_runs.discard(run_id)
            self._run_tasks.pop(run_id, None)

    async def _execute_llm_node(
        self,
        node_meta: Any,
        state: dict[str, Any],
    ) -> dict[str, Any]:
        """Execute an LLM node."""
        # Placeholder - would integrate with LLM providers
        config = node_meta.config
        model = config.get("model", "gpt-4o-mini")

        # For now, just echo the state
        return {"llm_response": f"[{model}] Processed state"}

    async def _execute_tool_node(
        self,
        node_meta: Any,
        state: dict[str, Any],
    ) -> dict[str, Any]:
        """Execute a tool node."""
        # Placeholder - would execute registered tools
        return state

    async def _execute_human_node(
        self,
        run_id: str,
        node_meta: Any,
        state: dict[str, Any],
    ) -> dict[str, Any] | None:
        """Execute a human-in-the-loop node."""
        config = node_meta.config
        prompt = config.get("prompt", "Please review")

        # Signal that human input is required
        await self._send_event(
            run_id,
            "run_requires_action",
            {
                "action_type": "human_review",
                "prompt": prompt,
                "state": state,
            },
        )

        # Return None to indicate the run is waiting
        return None

    async def _send_event(
        self,
        run_id: str,
        event_type: str,
        data: dict[str, Any],
    ) -> None:
        """Send an event to the control plane."""
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
        """Send heartbeat to control plane with health metrics."""
        if self._client is None or self._worker_id is None:
            return

        self._health_metrics["last_heartbeat"] = time.time()

        # Calculate uptime
        uptime = None
        if self._health_metrics["uptime_start"]:
            uptime = int(time.time() - self._health_metrics["uptime_start"])

        payload = {
            "status": self._status.value,
            "metrics": {
                "active_runs": len(self._active_runs),
                "runs_completed": self._health_metrics["runs_completed"],
                "runs_failed": self._health_metrics["runs_failed"],
                "uptime_seconds": uptime,
            },
        }

        try:
            response = await self._client.post(
                f"{self.control_plane_url}/api/v1/workers/{self._worker_id}/heartbeat",
                json=payload,
            )
            response.raise_for_status()
        except httpx.HTTPStatusError as e:
            if e.response.status_code == 404:
                # Worker not found, re-register
                print("Worker not found during heartbeat, re-registering...")
                self._worker_id = await self._register_with_control_plane()
        except (httpx.ConnectError, httpx.TimeoutException):
            print("Failed to send heartbeat (connection issue)")
        except Exception as e:
            print(f"Failed to send heartbeat: {e}")

    async def _run_loop(self) -> None:
        """Main worker loop with concurrent run execution and time-based heartbeat."""
        self._status = WorkerStatus.STARTING
        self._health_metrics["uptime_start"] = time.time()

        # Register with control plane
        print(f"🚀 Starting worker '{self.name}'...")
        try:
            self._worker_id = await self._register_with_control_plane()
            print(f"✓ Registered with worker_id: {self._worker_id}")
            self._status = WorkerStatus.READY
        except Exception as e:
            print(f"✗ Failed to register worker: {e}")
            raise

        # Create background tasks for heartbeat and polling
        heartbeat_task = asyncio.create_task(self._heartbeat_loop())
        poll_task = asyncio.create_task(self._poll_loop())

        try:
            # Wait for both tasks (they should run forever unless cancelled)
            await asyncio.gather(heartbeat_task, poll_task)
        except asyncio.CancelledError:
            # Tasks were cancelled during shutdown
            pass

    async def _heartbeat_loop(self) -> None:
        """Separate loop for sending heartbeats every N seconds."""
        while self._status not in (WorkerStatus.STOPPED,):
            await self._heartbeat()
            await asyncio.sleep(self.heartbeat_interval)

    async def _poll_loop(self) -> None:
        """Separate loop for polling and executing work."""
        while self._status not in (WorkerStatus.STOPPED,):
            # Check if we can accept new work
            if (
                len(self._active_runs) < self.max_concurrent_runs
                and self._status != WorkerStatus.DRAINING
            ):
                work = await self._poll_for_work()
                if work:
                    run_id = work.get("run_id", "unknown")
                    print(f"📥 Received work: {run_id}")

                    # Update status if needed
                    if self._status == WorkerStatus.READY:
                        self._status = WorkerStatus.BUSY

                    # Execute run concurrently
                    task = asyncio.create_task(self._execute_run(work))
                    self._run_tasks[run_id] = task

                    # Clean up completed tasks
                    completed_tasks = [rid for rid, t in self._run_tasks.items() if t.done()]
                    for rid in completed_tasks:
                        del self._run_tasks[rid]

            # Update status based on active runs
            if self._status == WorkerStatus.BUSY and len(self._active_runs) == 0:
                self._status = WorkerStatus.READY

            await asyncio.sleep(self.poll_interval)

    def run(self) -> None:
        """Run the worker (blocking)."""
        loop = asyncio.new_event_loop()
        asyncio.set_event_loop(loop)

        # Handle shutdown signals
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
            if self._client:
                await self._client.aclose()

    async def _graceful_shutdown(self) -> None:
        """Gracefully shutdown the worker, waiting for active runs to complete."""
        if self._status == WorkerStatus.STOPPED:
            return

        print("\n🛑 Initiating graceful shutdown...")
        self._status = WorkerStatus.DRAINING

        # Send status update to control plane
        await self._heartbeat()

        if self._active_runs:
            print(f"⏳ Waiting for {len(self._active_runs)} active run(s) to complete...")
            print(f"   Active runs: {', '.join(self._active_runs)}")

            # Wait for active runs with timeout
            start_time = time.time()
            while self._active_runs and (time.time() - start_time) < self.shutdown_timeout:
                await asyncio.sleep(1)
                remaining = len(self._active_runs)
                if remaining > 0:
                    elapsed = int(time.time() - start_time)
                    print(f"   Still waiting for {remaining} run(s) - {elapsed}s elapsed...")

            if self._active_runs:
                print(
                    f"⚠️  Timeout reached, forcing shutdown with {len(self._active_runs)} run(s) still active"
                )
                # Cancel remaining tasks
                for task in self._run_tasks.values():
                    if not task.done():
                        task.cancel()
            else:
                print("✓ All runs completed successfully")

        self._status = WorkerStatus.STOPPED
        print("✓ Worker shut down gracefully")

    def _shutdown(self) -> None:
        """Legacy shutdown method for compatibility."""
        asyncio.create_task(self._graceful_shutdown())

    def stop(self) -> None:
        """Stop the worker."""
        asyncio.create_task(self._graceful_shutdown())
