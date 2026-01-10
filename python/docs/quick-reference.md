# DuraGraph Python SDK Quick Reference

## Installation

```bash
pip install duragraph
pip install duragraph[all]  # All optional dependencies
```

## Core Imports

```python
from duragraph import (
    Graph, entrypoint, node, llm_node, tool_node, router_node, human_node,
    edge, tool, get_global_registry, resolve_tool_calls,
    State, Message, HumanMessage, AIMessage, ToolMessage,
    create_embedding_provider, get_embedding_provider,
    create_vectorstore, get_vectorstore,
    TextFileLoader, DirectoryLoader, CharacterTextSplitter, RecursiveCharacterTextSplitter,
)
```

## Graph Definition

```python
@Graph(id="my_agent", description="Description", version="1.0.0")
class MyAgent:
    @entrypoint
    @llm_node(model="gpt-4o-mini")
    def start(self, state):
        return state
    
    @node()
    def process(self, state):
        return {"processed": True}
    
    start >> process  # Define edge
```

## Node Types

| Decorator | Purpose | Key Parameters |
|-----------|---------|----------------|
| `@node()` | Custom logic | `name`, `retry_on`, `max_retries`, `retry_delay` |
| `@llm_node()` | LLM calls | `model`, `temperature`, `max_tokens`, `system_prompt`, `tools`, `stream` |
| `@tool_node()` | Tool execution | `name`, `timeout`, `retry_on`, `max_retries` |
| `@router_node()` | Conditional routing | `name` |
| `@human_node()` | Human review | `prompt`, `timeout`, `interrupt_before` |

## Tools

```python
@tool(description="Search for information")
def search(query: str, max_results: int = 10) -> str:
    return "results"

@llm_node(model="gpt-4o-mini", tools=[search])
def process(self, state):
    return state
```

## Embeddings

```python
from duragraph.embeddings import create_embedding_provider

provider = create_embedding_provider("openai", model="text-embedding-3-small")
embeddings = await provider.aembed_documents(["text1", "text2"])
embedding = await provider.aembed_query("query text")
```

Providers: `openai`, `anthropic`

## Vector Stores

```python
from duragraph.vectorstores import create_vectorstore, Document

store = create_vectorstore("memory", embedding_function=provider)
store = create_vectorstore("chroma", embedding_function=provider, collection_name="my_collection")

# Add documents
docs = [Document(page_content="content", metadata={"key": "value"})]
await store.aadd_documents(docs)

# Search
results = await store.asimilarity_search("query", k=5)
results = await store.asimilarity_search("query", k=5, filter={"key": "value"})

# Delete
await store.adelete(ids=["id1", "id2"])
await store.adelete(filter={"key": "value"})
```

Stores: `memory`, `chroma`

## Document Loaders

```python
from duragraph.document_loaders import (
    TextFileLoader, DirectoryLoader, CSVLoader, JSONLoader,
    CharacterTextSplitter, RecursiveCharacterTextSplitter
)

# Load files
loader = TextFileLoader("file.txt")
loader = DirectoryLoader("./docs", glob="**/*.txt", recursive=True)
loader = CSVLoader("data.csv", content_columns=["title", "content"])
loader = JSONLoader("data.json", content_key="content")

documents = loader.load()

# Split documents
splitter = RecursiveCharacterTextSplitter(chunk_size=500, chunk_overlap=50)
chunks = splitter.split_documents(documents)
```

## Execution

```python
agent = MyAgent()

# Sync
result = agent.run({"message": "Hello"})

# Async
result = await agent.arun({"message": "Hello"})

# Streaming
async for event in agent.stream({"message": "Hello"}):
    print(event.type, event.data)
```

## CLI Commands

| Command | Description |
|---------|-------------|
| `duragraph init <name> --template minimal` | Create project |
| `duragraph dev <file>` | Development mode |
| `duragraph deploy <file> --control-plane <url>` | Deploy |
| `duragraph visualize <file> --format mermaid` | Visualize |

Templates: `minimal`, `chatbot`, `tools`, `full`
Formats: `mermaid`, `dot`, `json`

## Worker

```python
from duragraph.worker import Worker

worker = Worker(
    control_plane_url="http://localhost:8081",
    name="my-worker",
    capabilities=["openai", "tools"],
    heartbeat_interval=30.0,
    max_concurrent_runs=10,
    shutdown_timeout=60.0
)

worker.register_graph(graph_definition)
worker.run()  # or await worker.arun()
```

## State & Messages

```python
state = {
    "messages": [
        {"role": "user", "content": "Hello"},
        {"role": "assistant", "content": "Hi there!"}
    ],
    "custom_key": "custom_value"
}
```

## RunResult

```python
result = agent.run(state)

result.run_id           # Execution ID
result.status           # "completed", "failed", etc.
result.output           # Final state  
result.nodes_executed   # List of executed nodes
```

## Common Patterns

### RAG (Retrieval-Augmented Generation)

```python
@tool(description="Search knowledge base")
def search_kb(query: str) -> str:
    results = vector_store.similarity_search(query, k=5)
    return "\n".join([doc.page_content for doc in results])

@Graph(id="rag_agent")
class RAGAgent:
    @entrypoint
    @llm_node(model="gpt-4o-mini", tools=[search_kb])
    def answer(self, state):
        return state
```

### Router Pattern

```python
@Graph(id="router_agent")
class RouterAgent:
    @entrypoint
    @llm_node(model="gpt-4o-mini", temperature=0.1)
    def classify(self, state):
        return state
    
    @router_node()
    def route(self, state):
        intent = state.get("intent", "").lower()
        if "billing" in intent:
            return "billing_handler"
        return "general_handler"
    
    @llm_node(model="gpt-4o-mini")
    def billing_handler(self, state):
        return state
    
    @llm_node(model="gpt-4o-mini")
    def general_handler(self, state):
        return state
    
    classify >> route
    billing_handler >> output
    general_handler >> output
```

### Human-in-the-Loop

```python
@Graph(id="hitl_agent")
class HITLAgent:
    @entrypoint
    @llm_node(model="gpt-4o-mini")
    def generate(self, state):
        return state
    
    @human_node(prompt="Please review and approve")
    def review(self, state):
        return state
    
    @node()
    def finalize(self, state):
        return state
    
    generate >> review >> finalize
```

## Environment Variables

| Variable | Description |
|----------|-------------|
| `OPENAI_API_KEY` | OpenAI API key |
| `ANTHROPIC_API_KEY` | Anthropic API key |
| `DURAGRAPH_CONTROL_PLANE_URL` | Control plane URL |

## Links

- [Full Documentation](docs/index.md)
- [Examples](examples/)
- [Contributing](CONTRIBUTING.md)