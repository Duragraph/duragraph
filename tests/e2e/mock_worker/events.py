"""Event types and emission for mock worker.

Events follow the LangGraph streaming event format.
"""

from dataclasses import dataclass, field
from datetime import datetime, timezone
from enum import Enum
from typing import Any, Optional
import json


class EventType(str, Enum):
    """Event types emitted during execution."""

    # Run lifecycle
    RUN_STARTED = "run.started"
    RUN_COMPLETED = "run.completed"
    RUN_FAILED = "run.failed"
    RUN_INTERRUPTED = "run.interrupted"

    # Node lifecycle
    NODE_STARTED = "node.started"
    NODE_COMPLETED = "node.completed"
    NODE_FAILED = "node.failed"

    # LLM events
    LLM_START = "llm.start"
    LLM_END = "llm.end"
    LLM_STREAM = "llm.stream"

    # Tool events
    TOOL_START = "tool.start"
    TOOL_END = "tool.end"

    # Checkpoint events
    CHECKPOINT_CREATED = "checkpoint.created"

    # State events
    STATE_UPDATE = "state.update"


@dataclass
class Event:
    """An event emitted during execution."""

    type: EventType
    run_id: str
    timestamp: datetime = field(default_factory=lambda: datetime.now(timezone.utc))
    node_id: Optional[str] = None
    data: dict = field(default_factory=dict)

    def to_dict(self) -> dict:
        """Convert to dictionary for JSON serialization."""
        return {
            "event": self.type.value,
            "run_id": self.run_id,
            "timestamp": self.timestamp.isoformat(),
            "node_id": self.node_id,
            "data": self.data,
        }

    def to_sse(self) -> str:
        """Convert to Server-Sent Events format."""
        data = json.dumps(self.to_dict())
        return f"event: {self.type.value}\ndata: {data}\n\n"


class EventEmitter:
    """Collects and emits events during execution."""

    def __init__(self, run_id: str):
        self.run_id = run_id
        self.events: list[Event] = []
        self._listeners: list[callable] = []

    def add_listener(self, callback: callable):
        """Add a listener for real-time events."""
        self._listeners.append(callback)

    def emit(self, event: Event):
        """Emit an event."""
        self.events.append(event)
        for listener in self._listeners:
            try:
                listener(event)
            except Exception:
                pass  # Don't let listener errors break execution

    def run_started(self, input_data: dict):
        """Emit run.started event."""
        self.emit(Event(
            type=EventType.RUN_STARTED,
            run_id=self.run_id,
            data={"input": input_data},
        ))

    def run_completed(self, output: dict, duration_ms: int):
        """Emit run.completed event."""
        self.emit(Event(
            type=EventType.RUN_COMPLETED,
            run_id=self.run_id,
            data={
                "output": output,
                "duration_ms": duration_ms,
            },
        ))

    def run_failed(self, error: str, node_id: Optional[str] = None):
        """Emit run.failed event."""
        self.emit(Event(
            type=EventType.RUN_FAILED,
            run_id=self.run_id,
            node_id=node_id,
            data={"error": error},
        ))

    def run_interrupted(self, node_id: str, interrupt_data: dict):
        """Emit run.interrupted event."""
        self.emit(Event(
            type=EventType.RUN_INTERRUPTED,
            run_id=self.run_id,
            node_id=node_id,
            data=interrupt_data,
        ))

    def node_started(self, node_id: str, node_type: str, input_data: dict):
        """Emit node.started event."""
        self.emit(Event(
            type=EventType.NODE_STARTED,
            run_id=self.run_id,
            node_id=node_id,
            data={
                "node_type": node_type,
                "input": input_data,
            },
        ))

    def node_completed(self, node_id: str, output: dict, duration_ms: int):
        """Emit node.completed event."""
        self.emit(Event(
            type=EventType.NODE_COMPLETED,
            run_id=self.run_id,
            node_id=node_id,
            data={
                "output": output,
                "duration_ms": duration_ms,
            },
        ))

    def node_failed(self, node_id: str, error: str):
        """Emit node.failed event."""
        self.emit(Event(
            type=EventType.NODE_FAILED,
            run_id=self.run_id,
            node_id=node_id,
            data={"error": error},
        ))

    def llm_start(self, node_id: str, model: str, prompt: str):
        """Emit llm.start event."""
        self.emit(Event(
            type=EventType.LLM_START,
            run_id=self.run_id,
            node_id=node_id,
            data={
                "model": model,
                "prompt_preview": prompt[:200] + "..." if len(prompt) > 200 else prompt,
            },
        ))

    def llm_end(
        self,
        node_id: str,
        model: str,
        response: str,
        tokens_input: int,
        tokens_output: int,
        duration_ms: int,
    ):
        """Emit llm.end event."""
        self.emit(Event(
            type=EventType.LLM_END,
            run_id=self.run_id,
            node_id=node_id,
            data={
                "model": model,
                "response_preview": response[:200] + "..." if len(response) > 200 else response,
                "tokens": {
                    "input": tokens_input,
                    "output": tokens_output,
                    "total": tokens_input + tokens_output,
                },
                "duration_ms": duration_ms,
            },
        ))

    def tool_start(self, node_id: str, tool_name: str, arguments: dict):
        """Emit tool.start event."""
        self.emit(Event(
            type=EventType.TOOL_START,
            run_id=self.run_id,
            node_id=node_id,
            data={
                "tool": tool_name,
                "arguments": arguments,
            },
        ))

    def tool_end(self, node_id: str, tool_name: str, result: Any, duration_ms: int):
        """Emit tool.end event."""
        self.emit(Event(
            type=EventType.TOOL_END,
            run_id=self.run_id,
            node_id=node_id,
            data={
                "tool": tool_name,
                "result": result,
                "duration_ms": duration_ms,
            },
        ))

    def checkpoint_created(self, checkpoint_id: str, node_id: str, state: dict):
        """Emit checkpoint.created event."""
        self.emit(Event(
            type=EventType.CHECKPOINT_CREATED,
            run_id=self.run_id,
            node_id=node_id,
            data={
                "checkpoint_id": checkpoint_id,
                "state_keys": list(state.keys()),
            },
        ))

    def state_update(self, node_id: str, updates: dict):
        """Emit state.update event."""
        self.emit(Event(
            type=EventType.STATE_UPDATE,
            run_id=self.run_id,
            node_id=node_id,
            data={"updates": updates},
        ))

    def get_all_events(self) -> list[dict]:
        """Get all events as dictionaries."""
        return [e.to_dict() for e in self.events]

    def get_token_summary(self) -> dict:
        """Get summary of token usage from LLM events."""
        total_input = 0
        total_output = 0

        for event in self.events:
            if event.type == EventType.LLM_END:
                tokens = event.data.get("tokens", {})
                total_input += tokens.get("input", 0)
                total_output += tokens.get("output", 0)

        return {
            "input": total_input,
            "output": total_output,
            "total": total_input + total_output,
        }
