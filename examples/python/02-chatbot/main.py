"""
DuraGraph Chatbot with Memory Example

This example demonstrates:
- Conversation memory across multiple runs
- Using thread_id to track conversation context
- LLM integration with conversation history
- Simple in-memory conversation store
"""

import os
from collections import defaultdict
from typing import Any

from duragraph import Graph, node
from duragraph.worker import Worker


# Simple in-memory conversation store
# In production, use Redis, PostgreSQL, or another persistent store
class ConversationStore:
    """Stores conversation history by thread_id."""

    def __init__(self):
        self._store: dict[str, list[dict[str, str]]] = defaultdict(list)

    def get_messages(self, thread_id: str) -> list[dict[str, str]]:
        """Retrieve conversation history for a thread."""
        return self._store[thread_id].copy()

    def add_message(self, thread_id: str, role: str, content: str) -> None:
        """Add a message to conversation history."""
        self._store[thread_id].append({"role": role, "content": content})

    def clear(self, thread_id: str) -> None:
        """Clear conversation history for a thread."""
        if thread_id in self._store:
            del self._store[thread_id]


# Global store instance (shared across graph executions)
conversation_store = ConversationStore()


@Graph
class ChatbotWithMemory:
    """A chatbot that maintains conversation history using thread_id."""

    @node
    async def load_history(self, state: dict[str, Any]) -> dict[str, Any]:
        """Load conversation history from store."""
        thread_id = state.get("thread_id", "default")
        
        # Load existing messages for this thread
        state["messages"] = conversation_store.get_messages(thread_id)
        
        print(f"[load_history] Thread: {thread_id}, Messages: {len(state['messages'])}")
        return state

    @node
    async def add_user_message(self, state: dict[str, Any]) -> dict[str, Any]:
        """Add the user's new message to conversation."""
        user_input = state.get("input", "")
        thread_id = state.get("thread_id", "default")
        
        if not user_input:
            return state
        
        # Add user message to conversation
        state["messages"].append({
            "role": "user",
            "content": user_input
        })
        
        print(f"[add_user_message] User: {user_input}")
        return state

    @node
    async def generate_response(self, state: dict[str, Any]) -> dict[str, Any]:
        """Generate AI response based on conversation history."""
        messages = state.get("messages", [])
        
        # Simulate LLM response (in real implementation, call OpenAI/Anthropic)
        # Build context from conversation history
        context = "\n".join([
            f"{msg['role']}: {msg['content']}" 
            for msg in messages[-5:]  # Last 5 messages for context
        ])
        
        # Simple rule-based response for demo (replace with real LLM)
        user_message = messages[-1]["content"].lower() if messages else ""
        
        if "hello" in user_message or "hi" in user_message:
            response = "Hello! How can I help you today?"
        elif "how are you" in user_message:
            response = "I'm doing great, thank you for asking! How can I assist you?"
        elif "bye" in user_message or "goodbye" in user_message:
            response = "Goodbye! Feel free to come back anytime."
        elif "name" in user_message:
            response = "I'm DuraGraph Bot, your helpful assistant!"
        else:
            # Echo with context awareness
            msg_count = len(messages)
            response = f"I understand you said: '{messages[-1]['content']}'. " \
                      f"This is message #{msg_count} in our conversation. How can I help further?"
        
        state["response"] = response
        print(f"[generate_response] Assistant: {response}")
        return state

    @node
    async def save_response(self, state: dict[str, Any]) -> dict[str, Any]:
        """Save assistant response to conversation history."""
        thread_id = state.get("thread_id", "default")
        response = state.get("response", "")
        
        if not response:
            return state
        
        # Add assistant message to state
        state["messages"].append({
            "role": "assistant",
            "content": response
        })
        
        # Persist both user message and response
        conversation_store.add_message(thread_id, "user", state.get("input", ""))
        conversation_store.add_message(thread_id, "assistant", response)
        
        print(f"[save_response] Saved to thread: {thread_id}")
        return state


def main():
    # Get control plane URL from environment
    control_plane_url = os.getenv("DURAGRAPH_URL", "http://localhost:8081")

    print(f"Connecting to DuraGraph at {control_plane_url}")
    print("Starting Chatbot Worker with Conversation Memory")
    print("=" * 60)

    # Create worker
    worker = Worker(
        control_plane_url=control_plane_url,
        name="chatbot-worker",
    )

    # Register our chatbot graph
    worker.register_graph(ChatbotWithMemory)

    print("Worker registered. Waiting for runs...")
    print("Each thread_id maintains separate conversation history.")
    print("Press Ctrl+C to stop")
    print()

    # Run the worker (blocks until stopped)
    worker.run()


if __name__ == "__main__":
    main()
