import pytest
import pytest_asyncio
import os

# LangGraph SDK uses async operations
from langgraph_sdk import get_client


@pytest.fixture
def api_url():
    """Get the API base URL from environment or default."""
    return os.getenv("API_BASE_URL", "http://localhost:8082")


@pytest.fixture
def client(api_url):
    """Create a LangGraph SDK client."""
    return get_client(url=api_url)


@pytest_asyncio.fixture
async def async_client(api_url):
    """Create an async LangGraph SDK client."""
    return get_client(url=api_url)


@pytest_asyncio.fixture
async def assistant(async_client):
    """Create a test assistant and clean up after the test."""
    assistant = await async_client.assistants.create(
        graph_id="simple_echo",
        name="test-assistant",
        metadata={"test": True}
    )
    yield assistant
    try:
        await async_client.assistants.delete(assistant["assistant_id"])
    except Exception:
        pass  # Ignore cleanup errors


@pytest_asyncio.fixture
async def thread(async_client):
    """Create a test thread and clean up after the test."""
    thread = await async_client.threads.create()
    yield thread
    try:
        await async_client.threads.delete(thread["thread_id"])
    except Exception:
        pass  # Ignore cleanup errors


@pytest_asyncio.fixture
async def thread_with_state(async_client, assistant):
    """Create a thread with some initial state from a run."""
    thread = await async_client.threads.create()

    # Create and wait for a run to generate state
    run = await async_client.runs.create(
        thread_id=thread["thread_id"],
        assistant_id=assistant["assistant_id"],
        input={"message": "initial state"}
    )

    # Wait for run to complete (with timeout)
    import asyncio
    for _ in range(30):
        run_status = await async_client.runs.get(
            thread_id=thread["thread_id"],
            run_id=run["run_id"]
        )
        if run_status["status"] in ["success", "completed", "error", "failed"]:
            break
        await asyncio.sleep(0.5)

    yield thread

    try:
        await async_client.threads.delete(thread["thread_id"])
    except Exception:
        pass
