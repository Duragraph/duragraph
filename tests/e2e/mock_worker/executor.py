"""Graph executor for mock worker.

Executes graphs by traversing nodes and simulating behavior.
"""

import asyncio
import time
import uuid
from dataclasses import dataclass, field
from typing import Any, Optional

from .config import config
from .events import EventEmitter
from .graphs import Graph, Node, NodeType, get_graph


class ExecutionError(Exception):
    """Error during graph execution."""

    def __init__(self, message: str, node_id: Optional[str] = None):
        super().__init__(message)
        self.node_id = node_id


class InterruptError(Exception):
    """Execution interrupted for human input."""

    def __init__(self, node_id: str, prompt: str, required_fields: list[str]):
        super().__init__(f"Interrupted at {node_id}")
        self.node_id = node_id
        self.prompt = prompt
        self.required_fields = required_fields


@dataclass
class ExecutionState:
    """State during graph execution."""

    values: dict = field(default_factory=dict)
    messages: list[dict] = field(default_factory=list)
    current_node: Optional[str] = None
    nodes_executed: list[str] = field(default_factory=list)
    checkpoints: list[dict] = field(default_factory=list)

    def update(self, key: str, value: Any):
        """Update a state value."""
        self.values[key] = value

    def get(self, key: str, default: Any = None) -> Any:
        """Get a state value."""
        return self.values.get(key, default)

    def to_dict(self) -> dict:
        """Convert to dictionary."""
        return {
            "values": self.values,
            "messages": self.messages,
            "current_node": self.current_node,
            "nodes_executed": self.nodes_executed,
        }

    def create_checkpoint(self, node_id: str) -> dict:
        """Create a checkpoint after a node."""
        checkpoint = {
            "checkpoint_id": str(uuid.uuid4()),
            "node_id": node_id,
            "values": self.values.copy(),
            "messages": self.messages.copy(),
            "nodes_executed": self.nodes_executed.copy(),
        }
        self.checkpoints.append(checkpoint)
        return checkpoint


