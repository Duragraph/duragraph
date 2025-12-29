"""Prompt store client for DuraGraph."""

from typing import Any

import httpx


class PromptStore:
    """Client for interacting with the DuraGraph Prompt Store."""

    def __init__(
        self,
        base_url: str,
        *,
        api_key: str | None = None,
    ):
        """Initialize prompt store client.

        Args:
            base_url: URL of the prompt store API.
            api_key: Optional API key for authentication.
        """
        self.base_url = base_url.rstrip("/")
        self.api_key = api_key
        self._client = httpx.Client(timeout=30.0)

    def _headers(self) -> dict[str, str]:
        """Get request headers."""
        headers = {"Content-Type": "application/json"}
        if self.api_key:
            headers["Authorization"] = f"Bearer {self.api_key}"
        return headers

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
        return response.json()

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
