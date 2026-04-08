"""DuraGraph API client for managing assistants, threads, runs, store, and crons."""

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
        resp = self._client.get(f"/api/v1/assistants/{assistant_id}")
        resp.raise_for_status()
        return resp.json()

    def list_assistants(self, *, limit: int = 20, offset: int = 0) -> dict[str, Any]:
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
        payload: dict[str, Any] = {"limit": limit, "offset": offset}
        if graph_id:
            payload["graph_id"] = graph_id
        if metadata:
            payload["metadata"] = metadata
        resp = self._client.post("/api/v1/assistants/search", json=payload)
        resp.raise_for_status()
        return resp.json()

    def update_assistant(self, assistant_id: str, **kwargs: Any) -> dict[str, Any]:
        resp = self._client.patch(f"/api/v1/assistants/{assistant_id}", json=kwargs)
        resp.raise_for_status()
        return resp.json()

    def delete_assistant(self, assistant_id: str) -> dict[str, Any]:
        resp = self._client.delete(f"/api/v1/assistants/{assistant_id}")
        resp.raise_for_status()
        return resp.json()

    # -- Threads --

    def create_thread(self, *, metadata: dict[str, Any] | None = None) -> dict[str, Any]:
        payload: dict[str, Any] = {}
        if metadata:
            payload["metadata"] = metadata
        resp = self._client.post("/api/v1/threads", json=payload)
        resp.raise_for_status()
        return resp.json()

    def get_thread(self, thread_id: str) -> dict[str, Any]:
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
        payload: dict[str, Any] = {"values": values}
        if as_node:
            payload["as_node"] = as_node
        resp = self._client.post(f"/api/v1/threads/{thread_id}/state", json=payload)
        resp.raise_for_status()
        return resp.json()

    def get_thread_history(self, thread_id: str, *, limit: int = 10) -> list[dict[str, Any]]:
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
        payload: dict[str, Any] = {"namespace": namespace, "key": key, "value": value}
        if ttl_seconds is not None:
            payload["ttl_seconds"] = ttl_seconds
        resp = self._client.put("/api/v1/store/items", json=payload)
        resp.raise_for_status()
        return resp.json()

    def get_store_item(self, namespace: list[str], key: str) -> dict[str, Any]:
        resp = self._client.get(
            "/api/v1/store/items",
            params={"namespace": ".".join(namespace), "key": key},
        )
        resp.raise_for_status()
        return resp.json()

    def delete_store_item(self, namespace: list[str], key: str) -> dict[str, Any]:
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
