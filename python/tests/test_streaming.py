"""Tests for streaming improvements."""

import asyncio

from duragraph import Graph, entrypoint, node


def _collect_events(coro):
    """Helper to collect async iterator events."""
    loop = asyncio.get_event_loop()

    async def _gather():
        events = []
        async for event in coro:
            events.append(event)
        return events

    return loop.run_until_complete(_gather())


def test_stream_default_all_events():
    """Default stream yields all event types."""

    @Graph(id="stream_test")
    class StreamAgent:
        @entrypoint
        @node()
        def step_one(self, state):
            state["done"] = True
            return state

    agent = StreamAgent()
    events = _collect_events(agent.stream({"input": "hi"}))

    types = [e.type for e in events]
    assert "run_started" in types
    assert "node_started" in types
    assert "node_completed" in types
    assert "run_completed" in types


def test_stream_values_mode():
    """Stream with values mode includes state snapshots."""

    @Graph(id="values_test")
    class ValuesAgent:
        @entrypoint
        @node()
        def process(self, state):
            state["processed"] = True
            return state

    agent = ValuesAgent()
    events = _collect_events(
        agent.stream({"input": "test"}, stream_mode=["values"])
    )

    types = [e.type for e in events]
    assert "values" in types
    value_events = [e for e in events if e.type == "values"]
    assert len(value_events) >= 1
    assert "state" in value_events[0].data


def test_stream_updates_mode():
    """Stream with updates mode includes per-node updates."""

    @Graph(id="updates_test")
    class UpdatesAgent:
        @entrypoint
        @node()
        def step(self, state):
            state["updated"] = True
            return state

    agent = UpdatesAgent()
    events = _collect_events(
        agent.stream({"input": "test"}, stream_mode=["updates"])
    )

    types = [e.type for e in events]
    assert "updates" in types
    assert "node_started" in types
    assert "node_completed" in types


def test_stream_events_mode():
    """Stream with events mode yields all events."""

    @Graph(id="events_test")
    class EventsAgent:
        @entrypoint
        @node()
        def do_it(self, state):
            return {"result": "ok"}

    agent = EventsAgent()
    events = _collect_events(
        agent.stream({"input": "go"}, stream_mode=["events"])
    )

    types = [e.type for e in events]
    assert "node_started" in types
    assert "node_completed" in types


def test_stream_multi_node():
    """Stream works correctly with multi-node graphs."""

    @Graph(id="multi_stream")
    class MultiAgent:
        @entrypoint
        @node()
        def first(self, state):
            state["first"] = True
            return state

        @node()
        def second(self, state):
            state["second"] = True
            return state

        first >> second

    agent = MultiAgent()
    events = _collect_events(agent.stream({"input": "test"}))

    node_completed = [e for e in events if e.type == "node_completed"]
    assert len(node_completed) == 2

    node_ids = [e.node_id for e in node_completed]
    assert "first" in node_ids
    assert "second" in node_ids


def test_stream_no_entrypoint_fails():
    """Stream gracefully handles missing entrypoint."""

    @Graph(id="no_entry")
    class NoEntryAgent:
        @node()
        def orphan(self, state):
            return state

    agent = NoEntryAgent()
    events = _collect_events(agent.stream({"input": "test"}))

    types = [e.type for e in events]
    assert "run_failed" in types


def test_stream_combined_modes():
    """Stream with multiple modes includes events from all modes."""

    @Graph(id="combined_test")
    class CombinedAgent:
        @entrypoint
        @node()
        def work(self, state):
            state["worked"] = True
            return state

    agent = CombinedAgent()
    events = _collect_events(
        agent.stream({"input": "test"}, stream_mode=["values", "updates"])
    )

    types = {e.type for e in events}
    assert "values" in types
    assert "updates" in types
    assert "node_started" in types
