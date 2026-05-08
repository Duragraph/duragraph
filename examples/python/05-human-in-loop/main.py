"""
DuraGraph Human-in-the-Loop Example

Demonstrates:
- Using @human_node for approval workflows
- interrupt_before to pause execution for human review
- Resuming runs after human approval
- Combining LLM generation with human oversight
"""

import os

from duragraph import Graph, entrypoint, node, human_node


@Graph(id="content_approval")
class ContentApproval:
    """A workflow that generates content and requires human approval."""

    @entrypoint
    @node()
    async def generate_draft(self, state: dict) -> dict:
        """Generate a draft based on the user request."""
        topic = state.get("input", "general topic")
        state["draft"] = (
            f"Draft article about '{topic}':\n"
            f"1. Introduction to {topic}\n"
            f"2. Key benefits and considerations\n"
            f"3. Best practices and recommendations"
        )
        print(f"[generate_draft] Created draft for: {topic}")
        return state

    @human_node(
        prompt="Please review the generated draft. Approve to publish or reject to regenerate.",
        interrupt_before=True,
    )
    async def review_draft(self, state: dict) -> dict:
        """Human reviews the generated draft."""
        feedback = state.get("human_feedback", "")
        if feedback:
            state["draft"] += f"\n\n[Editor note: {feedback}]"
            print(f"[review_draft] Added feedback: {feedback}")
        else:
            print("[review_draft] Approved without changes")
        return state

    @node()
    async def publish(self, state: dict) -> dict:
        """Publish the approved content."""
        state["published"] = True
        state["response"] = f"Published: {state['draft']}"
        print("[publish] Content published!")
        return state

    generate_draft >> review_draft >> publish


def main():
    agent = ContentApproval()

    print("=== Human-in-the-Loop Example ===\n")

    # Local execution (human_node runs as a regular function node locally)
    result = agent.run({"input": "Event Sourcing in AI Systems"})
    print(f"\nStatus: {result.status}")
    print(f"Published: {result.output.get('published', False)}")
    print(f"Nodes: {result.nodes_executed}\n")

    # When served on the control plane, the human_node will pause
    # execution and set run status to "requires_action". The Studio
    # UI shows an ApprovalDialog for the reviewer to approve/reject.
    control_plane = os.getenv("DURAGRAPH_URL", "http://localhost:8081")
    print(f"=== Serving on {control_plane} ===")
    print("When a run reaches 'review_draft', it will pause for human review.")
    print("Use DuraGraph Studio to approve or reject the draft.")
    print("Press Ctrl+C to stop\n")
    agent.serve(control_plane, worker_name="hitl-worker")


if __name__ == "__main__":
    main()
