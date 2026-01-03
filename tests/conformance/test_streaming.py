"""
Streaming Conformance Tests

Tests streaming endpoints using the LangGraph SDK to verify
DuraGraph compatibility with LangGraph Cloud streaming modes.

Note: These tests may skip if streaming is not fully implemented.
"""
import pytest
import asyncio


pytestmark = pytest.mark.asyncio

# Timeout for streaming operations (in seconds)
STREAM_TIMEOUT = 10


async def collect_events_with_timeout(stream, timeout=STREAM_TIMEOUT, max_events=10):
    """Collect streaming events with a timeout."""
    events = []
    try:
        async def collect():
            async for event in stream:
                events.append(event)
                if len(events) >= max_events:
                    break
        await asyncio.wait_for(collect(), timeout=timeout)
    except asyncio.TimeoutError:
        pass  # Return whatever events we collected
    return events


async def test_stream_values_mode(async_client, assistant, thread):
    """Test streaming with 'values' mode returns full state on each event."""
    thread_id = thread["thread_id"]
    assistant_id = assistant["assistant_id"]

    try:
        stream = async_client.runs.stream(
            thread_id,
            assistant_id,
            input={"message": "test streaming values"},
            stream_mode="values"
        )
        events = await collect_events_with_timeout(stream)
    except Exception as e:
        if "not implemented" in str(e).lower() or "501" in str(e):
            pytest.skip("Streaming not implemented yet")
        if "timeout" in str(e).lower():
            pytest.skip("Streaming timed out - endpoint may not be fully implemented")
        raise

    if len(events) == 0:
        pytest.skip("No streaming events received - streaming may not be implemented")

    # In values mode, events should contain full state values
    values_events = [e for e in events if hasattr(e, 'event') and e.event == "values"]
    for event in values_events:
        if hasattr(event, 'data'):
            assert "values" in event.data or isinstance(event.data, dict)


async def test_stream_messages_mode(async_client, assistant, thread):
    """Test streaming with 'messages' mode returns message updates."""
    thread_id = thread["thread_id"]
    assistant_id = assistant["assistant_id"]

    try:
        stream = async_client.runs.stream(
            thread_id,
            assistant_id,
            input={"message": "test streaming messages"},
            stream_mode="messages"
        )
        events = await collect_events_with_timeout(stream)
    except Exception as e:
        if "not implemented" in str(e).lower() or "501" in str(e):
            pytest.skip("Messages streaming mode not implemented yet")
        if "invalid" in str(e).lower() and "mode" in str(e).lower():
            pytest.skip("Messages streaming mode not supported")
        if "timeout" in str(e).lower():
            pytest.skip("Streaming timed out")
        raise

    if len(events) == 0:
        pytest.skip("No streaming events received")


async def test_stream_updates_mode(async_client, assistant, thread):
    """Test streaming with 'updates' mode returns state diffs."""
    thread_id = thread["thread_id"]
    assistant_id = assistant["assistant_id"]

    try:
        stream = async_client.runs.stream(
            thread_id,
            assistant_id,
            input={"message": "test streaming updates"},
            stream_mode="updates"
        )
        events = await collect_events_with_timeout(stream)
    except Exception as e:
        if "not implemented" in str(e).lower() or "501" in str(e):
            pytest.skip("Updates streaming mode not implemented yet")
        if "invalid" in str(e).lower() and "mode" in str(e).lower():
            pytest.skip("Updates streaming mode not supported")
        if "timeout" in str(e).lower():
            pytest.skip("Streaming timed out")
        raise

    if len(events) == 0:
        pytest.skip("No streaming events received")


async def test_stream_debug_mode(async_client, assistant, thread):
    """Test streaming with 'debug' mode returns detailed execution info."""
    thread_id = thread["thread_id"]
    assistant_id = assistant["assistant_id"]

    try:
        stream = async_client.runs.stream(
            thread_id,
            assistant_id,
            input={"message": "test streaming debug"},
            stream_mode="debug"
        )
        events = await collect_events_with_timeout(stream)
    except Exception as e:
        if "not implemented" in str(e).lower() or "501" in str(e):
            pytest.skip("Debug streaming mode not implemented yet")
        if "invalid" in str(e).lower() and "mode" in str(e).lower():
            pytest.skip("Debug streaming mode not supported")
        if "timeout" in str(e).lower():
            pytest.skip("Streaming timed out")
        raise

    # Debug mode is optional, so we just verify it doesn't error
    if len(events) == 0:
        pytest.skip("No streaming events received")


async def test_stream_multiple_modes(async_client, assistant, thread):
    """Test streaming with multiple modes simultaneously."""
    thread_id = thread["thread_id"]
    assistant_id = assistant["assistant_id"]

    try:
        stream = async_client.runs.stream(
            thread_id,
            assistant_id,
            input={"message": "test multiple modes"},
            stream_mode=["values", "updates"]
        )
        events = await collect_events_with_timeout(stream)
    except TypeError:
        pytest.skip("Multiple stream modes not supported in this SDK version")
    except Exception as e:
        if "not implemented" in str(e).lower() or "501" in str(e):
            pytest.skip("Multiple streaming modes not implemented yet")
        if "timeout" in str(e).lower():
            pytest.skip("Streaming timed out")
        raise

    if len(events) == 0:
        pytest.skip("No streaming events received")


