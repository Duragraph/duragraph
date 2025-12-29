"""Edge definitions for DuraGraph workflows."""

from collections.abc import Callable
from typing import Any


class Edge:
    """Represents an edge between nodes in a graph."""

    def __init__(
        self,
        source: str,
        target: str | dict[str, str],
        condition: Callable[[dict[str, Any]], bool] | None = None,
    ):
        self.source = source
        self.target = target
        self.condition = condition

    def to_dict(self) -> dict[str, Any]:
        """Convert edge to dictionary representation."""
        result: dict[str, Any] = {
            "source": self.source,
            "target": self.target,
        }
        if self.condition is not None:
            result["conditional"] = True
        return result


class EdgeBuilder:
    """Builder for creating edges with fluent API."""

    def __init__(self, source: str):
        self.source = source
        self._edges: list[Edge] = []

    def to(
        self,
        target: str,
        *,
        condition: Callable[[dict[str, Any]], bool] | None = None,
    ) -> "EdgeBuilder":
        """Add an edge to a target node.

        Args:
            target: Name of the target node.
            condition: Optional condition function that takes state and returns bool.

        Returns:
            Self for chaining.
        """
        self._edges.append(Edge(self.source, target, condition))
        return self

    def to_conditional(
        self,
        mapping: dict[str, str],
    ) -> "EdgeBuilder":
        """Add conditional edges based on router output.

        Args:
            mapping: Dictionary mapping router output values to node names.

        Returns:
            Self for chaining.
        """
        self._edges.append(Edge(self.source, mapping))
        return self

    def build(self) -> list[Edge]:
        """Build and return all edges."""
        return self._edges


def edge(source: str) -> EdgeBuilder:
    """Create an edge builder starting from a source node.

    Args:
        source: Name of the source node.

    Returns:
        EdgeBuilder for fluent edge construction.

    Example:
        # Simple edge
        edge("classify").to("respond")

        # Conditional edge
        edge("router").to_conditional({
            "billing": "billing_handler",
            "support": "support_handler",
        })
    """
    return EdgeBuilder(source)


class NodeProxy:
    """Proxy object for node methods that enables >> operator for edge definition."""

    def __init__(self, name: str, graph: Any):
        self._name = name
        self._graph = graph

    def __rshift__(self, other: "NodeProxy") -> "NodeProxy":
        """Define an edge using >> operator.

        Example:
            classify >> respond
        """
        if self._graph is not None:
            self._graph._add_edge(self._name, other._name)
        return other

    def __repr__(self) -> str:
        return f"NodeProxy({self._name})"
