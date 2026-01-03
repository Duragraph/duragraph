"""Graph definitions for mock worker.

Each graph is a realistic simulation of agent behavior without actual LLM calls.
"""

from dataclasses import dataclass, field
from typing import Any, Callable, Optional
from enum import Enum


class NodeType(str, Enum):
    START = "start"
    END = "end"
    LLM = "llm"
    TOOL = "tool"
    ROUTER = "router"
    HUMAN = "human"


@dataclass
class Node:
    """A node in the graph."""

    id: str
    type: NodeType
    config: dict = field(default_factory=dict)


@dataclass
class Edge:
    """An edge connecting nodes."""

    source: str
    target: str
    condition: Optional[str] = None


@dataclass
class Graph:
    """A complete graph definition."""

    id: str
    name: str
    description: str
    nodes: list[Node]
    edges: list[Edge]
    entry_point: str = "__start__"


# =============================================================================
# GRAPH DEFINITIONS
# =============================================================================

SIMPLE_ECHO = Graph(
    id="simple_echo",
    name="Simple Echo",
    description="Single node that echoes input",
    nodes=[
        Node(id="__start__", type=NodeType.START),
        Node(
            id="echo",
            type=NodeType.LLM,
            config={
                "response_template": "Echo: {message}",
                "simulated_tokens": {"input": 10, "output": 15},
            },
        ),
        Node(id="__end__", type=NodeType.END),
    ],
    edges=[
        Edge(source="__start__", target="echo"),
        Edge(source="echo", target="__end__"),
    ],
    entry_point="echo",
)

MULTI_STEP = Graph(
    id="multi_step",
    name="Multi-Step Processing",
    description="Multiple sequential nodes with state passing",
    nodes=[
        Node(id="__start__", type=NodeType.START),
        Node(
            id="classify",
            type=NodeType.LLM,
            config={
                "response_template": "Classified intent: general_inquiry",
                "output_key": "intent",
                "output_value": "general_inquiry",
                "simulated_tokens": {"input": 50, "output": 20},
            },
        ),
        Node(
            id="process",
            type=NodeType.LLM,
            config={
                "response_template": "Processing intent: {intent}",
                "output_key": "processed",
                "output_value": True,
                "simulated_tokens": {"input": 80, "output": 40},
            },
        ),
        Node(
            id="respond",
            type=NodeType.LLM,
            config={
                "response_template": "Based on your inquiry, here is my response: {message}",
                "simulated_tokens": {"input": 120, "output": 100},
            },
        ),
        Node(id="__end__", type=NodeType.END),
    ],
    edges=[
        Edge(source="__start__", target="classify"),
        Edge(source="classify", target="process"),
        Edge(source="process", target="respond"),
        Edge(source="respond", target="__end__"),
    ],
    entry_point="classify",
)

BRANCHING = Graph(
    id="branching",
    name="Branching Router",
    description="Conditional routing based on input",
    nodes=[
        Node(id="__start__", type=NodeType.START),
        Node(
            id="router",
            type=NodeType.ROUTER,
            config={
                "route_key": "route",
                "routes": {
                    "a": "path_a",
                    "b": "path_b",
                },
                "default": "path_a",
            },
        ),
        Node(
            id="path_a",
            type=NodeType.LLM,
            config={
                "response_template": "Took path A for: {message}",
                "output_key": "path_taken",
                "output_value": "a",
                "simulated_tokens": {"input": 30, "output": 25},
            },
        ),
        Node(
            id="path_b",
            type=NodeType.LLM,
            config={
                "response_template": "Took path B for: {message}",
                "output_key": "path_taken",
                "output_value": "b",
                "simulated_tokens": {"input": 30, "output": 25},
            },
        ),
        Node(id="__end__", type=NodeType.END),
    ],
    edges=[
        Edge(source="__start__", target="router"),
        Edge(source="router", target="path_a", condition="route == 'a'"),
        Edge(source="router", target="path_b", condition="route == 'b'"),
        Edge(source="path_a", target="__end__"),
        Edge(source="path_b", target="__end__"),
    ],
    entry_point="router",
)

TOOL_CALLING = Graph(
    id="tool_calling",
    name="Tool Calling Agent",
    description="Agent that calls tools and uses results",
    nodes=[
        Node(id="__start__", type=NodeType.START),
        Node(
            id="agent_think",
            type=NodeType.LLM,
            config={
                "response_template": "I need to search for information about: {message}",
                "tool_calls": [
                    {
                        "name": "search",
                        "arguments": {"query": "{message}"},
                    }
                ],
                "simulated_tokens": {"input": 60, "output": 40},
            },
        ),
        Node(
            id="execute_tools",
            type=NodeType.TOOL,
            config={
                "tools": {
                    "search": {
                        "response": {
                            "results": [
                                "Result 1: Relevant information found",
                                "Result 2: Additional context",
                            ]
                        }
                    },
                    "calculator": {
                        "response": {"result": 42}
                    },
                }
            },
        ),
        Node(
            id="agent_respond",
            type=NodeType.LLM,
            config={
                "response_template": "Based on my search, I found: {tool_results}. Here's my answer to '{message}'.",
                "simulated_tokens": {"input": 150, "output": 80},
            },
        ),
        Node(id="__end__", type=NodeType.END),
    ],
    edges=[
        Edge(source="__start__", target="agent_think"),
        Edge(source="agent_think", target="execute_tools"),
        Edge(source="execute_tools", target="agent_respond"),
        Edge(source="agent_respond", target="__end__"),
    ],
    entry_point="agent_think",
)

