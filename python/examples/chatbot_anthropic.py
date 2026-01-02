"""Chatbot example using Anthropic Claude.

This example demonstrates:
- Using @llm_node decorator with Anthropic
- Multi-turn conversations
- Async execution

Requirements:
    uv add duragraph-python[anthropic]

Environment:
    export ANTHROPIC_API_KEY=your-api-key-here

Usage:
    uv run python examples/chatbot_anthropic.py
"""

import asyncio
import os

from duragraph import Graph, entrypoint, llm_node


@Graph(id="claude_chatbot", description="A chatbot using Claude 3.5 Sonnet")
class ClaudeChatbot:
    """A chatbot powered by Anthropic Claude."""

    @entrypoint
    @llm_node(
        model="claude-3-5-sonnet-20241022",
        temperature=0.7,
        system_prompt="You are Claude, a helpful AI assistant created by Anthropic.",
    )
    def chat(self, state):
        """Process user input using Claude."""
        return state


async def main():
    """Run the Claude chatbot example."""
    # Check for API key
    if not os.environ.get("ANTHROPIC_API_KEY"):
        print("Error: ANTHROPIC_API_KEY environment variable not set")
        print("Set it with: export ANTHROPIC_API_KEY=your-key-here")
        return

    # Create chatbot instance
    bot = ClaudeChatbot()

    print("=" * 60)
    print("Claude Chatbot Example")
    print("=" * 60)
    print()

    # Start conversation
    print("User: Hi! I'm learning about AI agents. Can you explain what they are?")
    result = await bot.arun(
        {
            "messages": [
                {
                    "role": "user",
                    "content": "Hi! I'm learning about AI agents. Can you explain what they are?",
                }
            ]
        }
    )
    print(f"Claude: {result.output['messages'][-1]['content']}")
    print()

    # Follow-up
    messages = result.output["messages"]
    messages.append(
        {
            "role": "user",
            "content": "How do they differ from regular LLM applications?",
        }
    )

    print("User: How do they differ from regular LLM applications?")
    result = await bot.arun({"messages": messages})
    print(f"Claude: {result.output['messages'][-1]['content']}")
    print()

    print("=" * 60)
    print(f"Conversation completed! Run ID: {result.run_id}")
    print("=" * 60)


if __name__ == "__main__":
    asyncio.run(main())