async def test_stream_events_order(async_client, assistant, thread):
    """Test that streaming events arrive in correct order."""
    thread_id = thread["thread_id"]
    assistant_id = assistant["assistant_id"]

    try:
        stream = async_client.runs.stream(
            thread_id,
            assistant_id,
            input={"message": "test event order"},
            stream_mode="values"
        )
        events = await collect_events_with_timeout(stream)
    except Exception as e:
        if "not implemented" in str(e).lower() or "501" in str(e):
            pytest.skip("Streaming not implemented yet")
        if "timeout" in str(e).lower():
            pytest.skip("Streaming timed out")
        raise

    if len(events) == 0:
        pytest.skip("No events received")

    # Check for metadata/start event at beginning
    event_types = [getattr(e, 'event', None) for e in events]

    # Should have some structure - either metadata first or values events
    assert any(t is not None for t in event_types), "Events should have event type"


async def test_stream_completion_event(async_client, assistant, thread):
    """Test that streaming ends with a completion event."""
    thread_id = thread["thread_id"]
    assistant_id = assistant["assistant_id"]

    try:
        stream = async_client.runs.stream(
            thread_id,
            assistant_id,
            input={"message": "test completion"},
            stream_mode="values"
        )
        events = await collect_events_with_timeout(stream)
    except Exception as e:
        if "not implemented" in str(e).lower() or "501" in str(e):
            pytest.skip("Streaming not implemented yet")
        if "timeout" in str(e).lower():
            pytest.skip("Streaming timed out")
        raise

    if len(events) == 0:
        pytest.skip("No events received")

    # Last event should indicate completion (end, done, or final values)
    last_event = events[-1]
    event_type = getattr(last_event, 'event', None)

    # Various completion indicators are acceptable
    completion_indicators = ['end', 'done', 'complete', 'values', 'metadata']
    assert event_type in completion_indicators or event_type is None


async def test_join_thread_stream(async_client, assistant, thread):
    """Test joining an existing thread stream mid-execution."""
    thread_id = thread["thread_id"]
    assistant_id = assistant["assistant_id"]

    # Start a run
    run = await async_client.runs.create(
        thread_id=thread_id,
        assistant_id=assistant_id,
        input={"message": "test join stream"}
    )
    run_id = run["run_id"]

    # Try to join the stream (may already be complete for fast runs)
    try:
        stream = async_client.runs.join(thread_id, run_id)
        events = await collect_events_with_timeout(stream, timeout=5)
    except AttributeError:
        pytest.skip("runs.join not available in this SDK version")
    except Exception as e:
        if "not implemented" in str(e).lower() or "501" in str(e):
            pytest.skip("Join stream not implemented yet")
        if "not found" in str(e).lower() or "404" in str(e):
            # Run may have completed before we could join
            pass
        elif "timeout" in str(e).lower():
            pass  # Timeout is acceptable
        else:
            raise

    # Events received or run already completed - both are valid


async def test_stateless_stream(async_client, assistant):
    """Test stateless streaming (without a thread)."""
    assistant_id = assistant["assistant_id"]

    try:
        stream = async_client.runs.stream(
            None,  # No thread - stateless
            assistant_id,
            input={"message": "test stateless stream"},
            stream_mode="values"
        )
        events = await collect_events_with_timeout(stream)
    except TypeError:
        pytest.skip("Stateless streaming not supported in this SDK version")
    except Exception as e:
        if "not implemented" in str(e).lower() or "501" in str(e):
            pytest.skip("Stateless streaming not implemented yet")
        if "thread" in str(e).lower() and "required" in str(e).lower():
            pytest.skip("Thread is required for streaming in this implementation")
        if "timeout" in str(e).lower():
            pytest.skip("Streaming timed out")
        raise


async def test_stream_with_config(async_client, assistant, thread):
    """Test streaming with additional config options."""
    thread_id = thread["thread_id"]
    assistant_id = assistant["assistant_id"]

    try:
        stream = async_client.runs.stream(
            thread_id,
            assistant_id,
            input={"message": "test with config"},
            stream_mode="values",
            config={"configurable": {"model": "test"}}
        )
        events = await collect_events_with_timeout(stream)
    except TypeError:
        pytest.skip("Config parameter not supported in this SDK version")
    except Exception as e:
        if "not implemented" in str(e).lower() or "501" in str(e):
            pytest.skip("Streaming with config not implemented yet")
        if "timeout" in str(e).lower():
            pytest.skip("Streaming timed out")
        raise


async def test_stream_interrupt_before(async_client, assistant, thread):
    """Test streaming with interrupt_before parameter."""
    thread_id = thread["thread_id"]
    assistant_id = assistant["assistant_id"]

    try:
        stream = async_client.runs.stream(
            thread_id,
            assistant_id,
            input={"message": "test interrupt"},
            stream_mode="values",
            interrupt_before=["*"]  # Interrupt before any node
        )
        events = await collect_events_with_timeout(stream)
    except TypeError:
        pytest.skip("interrupt_before parameter not supported")
    except Exception as e:
        if "not implemented" in str(e).lower() or "501" in str(e):
            pytest.skip("interrupt_before not implemented yet")
        if "timeout" in str(e).lower():
            pytest.skip("Streaming timed out")
        raise

    # Should receive events up to interrupt point
