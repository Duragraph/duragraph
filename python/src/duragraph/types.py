"""Type definitions for DuraGraph SDK."""

from typing import Any, Literal, TypedDict, Union

from pydantic import BaseModel, Field

# State is a dictionary that flows through the graph
State = dict[str, Any]


class Message(BaseModel):
    """Base message type."""

    role: Literal["human", "assistant", "tool", "system"]
    content: str
    name: str | None = None
    metadata: dict[str, Any] = Field(default_factory=dict)


class HumanMessage(Message):
    """Message from a human user."""

    role: Literal["human"] = "human"


class AIMessage(Message):
    """Message from an AI assistant."""

    role: Literal["assistant"] = "assistant"
    tool_calls: list[dict[str, Any]] | None = None


class ToolMessage(Message):
    """Result from a tool call."""

    role: Literal["tool"] = "tool"
    tool_call_id: str


class SystemMessage(Message):
    """System message for LLM context."""

    role: Literal["system"] = "system"


# Union of all message types
AnyMessage = Union[HumanMessage, AIMessage, ToolMessage, SystemMessage]


class NodeConfig(TypedDict, total=False):
    """Configuration for a node."""

    model: str
    temperature: float
    max_tokens: int
    system_prompt: str
    tools: list[str]
    stream: bool
    retry_on: list[str]
    max_retries: int
    retry_delay: float


class GraphConfig(TypedDict, total=False):
    """Configuration for graph execution."""

    checkpoint_id: str
    stream_mode: list[Literal["values", "updates", "messages", "events"]]
    recursion_limit: int
    timeout: float


class RunResult(BaseModel):
    """Result of a graph execution."""

    run_id: str
    status: Literal["completed", "failed", "interrupted", "cancelled"]
    output: dict[str, Any]
    error: str | None = None
    nodes_executed: list[str] = Field(default_factory=list)
    tokens: dict[str, int] | None = None
    duration_ms: float | None = None


class Event(BaseModel):
    """Streaming event from graph execution."""

    type: Literal[
        "run_started",
        "run_completed",
        "run_failed",
        "node_started",
        "node_completed",
        "token",
        "checkpoint",
    ]
    run_id: str
    node_id: str | None = None
    data: dict[str, Any] = Field(default_factory=dict)
    timestamp: str
