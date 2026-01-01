# DuraGraph Python SDK

[![PyPI version](https://badge.fury.io/py/duragraph.svg)](https://badge.fury.io/py/duragraph)
[![Python](https://img.shields.io/pypi/pyversions/duragraph.svg)](https://pypi.org/project/duragraph/)
[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](LICENSE)
[![CI](https://github.com/Duragraph/duragraph-python/actions/workflows/ci.yml/badge.svg)](https://github.com/Duragraph/duragraph-python/actions/workflows/ci.yml)

Python SDK for [DuraGraph](https://github.com/Duragraph/duragraph) - Reliable AI Workflow Orchestration.

Build AI agents with decorators, deploy to a control plane, and get full observability out of the box.

## Installation

```bash
pip install duragraph

# With OpenAI support
pip install duragraph[openai]

# With Anthropic support
pip install duragraph[anthropic]

# All features
pip install duragraph[all]
```

## Quick Start

```python
from duragraph import Graph, llm_node, entrypoint

@Graph(id="customer_support")
class CustomerSupportAgent:
    """A customer support agent that classifies and responds to queries."""

    @entrypoint
    @llm_node(model="gpt-4o-mini")
    def classify(self, state):
        """Classify the customer intent."""
        return {"intent": "billing"}

    @llm_node(model="gpt-4o-mini")
    def respond(self, state):
        """Generate a response based on intent."""
        return {"response": f"I'll help you with {state['intent']}."}

    # Define flow
    classify >> respond


# Run locally
agent = CustomerSupportAgent()
result = agent.run({"message": "I have a billing question"})
print(result)

# Or deploy to control plane
agent.serve("http://localhost:8081")
```

## Features

### Decorator-Based Graph Definition

```python
from duragraph import Graph, llm_node, tool_node, router_node, human_node

@Graph(id="my_agent")
class MyAgent:
    @llm_node(model="gpt-4o-mini", temperature=0.7)
    def process(self, state):
        return state

    @tool_node
    def search(self, state):
        results = my_search_function(state["query"])
        return {"results": results}

    @router_node
    def route(self, state):
        return "path_a" if state["condition"] else "path_b"

    @human_node(prompt="Please review")
    def review(self, state):
        return state
```

### Streaming

```python
async for event in agent.stream({"message": "Hello"}):
    if event.type == "token":
        print(event.data, end="")
    elif event.type == "node_complete":
        print(f"\nNode {event.node_id} completed")
```

### Subgraphs

```python
@Graph(id="research")
class ResearchAgent:
    @llm_node
    def research(self, state):
        return {"findings": "..."}

@Graph(id="main")
class MainAgent:
    research = ResearchAgent.as_subgraph()

    @entrypoint
    def plan(self, state):
        return state

    plan >> research
```

## Requirements

- Python 3.10+
- DuraGraph Control Plane (for deployment)

## Documentation

- [Full Documentation](https://duragraph.ai/docs)
- [API Reference](https://duragraph.ai/docs/api-reference/overview)
- [Examples](https://github.com/Duragraph/duragraph-examples)

## Related Repositories

| Repository | Description |
|------------|-------------|
| [duragraph](https://github.com/Duragraph/duragraph) | Core API server |
| [duragraph-go](https://github.com/Duragraph/duragraph-go) | Go SDK |
| [duragraph-examples](https://github.com/Duragraph/duragraph-examples) | Example projects |
| [duragraph-docs](https://github.com/Duragraph/duragraph-docs) | Documentation |

## Contributing

See [CONTRIBUTING.md](https://github.com/Duragraph/.github/blob/main/CONTRIBUTING.md) for guidelines.

## License

Apache 2.0 - See [LICENSE](LICENSE) for details.
