"""Simple chatbot example using LLM providers.

This example demonstrates:
- Using @llm_node decorator with OpenAI
- Conversation state management
- Async execution

Requirements:
    uv add duragraph-python[openai]

Environment:
    export OPENAI_API_KEY=your-api-key-here

Usage:
    uv run python examples/chatbot_simple.py
"""

import asyncio
import os

from duragraph import Graph, entrypoint, llm_node


@Graph(id="simple_chatbot", description="A simple chatbot using GPT-4o-mini")
class SimpleChatbot:
    """A basic chatbot that responds to user messages."""

    @entrypoint
    @llm_node(
        model="gpt-4o-mini",
        temperature=0.7,
        system_prompt="You are a helpful and friendly assistant.",
    )
    def chat(self, state):
        """Process user input and generate response."""
        return state


async def main():
    """Run the chatbot example."""
    # Check for API key
    if not os.environ.get("OPENAI_API_KEY"):
        print("Error: OPENAI_API_KEY environment variable not set")
        print("Set it with: export OPENAI_API_KEY=your-key-here")
        return

    # Create chatbot instance
    bot = SimpleChatbot()

    # Example conversation
    print("=" * 60)
    print("Simple Chatbot Example")
    print("=" * 60)
    print()

    # First message
    print("User: Hello! What can you help me with?")
    result = await bot.arun(
        {
            "messages": [
                {"role": "user", "content": "Hello! What can you help me with?"}
            ]
        }
    )
    print(f"Assistant: {result.output['messages'][-1]['content']}")
    print()

    # Follow-up message
    messages = result.output["messages"]
    messages.append({"role": "user", "content": "Tell me a joke about programming"})

    print("User: Tell me a joke about programming")
    result = await bot.arun({"messages": messages})
    print(f"Assistant: {result.output['messages'][-1]['content']}")
    print()

    print("=" * 60)
    print(f"Conversation completed! Nodes executed: {result.nodes_executed}")
    print("=" * 60)


if __name__ == "__main__":
    asyncio.run(main())
