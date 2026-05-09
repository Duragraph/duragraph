"""Subgraph support for composing graphs within graphs."""

from typing import Any

from duragraph.nodes import NodeDescriptor, NodeMetadata
from duragraph.types import State


class SubgraphNode:
    """Wraps a graph instance so it can be used as a single node in a parent graph.

    The child graph runs to completion when this node executes. State is passed
    from the parent into the child, and the child's final output state is merged
    back into the parent.

    Example::

        @Graph(id="summarizer")
        class Summarizer:
            @entrypoint
            @node()
            def summarize(self, state):
                return {"summary": state["input"][:50] + "..."}

        @Graph(id="pipeline")
        class Pipeline:
            @entrypoint
            @node()
            def preprocess(self, state):
                return {"input": state["input"].strip()}

            summarize = SubgraphNode.from_graph(Summarizer, name="summarize")

            preprocess >> summarize
    """

    def __init__(
        self,
        graph_cls: type,
        *,
        name: str | None = None,
        input_map: dict[str, str] | None = None,
        output_map: dict[str, str] | None = None,
    ):
        self._graph_cls = graph_cls
        self._name = name or getattr(graph_cls, "_graph_id", graph_cls.__name__.lower())
        self._input_map = input_map or {}
        self._output_map = output_map or {}

    @classmethod
    def from_graph(
        cls,
        graph_cls: type,
        *,
        name: str | None = None,
        input_map: dict[str, str] | None = None,
        output_map: dict[str, str] | None = None,
    ) -> NodeDescriptor:
        """Create a NodeDescriptor that runs a child graph as a subgraph node.

        Args:
            graph_cls: A class decorated with ``@Graph(...)``.
            name: Node name in the parent graph (defaults to child graph id).
            input_map: Map parent state keys to child state keys before execution.
            output_map: Map child output keys to parent state keys after execution.

        Returns:
            A NodeDescriptor usable inside a parent ``@Graph`` class body.
        """
        node_name = name or getattr(graph_cls, "__name__", "subgraph").lower()
        meta = NodeMetadata(
            node_type="subgraph",
            name=node_name,
            config={
                "graph_cls": graph_cls,
                "input_map": input_map or {},
                "output_map": output_map or {},
            },
        )

        def _subgraph_exec(_self: Any, state: State) -> State:
            return _run_subgraph_sync(graph_cls, state, input_map or {}, output_map or {})

        _subgraph_exec.__name__ = node_name
        _subgraph_exec.__qualname__ = node_name

        return NodeDescriptor(_subgraph_exec, meta)


def _remap_state(state: State, mapping: dict[str, str]) -> State:
    """Rename state keys according to a mapping."""
    if not mapping:
        return state.copy()
    out = state.copy()
    for src, dst in mapping.items():
        if src in out:
            out[dst] = out.pop(src)
    return out


def _run_subgraph_sync(
    graph_cls: type,
    state: State,
    input_map: dict[str, str],
    output_map: dict[str, str],
) -> State:
    """Execute a child graph synchronously and return merged output."""
    child = graph_cls()
    child_input = _remap_state(state, input_map)
    result = child.run(child_input)
    child_output = result.output
    return _remap_state(child_output, output_map)


async def execute_subgraph(
    graph_cls: type,
    state: State,
    input_map: dict[str, str],
    output_map: dict[str, str],
) -> State:
    """Execute a child graph asynchronously and return merged output."""
    child = graph_cls()
    child_input = _remap_state(state, input_map)
    result = await child.arun(child_input)
    child_output = result.output
    return _remap_state(child_output, output_map)
