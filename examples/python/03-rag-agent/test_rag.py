"""
Tests for the RAG agent example.

All tests run locally without API keys or external services.
"""

import pytest

from main import (
    KNOWLEDGE_BASE,
    RAGAgent,
    SimpleEmbedding,
    embedding_fn,
    text_splitter,
    vector_store,
)


class TestSimpleEmbedding:
    def test_embed_query_returns_vector(self):
        emb = SimpleEmbedding(dimension=64)
        vec = emb.embed_query("hello world")
        assert len(vec) == 64
        assert isinstance(vec[0], float)

    def test_embed_query_normalized(self):
        emb = SimpleEmbedding(dimension=64)
        vec = emb.embed_query("test embedding normalization")
        norm = sum(v * v for v in vec) ** 0.5
        assert abs(norm - 1.0) < 1e-6

    def test_embed_documents(self):
        emb = SimpleEmbedding(dimension=64)
        vecs = emb.embed_documents(["hello", "world"])
        assert len(vecs) == 2
        assert len(vecs[0]) == 64

    def test_similar_texts_closer(self):
        emb = SimpleEmbedding(dimension=128)
        v1 = emb.embed_query("DuraGraph workflow orchestration")
        v2 = emb.embed_query("DuraGraph workflow platform")
        v3 = emb.embed_query("pizza recipe ingredients cooking")

        sim_12 = sum(a * b for a, b in zip(v1, v2))
        sim_13 = sum(a * b for a, b in zip(v1, v3))
        assert sim_12 > sim_13

    def test_empty_text(self):
        emb = SimpleEmbedding(dimension=64)
        vec = emb.embed_query("")
        assert len(vec) == 64
        assert all(v == 0.0 for v in vec)


class TestTextSplitter:
    def test_splits_long_text(self):
        from duragraph.vectorstores.base import Document

        doc = Document(
            page_content="word " * 200,
            metadata={"source": "test"},
        )
        chunks = text_splitter.split_documents([doc])
        assert len(chunks) > 1

    def test_preserves_metadata(self):
        from duragraph.vectorstores.base import Document

        doc = Document(
            page_content="word " * 200,
            metadata={"source": "test", "topic": "demo"},
        )
        chunks = text_splitter.split_documents([doc])
        for chunk in chunks:
            assert chunk.metadata["source"] == "test"
            assert chunk.metadata["topic"] == "demo"

    def test_short_text_single_chunk(self):
        from duragraph.vectorstores.base import Document

        doc = Document(page_content="short text", metadata={})
        chunks = text_splitter.split_documents([doc])
        assert len(chunks) == 1
        assert chunks[0].page_content == "short text"


class TestVectorStore:
    def setup_method(self):
        vector_store._documents.clear()
        vector_store._embeddings.clear()

    @pytest.mark.asyncio
    async def test_add_and_search(self):
        from duragraph.vectorstores.base import Document

        docs = [
            Document(page_content="Python is a programming language", metadata={}),
            Document(page_content="Go is a compiled language", metadata={}),
            Document(page_content="Cats are furry animals", metadata={}),
        ]
        await vector_store.aadd_documents(docs)
        assert vector_store.get_document_count() == 3

        results = await vector_store.asimilarity_search("programming language", k=2)
        assert len(results) == 2

    @pytest.mark.asyncio
    async def test_empty_store_search(self):
        results = await vector_store.asimilarity_search("anything", k=3)
        assert results == []

    def teardown_method(self):
        vector_store._documents.clear()
        vector_store._embeddings.clear()


class TestRAGAgent:
    def setup_method(self):
        vector_store._documents.clear()
        vector_store._embeddings.clear()

    @pytest.fixture
    def agent(self):
        return RAGAgent()

    @pytest.mark.asyncio
    async def test_ingest_documents(self, agent):
        state = {"documents": KNOWLEDGE_BASE, "query": ""}
        result = await agent.ingest_documents(state)
        assert result["num_indexed"] > 0
        assert result["num_chunks"] > 0
        assert vector_store.get_document_count() > 0

    @pytest.mark.asyncio
    async def test_retrieve_with_query(self, agent):
        state = {"documents": KNOWLEDGE_BASE, "query": "What is DuraGraph?"}
        state = await agent.ingest_documents(state)
        state = await agent.retrieve(state)
        assert "context" in state
        assert len(state["context"]) > 0
        assert len(state["sources"]) > 0

    @pytest.mark.asyncio
    async def test_retrieve_empty_query(self, agent):
        state = {"documents": KNOWLEDGE_BASE, "query": ""}
        state = await agent.ingest_documents(state)
        state = await agent.retrieve(state)
        assert state["context"] == ""
        assert state["sources"] == []

    @pytest.mark.asyncio
    async def test_generate_with_context(self, agent):
        state = {
            "query": "What is DuraGraph?",
            "context": "DuraGraph is a workflow platform.",
            "sources": [{"content": "DuraGraph is a workflow platform.", "metadata": {}}],
        }
        result = await agent.generate_response(state)
        assert "response" in result
        assert "DuraGraph" in result["response"]

    @pytest.mark.asyncio
    async def test_generate_without_context(self, agent):
        state = {"query": "unknown topic", "context": "", "sources": []}
        result = await agent.generate_response(state)
        assert "don't have enough information" in result["response"]

    @pytest.mark.asyncio
    async def test_custom_top_k(self, agent):
        state = {
            "documents": KNOWLEDGE_BASE,
            "query": "DuraGraph features",
            "top_k": 2,
        }
        state = await agent.ingest_documents(state)
        state = await agent.retrieve(state)
        assert len(state["sources"]) <= 2

    def teardown_method(self):
        vector_store._documents.clear()
        vector_store._embeddings.clear()


class TestIntegration:
    def setup_method(self):
        vector_store._documents.clear()
        vector_store._embeddings.clear()

    @pytest.mark.asyncio
    async def test_full_pipeline(self):
        agent = RAGAgent()

        state = {"query": "How do I define a graph?"}
        state = await agent.ingest_documents(state)
        state = await agent.retrieve(state)
        state = await agent.generate_response(state)

        assert state["num_indexed"] > 0
        assert len(state["sources"]) > 0
        assert "response" in state
        assert len(state["response"]) > 0

    @pytest.mark.asyncio
    async def test_multiple_queries(self):
        agent = RAGAgent()

        state = {"query": "event sourcing"}
        state = await agent.ingest_documents(state)
        state = await agent.retrieve(state)
        state = await agent.generate_response(state)
        assert "response" in state

        vector_store._documents.clear()
        vector_store._embeddings.clear()

        state2 = {"query": "vector stores"}
        state2 = await agent.ingest_documents(state2)
        state2 = await agent.retrieve(state2)
        state2 = await agent.generate_response(state2)
        assert "response" in state2

    @pytest.mark.asyncio
    async def test_custom_documents(self):
        agent = RAGAgent()
        custom_docs = [
            {
                "content": "Kubernetes orchestrates containerized applications.",
                "metadata": {"source": "custom"},
            },
            {
                "content": "Docker packages applications into containers.",
                "metadata": {"source": "custom"},
            },
        ]
        state = {"documents": custom_docs, "query": "containers"}
        state = await agent.ingest_documents(state)
        state = await agent.retrieve(state)
        state = await agent.generate_response(state)

        assert state["num_indexed"] > 0
        assert "response" in state

    def teardown_method(self):
        vector_store._documents.clear()
        vector_store._embeddings.clear()
