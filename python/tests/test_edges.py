"""Tests for edge operator in class body."""


from duragraph import Graph
from duragraph.nodes import entrypoint, llm_node, node


def test_edge_operator_in_class_body():
    """Test that >> operator works in class body."""

    @Graph(id="test_graph")
    class TestGraph:
        @entrypoint
        @node()
        def start(self, state):
            return {"step": "start"}

        @node()
        def middle(self, state):
            return {"step": "middle"}

        @node()
        def end(self, state):
            return {"step": "end"}

        # Define edges using >> operator
        start >> middle >> end

    # Create instance and check edges
    graph = TestGraph()
    _definition = graph._get_definition()  # Call to trigger edge processing

    # Check that edges were created
    assert len(graph._edges) == 2

    # Check edge connections
    edges = [(e.source, e.target) for e in graph._edges]
    assert ("start", "middle") in edges
    assert ("middle", "end") in edges


def test_multiple_edge_chains():
    """Test multiple edge chains in class body."""

    @Graph(id="complex_graph")
    class ComplexGraph:
        @entrypoint
        @node()
        def start(self, state):
            return state

        @node()
        def process_a(self, state):
            return state

        @node()
        def process_b(self, state):
            return state

        @node()
        def merge(self, state):
            return state

        @node()
        def finish(self, state):
            return state

        # Multiple edge chains
        start >> process_a >> merge
        start >> process_b >> merge
        merge >> finish

    graph = ComplexGraph()
    edges = [(e.source, e.target) for e in graph._edges]

    # Check all edges were created
    assert len(edges) == 5
    assert ("start", "process_a") in edges
    assert ("process_a", "merge") in edges
    assert ("start", "process_b") in edges
    assert ("process_b", "merge") in edges
    assert ("merge", "finish") in edges


def test_edge_operator_with_llm_nodes():
    """Test >> operator with LLM nodes."""

    @Graph(id="llm_graph")
    class LLMGraph:
        @entrypoint
        @llm_node(model="gpt-4o-mini")
        def classify(self, state):
            return state

        @llm_node(model="gpt-4o-mini")
        def respond(self, state):
            return state

        classify >> respond

    graph = LLMGraph()
    edges = [(e.source, e.target) for e in graph._edges]

    assert len(edges) == 1
    assert ("classify", "respond") in edges


def test_edge_operator_preserves_metadata():
    """Test that node metadata is preserved when using >> operator."""

    @Graph(id="metadata_graph")
    class MetadataGraph:
        @entrypoint
        @llm_node(model="claude-3-sonnet", temperature=0.5)
        def node1(self, state):
            return state

        @node(retry_on=["ValueError"], max_retries=5)
        def node2(self, state):
            return state

        node1 >> node2

    graph = MetadataGraph()
    definition = graph._get_definition()

    # Check node1 metadata
    assert definition.nodes["node1"].node_type == "llm"
    assert definition.nodes["node1"].config["model"] == "claude-3-sonnet"
    assert definition.nodes["node1"].config["temperature"] == 0.5
    assert definition.nodes["node1"].config["is_entrypoint"] is True

    # Check node2 metadata
    assert definition.nodes["node2"].node_type == "function"
    assert definition.nodes["node2"].config["retry_on"] == ["ValueError"]
    assert definition.nodes["node2"].config["max_retries"] == 5

    # Check edge
    edges = [(e.source, e.target) for e in graph._edges]
    assert ("node1", "node2") in edges


def test_async_nodes_with_edge_operator():
    """Test that async nodes work with >> operator."""

    @Graph(id="async_graph")
    class AsyncGraph:
        @entrypoint
        @node()
        async def async_start(self, state):
            return {"async": True}

        @node()
        def sync_middle(self, state):
            return {"sync": True}

        @node()
        async def async_end(self, state):
            return {"done": True}

        async_start >> sync_middle >> async_end

    graph = AsyncGraph()
    definition = graph._get_definition()

    # Check async flags
    assert definition.nodes["async_start"].is_async is True
    assert definition.nodes["sync_middle"].is_async is False
    assert definition.nodes["async_end"].is_async is True

    # Check edges
    edges = [(e.source, e.target) for e in graph._edges]
    assert ("async_start", "sync_middle") in edges
    assert ("sync_middle", "async_end") in edges


def test_graph_execution_with_edge_operator():
    """Test that graphs with >> operator edges execute correctly."""

    @Graph(id="exec_graph")
    class ExecutionGraph:
        @entrypoint
        @node()
        def step1(self, state):
            state["path"] = state.get("path", [])
            state["path"].append("step1")
            return state

        @node()
        def step2(self, state):
            state["path"].append("step2")
            return state

        @node()
        def step3(self, state):
            state["path"].append("step3")
            return state

        step1 >> step2 >> step3

    graph = ExecutionGraph()
    result = graph.run({})

    # Check execution path
    assert result.output["path"] == ["step1", "step2", "step3"]
