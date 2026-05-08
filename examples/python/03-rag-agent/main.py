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


# --- LLM client (lazy, optional) ---------------------------------------
# Mirrors 02-chatbot: prefer OpenRouter, fall back to OpenAI direct, then
# template-based response when neither is configured.
_llm_client: Any = None
_llm_model: str = "template"
_llm_provider: str = "template"

try:
    from openai import AsyncOpenAI

    if os.environ.get("OPENROUTER_API_KEY"):
        _llm_client = AsyncOpenAI(
            api_key=os.environ["OPENROUTER_API_KEY"],
            base_url="https://openrouter.ai/api/v1",
            default_headers={
                "HTTP-Referer": os.environ.get(
                    "OPENROUTER_REFERER", "https://duragraph.io"
                ),
                "X-Title": os.environ.get(
                    "OPENROUTER_TITLE", "DuraGraph RAG Demo"
                ),
            },
        )
        _llm_model = os.environ.get("OPENROUTER_MODEL", "openai/gpt-4o-mini")
        _llm_provider = "openrouter"
    elif os.environ.get("OPENAI_API_KEY"):
        _llm_client = AsyncOpenAI()
        _llm_model = os.environ.get("OPENAI_MODEL", "gpt-4o-mini")
        _llm_provider = "openai"
except ImportError:
    pass


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

        Accepts both input shapes:
          - LangChain / Studio: {"messages": [{"role": "user", "content": "..."}]}
          - Legacy / curl demo: {"query": "..."}
        Uses cosine-similarity search; result count from state["top_k"] (default 3).
        """
        # Resolve the query from either shape
        query = state.get("query") or ""
        if not query:
            messages = state.get("messages") or []
            for m in reversed(messages):
                if m.get("role") == "user":
                    query = m.get("content", "")
                    break
        state["query"] = query

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

        Uses an LLM (OpenRouter / OpenAI) when configured, with the
        retrieved chunks injected into the system prompt. Falls back to a
        template-based response when no LLM is available.
        """
        query = state.get("query", "")
        context = state.get("context", "")

        if not context:
            state["response"] = (
                "I don't have enough information to answer that question. "
                "Please try a different query."
            )
            state["model"] = "none"
            return state

        if _llm_client is not None:
            system_prompt = (
                "You are a documentation assistant for DuraGraph, an AI "
                "workflow orchestration platform. Answer the user's "
                "question using ONLY the context below. If the context "
                "does not contain the answer, say so plainly. Be concise "
                "(2-3 sentences). Do not invent facts.\n\n"
                f"Context:\n{context}"
            )
            try:
                resp = await _llm_client.chat.completions.create(
                    model=_llm_model,
                    messages=[
                        {"role": "system", "content": system_prompt},
                        {"role": "user", "content": query},
                    ],
                    temperature=0.2,
                )
                state["response"] = resp.choices[0].message.content or ""
                state["model"] = _llm_model
                state["provider"] = _llm_provider
                if resp.usage:
                    state["usage"] = {
                        "prompt_tokens": resp.usage.prompt_tokens,
                        "completion_tokens": resp.usage.completion_tokens,
                        "total_tokens": resp.usage.total_tokens,
                    }
            except Exception as exc:  # noqa: BLE001
                # Graceful degrade to template if the LLM call fails
                state["response"] = (
                    f"[LLM error: {exc}]\n\n"
                    f"Falling back to retrieved context for \"{query}\":\n\n{context}"
                )
                state["model"] = "error"
            return state

        # Template fallback (no LLM configured)
        state["response"] = (
            f"Based on the available knowledge base, here is what I found "
            f"about your question \"{query}\":\n\n{context}"
        )
        state["model"] = "template"
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