class GraphExecutor:
    """Executes a graph with simulated behavior."""

    def __init__(
        self,
        graph: Graph,
        emitter: EventEmitter,
        delay_ms: int = 100,
        fail_at_node: Optional[str] = None,
        interrupt_at_node: Optional[str] = None,
        token_count: int = 100,
    ):
        self.graph = graph
        self.emitter = emitter
        self.delay_ms = delay_ms
        self.fail_at_node = fail_at_node
        self.interrupt_at_node = interrupt_at_node
        self.token_count = token_count

        # Build node lookup
        self._nodes = {n.id: n for n in graph.nodes}
        # Build adjacency list
        self._edges = {}
        for edge in graph.edges:
            if edge.source not in self._edges:
                self._edges[edge.source] = []
            self._edges[edge.source].append(edge)

    async def execute(
        self,
        input_data: dict,
        initial_state: Optional[dict] = None,
        resume_from: Optional[str] = None,
    ) -> ExecutionState:
        """Execute the graph from start to end.

        Args:
            input_data: Input data from the run request
            initial_state: State to restore from checkpoint
            resume_from: Node ID to resume from after interrupt

        Returns:
            Final execution state
        """
        # Initialize state
        state = ExecutionState()
        if initial_state:
            state.values = initial_state.get("values", {})
            state.messages = initial_state.get("messages", [])
            state.nodes_executed = initial_state.get("nodes_executed", [])

        # Merge input into state
        state.values.update(input_data)

        # Determine starting node
        if resume_from:
            current_node_id = resume_from
        else:
            current_node_id = self.graph.entry_point

        # Execute nodes until we reach __end__
        while current_node_id and current_node_id != "__end__":
            node = self._nodes.get(current_node_id)
            if not node:
                raise ExecutionError(f"Node not found: {current_node_id}")

            state.current_node = current_node_id

            # Check for forced failure
            if self.fail_at_node == current_node_id:
                node.config["should_fail"] = True

            # Check for forced interrupt
            if self.interrupt_at_node == current_node_id:
                node.type = NodeType.HUMAN
                node.config["interrupt"] = True

            # Execute the node
            try:
                await self._execute_node(node, state)
            except InterruptError:
                raise  # Re-raise interrupts
            except ExecutionError:
                raise  # Re-raise execution errors
            except Exception as e:
                raise ExecutionError(str(e), current_node_id)

            state.nodes_executed.append(current_node_id)

            # Create checkpoint after node
            checkpoint = state.create_checkpoint(current_node_id)
            self.emitter.checkpoint_created(
                checkpoint["checkpoint_id"],
                current_node_id,
                state.values,
            )

            # Determine next node
            current_node_id = self._get_next_node(current_node_id, state)

            # Delay between nodes (simulates processing time)
            if self.delay_ms > 0 and current_node_id and current_node_id != "__end__":
                await asyncio.sleep(self.delay_ms / 1000)

        state.current_node = None
        return state

    async def _execute_node(self, node: Node, state: ExecutionState):
        """Execute a single node."""
        start_time = time.time()

        # Emit node started
        self.emitter.node_started(
            node.id,
            node.type.value,
            {"state_keys": list(state.values.keys())},
        )

        try:
            if node.type == NodeType.START:
                pass  # No-op

            elif node.type == NodeType.END:
                pass  # No-op

            elif node.type == NodeType.LLM:
                await self._execute_llm_node(node, state)

            elif node.type == NodeType.TOOL:
                await self._execute_tool_node(node, state)

            elif node.type == NodeType.ROUTER:
                pass  # Routing is handled in _get_next_node

            elif node.type == NodeType.HUMAN:
                await self._execute_human_node(node, state)

            else:
                raise ExecutionError(f"Unknown node type: {node.type}", node.id)

            # Emit node completed
            duration_ms = int((time.time() - start_time) * 1000)
            self.emitter.node_completed(
                node.id,
                {"state_keys": list(state.values.keys())},
                duration_ms,
            )

        except InterruptError:
            raise
        except Exception as e:
            self.emitter.node_failed(node.id, str(e))
            raise ExecutionError(str(e), node.id)

    async def _execute_llm_node(self, node: Node, state: ExecutionState):
        """Execute an LLM node (simulated)."""
        config = node.config

        # Check for failure
        if config.get("should_fail"):
            error_msg = config.get("error_message", "Simulated LLM failure")
            raise ExecutionError(error_msg, node.id)

        # Simulate LLM call
        model = config.get("model", "gpt-4-mock")
        template = config.get("response_template", "Mock response")

        # Format template with state values
        response = template
        for key, value in state.values.items():
            response = response.replace(f"{{{key}}}", str(value))

        # Get token counts
        tokens = config.get("simulated_tokens", {})
        tokens_input = tokens.get("input", self.token_count)
        tokens_output = tokens.get("output", self.token_count // 2)

        # Emit LLM events
        self.emitter.llm_start(node.id, model, template)

        # Simulate processing time
        await asyncio.sleep(0.05)  # 50ms minimum for LLM call

        self.emitter.llm_end(
            node.id,
            model,
            response,
            tokens_input,
            tokens_output,
            50,  # Duration
        )

        # Update state with output
        if "output_key" in config:
            state.update(config["output_key"], config.get("output_value", response))

        # Store response
        state.update("last_response", response)
        state.messages.append({
            "role": "assistant",
            "content": response,
            "node_id": node.id,
        })

        # Handle tool calls
        if "tool_calls" in config:
            state.update("pending_tool_calls", config["tool_calls"])

        # Handle loops
        if "loop_count" in config:
            iteration = state.get("_loop_iteration", 0)
            if iteration < config["loop_count"] - 1:
                state.update("_loop_iteration", iteration + 1)
                # Loop delay
                delay = config.get("loop_delay_ms", 500)
                await asyncio.sleep(delay / 1000)

    async def _execute_tool_node(self, node: Node, state: ExecutionState):
        """Execute a tool node (simulated)."""
        config = node.config
        tools = config.get("tools", {})
        pending_calls = state.get("pending_tool_calls", [])

        tool_results = []

        for call in pending_calls:
            tool_name = call.get("name")
            arguments = call.get("arguments", {})

            # Format arguments
            formatted_args = {}
            for key, value in arguments.items():
                if isinstance(value, str):
                    for state_key, state_value in state.values.items():
                        value = value.replace(f"{{{state_key}}}", str(state_value))
                formatted_args[key] = value

            self.emitter.tool_start(node.id, tool_name, formatted_args)

            # Get tool response
            tool_config = tools.get(tool_name, {})
            result = tool_config.get("response", {"result": "mock_result"})

            # Simulate tool execution time
            await asyncio.sleep(0.02)  # 20ms for tool call

            self.emitter.tool_end(node.id, tool_name, result, 20)

            tool_results.append({
                "tool": tool_name,
                "result": result,
            })

        # Store results in state
        state.update("tool_results", tool_results)
        state.update("pending_tool_calls", [])

    async def _execute_human_node(self, node: Node, state: ExecutionState):
        """Execute a human interrupt node."""
        config = node.config

        if config.get("interrupt", False):
            # Check if we have approval in state (from resume)
            if state.get("_human_approved"):
                state.update("_human_approved", False)
                return

            # Raise interrupt
            raise InterruptError(
                node_id=node.id,
                prompt=config.get("prompt", "Human review required"),
                required_fields=config.get("required_fields", []),
            )

    def _get_next_node(self, current_node_id: str, state: ExecutionState) -> Optional[str]:
        """Determine the next node based on edges and conditions."""
        edges = self._edges.get(current_node_id, [])

        if not edges:
            return "__end__"

        # For router nodes, evaluate conditions
        current_node = self._nodes.get(current_node_id)
        if current_node and current_node.type == NodeType.ROUTER:
            config = current_node.config
            route_key = config.get("route_key", "route")
            route_value = state.get(route_key)
            routes = config.get("routes", {})

            if route_value in routes:
                return routes[route_value]
            return config.get("default", edges[0].target)

        # For conditional edges, evaluate
        for edge in edges:
            if edge.condition:
                # Simple condition evaluation
                # Format: "key == 'value'" or "key == value"
                if self._evaluate_condition(edge.condition, state):
                    return edge.target
            else:
                # Default edge (no condition)
                return edge.target

        return edges[0].target if edges else "__end__"

    def _evaluate_condition(self, condition: str, state: ExecutionState) -> bool:
        """Evaluate a simple condition against state."""
        # Parse simple conditions like "route == 'a'"
        if "==" in condition:
            parts = condition.split("==")
            if len(parts) == 2:
                key = parts[0].strip()
                expected = parts[1].strip().strip("'\"")
                actual = state.get(key)
                return str(actual) == expected
        return False


async def execute_run(
    run_id: str,
    graph_id: str,
    input_data: dict,
    initial_state: Optional[dict] = None,
    resume_from: Optional[str] = None,
) -> tuple[ExecutionState, EventEmitter]:
    """Execute a run with the specified graph.

    Args:
        run_id: Unique run identifier
        graph_id: Graph to execute
        input_data: Input data from run request
        initial_state: State to restore (for resume)
        resume_from: Node to resume from

    Returns:
        Tuple of (final_state, event_emitter)
    """
    # Check for per-run config override
    mock_config = input_data.pop("_mock_config", None)

    graph = get_graph(mock_config.get("graph", graph_id) if mock_config else graph_id)
    emitter = EventEmitter(run_id)

    # Apply config overrides
    delay_ms = mock_config.get("delay_ms", config.mock_delay_ms) if mock_config else config.mock_delay_ms
    fail_at = mock_config.get("fail_at", config.mock_fail_at_node) if mock_config else config.mock_fail_at_node
    interrupt_at = mock_config.get("interrupt_at", config.mock_interrupt_at_node) if mock_config else config.mock_interrupt_at_node
    token_count = mock_config.get("simulate_tokens", config.mock_token_count) if mock_config else config.mock_token_count

    executor = GraphExecutor(
        graph=graph,
        emitter=emitter,
        delay_ms=delay_ms,
        fail_at_node=fail_at,
        interrupt_at_node=interrupt_at,
        token_count=token_count,
    )

    # Emit run started
    emitter.run_started(input_data)

    start_time = time.time()

    try:
        state = await executor.execute(
            input_data,
            initial_state=initial_state,
            resume_from=resume_from,
        )

        duration_ms = int((time.time() - start_time) * 1000)
        emitter.run_completed(state.to_dict(), duration_ms)

        return state, emitter

    except InterruptError as e:
        emitter.run_interrupted(e.node_id, {
            "prompt": e.prompt,
            "required_fields": e.required_fields,
        })
        raise

    except ExecutionError as e:
        emitter.run_failed(str(e), e.node_id)
        raise

    except Exception as e:
        emitter.run_failed(str(e))
        raise ExecutionError(str(e))
