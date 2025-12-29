"""Graph decorator and class for DuraGraph workflows."""

from collections.abc import AsyncIterator, Callable
from typing import Any, TypeVar

from duragraph.edges import Edge, NodeProxy
from duragraph.nodes import NodeMetadata
from duragraph.types import Event, GraphConfig, RunResult, State

T = TypeVar("T")


class GraphDefinition:
    """Internal representation of a graph definition."""

    def __init__(
        self,
        graph_id: str,
        nodes: dict[str, NodeMetadata],
        edges: list[Edge],
        entrypoint: str | None = None,
    ):
        self.graph_id = graph_id
        self.nodes = nodes
        self.edges = edges
        self.entrypoint = entrypoint

    def to_ir(self) -> dict[str, Any]:
        """Convert to Intermediate Representation for the control plane."""
        nodes_ir = []
        for name, meta in self.nodes.items():
            node_ir = {
                "id": name,
                "type": meta.node_type,
                "config": meta.config,
            }
            nodes_ir.append(node_ir)

        edges_ir = []
        for edge in self.edges:
            edge_ir = edge.to_dict()
            edges_ir.append(edge_ir)

        return {
            "version": "1.0",
            "graph": {
                "id": self.graph_id,
                "entrypoint": self.entrypoint,
                "nodes": nodes_ir,
                "edges": edges_ir,
            },
        }


class GraphInstance:
    """Runtime instance of a graph that can be executed."""

    def __init__(self, definition: GraphDefinition, instance: Any):
        self._definition = definition
        self._instance = instance
        self._control_plane_url: str | None = None

    def run(
        self,
        input: State,
        *,
        config: GraphConfig | None = None,
        thread_id: str | None = None,
    ) -> RunResult:
        """Execute the graph synchronously.

        Args:
            input: Initial state for the graph.
            config: Optional execution configuration.
            thread_id: Optional thread ID for conversation context.

        Returns:
            RunResult with execution output.
        """
        # Local execution - traverse graph and execute nodes
        state = input.copy()
        nodes_executed: list[str] = []

        current_node = self._definition.entrypoint
        if current_node is None:
            raise ValueError("No entrypoint defined for graph")

        while current_node is not None:
            # Execute node
            node_method = getattr(self._instance, current_node, None)
            if node_method is None:
                raise ValueError(f"Node method '{current_node}' not found")

            result = node_method(state)
            if isinstance(result, dict):
                state.update(result)
            nodes_executed.append(current_node)

            # Find next node
            next_node = None
            for edge in self._definition.edges:
                if edge.source == current_node:
                    if isinstance(edge.target, str):
                        next_node = edge.target
                    elif isinstance(edge.target, dict):
                        # Router node - result should be the key
                        if isinstance(result, str) and result in edge.target:
                            next_node = edge.target[result]
                    break

            current_node = next_node

        return RunResult(
            run_id="local-run",
            status="completed",
            output=state,
            nodes_executed=nodes_executed,
        )

    async def arun(
        self,
        input: State,
        *,
        config: GraphConfig | None = None,
        thread_id: str | None = None,
    ) -> RunResult:
        """Execute the graph asynchronously.

        Args:
            input: Initial state for the graph.
            config: Optional execution configuration.
            thread_id: Optional thread ID for conversation context.

        Returns:
            RunResult with execution output.
        """
        # For now, delegate to sync implementation
        return self.run(input, config=config, thread_id=thread_id)

    async def stream(
        self,
        input: State,
        *,
        config: GraphConfig | None = None,
        thread_id: str | None = None,
    ) -> AsyncIterator[Event]:
        """Stream graph execution events.

        Args:
            input: Initial state for the graph.
            config: Optional execution configuration.
            thread_id: Optional thread ID for conversation context.

        Yields:
            Event objects for each execution step.
        """
        from datetime import datetime

        run_id = "local-stream"
        state = input.copy()

        yield Event(
            type="run_started",
            run_id=run_id,
            data={"input": input},
            timestamp=datetime.utcnow().isoformat(),
        )

        current_node = self._definition.entrypoint
        if current_node is None:
            yield Event(
                type="run_failed",
                run_id=run_id,
                data={"error": "No entrypoint defined"},
                timestamp=datetime.utcnow().isoformat(),
            )
            return

        while current_node is not None:
            yield Event(
                type="node_started",
                run_id=run_id,
                node_id=current_node,
                data={},
                timestamp=datetime.utcnow().isoformat(),
            )

            node_method = getattr(self._instance, current_node, None)
            if node_method is None:
                yield Event(
                    type="run_failed",
                    run_id=run_id,
                    data={"error": f"Node '{current_node}' not found"},
                    timestamp=datetime.utcnow().isoformat(),
                )
                return

            result = node_method(state)
            if isinstance(result, dict):
                state.update(result)

            yield Event(
                type="node_completed",
                run_id=run_id,
                node_id=current_node,
                data={"output": result},
                timestamp=datetime.utcnow().isoformat(),
            )

            # Find next node
            next_node = None
            for edge in self._definition.edges:
                if edge.source == current_node:
                    if isinstance(edge.target, str):
                        next_node = edge.target
                    elif isinstance(edge.target, dict):
                        if isinstance(result, str) and result in edge.target:
                            next_node = edge.target[result]
                    break

            current_node = next_node

        yield Event(
            type="run_completed",
            run_id=run_id,
            data={"output": state},
            timestamp=datetime.utcnow().isoformat(),
        )

    def serve(
        self,
        control_plane_url: str,
        *,
        worker_name: str | None = None,
        capabilities: list[str] | None = None,
    ) -> None:
        """Register and serve this graph on the control plane.

        Args:
            control_plane_url: URL of the DuraGraph control plane.
            worker_name: Optional name for the worker.
            capabilities: Optional list of worker capabilities.
        """
        from duragraph.worker import Worker

        worker = Worker(
            control_plane_url=control_plane_url,
            name=worker_name,
            capabilities=capabilities,
        )
        worker.register_graph(self._definition)
        worker.run()

    async def aserve(
        self,
        control_plane_url: str,
        *,
        worker_name: str | None = None,
        capabilities: list[str] | None = None,
    ) -> None:
        """Async version of serve().

        Args:
            control_plane_url: URL of the DuraGraph control plane.
            worker_name: Optional name for the worker.
            capabilities: Optional list of worker capabilities.
        """
        from duragraph.worker import Worker

        worker = Worker(
            control_plane_url=control_plane_url,
            name=worker_name,
            capabilities=capabilities,
        )
        worker.register_graph(self._definition)
        await worker.arun()


