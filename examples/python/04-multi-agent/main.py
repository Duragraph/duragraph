"""
DuraGraph Multi-Agent Example

Demonstrates:
- Multiple graphs coordinating via a supervisor pattern
- SubgraphNode for composing graphs as nodes in a parent graph
- State passing between parent and child graphs
- Supervisor routing to specialist agents
"""

import os

from duragraph import Graph, entrypoint, node, router_node
from duragraph.subgraph import SubgraphNode


@Graph(id="researcher")
class Researcher:
    """Specialist agent that researches a topic."""

    @entrypoint
    @node()
    async def research(self, state: dict) -> dict:
        query = state.get("query", "")
        print(f"  [researcher] Researching: {query}")
        state["research_results"] = f"Key findings about '{query}': fact1, fact2, fact3"
        return state


@Graph(id="writer")
class Writer:
    """Specialist agent that writes content from research."""

    @entrypoint
    @node()
    async def write(self, state: dict) -> dict:
        results = state.get("research_results", "")
        print(f"  [writer] Writing from: {results[:50]}...")
        state["draft"] = f"Article based on research: {results}"
        return state


@Graph(id="reviewer")
class Reviewer:
    """Specialist agent that reviews and improves content."""

    @entrypoint
    @node()
    async def review(self, state: dict) -> dict:
        draft = state.get("draft", "")
        print(f"  [reviewer] Reviewing: {draft[:50]}...")
        state["final_output"] = f"[REVIEWED] {draft}"
        state["review_complete"] = True
        return state


@Graph(id="multi_agent_supervisor")
class Supervisor:
    """Supervisor that coordinates specialist agents."""

    @entrypoint
    @node()
    async def plan(self, state: dict) -> dict:
        """Plan the workflow based on user input."""
        user_input = state.get("input", "")
        state["query"] = user_input
        state["step"] = "research"
        print(f"[supervisor] Planning workflow for: {user_input}")
        return state

    research = SubgraphNode.from_graph(Researcher, name="research")
    write = SubgraphNode.from_graph(Writer, name="write")
    review = SubgraphNode.from_graph(Reviewer, name="review")

    @node()
    async def summarize(self, state: dict) -> dict:
        """Produce final summary."""
        final = state.get("final_output", state.get("draft", "No output"))
        state["response"] = f"Final result:\n{final}"
        print(f"[supervisor] Done: {state['response'][:80]}...")
        return state

    plan >> research >> write >> review >> summarize


def main():
    agent = Supervisor()

    print("=== Multi-Agent Execution ===\n")
    result = agent.run({"input": "Benefits of event sourcing in AI systems"})
    print(f"\nStatus: {result.status}")
    print(f"Response: {result.output.get('response', 'N/A')}")
    print(f"Nodes executed: {result.nodes_executed}\n")

    control_plane = os.getenv("DURAGRAPH_URL", "http://localhost:8081")
    print(f"=== Serving on {control_plane} ===")
    print("Press Ctrl+C to stop\n")
    agent.serve(control_plane, worker_name="multi-agent-worker")


if __name__ == "__main__":
    main()
