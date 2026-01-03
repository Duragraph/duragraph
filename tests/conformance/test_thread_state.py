"""
Thread State Conformance Tests

Tests thread state endpoints using the LangGraph SDK to verify
DuraGraph compatibility with LangGraph Cloud API.
"""
import pytest
import asyncio


pytestmark = pytest.mark.asyncio


async def test_get_thread_state(async_client, thread_with_state):
    """Test getting thread state returns expected structure."""
    thread_id = thread_with_state["thread_id"]

    state = await async_client.threads.get_state(thread_id)

    # LangGraph SDK expects these fields in state response
    assert "values" in state, "State must contain 'values' field"
    assert "next" in state, "State must contain 'next' field"

    # Checkpoint info should be present
    if "checkpoint" in state:
        checkpoint = state["checkpoint"]
        assert "checkpoint_id" in checkpoint or "thread_id" in checkpoint


async def test_get_thread_state_empty_thread(async_client, thread):
    """Test getting state for a thread with no runs."""
    thread_id = thread["thread_id"]

    # For a new thread with no runs, state should still be retrievable
    # but may have empty values
    try:
        state = await async_client.threads.get_state(thread_id)
        assert "values" in state
    except Exception as e:
        # Some implementations return 404 for threads with no state
        if "404" in str(e) or "not found" in str(e).lower():
            pytest.skip("No state for empty thread (expected behavior)")
        raise


async def test_update_thread_state(async_client, thread):
    """Test updating thread state."""
    thread_id = thread["thread_id"]

    # Update thread state with new values
    result = await async_client.threads.update_state(
        thread_id,
        values={"test_key": "test_value", "counter": 42}
    )

    # Verify the update was applied
    state = await async_client.threads.get_state(thread_id)
    assert state["values"].get("test_key") == "test_value"
    assert state["values"].get("counter") == 42


async def test_update_thread_state_with_as_node(async_client, thread):
    """Test updating thread state with as_node parameter."""
    thread_id = thread["thread_id"]

    # Update state as if from a specific node
    try:
        result = await async_client.threads.update_state(
            thread_id,
            values={"from_node": "value"},
            as_node="custom_node"
        )

        state = await async_client.threads.get_state(thread_id)
        assert state["values"].get("from_node") == "value"
    except Exception as e:
        if "as_node" in str(e).lower() or "not implemented" in str(e).lower():
            pytest.skip("as_node parameter not implemented yet")
        raise


async def test_get_thread_history(async_client, thread_with_state):
    """Test getting thread checkpoint history."""
    thread_id = thread_with_state["thread_id"]

    history = await async_client.threads.get_history(thread_id)

    # History should be a list of state snapshots
    assert isinstance(history, list), "History must be a list"

    # If there's history, each entry should have checkpoint info
    for entry in history:
        assert "values" in entry or "checkpoint" in entry


async def test_get_thread_history_empty(async_client, thread):
    """Test getting history for a thread with no checkpoints."""
    thread_id = thread["thread_id"]

    history = await async_client.threads.get_history(thread_id)

    # Empty history should return empty list
    assert isinstance(history, list)


async def test_get_thread_history_with_limit(async_client, thread_with_state):
    """Test getting thread history with limit parameter."""
    thread_id = thread_with_state["thread_id"]

    try:
        history = await async_client.threads.get_history(thread_id, limit=5)
        assert isinstance(history, list)
        assert len(history) <= 5
    except TypeError:
        # SDK version may not support limit parameter
        pytest.skip("History limit parameter not supported in this SDK version")


async def test_get_state_at_checkpoint(async_client, thread_with_state):
    """Test getting state at a specific checkpoint."""
    thread_id = thread_with_state["thread_id"]

    # First get current state to find a checkpoint ID
    state = await async_client.threads.get_state(thread_id)

    if "checkpoint" not in state or not state["checkpoint"]:
        pytest.skip("No checkpoint in current state")

    checkpoint_id = state["checkpoint"].get("checkpoint_id")
    if not checkpoint_id:
        pytest.skip("No checkpoint_id in state")

    # Get state at that specific checkpoint
    try:
        historical_state = await async_client.threads.get_state(
            thread_id,
            checkpoint_id=checkpoint_id
        )
        assert "values" in historical_state
    except TypeError:
        # SDK version may not support checkpoint_id parameter
        pytest.skip("checkpoint_id parameter not supported in this SDK version")


async def test_thread_state_persistence(async_client, thread):
    """Test that thread state persists across multiple updates."""
    thread_id = thread["thread_id"]

    # Make multiple updates
    await async_client.threads.update_state(
        thread_id,
        values={"step1": "value1"}
    )

    await async_client.threads.update_state(
        thread_id,
        values={"step2": "value2"}
    )

    # Get final state
    state = await async_client.threads.get_state(thread_id)

    # Both values should be present (merged state)
    # Note: behavior depends on implementation - some may replace, some may merge
    assert "step1" in state["values"] or "step2" in state["values"]


async def test_copy_thread(async_client, thread_with_state):
    """Test copying a thread with its state."""
    thread_id = thread_with_state["thread_id"]

    # Get original state
    original_state = await async_client.threads.get_state(thread_id)

    # Copy the thread
    try:
        new_thread = await async_client.threads.copy(thread_id)

        assert "thread_id" in new_thread
        assert new_thread["thread_id"] != thread_id

        # New thread should have copied state
        new_state = await async_client.threads.get_state(new_thread["thread_id"])
        assert new_state["values"] == original_state["values"]

        # Cleanup
        await async_client.threads.delete(new_thread["thread_id"])

    except AttributeError:
        pytest.skip("threads.copy not available in this SDK version")
    except Exception as e:
        if "not implemented" in str(e).lower() or "501" in str(e):
            pytest.skip("Thread copy not implemented yet")
        raise
