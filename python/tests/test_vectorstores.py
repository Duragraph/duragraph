"""Tests for vector stores."""


import pytest

from duragraph.vectorstores import (
    Document,
    create_vectorstore,
    get_vectorstore,
    list_vectorstores,
    register_vectorstore,
)
from duragraph.vectorstores.memory import InMemoryVectorStore


class MockEmbeddingFunction:
    """Mock embedding function for testing."""

    def __init__(self, dimension: int = 768):
        self.dimension = dimension

    def embed_query(self, text: str):
        """Generate deterministic embedding from text hash."""
        import hashlib

        hash_obj = hashlib.md5(text.encode())
        hash_int = int(hash_obj.hexdigest(), 16)

        # Create embedding from hash
        embedding = []
        for i in range(self.dimension):
            embedding.append((hash_int >> (i % 32)) / (2**32))

        return embedding

    def embed_documents(self, texts):
        """Embed multiple documents."""
        return [self.embed_query(text) for text in texts]


def test_document_model():
    """Test Document model."""
    doc = Document(page_content="Test content", metadata={"source": "test"})
    assert doc.page_content == "Test content"
    assert doc.metadata["source"] == "test"
    assert str(doc) == "Test content"


def test_vectorstore_registry():
    """Test vector store registration and retrieval."""
    # Register mock store
    register_vectorstore("mock", InMemoryVectorStore)

    # Check it's listed
    assert "mock" in list_vectorstores()
    assert "memory" in list_vectorstores()  # Built-in store

    # Get store instance
    embedding_fn = MockEmbeddingFunction()
    store = get_vectorstore("mock", embedding_function=embedding_fn)
    assert isinstance(store, InMemoryVectorStore)


def test_create_vectorstore():
    """Test vector store creation with common parameters."""
    embedding_fn = MockEmbeddingFunction()
    store = create_vectorstore("memory", embedding_function=embedding_fn)
    assert isinstance(store, InMemoryVectorStore)
    assert store.embedding_function == embedding_fn


def test_unknown_vectorstore():
    """Test error handling for unknown vector store."""
    with pytest.raises(ValueError, match="Unknown vector store"):
        get_vectorstore("nonexistent")


