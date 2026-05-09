"""DuraGraph REST API clients for managing assistants, threads, runs, store, and crons.

Provides synchronous (:class:`DuraGraphClient`) and asynchronous
(:class:`AsyncDuraGraphClient`) clients that wrap the full DuraGraph control
plane API.  Both clients are compatible with the LangGraph Cloud API.

Typical usage::

    from duragraph import DuraGraphClient

    with DuraGraphClient("http://localhost:8081") as client:
        assistant = client.create_assistant("My Agent", graph_id="chatbot")
        thread = client.create_thread()
        run = client.create_run(
            thread["thread_id"],
            assistant["assistant_id"],
            input={"messages": [{"role": "user", "content": "Hello!"}]},
        )
"""

from __future__ import annotations

from typing import Any

import httpx


class DuraGraphClient:
    """Synchronous client for the DuraGraph REST API.

    Compatible with LangGraph Cloud API endpoints.

    Args:
        base_url: Base URL of the DuraGraph server (e.g. "http://localhost:8081").
        api_key: Optional API key for authentication.
        timeout: Request timeout in seconds.
    """

    def __init__(
        self,
        base_url: str = "http://localhost:8081",
        *,
        api_key: str | None = None,
        timeout: float = 30.0,
    ) -> None:
        headers: dict[str, str] = {"Content-Type": "application/json"}
        if api_key:
            headers["X-Api-Key"] = api_key
        self._client = httpx.Client(
            base_url=base_url.rstrip("/"),
            headers=headers,
            timeout=timeout,
        )

    def close(self) -> None:
        self._client.close()

    def __enter__(self) -> DuraGraphClient:
        return self

    def __exit__(self, *args: object) -> None:
        self.close()

    # -- Assistants --

    def create_assistant(
        self,
        name: str,
        *,
        graph_id: str | None = None,
        description: str | None = None,
        model: str | None = None,
        instructions: str | None = None,
        metadata: dict[str, Any] | None = None,
        config: dict[str, Any] | None = None,
    ) -> dict[str, Any]:
        """Create a new assistant.

        Args:
            name: Display name for the assistant.
            graph_id: Graph definition to back this assistant.
            description: Human-readable description.
            model: Default LLM model identifier.
            instructions: System-level instructions.
            metadata: Arbitrary key-value metadata.
            config: Runtime configuration overrides.

        Returns:
            Assistant resource dict containing ``assistant_id``, ``name``,
            ``graph_id``, ``created_at``, and ``updated_at``.
        """
        payload: dict[str, Any] = {"name": name}
        if graph_id:
            payload["graph_id"] = graph_id
        if description:
            payload["description"] = description
        if model:
            payload["model"] = model
        if instructions:
            payload["instructions"] = instructions
        if metadata:
            payload["metadata"] = metadata
        if config:
            payload["config"] = config
        resp = self._client.post("/api/v1/assistants", json=payload)
        resp.raise_for_status()
        return resp.json()

    def get_assistant(self, assistant_id: str) -> dict[str, Any]:
        """Retrieve an assistant by ID."""
        resp = self._client.get(f"/api/v1/assistants/{assistant_id}")
        resp.raise_for_status()
        return resp.json()

    def list_assistants(self, *, limit: int = 20, offset: int = 0) -> dict[str, Any]:
        """List assistants with pagination."""
        resp = self._client.get("/api/v1/assistants", params={"limit": limit, "offset": offset})
        resp.raise_for_status()
        return resp.json()

    def search_assistants(
        self,
        *,
        graph_id: str | None = None,
        metadata: dict[str, Any] | None = None,
        limit: int = 10,
        offset: int = 0,
    ) -> list[dict[str, Any]]:
        """Search assistants by graph ID or metadata."""
        payload: dict[str, Any] = {"limit": limit, "offset": offset}
        if graph_id:
            payload["graph_id"] = graph_id
        if metadata:
            payload["metadata"] = metadata
        resp = self._client.post("/api/v1/assistants/search", json=payload)
        resp.raise_for_status()
        return resp.json()

    def update_assistant(self, assistant_id: str, **kwargs: Any) -> dict[str, Any]:
        """Update an assistant. Pass any assistant fields as keyword arguments."""
        resp = self._client.patch(f"/api/v1/assistants/{assistant_id}", json=kwargs)
        resp.raise_for_status()
        return resp.json()

    def delete_assistant(self, assistant_id: str) -> dict[str, Any]:
        """Delete an assistant by ID."""
        resp = self._client.delete(f"/api/v1/assistants/{assistant_id}")
        resp.raise_for_status()
        return resp.json()

    # -- Threads --

    def create_thread(self, *, metadata: dict[str, Any] | None = None) -> dict[str, Any]:
        """Create a new thread.

        Args:
            metadata: Optional metadata to attach to the thread.

        Returns:
            Thread resource dict with ``thread_id``, ``created_at``, ``updated_at``.
        """
        payload: dict[str, Any] = {}
        if metadata:
            payload["metadata"] = metadata
        resp = self._client.post("/api/v1/threads", json=payload)
        resp.raise_for_status()
        return resp.json()

    def get_thread(self, thread_id: str) -> dict[str, Any]:
        """Retrieve a thread by ID."""
        resp = self._client.get(f"/api/v1/threads/{thread_id}")
        resp.raise_for_status()
        return resp.json()

    def list_threads(self, *, limit: int = 20, offset: int = 0) -> dict[str, Any]:
        resp = self._client.get("/api/v1/threads", params={"limit": limit, "offset": offset})
        resp.raise_for_status()
        return resp.json()

    def search_threads(
        self,
        *,
        status: str | None = None,
        metadata: dict[str, Any] | None = None,
        limit: int = 10,
        offset: int = 0,
    ) -> list[dict[str, Any]]:
        payload: dict[str, Any] = {"limit": limit, "offset": offset}
        if status:
            payload["status"] = status
        if metadata:
            payload["metadata"] = metadata
        resp = self._client.post("/api/v1/threads/search", json=payload)
        resp.raise_for_status()
        return resp.json()

    def update_thread(self, thread_id: str, *, metadata: dict[str, Any]) -> dict[str, Any]:
        resp = self._client.patch(f"/api/v1/threads/{thread_id}", json={"metadata": metadata})
        resp.raise_for_status()
        return resp.json()

    def delete_thread(self, thread_id: str) -> dict[str, Any]:
        resp = self._client.delete(f"/api/v1/threads/{thread_id}")
        resp.raise_for_status()
        return resp.json()

    def get_thread_state(self, thread_id: str) -> dict[str, Any]:
        """Get the current state of a thread."""
        resp = self._client.get(f"/api/v1/threads/{thread_id}/state")
        resp.raise_for_status()
        return resp.json()

    def update_thread_state(
        self,
        thread_id: str,
        *,
        values: dict[str, Any],
        as_node: str | None = None,
    ) -> dict[str, Any]:
        """Update thread state, optionally as if from a specific node."""
        payload: dict[str, Any] = {"values": values}
        if as_node:
            payload["as_node"] = as_node
        resp = self._client.post(f"/api/v1/threads/{thread_id}/state", json=payload)
        resp.raise_for_status()
        return resp.json()

    def get_thread_history(self, thread_id: str, *, limit: int = 10) -> list[dict[str, Any]]:
        """Retrieve checkpoint history for a thread."""
        resp = self._client.get(f"/api/v1/threads/{thread_id}/history", params={"limit": limit})
        resp.raise_for_status()
        return resp.json()

    # -- Runs --

    def create_run(
        self,
        thread_id: str,
        assistant_id: str,
        *,
        input: dict[str, Any] | None = None,
        config: dict[str, Any] | None = None,
        metadata: dict[str, Any] | None = None,
        multitask_strategy: str | None = None,
        interrupt_before: list[str] | None = None,
        interrupt_after: list[str] | None = None,
    ) -> dict[str, Any]:
        """Create a run within a thread.

        Args:
            thread_id: Thread to execute in.
            assistant_id: Assistant (graph) to run.
            input: Input state for the graph.
            config: Runtime configuration overrides.
            metadata: Arbitrary run metadata.
            multitask_strategy: How to handle concurrent runs
                (``"reject"``, ``"enqueue"``, ``"rollback"``).
            interrupt_before: Pause execution before these nodes.
            interrupt_after: Pause execution after these nodes.

        Returns:
            Run resource dict with ``run_id``, ``status``, etc.
        """
        payload: dict[str, Any] = {"assistant_id": assistant_id}
        if input:
            payload["input"] = input
        if config:
            payload["config"] = config
        if metadata:
            payload["metadata"] = metadata
        if multitask_strategy:
            payload["multitask_strategy"] = multitask_strategy
        if interrupt_before:
            payload["interrupt_before"] = interrupt_before
        if interrupt_after:
            payload["interrupt_after"] = interrupt_after
        resp = self._client.post(f"/api/v1/threads/{thread_id}/runs", json=payload)
        resp.raise_for_status()
        return resp.json()

    def create_stateless_run(
        self,
        assistant_id: str,
        *,
        input: dict[str, Any] | None = None,
        config: dict[str, Any] | None = None,
        metadata: dict[str, Any] | None = None,
    ) -> dict[str, Any]:
        """Create a stateless run (no thread). Useful for one-shot executions."""
        payload: dict[str, Any] = {"assistant_id": assistant_id}
        if input:
            payload["input"] = input
        if config:
            payload["config"] = config
        if metadata:
            payload["metadata"] = metadata
        resp = self._client.post("/api/v1/runs", json=payload)
        resp.raise_for_status()
        return resp.json()

    def get_run(self, run_id: str, *, thread_id: str | None = None) -> dict[str, Any]:
        if thread_id:
            resp = self._client.get(f"/api/v1/threads/{thread_id}/runs/{run_id}")
        else:
            resp = self._client.get(f"/api/v1/runs/{run_id}")
        resp.raise_for_status()
        return resp.json()

    def list_runs(self, thread_id: str) -> list[dict[str, Any]]:
        resp = self._client.get(f"/api/v1/threads/{thread_id}/runs")
        resp.raise_for_status()
        return resp.json()

    def cancel_run(self, thread_id: str, run_id: str) -> dict[str, Any]:
        resp = self._client.post(f"/api/v1/threads/{thread_id}/runs/{run_id}/cancel")
        resp.raise_for_status()
        return resp.json()

    def wait_for_run(
        self,
        assistant_id: str,
        *,
        input: dict[str, Any] | None = None,
        config: dict[str, Any] | None = None,
    ) -> dict[str, Any]:
        payload: dict[str, Any] = {"assistant_id": assistant_id}
        if input:
            payload["input"] = input
        if config:
            payload["config"] = config
        resp = self._client.post("/api/v1/runs/wait", json=payload)
        resp.raise_for_status()
        return resp.json()

    def join_run(self, thread_id: str, run_id: str) -> dict[str, Any]:
        resp = self._client.get(f"/api/v1/threads/{thread_id}/runs/{run_id}/join")
        resp.raise_for_status()
        return resp.json()

    # -- Store --

    def put_store_item(
        self,
        namespace: list[str],
        key: str,
        value: dict[str, Any],
        *,
        ttl_seconds: int | None = None,
    ) -> dict[str, Any]:
        """Create or update an item in the key-value store.

        Args:
            namespace: Hierarchical namespace (e.g. ``["users", "prefs"]``).
            key: Item key within the namespace.
            value: JSON-serializable value.
            ttl_seconds: Optional time-to-live in seconds.
        """
        payload: dict[str, Any] = {"namespace": namespace, "key": key, "value": value}
        if ttl_seconds is not None:
            payload["ttl_seconds"] = ttl_seconds
        resp = self._client.put("/api/v1/store/items", json=payload)
        resp.raise_for_status()
        return resp.json()

    def get_store_item(self, namespace: list[str], key: str) -> dict[str, Any]:
        """Retrieve an item from the store by namespace and key."""
        resp = self._client.get(
            "/api/v1/store/items",
            params={"namespace": ".".join(namespace), "key": key},
        )
        resp.raise_for_status()
        return resp.json()

    def delete_store_item(self, namespace: list[str], key: str) -> dict[str, Any]:
        """Delete an item from the store."""
        resp = self._client.request(
            "DELETE",
            "/api/v1/store/items",
            json={"namespace": namespace, "key": key},
        )
        resp.raise_for_status()
        return resp.json()

    def search_store(
        self,
        namespace_prefix: list[str],
        *,
        filter: dict[str, Any] | None = None,
        limit: int = 10,
        offset: int = 0,
    ) -> list[dict[str, Any]]:
        """Search store items within a namespace prefix."""
        payload: dict[str, Any] = {
            "namespace_prefix": namespace_prefix,
            "limit": limit,
            "offset": offset,
        }
        if filter:
            payload["filter"] = filter
        resp = self._client.post("/api/v1/store/items/search", json=payload)
        resp.raise_for_status()
        return resp.json()

    def list_namespaces(
        self,
        *,
        prefix: list[str] | None = None,
        suffix: list[str] | None = None,
        max_depth: int | None = None,
        limit: int = 100,
        offset: int = 0,
    ) -> list[list[str]]:
        """List namespaces in the store, optionally filtered by prefix/suffix."""
        payload: dict[str, Any] = {"limit": limit, "offset": offset}
        if prefix:
            payload["prefix"] = prefix
        if suffix:
            payload["suffix"] = suffix
        if max_depth is not None:
            payload["max_depth"] = max_depth
        resp = self._client.post("/api/v1/store/namespaces", json=payload)
        resp.raise_for_status()
        return resp.json()

    # -- Crons --

    def create_cron(
        self,
        assistant_id: str,
        schedule: str,
        *,
        thread_id: str | None = None,
        payload: dict[str, Any] | None = None,
        metadata: dict[str, Any] | None = None,
    ) -> dict[str, Any]:
        """Create a scheduled cron job.

        Args:
            assistant_id: Assistant to run on schedule.
            schedule: Cron expression (5-field, e.g. ``"0 */6 * * *"``).
            thread_id: Optional thread to run in (creates new if omitted).
            payload: Input data passed to each run.
            metadata: Arbitrary cron metadata.
        """
        body: dict[str, Any] = {
            "assistant_id": assistant_id,
            "schedule": schedule,
        }
        if payload:
            body["payload"] = payload
        if metadata:
            body["metadata"] = metadata
        if thread_id:
            resp = self._client.post(f"/api/v1/threads/{thread_id}/runs/crons", json=body)
        else:
            resp = self._client.post("/api/v1/runs/crons", json=body)
        resp.raise_for_status()
        return resp.json()

    def delete_cron(self, cron_id: str) -> dict[str, Any]:
        """Delete a cron job by ID."""
        resp = self._client.delete(f"/api/v1/runs/crons/{cron_id}")
        resp.raise_for_status()
        return resp.json()

    def search_crons(
        self,
        *,
        assistant_id: str | None = None,
        limit: int = 10,
        offset: int = 0,
    ) -> list[dict[str, Any]]:
        """Search cron jobs, optionally filtered by assistant."""
        payload: dict[str, Any] = {"limit": limit, "offset": offset}
        if assistant_id:
            payload["assistant_id"] = assistant_id
        resp = self._client.post("/api/v1/runs/crons/search", json=payload)
        resp.raise_for_status()
        return resp.json()


