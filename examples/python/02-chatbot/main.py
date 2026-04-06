"""
DuraGraph Chatbot with Memory Example

Demonstrates:
- Conversation memory using thread_id
- Multi-node processing pipeline with >> edges
- Serving as a worker on the control plane
"""

import os
from collections import defaultdict
from typing import Any

from duragraph import Graph, node, entrypoint


class ConversationStore:
    """In-memory conversation store keyed by thread_id."""

    def __init__(self) -> None:
        self._store: dict[str, list[dict[str, str]]] = defaultdict(list)

    def get_messages(self, thread_id: str) -> list[dict[str, str]]:
        return self._store[thread_id].copy()

    def add_message(self, thread_id: str, role: str, content: str) -> None:
        self._store[thread_id].append({"role": role, "content": content})


conversation_store = ConversationStore()


@Graph(id="chatbot_with_memory", description="A chatbot that remembers conversation history")
class ChatbotWithMemory:
    """Chatbot that maintains per-thread conversation history."""

    @entrypoint
    @node()
    async def load_history(self, state: dict[str, Any]) -> dict[str, Any]:
        """Load prior messages for this thread."""
        thread_id = state.get("thread_id", "default")
        state["messages"] = conversation_store.get_messages(thread_id)
        return state

    @node()
    async def add_user_message(self, state: dict[str, Any]) -> dict[str, Any]:
        """Append the user's new input to the message list."""
        user_input = state.get("input", "")
        if user_input:
            state.setdefault("messages", []).append(
                {"role": "user", "content": user_input}
            )
        return state

    @node()
    async def generate_response(self, state: dict[str, Any]) -> dict[str, Any]:
        """Generate a response (rule-based demo; replace with LLM in production)."""
        messages: list[dict[str, str]] = state.get("messages", [])
        if not messages:
            state["response"] = "Hello! How can I help you?"
            return state

        last = messages[-1].get("content", "").lower()
        if any(w in last for w in ("hello", "hi", "hey")):
            response = "Hello! How can I help you today?"
        elif "how are you" in last:
            response = "I'm doing great, thanks for asking!"
        elif any(w in last for w in ("bye", "goodbye")):
            response = "Goodbye! Come back anytime."
        else:
            count = len(messages)
            response = (
                f"You said: '{messages[-1]['content']}'. "
                f"This is message #{count} in our conversation."
            )

        state["response"] = response
        return state

    @node()
    async def save_response(self, state: dict[str, Any]) -> dict[str, Any]:
        """Persist both the user message and response to the store."""
        thread_id = state.get("thread_id", "default")
        response = state.get("response", "")
        user_input = state.get("input", "")

        if user_input:
            conversation_store.add_message(thread_id, "user", user_input)
        if response:
            conversation_store.add_message(thread_id, "assistant", response)
            state.setdefault("messages", []).append(
                {"role": "assistant", "content": response}
            )
        return state

    load_history >> add_user_message >> generate_response >> save_response


def main() -> None:
    agent = ChatbotWithMemory()

    # --- Local interactive mode ---
    print("DuraGraph Chatbot (type 'quit' to exit, 'serve' to connect to control plane)")
    print("=" * 60)

    thread_id = "local-demo"
    while True:
        user_input = input("You: ").strip()
        if not user_input:
            continue
        if user_input.lower() in ("quit", "exit"):
            print("Goodbye!")
            break
        if user_input.lower() == "serve":
            control_plane = os.getenv("DURAGRAPH_URL", "http://localhost:8081")
            print(f"\nServing on {control_plane}...")
            print("Press Ctrl+C to stop\n")
            agent.serve(control_plane, worker_name="chatbot-worker")
            return

        result = agent.run({"input": user_input, "thread_id": thread_id})
        print(f"Bot: {result.output.get('response', 'No response')}")


if __name__ == "__main__":
    main()