HUMAN_INTERRUPT = Graph(
    id="human_interrupt",
    name="Human-in-the-Loop",
    description="Requires human approval before completing",
    nodes=[
        Node(id="__start__", type=NodeType.START),
        Node(
            id="draft",
            type=NodeType.LLM,
            config={
                "response_template": "Draft response: I've prepared an answer to '{message}'",
                "output_key": "draft",
                "output_value": "Prepared draft response",
                "simulated_tokens": {"input": 50, "output": 60},
            },
        ),
        Node(
            id="human_review",
            type=NodeType.HUMAN,
            config={
                "interrupt": True,
                "prompt": "Please review and approve this draft response",
                "required_fields": ["approved"],
            },
        ),
        Node(
            id="finalize",
            type=NodeType.LLM,
            config={
                "response_template": "Final approved response: {draft}",
                "simulated_tokens": {"input": 80, "output": 50},
            },
        ),
        Node(id="__end__", type=NodeType.END),
    ],
    edges=[
        Edge(source="__start__", target="draft"),
        Edge(source="draft", target="human_review"),
        Edge(source="human_review", target="finalize"),
        Edge(source="finalize", target="__end__"),
    ],
    entry_point="draft",
)

LONG_RUNNING = Graph(
    id="long_running",
    name="Long Running Process",
    description="Multiple iterations with delays for testing timeouts",
    nodes=[
        Node(id="__start__", type=NodeType.START),
        Node(
            id="init",
            type=NodeType.LLM,
            config={
                "response_template": "Initializing long process for: {message}",
                "output_key": "iteration",
                "output_value": 0,
                "simulated_tokens": {"input": 20, "output": 15},
            },
        ),
        Node(
            id="loop",
            type=NodeType.LLM,
            config={
                "response_template": "Processing iteration {iteration}",
                "loop_count": 5,
                "loop_delay_ms": 500,
                "simulated_tokens": {"input": 30, "output": 20},
            },
        ),
        Node(
            id="complete",
            type=NodeType.LLM,
            config={
                "response_template": "Completed all iterations. Final result for: {message}",
                "simulated_tokens": {"input": 40, "output": 30},
            },
        ),
        Node(id="__end__", type=NodeType.END),
    ],
    edges=[
        Edge(source="__start__", target="init"),
        Edge(source="init", target="loop"),
        Edge(source="loop", target="complete"),
        Edge(source="complete", target="__end__"),
    ],
    entry_point="init",
)

FAILURE = Graph(
    id="failure",
    name="Failure Test",
    description="Fails at a specific node for error testing",
    nodes=[
        Node(id="__start__", type=NodeType.START),
        Node(
            id="start_ok",
            type=NodeType.LLM,
            config={
                "response_template": "Starting process...",
                "simulated_tokens": {"input": 10, "output": 10},
            },
        ),
        Node(
            id="fail_here",
            type=NodeType.LLM,
            config={
                "should_fail": True,
                "error_message": "Simulated failure: LLM rate limit exceeded",
            },
        ),
        Node(
            id="never_reached",
            type=NodeType.LLM,
            config={
                "response_template": "This should never execute",
            },
        ),
        Node(id="__end__", type=NodeType.END),
    ],
    edges=[
        Edge(source="__start__", target="start_ok"),
        Edge(source="start_ok", target="fail_here"),
        Edge(source="fail_here", target="never_reached"),
        Edge(source="never_reached", target="__end__"),
    ],
    entry_point="start_ok",
)


# =============================================================================
# GRAPH REGISTRY
# =============================================================================

GRAPHS: dict[str, Graph] = {
    "simple_echo": SIMPLE_ECHO,
    "multi_step": MULTI_STEP,
    "branching": BRANCHING,
    "tool_calling": TOOL_CALLING,
    "human_interrupt": HUMAN_INTERRUPT,
    "long_running": LONG_RUNNING,
    "failure": FAILURE,
}


def get_graph(graph_id: str) -> Graph:
    """Get a graph by ID."""
    if graph_id not in GRAPHS:
        raise ValueError(f"Unknown graph: {graph_id}. Available: {list(GRAPHS.keys())}")
    return GRAPHS[graph_id]


def list_graphs() -> list[str]:
    """List all available graph IDs."""
    return list(GRAPHS.keys())
