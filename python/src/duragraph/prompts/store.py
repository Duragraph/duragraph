"""Prompt store client for DuraGraph."""

import time
from typing import Any

import httpx


class _CacheEntry:
    """Internal cache entry with TTL."""

    __slots__ = ("value", "expires_at")

    def __init__(self, value: Any, ttl: float):
        self.value = value
        self.expires_at = time.monotonic() + ttl

    @property
    def expired(self) -> bool:
        return time.monotonic() >= self.expires_at


class PromptStore:
    """Client for interacting with the DuraGraph Prompt Store."""

    def __init__(
        self,
        base_url: str,
        *,
        api_key: str | None = None,
        cache_ttl: float = 300.0,
    ):
        """Initialize prompt store client.

        Args:
            base_url: URL of the prompt store API.
            api_key: Optional API key for authentication.
            cache_ttl: Cache time-to-live in seconds (default 5 minutes, 0 to disable).
        """
        self.base_url = base_url.rstrip("/")
        self.api_key = api_key
        self._client = httpx.Client(timeout=30.0)
        self._cache_ttl = cache_ttl
        self._cache: dict[str, _CacheEntry] = {}

    def _headers(self) -> dict[str, str]:
        """Get request headers."""
        headers = {"Content-Type": "application/json"}
        if self.api_key:
            headers["Authorization"] = f"Bearer {self.api_key}"
        return headers

    def _cache_key(self, prompt_id: str, version: str | None, variant: str | None) -> str:
        return f"{prompt_id}:{version or 'latest'}:{variant or 'default'}"

    def _get_cached(self, key: str) -> dict[str, Any] | None:
        if self._cache_ttl <= 0:
            return None
        entry = self._cache.get(key)
        if entry is None or entry.expired:
            self._cache.pop(key, None)
            return None
        return entry.value

    def _set_cached(self, key: str, value: dict[str, Any]) -> None:
        if self._cache_ttl > 0:
            self._cache[key] = _CacheEntry(value, self._cache_ttl)

    def invalidate(self, prompt_id: str | None = None) -> None:
        """Invalidate cached prompts.

        Args:
            prompt_id: If given, only invalidate entries for this prompt.
                       Otherwise clear the entire cache.
        """
        if prompt_id is None:
            self._cache.clear()
        else:
            keys_to_remove = [k for k in self._cache if k.startswith(f"{prompt_id}:")]
            for k in keys_to_remove:
                del self._cache[k]

    def get_prompt(
        self,
        prompt_id: str,
        *,
        version: str | None = None,
        variant: str | None = None,
    ) -> dict[str, Any]:
        """Get a prompt from the store.

        Args:
            prompt_id: Prompt identifier.
            version: Optional version (default: latest).
            variant: Optional A/B variant.

        Returns:
            Prompt data including content and metadata.
        """
        cache_key = self._cache_key(prompt_id, version, variant)
        cached = self._get_cached(cache_key)
        if cached is not None:
            return cached

        params: dict[str, str] = {}
        if version:
            params["version"] = version
        if variant:
            params["variant"] = variant

        response = self._client.get(
            f"{self.base_url}/api/v1/prompts/{prompt_id}",
            headers=self._headers(),
            params=params,
        )
        response.raise_for_status()
        result = response.json()
        self._set_cached(cache_key, result)
        return result

    def list_prompts(
        self,
        *,
        namespace: str | None = None,
        tag: str | None = None,
    ) -> list[dict[str, Any]]:
        """List prompts in the store.

        Args:
            namespace: Optional namespace filter.
            tag: Optional tag filter.

        Returns:
            List of prompt metadata.
        """
        params: dict[str, str] = {}
        if namespace:
            params["namespace"] = namespace
        if tag:
            params["tag"] = tag

        response = self._client.get(
            f"{self.base_url}/api/v1/prompts",
            headers=self._headers(),
            params=params,
        )
        response.raise_for_status()
        return response.json()["prompts"]

    def create_prompt(
        self,
        prompt_id: str,
        content: str,
        *,
        description: str | None = None,
        tags: list[str] | None = None,
        metadata: dict[str, Any] | None = None,
    ) -> dict[str, Any]:
        """Create a new prompt.

        Args:
            prompt_id: Prompt identifier.
            content: Prompt content template.
            description: Optional description.
            tags: Optional tags for categorization.
            metadata: Optional additional metadata.

        Returns:
            Created prompt data.
        """
        payload = {
            "prompt_id": prompt_id,
            "content": content,
        }
        if description:
            payload["description"] = description
        if tags:
            payload["tags"] = tags
        if metadata:
            payload["metadata"] = metadata

        response = self._client.post(
            f"{self.base_url}/api/v1/prompts",
            headers=self._headers(),
            json=payload,
        )
        response.raise_for_status()
        return response.json()

    def create_version(
        self,
        prompt_id: str,
        content: str,
        *,
        change_log: str | None = None,
    ) -> dict[str, Any]:
        """Create a new version of an existing prompt.

        Args:
            prompt_id: Prompt identifier.
            content: New prompt content.
            change_log: Optional change description.

        Returns:
            New version data.
        """
        payload = {"content": content}
        if change_log:
            payload["change_log"] = change_log

        response = self._client.post(
            f"{self.base_url}/api/v1/prompts/{prompt_id}/versions",
            headers=self._headers(),
            json=payload,
        )
        response.raise_for_status()
        return response.json()

    def close(self) -> None:
        """Close the HTTP client."""
        self._client.close()

    def __enter__(self) -> "PromptStore":
        return self

    def __exit__(self, *args: Any) -> None:
        self.close()
