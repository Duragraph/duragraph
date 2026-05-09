"""Tests for subgraph support."""

import asyncio

from duragraph import Graph, entrypoint, node
from duragraph.subgraph import SubgraphNode


def test_subgraph_sync_execution():
    """Test subgraph runs as a node inside a parent graph."""

    @Graph(id="child")
    class ChildGraph:
        @entrypoint
        @node()
        def process(self, state):
            state["child_ran"] = True
            return state

    @Graph(id="parent")
    class ParentGraph:
        @entrypoint
        @node()
        def prepare(self, state):
            state["prepared"] = True
            return state

        child = SubgraphNode.from_graph(ChildGraph, name="child")

        prepare >> child

    agent = ParentGraph()
    result = agent.run({"input": "test"})

    assert result.status == "completed"
    assert result.output["prepared"] is True
    assert result.output["child_ran"] is True


def test_subgraph_async_execution():
    """Test subgraph works with async execution."""

    @Graph(id="child_async")
    class ChildGraph:
        @entrypoint
        @node()
        def transform(self, state):
            state["transformed"] = True
            return state

    @Graph(id="parent_async")
    class ParentGraph:
        @entrypoint
        @node()
        def start(self, state):
            state["started"] = True
            return state

        child = SubgraphNode.from_graph(ChildGraph, name="child")

        start >> child

    agent = ParentGraph()
    result = asyncio.get_event_loop().run_until_complete(
        agent.arun({"input": "test"})
    )

    assert result.status == "completed"
    assert result.output["started"] is True
    assert result.output["transformed"] is True


def test_subgraph_with_input_output_map():
    """Test state key remapping between parent and child."""

    @Graph(id="mapped_child")
    class MappedChild:
        @entrypoint
        @node()
        def compute(self, state):
            state["result"] = state.get("data", "") + "_processed"
            return state

    @Graph(id="mapped_parent")
    class MappedParent:
        @entrypoint
        @node()
        def init(self, state):
            state["raw_data"] = "hello"
            return state

        child = SubgraphNode.from_graph(
            MappedChild,
            name="child",
            input_map={"raw_data": "data"},
            output_map={"result": "final_result"},
        )

        init >> child

    agent = MappedParent()
    result = agent.run({"input": "test"})

    assert result.status == "completed"
    assert result.output["final_result"] == "hello_processed"


def test_as_subgraph_classmethod():
    """Test the as_subgraph classmethod on @Graph classes."""

    @Graph(id="sub")
    class SubGraph:
        @entrypoint
        @node()
        def action(self, state):
            state["sub_done"] = True
            return state

    descriptor = SubGraph.as_subgraph(name="my_sub")
    assert descriptor.metadata.node_type == "subgraph"
    assert descriptor.name == "my_sub"