class TestInMemoryVectorStore:
    """Tests for InMemoryVectorStore."""

    @pytest.fixture
    def embedding_function(self):
        """Create mock embedding function."""
        return MockEmbeddingFunction(dimension=384)

    @pytest.fixture
    def store(self, embedding_function):
        """Create in-memory vector store."""
        return InMemoryVectorStore(embedding_function=embedding_function)

    @pytest.fixture
    def sample_documents(self):
        """Create sample documents."""
        return [
            Document(
                page_content="The quick brown fox jumps over the lazy dog",
                metadata={"category": "animals", "length": "short"},
            ),
            Document(
                page_content="Python is a high-level programming language",
                metadata={"category": "programming", "length": "short"},
            ),
            Document(
                page_content="Machine learning is a subset of artificial intelligence",
                metadata={"category": "ai", "length": "medium"},
            ),
            Document(
                page_content="Vector databases store and search high-dimensional vectors efficiently",
                metadata={"category": "databases", "length": "medium"},
            ),
        ]

    @pytest.mark.asyncio
    async def test_add_documents(self, store, sample_documents):
        """Test adding documents to vector store."""
        ids = await store.aadd_documents(sample_documents)

        assert len(ids) == 4
        assert store.get_document_count() == 4
        assert all(isinstance(doc_id, str) for doc_id in ids)

    @pytest.mark.asyncio
    async def test_add_documents_with_ids(self, store, sample_documents):
        """Test adding documents with specific IDs."""
        custom_ids = ["doc1", "doc2", "doc3", "doc4"]
        ids = await store.aadd_documents(sample_documents, ids=custom_ids)

        assert ids == custom_ids
        assert store.get_document_count() == 4

        # Check we can retrieve by ID
        doc = store.get_document_by_id("doc1")
        assert doc.page_content == sample_documents[0].page_content

    @pytest.mark.asyncio
    async def test_similarity_search(self, store, sample_documents):
        """Test similarity search."""
        # Add documents
        await store.aadd_documents(sample_documents)

        # Search for programming-related content
        results = await store.asimilarity_search("programming language", k=2)

        assert len(results) <= 2
        assert all(isinstance(doc, Document) for doc in results)

        # Should return some documents (exact match depends on hash function)
        assert len(results) > 0

    @pytest.mark.asyncio
    async def test_similarity_search_with_score(self, store, sample_documents):
        """Test similarity search with scores."""
        await store.aadd_documents(sample_documents)

        results = await store.asimilarity_search_with_score("artificial intelligence", k=3)

        assert len(results) <= 3
        assert all(isinstance(result, tuple) and len(result) == 2 for result in results)

        doc, score = results[0]
        assert isinstance(doc, Document)
        assert isinstance(score, float)
        # Allow small floating-point tolerance (scores should be roughly between 0 and 1)
        assert -1e-9 <= score <= 1.0 + 1e-9

        # Scores should be in descending order
        scores = [score for _, score in results]
        assert scores == sorted(scores, reverse=True)

    @pytest.mark.asyncio
    async def test_similarity_search_with_filter(self, store, sample_documents):
        """Test similarity search with metadata filter."""
        await store.aadd_documents(sample_documents)

        # Search only in "ai" category
        results = await store.asimilarity_search("machine learning", k=5, filter={"category": "ai"})

        assert len(results) == 1  # Only one AI document
        assert results[0].metadata["category"] == "ai"
        assert "Machine learning" in results[0].page_content

    @pytest.mark.asyncio
    async def test_complex_filter(self, store, sample_documents):
        """Test complex metadata filters."""
        await store.aadd_documents(sample_documents)

        # Search for medium-length documents
        results = await store.asimilarity_search(
            "information", k=5, filter={"length": {"$eq": "medium"}}
        )

        assert len(results) == 2  # Two medium documents
        assert all(doc.metadata["length"] == "medium" for doc in results)

    @pytest.mark.asyncio
    async def test_search_by_vector(self, store, sample_documents, embedding_function):
        """Test similarity search by embedding vector."""
        await store.aadd_documents(sample_documents)

        # Get embedding for query
        query_embedding = embedding_function.embed_query("programming")

        # Search by vector
        results = await store.asimilarity_search_by_vector(query_embedding, k=2)

        assert len(results) <= 2
        assert all(isinstance(doc, Document) for doc in results)

    @pytest.mark.asyncio
    async def test_delete_by_ids(self, store, sample_documents):
        """Test deleting documents by IDs."""
        ids = await store.aadd_documents(sample_documents)
        assert store.get_document_count() == 4

        # Delete first two documents
        success = await store.adelete(ids=ids[:2])
        assert success is True
        assert store.get_document_count() == 2

        # Check documents are gone
        assert store.get_document_by_id(ids[0]) is None
        assert store.get_document_by_id(ids[1]) is None
        assert store.get_document_by_id(ids[2]) is not None

    @pytest.mark.asyncio
    async def test_delete_by_filter(self, store, sample_documents):
        """Test deleting documents by metadata filter."""
        await store.aadd_documents(sample_documents)
        assert store.get_document_count() == 4

        # Delete all short documents
        success = await store.adelete(filter={"length": "short"})
        assert success is True
        assert store.get_document_count() == 2

        # Remaining documents should be medium length
        remaining_docs = [store.get_document_by_id(doc_id) for doc_id in store.list_document_ids()]
        assert all(doc.metadata["length"] == "medium" for doc in remaining_docs if doc)

    @pytest.mark.asyncio
    async def test_clear_all(self, store, sample_documents):
        """Test clearing all documents."""
        await store.aadd_documents(sample_documents)
        assert store.get_document_count() == 4

        # Clear all
        success = await store.adelete()
        assert success is True
        assert store.get_document_count() == 0

    def test_sync_methods(self, store, sample_documents):
        """Test synchronous methods."""
        # Add documents synchronously
        ids = store.add_documents(sample_documents)
        assert len(ids) == 4

        # Search synchronously
        results = store.similarity_search("programming", k=2)
        assert len(results) <= 2

        # Search with scores synchronously
        results_with_scores = store.similarity_search_with_score("AI", k=2)
        assert len(results_with_scores) <= 2
        assert all(isinstance(result, tuple) for result in results_with_scores)

        # Delete synchronously
        success = store.delete(ids=ids[:1])
        assert success is True
        assert store.get_document_count() == 3


def test_from_documents():
    """Test creating vector store from documents."""
    embedding_fn = MockEmbeddingFunction()
    documents = [
        Document(page_content="Document 1", metadata={"id": 1}),
        Document(page_content="Document 2", metadata={"id": 2}),
    ]

    store = InMemoryVectorStore.from_documents(documents, embedding_fn)
    assert store.get_document_count() == 2


def test_from_texts():
    """Test creating vector store from texts."""
    embedding_fn = MockEmbeddingFunction()
    texts = ["Text 1", "Text 2", "Text 3"]
    metadatas = [{"id": 1}, {"id": 2}, {"id": 3}]

    store = InMemoryVectorStore.from_texts(texts, embedding_fn, metadatas)
    assert store.get_document_count() == 3

    # Check that documents were created correctly
    docs = [store.get_document_by_id(doc_id) for doc_id in store.list_document_ids()]
    contents = [doc.page_content for doc in docs if doc]
    assert "Text 1" in contents
    assert "Text 2" in contents
    assert "Text 3" in contents


def test_no_embedding_function():
    """Test vector store without embedding function (uses hash-based)."""
    store = InMemoryVectorStore()

    documents = [
        Document(page_content="Test document", metadata={}),
    ]

    ids = store.add_documents(documents)
    assert len(ids) == 1

    # Should still be able to search (using hash-based embeddings)
    results = store.similarity_search("Test", k=1)
    assert len(results) == 1
