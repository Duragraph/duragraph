"""
DuraGraph RAG Agent Example

Demonstrates:
- Document loading and text splitting
- Embedding generation and vector storage
- Retrieval-augmented generation (RAG) pipeline
- Running locally with InMemoryVectorStore (no external dependencies)
"""

import os
from typing import Any

from duragraph import Graph, entrypoint, node
from duragraph.document_loaders.text_splitter import RecursiveCharacterTextSplitter
from duragraph.vectorstores.base import Document
from duragraph.vectorstores.memory import InMemoryVectorStore


# ---------------------------------------------------------------------------
# Sample knowledge base (replace with real documents in production)
# ---------------------------------------------------------------------------
KNOWLEDGE_BASE = [
    {
        "content": (
            "DuraGraph is an enterprise-ready AI workflow orchestration platform. "
            "It provides a LangGraph Cloud-compatible API for self-hosted deployment. "
            "DuraGraph uses event sourcing and CQRS patterns for reliable execution."
        ),
        "metadata": {"source": "overview", "topic": "architecture"},
    },
    {
        "content": (
            "Graphs in DuraGraph are defined using the @Graph decorator. Nodes are "
            "defined with @node() and connected using the >> operator. Each node "
            "receives a state dictionary, modifies it, and returns the updated state."
        ),
        "metadata": {"source": "guide", "topic": "graphs"},
    },
    {
        "content": (
            "DuraGraph supports multiple vector store backends including "
            "InMemoryVectorStore for development, ChromaDB, PgVector, Pinecone, "
            "Qdrant, and Weaviate for production deployments."
        ),
        "metadata": {"source": "guide", "topic": "vectorstores"},
    },
    {
        "content": (
            "Workers in DuraGraph connect to the control plane, register their "
            "available graphs, and poll for work. When a run is assigned, the "
            "worker executes the graph and reports results back via the API."
        ),
        "metadata": {"source": "guide", "topic": "workers"},
    },
    {
        "content": (
            "DuraGraph provides Python and Go SDKs. The Python SDK uses decorators "
            "like @node(), @llm_node(), @router_node(), and @tool_node(). The Go SDK "
            "uses generics with interfaces like Node[S] and Router[S]."
        ),
        "metadata": {"source": "guide", "topic": "sdks"},
    },
    {
        "content": (
            "Event sourcing in DuraGraph means all state changes are stored as "
            "immutable events. The run aggregate tracks lifecycle events: "
            "RunCreated, RunStarted, RunCompleted, and RunFailed. Events are "
            "replayed to reconstruct state."
        ),
        "metadata": {"source": "deep-dive", "topic": "event-sourcing"},
    },
    {
        "content": (
            "The DuraGraph control plane exposes a REST API with endpoints for "
            "managing assistants, threads, and runs. Assistants map to graph "
            "definitions, threads group related runs, and runs represent "
            "individual workflow executions."
        ),
        "metadata": {"source": "api-reference", "topic": "api"},
    },
    {
        "content": (
            "DuraGraph Studio is a visual workflow editor built with React and "
            "TypeScript. It provides a chat interface for interacting with agents, "
            "trace visualization for debugging, and thread selection for managing "
            "conversations."
        ),
        "metadata": {"source": "guide", "topic": "studio"},
    },
]


# ---------------------------------------------------------------------------
# Simple embedding function (no API key required)
# ---------------------------------------------------------------------------
class SimpleEmbedding:
    """Bag-of-words embedding for demo purposes.

    Production usage should substitute OpenAIEmbeddingProvider or similar.
    """

    def __init__(self, dimension: int = 128) -> None:
        self._dimension = dimension

    def embed_query(self, text: str) -> list[float]:
        return self._embed(text)

    def embed_documents(self, texts: list[str]) -> list[list[float]]:
        return [self._embed(t) for t in texts]

    def _embed(self, text: str) -> list[float]:
        words = text.lower().split()
        vec = [0.0] * self._dimension
        for word in words:
            idx = hash(word) % self._dimension
            vec[idx] += 1.0
        norm = sum(v * v for v in vec) ** 0.5
        if norm > 0:
            vec = [v / norm for v in vec]
        return vec


