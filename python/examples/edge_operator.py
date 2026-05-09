"""Example demonstrating the >> edge operator for defining graph flows."""

from duragraph import Graph
from duragraph.nodes import entrypoint, llm_node, node, router_node


@Graph(id="customer_support_flow", description="Customer support workflow with edge operators")
class CustomerSupportFlow:
    """A customer support graph using >> operator to define the flow."""

    @entrypoint
    @llm_node(
        model="gpt-4o-mini",
        temperature=0.3,
        system_prompt="You are a customer intent classifier. Classify the customer's message into one of: billing, technical, general",
    )
    def classify_intent(self, state):
        """Classify customer intent from their message."""
        # In real implementation, this would use the LLM to classify
        # For demo, we'll use a simple rule
        message = state.get("message", "").lower()
        if "bill" in message or "payment" in message:
            state["intent"] = "billing"
        elif "error" in message or "bug" in message:
            state["intent"] = "technical"
        else:
            state["intent"] = "general"
        return state

    @router_node()
    def route_by_intent(self, state):
        """Route to appropriate handler based on intent."""
        intent = state.get("intent", "general")
        return f"handle_{intent}"

    @node()
    def handle_billing(self, state):
        """Handle billing inquiries."""
        state["response"] = "I'll help you with your billing question. Let me check your account..."
        state["department"] = "billing"
        return state

    @node()
    def handle_technical(self, state):
        """Handle technical support."""
        state["response"] = (
            "I understand you're having a technical issue. Let me help troubleshoot..."
        )
        state["department"] = "technical"
        return state

    @node()
    def handle_general(self, state):
        """Handle general inquiries."""
        state["response"] = "I'm here to help! What can I assist you with today?"
        state["department"] = "general"
        return state

    @llm_node(
        model="gpt-4o-mini",
        temperature=0.7,
        system_prompt="You are a helpful customer support agent. Provide a detailed response based on the department and initial classification.",
    )
    def generate_response(self, state):
        """Generate final response based on department handling."""
        # In real implementation, this would use the LLM
        department = state.get("department", "general")
        state["final_response"] = (
            f"[{department.upper()}] {state.get('response', 'How can I help you?')}"
        )
        return state

    @node()
    def log_interaction(self, state):
        """Log the customer interaction for analytics."""
        state["logged"] = True
        state["timestamp"] = "2024-01-01T12:00:00Z"  # Would use real timestamp
        print(f"Logged interaction: {state.get('intent')} -> {state.get('department')}")
        return state

    # Define the flow using >> operator
    classify_intent >> route_by_intent

    # Router connections (these would normally use conditional edges)
    # For demo purposes, we're showing the >> operator pattern
    handle_billing >> generate_response
    handle_technical >> generate_response
    handle_general >> generate_response

    generate_response >> log_interaction


@Graph(id="simple_pipeline", description="Simple data processing pipeline")
class SimpleDataPipeline:
    """A simple sequential data processing pipeline."""

    @entrypoint
    @node()
    def load_data(self, state):
        """Load initial data."""
        state["data"] = [1, 2, 3, 4, 5]
        state["steps_completed"] = ["load"]
        return state

    @node()
    def validate_data(self, state):
        """Validate the loaded data."""
        data = state.get("data", [])
        state["is_valid"] = len(data) > 0 and all(isinstance(x, (int, float)) for x in data)
        state["steps_completed"].append("validate")
        return state

    @node()
    def transform_data(self, state):
        """Transform the data."""
        if state.get("is_valid"):
            state["data"] = [x * 2 for x in state.get("data", [])]
        state["steps_completed"].append("transform")
        return state

    @node()
    def aggregate_data(self, state):
        """Aggregate the transformed data."""
        state["sum"] = sum(state.get("data", []))
        state["avg"] = state["sum"] / len(state.get("data", [1]))
        state["steps_completed"].append("aggregate")
        return state

    @node()
    def save_results(self, state):
        """Save the final results."""
        state["saved"] = True
        state["steps_completed"].append("save")
        print(f"Pipeline complete! Sum: {state['sum']}, Avg: {state['avg']}")
        return state

    # Define a linear pipeline using >> operator
    load_data >> validate_data >> transform_data >> aggregate_data >> save_results


def main():
    """Run the example graphs."""
    print("=" * 60)
    print("Customer Support Flow Example")
    print("=" * 60)

    # Create and run customer support flow
    support_flow = CustomerSupportFlow()

    # Test with billing question
    result = support_flow.run({"message": "I have a question about my bill"})
    print("\nBilling query result:")
    print(f"  Intent: {result.output.get('intent')}")
    print(f"  Department: {result.output.get('department')}")
    print(f"  Response: {result.output.get('final_response')}")

    # Test with technical issue
    result = support_flow.run({"message": "I'm getting an error when I try to login"})
    print("\nTechnical query result:")
    print(f"  Intent: {result.output.get('intent')}")
    print(f"  Department: {result.output.get('department')}")
    print(f"  Response: {result.output.get('final_response')}")

    print("\n" + "=" * 60)
    print("Simple Data Pipeline Example")
    print("=" * 60)

    # Create and run data pipeline
    pipeline = SimpleDataPipeline()
    result = pipeline.run({})

    print("\nPipeline result:")
    print("  Original data: [1, 2, 3, 4, 5]")
    print(f"  Transformed data: {result.output.get('data')}")
    print(f"  Sum: {result.output.get('sum')}")
    print(f"  Average: {result.output.get('avg')}")
    print(f"  Steps completed: {' -> '.join(result.output.get('steps_completed', []))}")
    print(f"  Nodes executed: {' -> '.join(result.nodes_executed)}")


if __name__ == "__main__":
    main()
