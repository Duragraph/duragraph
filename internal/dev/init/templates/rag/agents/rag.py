"""Minimal retrieval-augmented generation (RAG) graph.

Uses an in-memory vector store and a hash-based embedder so the demo
runs with zero external services. Replace `SimpleEmbedding` and
`InMemoryVectorStore` with real backends (OpenAI embeddings + Qdrant)
in production.
"""

import os

from duragraph import Graph, entrypoint, node
from duragraph.vectorstores.base import Document
from duragraph.vectorstores.memory import InMemoryVectorStore


class SimpleEmbedding:
    """Bag-of-words embedding for offline demos."""

    def embed_query(self, text: str) -> list[float]:
        return self._embed(text)

    def embed_documents(self, texts: list[str]) -> list[list[float]]:
        return [self._embed(t) for t in texts]

    def _embed(self, text: str) -> list[float]:
        vec = [0.0] * 128
        for w in text.lower().split():
            vec[hash(w) % 128] += 1.0
        return vec


KB = [
    "DuraGraph orchestrates AI workflows with event sourcing and CQRS.",
    "Graphs are defined with @Graph and connected nodes via the >> operator.",
    "Workers register with the control plane and poll for runs to execute.",
]

store = InMemoryVectorStore(embedding_function=SimpleEmbedding())


@Graph(id="rag", description="Retrieval-augmented generation over an in-memory KB")
class RAG:
    @entrypoint
    @node()
    async def ingest(self, state: dict) -> dict:
        if not state.get("_ingested"):
            await store.aadd_documents([Document(page_content=d) for d in KB])
            state["_ingested"] = True
        return state

    @node()
    async def retrieve(self, state: dict) -> dict:
        query = state.get("query") or ""
        if not query and (msgs := state.get("messages") or []):
            query = next((m["content"] for m in reversed(msgs) if m.get("role") == "user"), "")
        results = await store.asimilarity_search(query, k=2) if query else []
        state["context"] = "\n".join(d.page_content for d in results)
        state["response"] = f"Q: {query}\nContext:\n{state['context']}" if query else "(no query)"
        return state

    ingest >> retrieve


if __name__ == "__main__":
    agent = RAG()
    control_plane = os.getenv("DURAGRAPH_URL", "http://localhost:8081")
    agent.serve(control_plane, worker_name="rag-worker")
