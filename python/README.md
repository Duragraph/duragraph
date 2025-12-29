# DuraGraph Python SDK

[![PyPI version](https://badge.fury.io/py/duragraph-python.svg)](https://badge.fury.io/py/duragraph-python)
[![Python](https://img.shields.io/pypi/pyversions/duragraph-python.svg)](https://pypi.org/project/duragraph-python/)
[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](https://opensource.org/licenses/Apache-2.0)

Python SDK for [DuraGraph](https://github.com/duragraph/duragraph) - Reliable AI Workflow Orchestration.

Build AI agents with decorators, deploy to a control plane, and get full observability out of the box.

## Installation

```bash
pip install duragraph-python

# With OpenAI support
pip install duragraph-python[openai]

# With Anthropic support
pip install duragraph-python[anthropic]

# All features
pip install duragraph-python[all]
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

### Prompt Management

```python
from duragraph.prompts import prompt

@llm_node(model="gpt-4o-mini")
@prompt("support/classify_intent", version="2.1.0")
def classify(self, state):
    # Prompt is fetched from prompt store automatically
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

## CLI

```bash
# Initialize new project
duragraph init my-agent

# Run locally in development mode
duragraph dev

# Deploy to control plane
duragraph deploy --control-plane http://localhost:8081

# Visualize graph
duragraph visualize my_agent.py
```

## Configuration

```toml
# pyproject.toml
[tool.duragraph]
control_plane = "http://localhost:8081"

[tool.duragraph.llm]
default_model = "gpt-4o-mini"

[tool.duragraph.llm.providers.openai]
api_key = "${OPENAI_API_KEY}"
```

## Documentation

- [Full Documentation](https://docs.duragraph.io)
- [API Reference](https://docs.duragraph.io/api)
- [Examples](https://github.com/duragraph/duragraph-python/tree/main/examples)

## Requirements

- Python 3.10+
- DuraGraph Control Plane (for deployment)

## Contributing

Contributions are welcome! Please read our [Contributing Guide](CONTRIBUTING.md).

## License

Apache 2.0 - see [LICENSE](LICENSE) for details.
