"""Tests for Graph decorator."""

import pytest
from duragraph import Graph, llm_node, node, entrypoint


def test_graph_basic():
    """Test basic graph creation."""

    @Graph(id="test_graph")
    class TestAgent:
        @entrypoint
        @node()
        def start(self, state):
            state["started"] = True
            return state

    agent = TestAgent()
    result = agent.run({"input": "hello"})

    assert result.status == "completed"
    assert result.output["started"] is True
    assert result.output["input"] == "hello"


def test_graph_with_edges():
    """Test graph with multiple nodes and edges."""

    @Graph(id="multi_node")
    class MultiNodeAgent:
        @entrypoint
        @node()
        def first(self, state):
            state["first"] = True
            return state

        @node()
        def second(self, state):
            state["second"] = True
            return state

    agent = MultiNodeAgent()
    agent._add_edge("first", "second")

    result = agent.run({"input": "test"})

    assert result.status == "completed"
    assert result.output["first"] is True
    assert result.output["second"] is True


def test_graph_definition_to_ir():
    """Test graph IR generation."""

    @Graph(id="ir_test")
    class IRTestAgent:
        @entrypoint
        @llm_node(model="gpt-4o-mini", temperature=0.5)
        def process(self, state):
            return state

    agent = IRTestAgent()
    definition = agent._get_definition()
    ir = definition.to_ir()

    assert ir["version"] == "1.0"
    assert ir["graph"]["id"] == "ir_test"
    assert ir["graph"]["entrypoint"] == "process"
    assert len(ir["graph"]["nodes"]) == 1
    assert ir["graph"]["nodes"][0]["type"] == "llm"
    assert ir["graph"]["nodes"][0]["config"]["model"] == "gpt-4o-mini"
