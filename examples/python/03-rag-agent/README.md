# RAG Agent with Vector Store

A retrieval-augmented generation (RAG) agent that indexes documents into a vector store and uses similarity search to ground responses in relevant context.

## What This Example Demonstrates

- **Document ingestion** - Loading and splitting text into chunks
- **Vector storage** - Embedding and indexing chunks with InMemoryVectorStore
- **Semantic retrieval** - Finding relevant documents via cosine similarity
- **Grounded generation** - Producing answers backed by retrieved context
- **Zero-dependency RAG** - Runs entirely in-memory with no external services

## Prerequisites

- Python 3.11+
- DuraGraph control plane running at `http://localhost:8081` (for serve mode)

## Quick Start

1. **Install dependencies:**
   ```bash
   pip install -r requirements.txt
   ```

2. **Run locally:**
   ```bash
   python main.py
   ```

3. **Run tests:**
   ```bash
   pytest test_rag.py -v
   ```

## Expected Output

```
DuraGraph RAG Agent
============================================================

Query: What is DuraGraph?
----------------------------------------
Response: Based on the available knowledge base, here is what I found ...
Sources:  3 chunk(s) retrieved

Query: How do I define a graph?
----------------------------------------
Response: Based on the available knowledge base, here is what I found ...
Sources:  3 chunk(s) retrieved

Query: What vector stores are supported?
----------------------------------------
Response: Based on the available knowledge base, here is what I found ...
Sources:  3 chunk(s) retrieved

Query: Tell me about event sourcing
----------------------------------------
Response: Based on the available knowledge base, here is what I found ...
Sources:  3 chunk(s) retrieved
```

## Code Walkthrough

### Pipeline

```
ingest_documents  →  retrieve  →  generate_response
```

1. **ingest_documents** - Converts raw text into `Document` objects, splits them with `RecursiveCharacterTextSplitter`, and indexes the chunks into `InMemoryVectorStore`.
2. **retrieve** - Embeds the user query and performs cosine-similarity search to find the top-k most relevant chunks.
3. **generate_response** - Formats the retrieved context into a grounded answer. In production, replace this with an `@llm_node()` call.

### Embedding

The example ships a simple bag-of-words `SimpleEmbedding` class so it works without API keys. Swap it for a real provider in production:

```python
from duragraph.embeddings.openai import OpenAIEmbeddingProvider

embedding_fn = OpenAIEmbeddingProvider(model="text-embedding-3-small")
vector_store = InMemoryVectorStore(embedding_function=embedding_fn)
```

### Custom Documents

Pass your own documents via the input state:

```python
result = agent.run({
    "query": "What is Kubernetes?",
    "documents": [
        {"content": "Kubernetes orchestrates containers.", "metadata": {"source": "docs"}},
    ],
})
```

### Serving on the Control Plane

```bash
# Trigger a run via the API
curl -X POST http://localhost:8081/api/v1/runs \
  -H "Content-Type: application/json" \
  -d '{
    "assistant_id": "rag_agent",
    "thread_id": "demo",
    "input": {"query": "What is DuraGraph?"}
  }'
```

## Production Considerations

### Real Embeddings

Replace `SimpleEmbedding` with `OpenAIEmbeddingProvider`, `CohereEmbeddingProvider`, or `OllamaEmbeddingProvider` for production-quality semantic search.

### Persistent Vector Store

Swap `InMemoryVectorStore` for a durable backend:

```python
from duragraph.vectorstores.pgvector import PgVectorStore

vector_store = PgVectorStore(
    connection_string="postgresql://user:pass@localhost/db",
    embedding_function=embedding_fn,
)
```

### LLM-Powered Generation

Replace the template-based `generate_response` node with an LLM call:

```python
@llm_node(
    model="gpt-4o-mini",
    system_prompt="Answer the user's question using ONLY the provided context.",
)
def generate_response(self, state: dict) -> dict:
    return state
```

## Configuration

| Environment Variable | Default | Description |
|---------------------|---------|-------------|
| `DURAGRAPH_URL` | `http://localhost:8081` | Control plane URL |

## Next Steps

- [04-multi-agent](../04-multi-agent) - Agent collaboration workflows
- [05-human-in-loop](../05-human-in-loop) - Approval workflows
- [06-tool-use](../06-tool-use) - External tool integration