class AsyncDuraGraphClient:
    """Async client for the DuraGraph REST API.

    Compatible with LangGraph Cloud API endpoints.

    Args:
        base_url: Base URL of the DuraGraph server (e.g. "http://localhost:8081").
        api_key: Optional API key for authentication.
        timeout: Request timeout in seconds.
    """

    def __init__(
        self,
        base_url: str = "http://localhost:8081",
        *,
        api_key: str | None = None,
        timeout: float = 30.0,
    ) -> None:
        headers: dict[str, str] = {"Content-Type": "application/json"}
        if api_key:
            headers["X-Api-Key"] = api_key
        self._client = httpx.AsyncClient(
            base_url=base_url.rstrip("/"),
            headers=headers,
            timeout=timeout,
        )

    async def close(self) -> None:
        await self._client.aclose()

    async def __aenter__(self) -> AsyncDuraGraphClient:
        return self

    async def __aexit__(self, *args: object) -> None:
        await self.close()

    # -- Assistants --

    async def create_assistant(
        self,
        name: str,
        *,
        graph_id: str | None = None,
        description: str | None = None,
        model: str | None = None,
        instructions: str | None = None,
        metadata: dict[str, Any] | None = None,
        config: dict[str, Any] | None = None,
    ) -> dict[str, Any]:
        payload: dict[str, Any] = {"name": name}
        if graph_id:
            payload["graph_id"] = graph_id
        if description:
            payload["description"] = description
        if model:
            payload["model"] = model
        if instructions:
            payload["instructions"] = instructions
        if metadata:
            payload["metadata"] = metadata
        if config:
            payload["config"] = config
        resp = await self._client.post("/api/v1/assistants", json=payload)
        resp.raise_for_status()
        return resp.json()

    async def get_assistant(self, assistant_id: str) -> dict[str, Any]:
        resp = await self._client.get(f"/api/v1/assistants/{assistant_id}")
        resp.raise_for_status()
        return resp.json()

    async def list_assistants(self, *, limit: int = 20, offset: int = 0) -> dict[str, Any]:
        resp = await self._client.get(
            "/api/v1/assistants", params={"limit": limit, "offset": offset}
        )
        resp.raise_for_status()
        return resp.json()

    async def search_assistants(
        self,
        *,
        graph_id: str | None = None,
        metadata: dict[str, Any] | None = None,
        limit: int = 10,
        offset: int = 0,
    ) -> list[dict[str, Any]]:
        payload: dict[str, Any] = {"limit": limit, "offset": offset}
        if graph_id:
            payload["graph_id"] = graph_id
        if metadata:
            payload["metadata"] = metadata
        resp = await self._client.post("/api/v1/assistants/search", json=payload)
        resp.raise_for_status()
        return resp.json()

    async def delete_assistant(self, assistant_id: str) -> dict[str, Any]:
        resp = await self._client.delete(f"/api/v1/assistants/{assistant_id}")
        resp.raise_for_status()
        return resp.json()

    # -- Threads --

    async def create_thread(self, *, metadata: dict[str, Any] | None = None) -> dict[str, Any]:
        payload: dict[str, Any] = {}
        if metadata:
            payload["metadata"] = metadata
        resp = await self._client.post("/api/v1/threads", json=payload)
        resp.raise_for_status()
        return resp.json()

    async def get_thread(self, thread_id: str) -> dict[str, Any]:
        resp = await self._client.get(f"/api/v1/threads/{thread_id}")
        resp.raise_for_status()
        return resp.json()

    async def delete_thread(self, thread_id: str) -> dict[str, Any]:
        resp = await self._client.delete(f"/api/v1/threads/{thread_id}")
        resp.raise_for_status()
        return resp.json()

    async def get_thread_state(self, thread_id: str) -> dict[str, Any]:
        resp = await self._client.get(f"/api/v1/threads/{thread_id}/state")
        resp.raise_for_status()
        return resp.json()

    async def update_thread_state(
        self,
        thread_id: str,
        *,
        values: dict[str, Any],
        as_node: str | None = None,
    ) -> dict[str, Any]:
        payload: dict[str, Any] = {"values": values}
        if as_node:
            payload["as_node"] = as_node
        resp = await self._client.post(f"/api/v1/threads/{thread_id}/state", json=payload)
        resp.raise_for_status()
        return resp.json()

    # -- Runs --

    async def create_run(
        self,
        thread_id: str,
        assistant_id: str,
        *,
        input: dict[str, Any] | None = None,
        config: dict[str, Any] | None = None,
        metadata: dict[str, Any] | None = None,
        multitask_strategy: str | None = None,
    ) -> dict[str, Any]:
        payload: dict[str, Any] = {"assistant_id": assistant_id}
        if input:
            payload["input"] = input
        if config:
            payload["config"] = config
        if metadata:
            payload["metadata"] = metadata
        if multitask_strategy:
            payload["multitask_strategy"] = multitask_strategy
        resp = await self._client.post(f"/api/v1/threads/{thread_id}/runs", json=payload)
        resp.raise_for_status()
        return resp.json()

    async def get_run(self, run_id: str, *, thread_id: str | None = None) -> dict[str, Any]:
        if thread_id:
            resp = await self._client.get(f"/api/v1/threads/{thread_id}/runs/{run_id}")
        else:
            resp = await self._client.get(f"/api/v1/runs/{run_id}")
        resp.raise_for_status()
        return resp.json()

    async def cancel_run(self, thread_id: str, run_id: str) -> dict[str, Any]:
        resp = await self._client.post(f"/api/v1/threads/{thread_id}/runs/{run_id}/cancel")
        resp.raise_for_status()
        return resp.json()

    async def wait_for_run(
        self,
        assistant_id: str,
        *,
        input: dict[str, Any] | None = None,
        config: dict[str, Any] | None = None,
    ) -> dict[str, Any]:
        payload: dict[str, Any] = {"assistant_id": assistant_id}
        if input:
            payload["input"] = input
        if config:
            payload["config"] = config
        resp = await self._client.post("/api/v1/runs/wait", json=payload)
        resp.raise_for_status()
        return resp.json()

    # -- Store --

    async def put_store_item(
        self,
        namespace: list[str],
        key: str,
        value: dict[str, Any],
    ) -> dict[str, Any]:
        payload: dict[str, Any] = {"namespace": namespace, "key": key, "value": value}
        resp = await self._client.put("/api/v1/store/items", json=payload)
        resp.raise_for_status()
        return resp.json()

    async def get_store_item(self, namespace: list[str], key: str) -> dict[str, Any]:
        resp = await self._client.get(
            "/api/v1/store/items",
            params={"namespace": ".".join(namespace), "key": key},
        )
        resp.raise_for_status()
        return resp.json()

    async def search_store(
        self,
        namespace_prefix: list[str],
        *,
        filter: dict[str, Any] | None = None,
        limit: int = 10,
    ) -> list[dict[str, Any]]:
        payload: dict[str, Any] = {
            "namespace_prefix": namespace_prefix,
            "limit": limit,
        }
        if filter:
            payload["filter"] = filter
        resp = await self._client.post("/api/v1/store/items/search", json=payload)
        resp.raise_for_status()
        return resp.json()

    # -- Crons --

    async def create_cron(
        self,
        assistant_id: str,
        schedule: str,
        *,
        payload: dict[str, Any] | None = None,
    ) -> dict[str, Any]:
        body: dict[str, Any] = {
            "assistant_id": assistant_id,
            "schedule": schedule,
        }
        if payload:
            body["payload"] = payload
        resp = await self._client.post("/api/v1/runs/crons", json=body)
        resp.raise_for_status()
        return resp.json()

    async def delete_cron(self, cron_id: str) -> dict[str, Any]:
        resp = await self._client.delete(f"/api/v1/runs/crons/{cron_id}")
        resp.raise_for_status()
        return resp.json()

    async def search_crons(
        self,
        *,
        assistant_id: str | None = None,
        limit: int = 10,
    ) -> list[dict[str, Any]]:
        payload: dict[str, Any] = {"limit": limit}
        if assistant_id:
            payload["assistant_id"] = assistant_id
        resp = await self._client.post("/api/v1/runs/crons/search", json=payload)
        resp.raise_for_status()
        return resp.json()
