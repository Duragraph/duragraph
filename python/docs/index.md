# DuraGraph Python SDK Documentation

Welcome to the DuraGraph Python SDK documentation. This guide covers all the features and capabilities of the SDK for building AI workflow orchestration systems.

## Table of Contents

1. [Getting Started](#getting-started)
2. [Installation](#installation)
3. [Quick Start](#quick-start)
4. [Core Concepts](#core-concepts)
5. [Graph Decorators](#graph-decorators)
6. [Node Types](#node-types)
7. [Edge Operators](#edge-operators)
8. [Tools System](#tools-system)
9. [Embedding Providers](#embedding-providers)
10. [Vector Stores](#vector-stores)
11. [Document Loaders](#document-loaders)
12. [CLI Reference](#cli-reference)
13. [Worker System](#worker-system)
14. [REST API Client](client-api.md)
15. [Examples](#examples)
16. [API Reference](#api-reference)

## Getting Started

DuraGraph is an enterprise-ready AI workflow orchestration platform designed for building reliable, observable, and scalable AI applications.

### Key Features

- **Event-sourced architecture** for complete audit trails
- **Human-in-the-loop support** for supervised workflows
- **LangGraph Cloud compatibility** for seamless migration
- **Production-grade reliability** with comprehensive error handling
- **Comprehensive observability** with built-in metrics and tracing

## Installation

### Basic Installation

```bash
pip install duragraph
```

### With Optional Dependencies

```bash
# OpenAI support
pip install duragraph[openai]

# Anthropic support
pip install duragraph[anthropic]

# All optional dependencies
pip install duragraph[all]

# Development dependencies
pip install duragraph[dev]
```

### Using uv (Recommended)

```bash
uv add duragraph
uv add "duragraph[all]"  # With all dependencies
```

## Quick Start

### Creating Your First Agent

```python
from duragraph import Graph, llm_node, entrypoint

@Graph(id="simple_agent")
class SimpleAgent:
    @entrypoint
    @llm_node(model="gpt-4o-mini")
    def process(self, state):
        return state

# Run the agent
agent = SimpleAgent()
result = agent.run({"messages": [{"role": "user", "content": "Hello!"}]})
print(result.output)
```

### Using the CLI

```bash
# Initialize a new project
duragraph init my-agent --template minimal

# Run in development mode with hot reload
duragraph dev src/agent.py

# Visualize the graph
duragraph visualize src/agent.py --format mermaid

# Deploy to control plane
duragraph deploy src/agent.py --control-plane http://localhost:8081
```

## Core Concepts

### Graphs

Graphs are the fundamental building blocks of DuraGraph workflows. They consist of:

- **Nodes**: Processing units that execute logic
- **Edges**: Connections between nodes defining execution flow
- **State**: Data passed through the graph during execution

### State Management

State is a dictionary that flows through the graph:

```python
# Input state
{"messages": [{"role": "user", "content": "Hello"}]}

# Nodes can modify and extend state
{"messages": [...], "intent": "greeting", "response": "Hi there!"}
```

### Execution Flow

1. Execution starts at the entrypoint node
2. Nodes process state and optionally modify it
3. Edges determine the next node to execute
4. Router nodes can conditionally branch execution
5. Execution completes when no more edges to follow

## Graph Decorators

### @Graph

Define a graph class:

```python
@Graph(
    id="my_graph",
    description="A helpful AI agent",
    version="1.0.0"
)
class MyGraph:
    # Node definitions...
```

### @entrypoint

Mark a node as the starting point:

```python
@entrypoint
@llm_node(model="gpt-4o-mini")
def start(self, state):
    return state
```

## Node Types

### @node()

Basic function node for custom logic:

```python
@node()
def process_data(self, state):
    state["processed"] = True
    return state
```

### @llm_node()

Node for LLM interactions:

```python
@llm_node(
    model="gpt-4o-mini",
    temperature=0.7,
    max_tokens=1000,
    system_prompt="You are a helpful assistant.",
    tools=[search_tool, calculate_tool]
)
def generate(self, state):
    return state
```

### @tool_node()

Node for tool/function execution:

```python
@tool_node(timeout=30.0, retry_on=["TimeoutError"], max_retries=3)
def search(self, state):
    results = search_api(state["query"])
    state["results"] = results
    return state
```

### @router_node()

Node for conditional branching:

```python
@router_node()
def route_by_intent(self, state):
    if state["intent"] == "billing":
        return "billing_handler"
    return "general_handler"
```

### @human_node()

Node for human-in-the-loop:

```python
@human_node(
    prompt="Please review this response",
    timeout=3600.0,
    interrupt_before=True
)
def human_review(self, state):
    return state
```

## Edge Operators

### Using >> Operator

Define edges using the `>>` operator in class body:

```python
@Graph(id="chat_agent")
class ChatAgent:
    @entrypoint
    @node()
    def start(self, state):
        return state
    
    @llm_node(model="gpt-4o-mini")
    def process(self, state):
        return state
    
    @node()
    def finish(self, state):
        return state
    
    # Define edges
    start >> process >> finish
```

### Conditional Edges

```python
router >> handler_a  # Direct edge
router >> handler_b  # Multiple edges from router

# Use router_node to return target node name
@router_node()
def route(self, state):
    if condition:
        return "handler_a"
    return "handler_b"
```

## Tools System

### Defining Tools

```python
from duragraph import tool

@tool(description="Search the web for information")
def web_search(query: str, max_results: int = 10) -> str:
    return search_api(query, max_results)

@tool(description="Calculate mathematical expressions")
def calculate(expression: str) -> float:
    return eval(expression)  # Use proper sanitization in production
```

### Using Tools in LLM Nodes

```python
@llm_node(model="gpt-4o-mini", tools=[web_search, calculate])
def process_with_tools(self, state):
    return state
```

### Tool Registry

```python
from duragraph import get_global_registry, resolve_tool_calls

# Get all registered tools
registry = get_global_registry()
tool_schemas = registry.get_tool_schemas()

# Execute tool calls from LLM response
results = resolve_tool_calls(tool_calls, registry)
```

## Embedding Providers

### OpenAI Embeddings

```python
from duragraph.embeddings import create_embedding_provider

# Create provider
provider = create_embedding_provider("openai", model="text-embedding-3-small")

# Embed documents
embeddings = await provider.aembed_documents(["Hello world", "How are you?"])

# Embed query
query_embedding = await provider.aembed_query("What is this about?")
```

### Anthropic Embeddings

```python
provider = create_embedding_provider("anthropic")
embeddings = await provider.aembed_documents(texts)
```

### Custom Providers

```python
from duragraph.embeddings import EmbeddingProvider, register_provider

class CustomEmbeddingProvider(EmbeddingProvider):
    async def aembed_documents(self, texts, **kwargs):
        # Your implementation
        pass
    
    async def aembed_query(self, text, **kwargs):
        # Your implementation
        pass
    
    @property
    def dimension(self):
        return 768

register_provider("custom", CustomEmbeddingProvider)
```

## Vector Stores

### In-Memory Store

```python
from duragraph.vectorstores import create_vectorstore, Document

# Create store
store = create_vectorstore("memory", embedding_function=embedding_provider)

# Add documents
documents = [
    Document(page_content="Hello world", metadata={"source": "test"}),
    Document(page_content="How are you?", metadata={"source": "test"}),
]
await store.aadd_documents(documents)

# Search
results = await store.asimilarity_search("greeting", k=5)
results_with_scores = await store.asimilarity_search_with_score("greeting", k=5)
```

### Filtered Search

```python
# Simple filter
results = await store.asimilarity_search(
    "query",
    k=5,
    filter={"source": "test"}
)

# Complex filter
results = await store.asimilarity_search(
    "query",
    k=5,
    filter={
        "category": {"$in": ["tech", "science"]},
        "score": {"$gte": 0.5}
    }
)
```

### ChromaDB Integration

```python
store = create_vectorstore(
    "chroma",
    embedding_function=embedding_provider,
    collection_name="my_collection",
    persist_directory="./chroma_db"
)
```

## Document Loaders

### Text File Loader

```python
from duragraph.document_loaders import TextFileLoader

loader = TextFileLoader("document.txt")
documents = loader.load()
```

### Directory Loader

```python
from duragraph.document_loaders import DirectoryLoader

loader = DirectoryLoader(
    path="./docs",
    glob="**/*.txt",
    recursive=True
)
documents = loader.load()
```

### CSV Loader

```python
from duragraph.document_loaders import CSVLoader

loader = CSVLoader(
    file_path="data.csv",
    content_columns=["title", "description"],
    metadata_columns=["category", "author"]
)
documents = loader.load()
```

### JSON Loader

```python
from duragraph.document_loaders import JSONLoader

loader = JSONLoader(
    file_path="data.json",
    content_key="content",
    metadata_keys=["title", "author"]
)
documents = loader.load()
```

### Text Splitters

```python
from duragraph.document_loaders import (
    CharacterTextSplitter,
    RecursiveCharacterTextSplitter,
    MarkdownTextSplitter,
    CodeTextSplitter
)

# Character-based splitting
splitter = CharacterTextSplitter(chunk_size=1000, chunk_overlap=100)
chunks = splitter.split_documents(documents)

# Recursive splitting (recommended)
splitter = RecursiveCharacterTextSplitter(
    chunk_size=500,
    chunk_overlap=50,
    separators=["\n\n", "\n", " ", ""]
)
chunks = splitter.split_documents(documents)

# Markdown-aware splitting
splitter = MarkdownTextSplitter(chunk_size=500, chunk_overlap=50)

# Code-aware splitting
splitter = CodeTextSplitter(language="python", chunk_size=500, chunk_overlap=50)
```

## CLI Reference

### duragraph init

Create a new DuraGraph project:

```bash
duragraph init <project-name> --template <template>
```

Templates:
- `minimal`: Simple single-node agent
- `chatbot`: Conversational chatbot with flow
- `tools`: Agent with tool capabilities
- `full`: Complete example with routing

### duragraph dev

Run in development mode with hot reload:

```bash
duragraph dev <file> --port 8000 --control-plane http://localhost:8081
```

### duragraph deploy

Deploy to a control plane:

```bash
duragraph deploy <file> --control-plane <url> --worker-name <name> --capabilities openai tools
```

### duragraph visualize

Generate graph visualizations:

```bash
duragraph visualize <file> --format mermaid --output graph.md
duragraph visualize <file> --format dot --output graph.dot
duragraph visualize <file> --format json --output graph.json
```

## Worker System

### Basic Worker

```python
from duragraph.worker import Worker

worker = Worker(
    control_plane_url="http://localhost:8081",
    name="my-worker",
    capabilities=["openai", "tools"],
    poll_interval=1.0,
    heartbeat_interval=30.0,
    max_concurrent_runs=10,
    shutdown_timeout=60.0
)

# Register graphs
worker.register_graph(graph_definition)

# Run worker
worker.run()  # Blocking
# or
await worker.arun()  # Async
```

### Graceful Shutdown

Workers handle graceful shutdown automatically:
- Stop accepting new work
- Wait for active runs to complete
- Timeout-based forced shutdown if needed

### Health Metrics

Workers track and report:
- Active runs count
- Completed runs count
- Failed runs count
- Uptime
- Registration attempts

## Examples

See the `examples/` directory for complete examples:

- `chatbot_simple.py`: Basic chatbot
- `chatbot_anthropic.py`: Chatbot with Anthropic
- `tool_usage.py`: Tool integration
- `edge_operator.py`: Edge definition patterns
- `embedding_usage.py`: Embedding providers
- `vectorstore_usage.py`: Vector store operations
- `document_processing.py`: Document loaders and splitters
- `rag_agent.py`: Complete RAG system
- `async_execution.py`: Async patterns
- `worker_lifecycle.py`: Worker management

## API Reference

### Types

```python
from duragraph.types import (
    State,           # Dict[str, Any]
    Message,         # Base message type
    HumanMessage,    # User message
    AIMessage,       # Assistant message
    ToolMessage,     # Tool result
    RunResult,       # Execution result
    Event,           # Streaming event
    GraphConfig,     # Configuration
)
```

### Graph Instance Methods

```python
graph = MyGraph()

# Synchronous execution
result = graph.run(state)
result = graph.run(state, config=config, thread_id=thread_id)

# Async execution
result = await graph.arun(state)

# Streaming execution
async for event in graph.stream(state):
    print(event.type, event.data)

# Serve as worker
graph.serve(control_plane_url)
await graph.aserve(control_plane_url)
```

### RunResult

```python
result = graph.run(state)

result.run_id          # Execution ID
result.status          # "completed", "failed", etc.
result.output          # Final state
result.nodes_executed  # List of executed nodes
```

## Best Practices

### Error Handling

```python
@node(retry_on=["NetworkError", "TimeoutError"], max_retries=3, retry_delay=1.0)
def resilient_node(self, state):
    # Will retry on specified errors
    return state
```

### State Management

```python
# Good: Return modified state
@node()
def process(self, state):
    state["processed"] = True
    return state

# Bad: Modifying state in place without return
@node()
def process(self, state):
    state["processed"] = True  # May not persist
```

### Async Patterns

```python
# Nodes can be async
@node()
async def async_process(self, state):
    await some_async_operation()
    return state

# Use arun for async execution
result = await graph.arun(state)
```

### Security

- Never log sensitive data (API keys, credentials)
- Validate user input before processing
- Use environment variables for secrets
- Implement proper authentication for workers

## Troubleshooting

### Common Issues

1. **Import errors**: Ensure optional dependencies are installed
2. **Connection errors**: Check control plane URL and network
3. **Type errors**: Ensure state is a dictionary
4. **Tool errors**: Verify tool function signatures match schema

### Debug Mode

```bash
duragraph --debug dev src/agent.py
```

### Logging

```python
import logging
logging.basicConfig(level=logging.DEBUG)
```

## Contributing

See [CONTRIBUTING.md](../CONTRIBUTING.md) for development guidelines.

## License

Apache 2.0 - See [LICENSE](../LICENSE) for details.