# ---------------------------------------------------------------------------
# Shared resources
# ---------------------------------------------------------------------------
embedding_fn = SimpleEmbedding(dimension=128)
vector_store = InMemoryVectorStore(embedding_function=embedding_fn)
text_splitter = RecursiveCharacterTextSplitter(chunk_size=200, chunk_overlap=50)


# ---------------------------------------------------------------------------
# Graph definition
# ---------------------------------------------------------------------------
@Graph(id="rag_agent", description="RAG agent with vector store retrieval")
class RAGAgent:
    """Retrieval-augmented generation agent.

    Pipeline:
        ingest_documents  →  retrieve  →  generate_response
    """

    @entrypoint
    @node()
    async def ingest_documents(self, state: dict[str, Any]) -> dict[str, Any]:
        """Load and index documents into the vector store.

        Reads documents from state["documents"] (list of dicts with
        "content" and optional "metadata" keys).  Falls back to the
        built-in KNOWLEDGE_BASE when no documents are supplied.

        After ingestion the store is available for subsequent retrieval.
        """
        raw_docs = state.get("documents", KNOWLEDGE_BASE)

        documents = [
            Document(
                page_content=d["content"],
                metadata=d.get("metadata", {}),
            )
            for d in raw_docs
        ]

        chunks = text_splitter.split_documents(documents)
        ids = await vector_store.aadd_documents(chunks)
        state["num_indexed"] = len(ids)
        state["num_chunks"] = len(chunks)
        return state

    @node()
    async def retrieve(self, state: dict[str, Any]) -> dict[str, Any]:
        """Retrieve relevant documents for the user query.

        Uses cosine-similarity search over the vector store.  The number
        of results is controlled by state["top_k"] (default 3).
        """
        query = state.get("query", "")
        top_k = state.get("top_k", 3)

        if not query:
            state["context"] = ""
            state["sources"] = []
            return state

        results = await vector_store.asimilarity_search(query, k=top_k)
        state["context"] = "\n\n".join(doc.page_content for doc in results)
        state["sources"] = [
            {"content": doc.page_content, "metadata": doc.metadata}
            for doc in results
        ]
        return state

    @node()
    async def generate_response(self, state: dict[str, Any]) -> dict[str, Any]:
        """Generate an answer grounded in the retrieved context.

        This demo uses a simple template-based response.  In production,
        replace the body with an LLM call (e.g. @llm_node) that receives
        the context as part of the prompt.
        """
        query = state.get("query", "")
        context = state.get("context", "")

        if not context:
            state["response"] = (
                "I don't have enough information to answer that question. "
                "Please try a different query."
            )
            return state

        state["response"] = (
            f"Based on the available knowledge base, here is what I found "
            f"about your question \"{query}\":\n\n{context}"
        )
        return state

    ingest_documents >> retrieve >> generate_response


def main() -> None:
    agent = RAGAgent()

    # --- Local execution ---
    print("DuraGraph RAG Agent")
    print("=" * 60)

    queries = [
        "What is DuraGraph?",
        "How do I define a graph?",
        "What vector stores are supported?",
        "Tell me about event sourcing",
    ]

    for query in queries:
        print(f"\nQuery: {query}")
        print("-" * 40)
        result = agent.run({"query": query})
        print(f"Response: {result.output.get('response', 'No response')}")
        sources = result.output.get("sources", [])
        if sources:
            print(f"Sources:  {len(sources)} chunk(s) retrieved")

    # --- Serve on control plane ---
    control_plane = os.getenv("DURAGRAPH_URL", "http://localhost:8081")
    print(f"\n{'=' * 60}")
    print(f"Serving on {control_plane}")
    print("Press Ctrl+C to stop\n")
    agent.serve(control_plane, worker_name="rag-agent-worker")


if __name__ == "__main__":
    main()