def Graph(
    id: str,
    *,
    description: str | None = None,
    version: str = "1.0.0",
) -> Callable[[type[T]], type[T]]:
    """Decorator to define a graph from a class.

    Args:
        id: Unique identifier for the graph.
        description: Optional description of the graph.
        version: Version string for the graph.

    Returns:
        Decorated class that can be instantiated as a graph.

    Example:
        @Graph(id="customer_support")
        class CustomerSupportAgent:
            @entrypoint
            @llm_node(model="gpt-4o-mini")
            def classify(self, state):
                return {"intent": "billing"}

            @llm_node(model="gpt-4o-mini")
            def respond(self, state):
                return {"response": "I'll help with billing."}

            classify >> respond
    """

    def decorator(cls: type[T]) -> type[T]:
        original_init = cls.__init__

        def new_init(self: Any, *args: Any, **kwargs: Any) -> None:
            original_init(self, *args, **kwargs)
            self._graph_id = id
            self._graph_description = description
            self._graph_version = version
            self._edges: list[Edge] = []
            self._setup_node_proxies()

        def _setup_node_proxies(self: Any) -> None:
            """Set up NodeProxy objects for >> operator."""
            for name in dir(self):
                if name.startswith("_"):
                    continue
                attr = getattr(self, name)
                if callable(attr) and hasattr(attr, "_node_metadata"):
                    # Create a proxy that enables >> operator
                    proxy = NodeProxy(name, self)
                    setattr(self, f"_{name}_proxy", proxy)

        def _add_edge(self: Any, source: str, target: str) -> None:
            """Add an edge between nodes."""
            self._edges.append(Edge(source, target))

        def _get_definition(self: Any) -> GraphDefinition:
            """Get the graph definition."""
            nodes: dict[str, NodeMetadata] = {}
            entrypoint: str | None = None

            for name in dir(self):
                if name.startswith("_"):
                    continue
                attr = getattr(self, name)
                if callable(attr) and hasattr(attr, "_node_metadata"):
                    meta: NodeMetadata = attr._node_metadata
                    nodes[name] = meta
                    if meta.config.get("is_entrypoint"):
                        entrypoint = name

            return GraphDefinition(
                graph_id=self._graph_id,
                nodes=nodes,
                edges=self._edges,
                entrypoint=entrypoint,
            )

        def run(
            self: Any,
            input: State,
            *,
            config: GraphConfig | None = None,
            thread_id: str | None = None,
        ) -> RunResult:
            """Execute the graph."""
            definition = self._get_definition()
            instance = GraphInstance(definition, self)
            return instance.run(input, config=config, thread_id=thread_id)

        async def arun(
            self: Any,
            input: State,
            *,
            config: GraphConfig | None = None,
            thread_id: str | None = None,
        ) -> RunResult:
            """Execute the graph asynchronously."""
            definition = self._get_definition()
            instance = GraphInstance(definition, self)
            return await instance.arun(input, config=config, thread_id=thread_id)

        async def stream(
            self: Any,
            input: State,
            *,
            config: GraphConfig | None = None,
            thread_id: str | None = None,
        ) -> AsyncIterator[Event]:
            """Stream graph execution events."""
            definition = self._get_definition()
            instance = GraphInstance(definition, self)
            async for event in instance.stream(input, config=config, thread_id=thread_id):
                yield event

        def serve(
            self: Any,
            control_plane_url: str,
            *,
            worker_name: str | None = None,
            capabilities: list[str] | None = None,
        ) -> None:
            """Register and serve this graph."""
            definition = self._get_definition()
            instance = GraphInstance(definition, self)
            instance.serve(
                control_plane_url,
                worker_name=worker_name,
                capabilities=capabilities,
            )

        def as_subgraph(cls_self: type[Any]) -> Any:
            """Return this graph as a subgraph node."""
            # Create a subgraph node that can be used in another graph
            instance = cls_self()
            return instance._get_definition()

        cls.__init__ = new_init
        cls._setup_node_proxies = _setup_node_proxies
        cls._add_edge = _add_edge
        cls._get_definition = _get_definition
        cls.run = run
        cls.arun = arun
        cls.stream = stream
        cls.serve = serve
        cls.as_subgraph = classmethod(as_subgraph)

        return cls

    return decorator
