"""
DuraGraph Chatbot — backed by OpenAI when OPENAI_API_KEY is set,
falls back to a rule-based responder otherwise.

Accepts both input shapes:
  - LangChain / Studio:  {"messages": [{"role": "user", "content": "..."}, ...]}
  - Legacy / curl demo:  {"thread_id": "alice", "input": "..."}
"""

import os
from collections import defaultdict
from typing import Any

from duragraph import Graph, node, entrypoint


# --- LLM client (lazy, optional) ---------------------------------------
# Prefer OpenRouter when its key is set (lets us use any provider through
# the OpenAI-compatible SDK). Fall back to OpenAI direct, then rule-based.
_llm_client: Any = None
_llm_model: str = "rule-based"
_llm_provider: str = "rule-based"

try:
    from openai import AsyncOpenAI

    if os.environ.get("OPENROUTER_API_KEY"):
        _llm_client = AsyncOpenAI(
            api_key=os.environ["OPENROUTER_API_KEY"],
            base_url="https://openrouter.ai/api/v1",
            default_headers={
                "HTTP-Referer": os.environ.get(
                    "OPENROUTER_REFERER", "https://duragraph.io"
                ),
                "X-Title": os.environ.get("OPENROUTER_TITLE", "DuraGraph Chatbot Demo"),
            },
        )
        _llm_model = os.environ.get("OPENROUTER_MODEL", "openai/gpt-4o-mini")
        _llm_provider = "openrouter"
    elif os.environ.get("OPENAI_API_KEY"):
        _llm_client = AsyncOpenAI()
        _llm_model = os.environ.get("OPENAI_MODEL", "gpt-4o-mini")
        _llm_provider = "openai"
except ImportError:
    pass


# --- Server-side conversation memory ----------------------------------
class ConversationStore:
    def __init__(self) -> None:
        self._store: dict[str, list[dict[str, str]]] = defaultdict(list)

    def get(self, thread_id: str) -> list[dict[str, str]]:
        return self._store[thread_id].copy()

    def append(self, thread_id: str, role: str, content: str) -> None:
        self._store[thread_id].append({"role": role, "content": content})


conversation_store = ConversationStore()


@Graph(id="chatbot_with_memory", description="LLM-backed chatbot with thread memory")
class ChatbotWithMemory:
    """Multi-turn chatbot. Uses OpenAI when configured, rules otherwise."""

    @entrypoint
    @node()
    async def load_history(self, state: dict[str, Any]) -> dict[str, Any]:
        """Resolve the message list from input + server-side store."""
        thread_id = state.get("thread_id", "default")

        # Studio / LangChain-style: messages already in input
        incoming = state.get("messages") or []
        if incoming:
            state["messages"] = list(incoming)
        else:
            # Legacy curl shape: load from store, append new user input
            user_input = state.get("input", "")
            state["messages"] = conversation_store.get(thread_id)
            if user_input:
                state["messages"].append(
                    {"role": "user", "content": user_input}
                )

        state["thread_id"] = thread_id
        return state

    @node()
    async def generate_response(self, state: dict[str, Any]) -> dict[str, Any]:
        """LLM call (OpenAI) or rule-based fallback."""
        messages: list[dict[str, str]] = state.get("messages", [])
        if not messages:
            state["response"] = "Hello! How can I help you?"
            state["model"] = "none"
            return state

        if _llm_client is not None:
            payload = [
                {
                    "role": "system",
                    "content": (
                        "You are a helpful assistant integrated with DuraGraph, "
                        "an AI workflow orchestration platform built on event "
                        "sourcing, CQRS, and worker dispatch via NATS JetStream. "
                        "Be concise."
                    ),
                }
            ] + [
                {"role": m.get("role", "user"), "content": m.get("content", "")}
                for m in messages
                if m.get("role") in ("user", "assistant", "system")
            ]
            try:
                resp = await _llm_client.chat.completions.create(
                    model=_llm_model,
                    messages=payload,
                    temperature=0.7,
                )
                state["response"] = resp.choices[0].message.content or ""
                state["model"] = _llm_model
                state["provider"] = _llm_provider
                if resp.usage:
                    state["usage"] = {
                        "prompt_tokens": resp.usage.prompt_tokens,
                        "completion_tokens": resp.usage.completion_tokens,
                        "total_tokens": resp.usage.total_tokens,
                    }
            except Exception as exc:  # noqa: BLE001
                state["response"] = f"[LLM error: {exc}]"
                state["model"] = "error"
            return state

        # Rule-based fallback (no OPENAI_API_KEY set)
        last = messages[-1].get("content", "").lower()
        if any(w in last for w in ("hello", "hi", "hey")):
            state["response"] = "Hello! How can I help you today?"
        elif "how are you" in last:
            state["response"] = "I'm doing great, thanks for asking!"
        elif any(w in last for w in ("bye", "goodbye")):
            state["response"] = "Goodbye! Come back anytime."
        else:
            state["response"] = (
                f"You said: '{messages[-1].get('content')}'. "
                "(Set OPENAI_API_KEY for a real LLM response.)"
            )
        state["model"] = "rule-based"
        return state

    @node()
    async def save_response(self, state: dict[str, Any]) -> dict[str, Any]:
        """Persist last user/assistant exchange and append assistant message."""
        thread_id = state.get("thread_id", "default")
        response = state.get("response", "")
        messages: list[dict[str, str]] = state.get("messages", [])

        if response:
            # Append assistant turn unless already there
            if not messages or messages[-1].get("role") != "assistant":
                messages.append({"role": "assistant", "content": response})
                state["messages"] = messages

            # Persist last user→assistant pair to server-side store
            for m in messages[-2:]:
                role = m.get("role")
                if role in ("user", "assistant"):
                    conversation_store.append(thread_id, role, m.get("content", ""))
        return state

    load_history >> generate_response >> save_response


def main() -> None:
    agent = ChatbotWithMemory()

    # Skip the interactive prompt — go straight to serving on the control plane.
    # (The previous interactive shell was kept for hand-driving; for a worker
    # process it just blocks on stdin which is fine to skip.)
    control_plane = os.getenv("DURAGRAPH_URL", "http://localhost:8081")
    backend = (
        f"{_llm_provider} ({_llm_model})" if _llm_client else "rule-based fallback"
    )
    print(f"Backend: {backend}")
    print(f"Serving on {control_plane}...")
    print("Press Ctrl+C to stop\n")
    agent.serve(control_plane, worker_name="chatbot-worker")


if __name__ == "__main__":
    main()
