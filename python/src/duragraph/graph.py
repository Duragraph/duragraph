"""Graph decorator and class for DuraGraph workflows."""

from collections.abc import AsyncIterator, Callable
from typing import Any, TypeVar

from duragraph.edges import Edge, NodeProxy
from duragraph.nodes import NodeMetadata
from duragraph.types import Event, GraphConfig, RunResult, State, StreamMode

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
        from duragraph.executor import execute_node

        state = input.copy()
        nodes_executed: list[str] = []

        current_node = self._definition.entrypoint
        if current_node is None:
            raise ValueError("No entrypoint defined for graph")

        while current_node is not None:
            # Get node metadata
            metadata = self._definition.nodes.get(current_node)
            if metadata is None:
                raise ValueError(f"Node metadata for '{current_node}' not found")

            # Get node method
            node_method = getattr(self._instance, current_node, None)
            if node_method is None:
                raise ValueError(f"Node method '{current_node}' not found")

            # Execute node
            result = await execute_node(current_node, metadata, node_method, state)
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

    async def stream(
        self,
        input: State,
        *,
        config: GraphConfig | None = None,
        thread_id: str | None = None,
        stream_mode: list[StreamMode] | None = None,
    ) -> AsyncIterator[Event]:
        """Stream graph execution events.

        Args:
            input: Initial state for the graph.
            config: Optional execution configuration.
            thread_id: Optional thread ID for conversation context.
            stream_mode: Filter which event types to yield.
                         Supported: "values", "updates", "messages", "events".
                         Default (None) yields all events.

        Yields:
            Event objects for each execution step.
        """
        from datetime import datetime

        modes = set(stream_mode or [])

        def _should_yield(event_type: str) -> bool:
            if not modes:
                return True
            if event_type in ("run_started", "run_completed", "run_failed"):
                return True
            if "events" in modes:
                return True
            if "values" in modes and event_type == "values":
                return True
            if "updates" in modes and event_type in ("node_started", "node_completed", "updates"):
                return True
            return "messages" in modes and event_type == "token"

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
            if _should_yield("node_started"):
                yield Event(
                    type="node_started",
                    run_id=run_id,
                    node_id=current_node,
                    data={},
                    timestamp=datetime.utcnow().isoformat(),
                )

            metadata = self._definition.nodes.get(current_node)
            if metadata is None:
                yield Event(
                    type="run_failed",
                    run_id=run_id,
                    data={"error": f"Node metadata for '{current_node}' not found"},
                    timestamp=datetime.utcnow().isoformat(),
                )
                return

            node_method = getattr(self._instance, current_node, None)
            if node_method is None:
                yield Event(
                    type="run_failed",
                    run_id=run_id,
                    data={"error": f"Node '{current_node}' not found"},
                    timestamp=datetime.utcnow().isoformat(),
                )
                return

            # Token-level streaming for LLM nodes when "messages" mode requested
            if metadata.node_type == "llm" and "messages" in modes:
                result = await self._stream_llm_node(
                    current_node, metadata, state, run_id, datetime
                )
                async for token_event in result["token_events"]:
                    yield token_event
                node_result = result["output"]
            else:
                from duragraph.executor import execute_node

                node_result = await execute_node(current_node, metadata, node_method, state)

            if isinstance(node_result, dict):
                state.update(node_result)

            if _should_yield("node_completed"):
                yield Event(
                    type="node_completed",
                    run_id=run_id,
                    node_id=current_node,
                    data={"output": node_result},
                    timestamp=datetime.utcnow().isoformat(),
                )

            if _should_yield("updates"):
                yield Event(
                    type="updates",
                    run_id=run_id,
                    node_id=current_node,
                    data={"updates": node_result if isinstance(node_result, dict) else {}},
                    timestamp=datetime.utcnow().isoformat(),
                )

            if _should_yield("values"):
                yield Event(
                    type="values",
                    run_id=run_id,
                    data={"state": state.copy()},
                    timestamp=datetime.utcnow().isoformat(),
                )

            next_node = None
            for edge in self._definition.edges:
                if edge.source == current_node:
                    if isinstance(edge.target, str):
                        next_node = edge.target
                    elif isinstance(edge.target, dict):
                        if isinstance(node_result, str) and node_result in edge.target:
                            next_node = edge.target[node_result]
                    break

            current_node = next_node

        yield Event(
            type="run_completed",
            run_id=run_id,
            data={"output": state},
            timestamp=datetime.utcnow().isoformat(),
        )

    async def _stream_llm_node(
        self,
        node_name: str,
        metadata: NodeMetadata,
        state: State,
        run_id: str,
        datetime_mod: Any,
    ) -> dict[str, Any]:
        """Stream tokens from an LLM node, collecting token events and final output."""
        from duragraph.llm import LLMRequest, get_provider

        config = metadata.config
        model = config.get("model", "gpt-4o-mini")
        temperature = config.get("temperature", 0.7)
        max_tokens = config.get("max_tokens")
        system_prompt = config.get("system_prompt")

        messages: list[dict[str, Any]] = []
        if "messages" in state:
            messages = state["messages"].copy()
        elif "input" in state:
            messages = [{"role": "user", "content": str(state["input"])}]
        else:
            messages = [{"role": "user", "content": str(state)}]

        provider = get_provider(model)
        request = LLMRequest(
            messages=messages,
            model=model,
            temperature=temperature,
            max_tokens=max_tokens,
            system_prompt=system_prompt,
        )

        collected_content = ""
        token_events: list[Event] = []

        try:
            async for chunk in provider.astream(request):
                if chunk.content:
                    collected_content += chunk.content
                    token_events.append(
                        Event(
                            type="token",
                            run_id=run_id,
                            node_id=node_name,
                            data={"token": chunk.content},
                            timestamp=datetime_mod.utcnow().isoformat(),
                        )
                    )
        except (AttributeError, NotImplementedError):
            response = await provider.acomplete(request)
            collected_content = response.content

        result: dict[str, Any] = {}
        if "messages" in state:
            messages.append({"role": "assistant", "content": collected_content})
            result["messages"] = messages
        result["response"] = collected_content

        async def _yield_tokens() -> AsyncIterator[Event]:
            for evt in token_events:
                yield evt

        return {"token_events": _yield_tokens(), "output": result}

    def serve(
        self,
        control_plane_url: str,
        *,
        worker_name: str | None = None,
        capabilities: list[str] | None = None,
        nats_url: str | None = None,
    ) -> None:
        """Register and serve this graph on the control plane.

        Args:
            control_plane_url: URL of the DuraGraph control plane.
            worker_name: Optional name for the worker.
            capabilities: Optional list of worker capabilities.
            nats_url: Optional NATS URL for JetStream task subscription.
        """
        from duragraph.worker import Worker

        worker = Worker(
            control_plane_url=control_plane_url,
            name=worker_name,
            capabilities=capabilities,
            nats_url=nats_url,
        )
        worker.register_graph(self._definition, instance=self._instance)
        worker.run()

    async def aserve(
        self,
        control_plane_url: str,
        *,
        worker_name: str | None = None,
        capabilities: list[str] | None = None,
        nats_url: str | None = None,
    ) -> None:
        """Async version of serve().

        Args:
            control_plane_url: URL of the DuraGraph control plane.
            worker_name: Optional name for the worker.
            capabilities: Optional list of worker capabilities.
            nats_url: Optional NATS URL for JetStream task subscription.
        """
        from duragraph.worker import Worker

        worker = Worker(
            control_plane_url=control_plane_url,
            name=worker_name,
            capabilities=capabilities,
            nats_url=nats_url,
        )
        worker.register_graph(self._definition, instance=self._instance)
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
        from duragraph.nodes import NodeDescriptor

        original_init = cls.__init__

        # Collect edges created at class definition time
        # Reset the class-level edge storage first to avoid accumulation
        class_edges = list(NodeDescriptor._all_edges)
        NodeDescriptor._all_edges.clear()

        def new_init(self: Any, *args: Any, **kwargs: Any) -> None:
            original_init(self, *args, **kwargs)
            self._graph_id = id
            self._graph_description = description
            self._graph_version = version
            self._edges: list[Edge] = []
            # Add edges defined at class level with >>
            for source, target in class_edges:
                self._edges.append(Edge(source, target))
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
                # Handle NodeDescriptor from class
                if hasattr(type(self), name):
                    class_attr = getattr(type(self), name)
                    if hasattr(class_attr, "metadata"):
                        # It's a NodeDescriptor
                        meta: NodeMetadata = class_attr.metadata
                        nodes[name] = meta
                        if meta.config.get("is_entrypoint"):
                            entrypoint = name
                # Fallback to old style (bound methods with _node_metadata)
                elif callable(attr) and hasattr(attr, "_node_metadata"):
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
            stream_mode: list[StreamMode] | None = None,
        ) -> AsyncIterator[Event]:
            """Stream graph execution events."""
            definition = self._get_definition()
            instance = GraphInstance(definition, self)
            async for event in instance.stream(
                input, config=config, thread_id=thread_id, stream_mode=stream_mode
            ):
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

        def as_subgraph(cls_self: type[Any], *, name: str | None = None) -> Any:
            """Return this graph as a subgraph node usable in a parent graph.

            Args:
                name: Optional node name. Defaults to the graph id.
            """
            from duragraph.subgraph import SubgraphNode

            return SubgraphNode.from_graph(cls_self, name=name)

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
