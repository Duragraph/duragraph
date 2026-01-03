"""Worker protocol implementation.

Handles registration, heartbeat, run assignment, and event streaming.
"""

import asyncio
import json
import uuid
from dataclasses import dataclass, field
from datetime import datetime, timezone
from typing import Any, Callable, Optional

import httpx
import structlog

from .config import config
from .events import Event, EventEmitter
from .executor import ExecutionError, InterruptError, execute_run
from .graphs import GRAPHS, get_graph

log = structlog.get_logger()


@dataclass
class WorkerState:
    """Current state of the worker."""

    worker_id: str
    status: str = "idle"  # idle, running, stopping
    active_runs: dict = field(default_factory=dict)
    total_runs: int = 0
    failed_runs: int = 0
    registered: bool = False
    last_heartbeat: Optional[datetime] = None


class Worker:
    """Mock worker that executes graphs for DuraGraph control plane."""

    def __init__(
        self,
        control_plane_url: str = config.control_plane_url,
        worker_id: Optional[str] = None,
        worker_name: str = config.worker_name,
    ):
        self.control_plane_url = control_plane_url.rstrip("/")
        self.worker_name = worker_name
        self.state = WorkerState(
            worker_id=worker_id or f"mock-worker-{uuid.uuid4().hex[:8]}"
        )

        self._client = httpx.AsyncClient(timeout=30.0)
        self._heartbeat_task: Optional[asyncio.Task] = None
        self._poll_task: Optional[asyncio.Task] = None
        self._running = False

        # Event callbacks
        self._on_event: Optional[Callable[[Event], None]] = None

    @property
    def api_url(self) -> str:
        """Base API URL."""
        return f"{self.control_plane_url}/api/v1"

    async def start(self):
        """Start the worker (register, heartbeat, poll for runs)."""
        log.info("Starting worker", worker_id=self.state.worker_id)

        self._running = True

        # Register with control plane
        await self._register()

        # Start background tasks
        self._heartbeat_task = asyncio.create_task(self._heartbeat_loop())
        self._poll_task = asyncio.create_task(self._poll_loop())

        log.info("Worker started", worker_id=self.state.worker_id)

    async def stop(self):
        """Stop the worker gracefully."""
        log.info("Stopping worker", worker_id=self.state.worker_id)

        self._running = False

        # Cancel background tasks
        if self._heartbeat_task:
            self._heartbeat_task.cancel()
            try:
                await self._heartbeat_task
            except asyncio.CancelledError:
                pass

        if self._poll_task:
            self._poll_task.cancel()
            try:
                await self._poll_task
            except asyncio.CancelledError:
                pass

        # Deregister
        await self._deregister()

        await self._client.aclose()

        log.info("Worker stopped", worker_id=self.state.worker_id)

    async def _register(self):
        """Register with the control plane."""
        # Build graph definitions for registration
        graph_definitions = []
        for graph_id, graph in GRAPHS.items():
            graph_definitions.append({
                "graph_id": graph_id,
                "name": graph.name,
                "description": graph.description,
                "nodes": [
                    {
                        "id": n.id,
                        "type": n.type.value,
                        "config": n.config,
                    }
                    for n in graph.nodes
                ],
                "edges": [
                    {
                        "source": e.source,
                        "target": e.target,
                        "condition": e.condition,
                    }
                    for e in graph.edges
                ],
                "entry_point": graph.entry_point,
            })

        payload = {
            "worker_id": self.state.worker_id,
            "name": self.worker_name,
            "capabilities": {
                "graphs": list(GRAPHS.keys()),
                "max_concurrent_runs": config.max_concurrent_runs,
            },
            "graph_definitions": graph_definitions,
            "status": "ready",
        }

        try:
            response = await self._client.post(
                f"{self.api_url}/workers/register",
                json=payload,
            )

            if response.status_code == 404:
                # Endpoint doesn't exist yet - that's okay for mock
                log.warning("Worker registration endpoint not found (expected during development)")
                self.state.registered = True
                return

            response.raise_for_status()
            result = response.json()

            self.state.worker_id = result.get("worker_id", self.state.worker_id)
            self.state.registered = True

            log.info(
                "Worker registered",
                worker_id=self.state.worker_id,
                graphs=list(GRAPHS.keys()),
            )

        except httpx.HTTPStatusError as e:
            log.error("Registration failed", status=e.response.status_code, error=str(e))
            raise
        except httpx.RequestError as e:
            log.error("Registration request failed", error=str(e))
            # Continue anyway for development
            self.state.registered = True

    async def _deregister(self):
        """Deregister from the control plane."""
        if not self.state.registered:
            return

        try:
            response = await self._client.post(
                f"{self.api_url}/workers/{self.state.worker_id}/deregister",
            )
            if response.status_code != 404:
                response.raise_for_status()
            log.info("Worker deregistered", worker_id=self.state.worker_id)
        except Exception as e:
            log.warning("Deregistration failed", error=str(e))

        self.state.registered = False

    async def _heartbeat_loop(self):
        """Send periodic heartbeats to control plane."""
        while self._running:
            try:
                await self._send_heartbeat()
                await asyncio.sleep(config.heartbeat_interval_seconds)
            except asyncio.CancelledError:
                break
            except Exception as e:
                log.error("Heartbeat error", error=str(e))
                await asyncio.sleep(5)  # Retry after short delay

    async def _send_heartbeat(self):
        """Send a single heartbeat."""
        payload = {
            "worker_id": self.state.worker_id,
            "status": self.state.status,
            "active_runs": len(self.state.active_runs),
            "total_runs": self.state.total_runs,
            "failed_runs": self.state.failed_runs,
            "timestamp": datetime.now(timezone.utc).isoformat(),
        }

        try:
            response = await self._client.post(
                f"{self.api_url}/workers/{self.state.worker_id}/heartbeat",
                json=payload,
            )
            if response.status_code != 404:
                response.raise_for_status()
            self.state.last_heartbeat = datetime.now(timezone.utc)
        except httpx.RequestError as e:
            log.warning("Heartbeat request failed", error=str(e))

    async def _poll_loop(self):
        """Poll for pending runs to execute."""
        while self._running:
            try:
                # Only poll if we have capacity
                if len(self.state.active_runs) < config.max_concurrent_runs:
                    await self._poll_for_runs()

                await asyncio.sleep(1)  # Poll every second
            except asyncio.CancelledError:
                break
            except Exception as e:
                log.error("Poll error", error=str(e))
                await asyncio.sleep(5)

    async def _poll_for_runs(self):
        """Poll control plane for pending runs."""
        try:
            response = await self._client.post(
                f"{self.api_url}/workers/{self.state.worker_id}/poll",
                json={
                    "max_tasks": config.max_concurrent_runs - len(self.state.active_runs),
                },
            )

            if response.status_code == 404:
                # Endpoint doesn't exist yet
                return

            if response.status_code == 204:
                # No runs available
                return

            response.raise_for_status()
            result = response.json()

            tasks = result.get("tasks", [])
            for task in tasks:
                # Map task fields to run format
                run_data = {
                    "run_id": task.get("run_id"),
                    "thread_id": task.get("thread_id"),
                    "assistant_id": task.get("assistant_id"),
                    "graph_id": task.get("graph_id"),
                    "input": task.get("input", {}),
                    "config": task.get("config", {}),
                }
                # Execute each run in background
                asyncio.create_task(self._execute_run(run_data))

        except httpx.RequestError as e:
            log.debug("Poll request failed (control plane may not be ready)", error=str(e))

    async def _execute_run(self, run_data: dict):
        """Execute a single run."""
        run_id = run_data.get("run_id")
        thread_id = run_data.get("thread_id")
        assistant_id = run_data.get("assistant_id")
        input_data = run_data.get("input", {})
        graph_id = run_data.get("graph_id", config.mock_graph)

        log.info(
            "Executing run",
            run_id=run_id,
            thread_id=thread_id,
            graph_id=graph_id,
        )

        self.state.active_runs[run_id] = {
            "started_at": datetime.now(timezone.utc),
            "thread_id": thread_id,
        }
        self.state.status = "running"
        self.state.total_runs += 1

        try:
            # Notify control plane that run is starting
            await self._update_run_status(thread_id, run_id, "in_progress")

            # Execute the graph
            state, emitter = await execute_run(
                run_id=run_id,
                graph_id=graph_id,
                input_data=input_data,
            )

            # Stream events to control plane
            await self._send_events(thread_id, run_id, emitter.get_all_events())

            # Update run with final state
            await self._complete_run(
                thread_id,
                run_id,
                status="success",
                output=state.to_dict(),
                tokens=emitter.get_token_summary(),
            )

            log.info("Run completed", run_id=run_id, status="success")

        except InterruptError as e:
            # Run is interrupted, waiting for human input
            await self._update_run_status(
                thread_id,
                run_id,
                "interrupted",
                metadata={
                    "interrupted_at": e.node_id,
                    "prompt": e.prompt,
                    "required_fields": e.required_fields,
                },
            )
            log.info("Run interrupted", run_id=run_id, node=e.node_id)

        except ExecutionError as e:
            self.state.failed_runs += 1
            await self._complete_run(
                thread_id,
                run_id,
                status="error",
                error=str(e),
                error_node=e.node_id,
            )
            log.error("Run failed", run_id=run_id, error=str(e), node=e.node_id)

        except Exception as e:
            self.state.failed_runs += 1
            await self._complete_run(
                thread_id,
                run_id,
                status="error",
                error=str(e),
            )
            log.error("Run failed unexpectedly", run_id=run_id, error=str(e))

        finally:
            del self.state.active_runs[run_id]
            if not self.state.active_runs:
                self.state.status = "idle"

    async def _update_run_status(
        self,
        thread_id: str,
        run_id: str,
        status: str,
        metadata: Optional[dict] = None,
    ):
        """Update run status in control plane via worker events API."""
        payload = {
            "run_id": run_id,
            "event_type": f"run_{status}",
            "data": {
                "status": status,
                "thread_id": thread_id,
            },
        }
        if metadata:
            payload["data"]["metadata"] = metadata

        try:
            response = await self._client.post(
                f"{self.api_url}/workers/{self.state.worker_id}/events",
                json=payload,
            )
            if response.status_code != 404:
                response.raise_for_status()
        except Exception as e:
            log.warning("Failed to update run status", run_id=run_id, error=str(e))

    async def _complete_run(
        self,
        thread_id: str,
        run_id: str,
        status: str,
        output: Optional[dict] = None,
        error: Optional[str] = None,
        error_node: Optional[str] = None,
        tokens: Optional[dict] = None,
    ):
        """Mark run as complete in control plane via worker events API."""
        event_type = "run_completed" if status == "success" else "run_failed"

        data = {
            "status": status,
            "thread_id": thread_id,
            "completed_at": datetime.now(timezone.utc).isoformat(),
        }

        if output:
            data["output"] = output
        if error:
            data["error"] = error
        if error_node:
            data["error_node"] = error_node
        if tokens:
            data["tokens"] = tokens

        payload = {
            "run_id": run_id,
            "event_type": event_type,
            "data": data,
        }

        try:
            response = await self._client.post(
                f"{self.api_url}/workers/{self.state.worker_id}/events",
                json=payload,
            )
            if response.status_code != 404:
                response.raise_for_status()
        except Exception as e:
            log.warning("Failed to complete run", run_id=run_id, error=str(e))

    async def _send_events(self, thread_id: str, run_id: str, events: list[dict]):
        """Send events to control plane."""
        try:
            response = await self._client.post(
                f"{self.api_url}/threads/{thread_id}/runs/{run_id}/events",
                json={"events": events},
            )
            if response.status_code != 404:
                response.raise_for_status()
        except Exception as e:
            log.warning("Failed to send events", run_id=run_id, error=str(e))

    async def execute_direct(
        self,
        run_id: str,
        graph_id: str,
        input_data: dict,
        initial_state: Optional[dict] = None,
        resume_from: Optional[str] = None,
    ) -> tuple[dict, list[dict]]:
        """Execute a run directly without control plane interaction.

        Useful for testing the executor in isolation.

        Returns:
            Tuple of (final_state, events)
        """
        state, emitter = await execute_run(
            run_id=run_id,
            graph_id=graph_id,
            input_data=input_data,
            initial_state=initial_state,
            resume_from=resume_from,
        )
        return state.to_dict(), emitter.get_all_events()
