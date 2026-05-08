"""Minimal chatbot graph. Accepts both LangChain/Studio message shape
and a legacy {"input": "..."} shape.

Set OPENAI_API_KEY to use a real LLM; otherwise a rule-based responder
is used so the demo runs offline.
"""

import os

from duragraph import Graph, entrypoint, node


@Graph(id="chatbot", description="Minimal echo/LLM chatbot")
class Chatbot:
    @entrypoint
    @node()
    async def respond(self, state: dict) -> dict:
        # Studio sends {"messages": [{role, content}, ...]}; curl demos
        # often send {"input": "..."}. Support both.
        messages = state.get("messages") or []
        if messages:
            user_text = messages[-1].get("content", "")
        else:
            user_text = state.get("input", "")

        if not os.getenv("OPENAI_API_KEY"):
            state["response"] = f"You said: {user_text!r}. (Set OPENAI_API_KEY for an LLM reply.)"
            return state

        # Real LLM path — kept tiny on purpose. Extend with system
        # prompts, tools, etc. as needed.
        from openai import AsyncOpenAI

        client = AsyncOpenAI()
        resp = await client.chat.completions.create(
            model=os.getenv("OPENAI_MODEL", "gpt-4o-mini"),
            messages=[{"role": "user", "content": user_text}],
        )
        state["response"] = resp.choices[0].message.content or ""
        return state


if __name__ == "__main__":
    agent = Chatbot()
    control_plane = os.getenv("DURAGRAPH_URL", "http://localhost:8081")
    agent.serve(control_plane, worker_name="chatbot-worker")
