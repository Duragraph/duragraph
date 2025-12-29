"""Worker implementation for DuraGraph control plane."""

import asyncio
import signal
from collections.abc import Callable
from typing import Any
from uuid import uuid4

import httpx

from duragraph.graph import GraphDefinition


class Worker:
    """Worker that connects to DuraGraph control plane and executes graphs."""

    def __init__(
        self,
        control_plane_url: str,
        *,
        name: str | None = None,
        capabilities: list[str] | None = None,
        poll_interval: float = 1.0,
    ):
        """Initialize worker.

        Args:
            control_plane_url: URL of the DuraGraph control plane.
            name: Optional name for this worker.
            capabilities: Optional list of capabilities (e.g., ["openai", "tools"]).
            poll_interval: Interval in seconds between polling for work.
        """
        self.control_plane_url = control_plane_url.rstrip("/")
        self.name = name or f"worker-{uuid4().hex[:8]}"
        self.capabilities = capabilities or []
        self.poll_interval = poll_interval

        self._worker_id: str | None = None
        self._graphs: dict[str, GraphDefinition] = {}
        self._executors: dict[str, Callable[..., Any]] = {}
        self._running = False
        self._client: httpx.AsyncClient | None = None

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

    async def _register_with_control_plane(self) -> str:
        """Register this worker with the control plane."""
        if self._client is None:
            self._client = httpx.AsyncClient(timeout=30.0)

        # Prepare graph definitions
        graphs = [
            {"graph_id": g.graph_id, "definition": g.to_ir()}
            for g in self._graphs.values()
        ]

        payload = {
            "name": self.name,
            "capabilities": self.capabilities,
            "graphs": graphs,
        }

        response = await self._client.post(
            f"{self.control_plane_url}/api/v1/workers/register",
            json=payload,
        )
        response.raise_for_status()

        data = response.json()
        return data["worker_id"]

    async def _poll_for_work(self) -> dict[str, Any] | None:
        """Poll the control plane for work."""
        if self._client is None or self._worker_id is None:
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
                self._worker_id = await self._register_with_control_plane()
            return None
        except Exception:
            return None

    async def _execute_run(self, work: dict[str, Any]) -> None:
        """Execute a run from the control plane."""
        run_id = work.get("run_id")
        graph_id = work.get("graph_id")
        input_data = work.get("input", {})
        thread_id = work.get("thread_id")

        if not run_id or not graph_id:
            return

        # Find the graph definition
        graph_def = self._graphs.get(graph_id)
        if not graph_def:
            await self._send_event(run_id, "run_failed", {
                "error": f"Graph '{graph_id}' not registered with this worker",
            })
            return

        # Start the run
        await self._send_event(run_id, "run_started", {"thread_id": thread_id})

        try:
            # Execute nodes
            state = input_data.copy()
            current_node = graph_def.entrypoint

            while current_node:
                await self._send_event(run_id, "node_started", {
                    "node_id": current_node,
                })

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
                    result = await self._execute_human_node(
                        run_id, node_meta, state
                    )
                    if result is None:
                        # Interrupted, waiting for human input
                        return
                else:
                    # Default function node - just pass through
                    result = state

                if isinstance(result, dict):
                    state.update(result)

                await self._send_event(run_id, "node_completed", {
                    "node_id": current_node,
                    "output": result,
                })

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
            await self._send_event(run_id, "run_completed", {
                "output": state,
                "thread_id": thread_id,
            })

        except Exception as e:
            await self._send_event(run_id, "run_failed", {
                "error": str(e),
                "thread_id": thread_id,
            })

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
        await self._send_event(run_id, "run_requires_action", {
            "action_type": "human_review",
            "prompt": prompt,
            "state": state,
        })

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
        """Send heartbeat to control plane."""
        if self._client is None or self._worker_id is None:
            return

        try:
            await self._client.post(
                f"{self.control_plane_url}/api/v1/workers/{self._worker_id}/heartbeat",
            )
        except Exception:
            pass

    async def _run_loop(self) -> None:
        """Main worker loop."""
        self._running = True

        # Register with control plane
        print(f"Registering worker '{self.name}' with control plane...")
        self._worker_id = await self._register_with_control_plane()
        print(f"Registered with worker_id: {self._worker_id}")

        heartbeat_counter = 0

        while self._running:
            # Poll for work
            work = await self._poll_for_work()
            if work:
                print(f"Received work: {work.get('run_id')}")
                await self._execute_run(work)

            # Periodic heartbeat
            heartbeat_counter += 1
            if heartbeat_counter >= 30:  # Every 30 poll intervals
                await self._heartbeat()
                heartbeat_counter = 0

            await asyncio.sleep(self.poll_interval)

    def run(self) -> None:
        """Run the worker (blocking)."""
        loop = asyncio.new_event_loop()
        asyncio.set_event_loop(loop)

        # Handle shutdown signals
        for sig in (signal.SIGTERM, signal.SIGINT):
            loop.add_signal_handler(sig, self._shutdown)

        try:
            loop.run_until_complete(self._run_loop())
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

    def _shutdown(self) -> None:
        """Shutdown the worker."""
        print("\nShutting down worker...")
        self._running = False

    def stop(self) -> None:
        """Stop the worker."""
        self._running = False
