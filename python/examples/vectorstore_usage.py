"""Example: Using vector stores in DuraGraph."""

import asyncio

from duragraph.embeddings import create_embedding_provider
from duragraph.vectorstores import Document, create_vectorstore


async def main():
    """Demonstrate vector store usage."""

    print("=== DuraGraph Vector Stores Example ===\n")

    # Create embedding function
    print("Creating embedding provider...")
    embedding_provider = create_embedding_provider("anthropic")
    print(f"Embedding dimension: {embedding_provider.dimension}")

    # Create sample documents
    documents = [
        Document(
            page_content="DuraGraph is a powerful AI workflow orchestration platform designed for production use.",
            metadata={"source": "docs", "category": "overview", "page": 1},
        ),
        Document(
            page_content="Vector databases enable efficient storage and retrieval of high-dimensional embeddings.",
            metadata={"source": "docs", "category": "technical", "page": 2},
        ),
        Document(
            page_content="Embedding models convert text into numerical vectors that capture semantic meaning.",
            metadata={"source": "docs", "category": "technical", "page": 3},
        ),
        Document(
            page_content="Semantic search uses embeddings to find documents similar in meaning, not just keywords.",
            metadata={"source": "docs", "category": "features", "page": 4},
        ),
        Document(
            page_content="RAG (Retrieval-Augmented Generation) combines search with language model generation.",
            metadata={"source": "docs", "category": "features", "page": 5},
        ),
        Document(
            page_content="Python SDK provides easy integration with DuraGraph's powerful workflow capabilities.",
            metadata={"source": "sdk", "category": "integration", "page": 1},
        ),
    ]

    print(f"Created {len(documents)} sample documents")

    # Try different vector store types
    store_types = ["memory"]
    # Uncomment if you have chromadb installed:
    # store_types.append("chroma")

    for store_type in store_types:
        print(f"\n--- Using {store_type} vector store ---")

        try:
            # Create vector store
            if store_type == "chroma":
                # For Chroma, specify collection name and persistence
                vector_store = create_vectorstore(
                    store_type,
                    embedding_function=embedding_provider,
                    collection_name="duragraph_example",
                    persist_directory="./chroma_db",
                )
            else:
                # For memory store
                vector_store = create_vectorstore(store_type, embedding_function=embedding_provider)

            print(f"Created {store_type} vector store")

            # Add documents to vector store
            print("Adding documents to vector store...")
            doc_ids = await vector_store.aadd_documents(documents)
            print(f"Added {len(doc_ids)} documents with IDs: {doc_ids[:3]}...")

            # Basic similarity search
            print("\n1. Basic Similarity Search:")
            query = "How does semantic search work?"
            results = await vector_store.asimilarity_search(query, k=3)

            print(f"Query: '{query}'")
            print("Top 3 results:")
            for i, doc in enumerate(results, 1):
                print(f"  {i}. {doc.page_content[:60]}...")
                print(f"     Metadata: {doc.metadata}")

            # Similarity search with scores
            print("\n2. Similarity Search with Scores:")
            results_with_scores = await vector_store.asimilarity_search_with_score(
                "vector embeddings", k=3
            )

            print("Query: 'vector embeddings'")
            print("Results with similarity scores:")
            for i, (doc, score) in enumerate(results_with_scores, 1):
                print(f"  {i}. Score: {score:.4f}")
                print(f"     Text: {doc.page_content[:50]}...")
                print(f"     Category: {doc.metadata.get('category', 'N/A')}")

            # Filtered search
            print("\n3. Filtered Search:")
            filtered_results = await vector_store.asimilarity_search(
                "DuraGraph platform", k=5, filter={"category": "overview"}
            )

            print("Query: 'DuraGraph platform' (category=overview only)")
            print(f"Found {len(filtered_results)} matching documents:")
            for doc in filtered_results:
                print(f"  - {doc.page_content[:50]}...")
                print(f"    Source: {doc.metadata.get('source', 'N/A')}")

            # Search by embedding vector
            print("\n4. Search by Embedding Vector:")
            query_text = "RAG and generation"
            query_embedding = await embedding_provider.aembed_query(query_text)

            vector_results = await vector_store.asimilarity_search_by_vector(query_embedding, k=2)

            print(f"Query embedding for: '{query_text}'")
            print("Results:")
            for doc in vector_results:
                print(f"  - {doc.page_content}")

            # Complex metadata filtering
            print("\n5. Complex Metadata Filtering:")
            complex_results = await vector_store.asimilarity_search(
                "Python integration",
                k=10,
                filter={"source": {"$in": ["sdk", "docs"]}, "page": {"$lte": 3}},
            )

            print("Query: 'Python integration' (source in ['sdk', 'docs'] AND page <= 3)")
            print(f"Found {len(complex_results)} matching documents:")
            for doc in complex_results:
                print(f"  - Page {doc.metadata.get('page', '?')}: {doc.page_content[:40]}...")

            # Document management
            print("\n6. Document Management:")
            if hasattr(vector_store, "get_document_count"):
                print(f"Total documents: {vector_store.get_document_count()}")

            # Delete some documents
            print("Deleting technical documents...")
            deleted = await vector_store.adelete(filter={"category": "technical"})
            print(f"Deletion successful: {deleted}")

            if hasattr(vector_store, "get_document_count"):
                print(f"Documents after deletion: {vector_store.get_document_count()}")

        except ImportError as e:
            print(f"Store type {store_type} not available: {e}")
        except Exception as e:
            print(f"Error with {store_type} store: {e}")

    # Demonstrate synchronous usage
    print("\n--- Synchronous Usage ---")
    try:
        # Create simple in-memory store
        sync_store = create_vectorstore("memory", embedding_function=embedding_provider)

        # Add documents synchronously
        sample_docs = documents[:2]  # Just first 2 docs
        sync_ids = sync_store.add_documents(sample_docs)
        print(f"Added {len(sync_ids)} documents synchronously")

        # Search synchronously
        sync_results = sync_store.similarity_search("DuraGraph", k=1)
        print(f"Sync search found: {sync_results[0].page_content[:50]}...")

    except Exception as e:
        print(f"Sync usage error: {e}")


def demo_integration_with_embeddings():
    """Demonstrate integration between embeddings and vector stores."""
    print("\n=== Embedding + Vector Store Integration ===")

    # Create embedding provider
    embedding_provider = create_embedding_provider("anthropic")

    # Create vector store with the embedding provider
    vector_store = create_vectorstore("memory", embedding_function=embedding_provider)

    # Create from texts (convenience method)
    texts = [
        "Machine learning models can be trained on large datasets.",
        "Deep learning is a subset of machine learning using neural networks.",
        "Natural language processing enables computers to understand human language.",
    ]

    metadatas = [
        {"topic": "ml", "difficulty": "beginner"},
        {"topic": "dl", "difficulty": "intermediate"},
        {"topic": "nlp", "difficulty": "intermediate"},
    ]

    # Create store from texts
    text_store = type(vector_store).from_texts(texts, embedding_provider, metadatas)

    print(f"Created store from {len(texts)} texts")

    # Search in the text-based store
    results = text_store.similarity_search("neural networks", k=2)
    print("Search results:")
    for doc in results:
        print(f"  - {doc.page_content}")
        print(f"    Topic: {doc.metadata.get('topic', 'N/A')}")


if __name__ == "__main__":
    asyncio.run(main())
    demo_integration_with_embeddings()
