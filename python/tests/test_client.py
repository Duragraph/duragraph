"""Tests for DuraGraph API client."""

from unittest.mock import MagicMock, patch

import pytest

from duragraph.client import AsyncDuraGraphClient, DuraGraphClient


class TestDuraGraphClient:
    def test_init_default(self) -> None:
        client = DuraGraphClient()
        assert client._client.base_url == "http://localhost:8081"
        client.close()

    def test_init_custom(self) -> None:
        client = DuraGraphClient("http://example.com:9090", api_key="test-key")
        assert client._client.base_url == "http://example.com:9090"
        assert client._client.headers["X-Api-Key"] == "test-key"
        client.close()

    def test_context_manager(self) -> None:
        with DuraGraphClient() as client:
            assert client is not None

    @patch("httpx.Client.post")
    def test_create_assistant(self, mock_post: MagicMock) -> None:
        mock_post.return_value = MagicMock(
            status_code=201,
            json=lambda: {"assistant_id": "abc", "name": "test"},
            raise_for_status=lambda: None,
        )
        with DuraGraphClient() as client:
            result = client.create_assistant("test", model="gpt-4")
        assert result["name"] == "test"
        mock_post.assert_called_once()

    @patch("httpx.Client.get")
    def test_get_assistant(self, mock_get: MagicMock) -> None:
        mock_get.return_value = MagicMock(
            status_code=200,
            json=lambda: {"assistant_id": "abc"},
            raise_for_status=lambda: None,
        )
        with DuraGraphClient() as client:
            result = client.get_assistant("abc")
        assert result["assistant_id"] == "abc"

    @patch("httpx.Client.post")
    def test_create_thread(self, mock_post: MagicMock) -> None:
        mock_post.return_value = MagicMock(
            status_code=201,
            json=lambda: {"thread_id": "t1"},
            raise_for_status=lambda: None,
        )
        with DuraGraphClient() as client:
            result = client.create_thread(metadata={"key": "val"})
        assert result["thread_id"] == "t1"

    @patch("httpx.Client.post")
    def test_create_run(self, mock_post: MagicMock) -> None:
        mock_post.return_value = MagicMock(
            status_code=201,
            json=lambda: {"run_id": "r1", "status": "pending"},
            raise_for_status=lambda: None,
        )
        with DuraGraphClient() as client:
            result = client.create_run("t1", "a1", input={"msg": "hello"})
        assert result["run_id"] == "r1"

    @patch("httpx.Client.put")
    def test_put_store_item(self, mock_put: MagicMock) -> None:
        mock_put.return_value = MagicMock(
            status_code=200,
            json=lambda: {"status": "ok"},
            raise_for_status=lambda: None,
        )
        with DuraGraphClient() as client:
            result = client.put_store_item(["ns", "sub"], "key1", {"data": 1})
        assert result["status"] == "ok"

    @patch("httpx.Client.post")
    def test_create_cron(self, mock_post: MagicMock) -> None:
        mock_post.return_value = MagicMock(
            status_code=201,
            json=lambda: {"cron_id": "c1"},
            raise_for_status=lambda: None,
        )
        with DuraGraphClient() as client:
            result = client.create_cron("a1", "*/5 * * * *")
        assert result["cron_id"] == "c1"

    @patch("httpx.Client.post")
    def test_search_assistants(self, mock_post: MagicMock) -> None:
        mock_post.return_value = MagicMock(
            status_code=200,
            json=lambda: [{"assistant_id": "abc"}],
            raise_for_status=lambda: None,
        )
        with DuraGraphClient() as client:
            result = client.search_assistants(graph_id="g1")
        assert len(result) == 1

    @patch("httpx.Client.get")
    def test_get_thread_state(self, mock_get: MagicMock) -> None:
        mock_get.return_value = MagicMock(
            status_code=200,
            json=lambda: {"values": {"key": "val"}, "next": []},
            raise_for_status=lambda: None,
        )
        with DuraGraphClient() as client:
            result = client.get_thread_state("t1")
        assert result["values"]["key"] == "val"


class TestAsyncDuraGraphClient:
    def test_init(self) -> None:
        client = AsyncDuraGraphClient("http://localhost:8081", api_key="key")
        assert client._client.headers["X-Api-Key"] == "key"

    @pytest.mark.asyncio
    async def test_context_manager(self) -> None:
        async with AsyncDuraGraphClient() as client:
            assert client is not None
