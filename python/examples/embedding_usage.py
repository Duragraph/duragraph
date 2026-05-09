"""Example: Using embedding providers in DuraGraph."""

import asyncio

from duragraph.embeddings import create_embedding_provider


async def main():
    """Demonstrate embedding provider usage."""

    print("=== DuraGraph Embedding Providers Example ===\n")

    # Example documents to embed
    documents = [
        "DuraGraph is an AI workflow orchestration platform.",
        "Embeddings convert text into numerical vectors.",
        "Vector databases store and search embeddings efficiently.",
        "Semantic search uses embeddings to find similar content.",
    ]

    query = "How does semantic search work?"

    # Try different embedding providers
    providers_to_try = [
        ("anthropic", "claude-3-haiku-20240307"),  # Placeholder implementation
        # Uncomment if you have API keys:
        # ("openai", "text-embedding-3-small"),
    ]

    for provider_name, model in providers_to_try:
        print(f"--- Using {provider_name} provider with {model} ---")

        try:
            # Create embedding provider
            provider = create_embedding_provider(
                provider=provider_name,
                model=model,
                # api_key="your-api-key-here"  # Set your API key
            )

            print(f"Provider dimension: {provider.dimension}")

            # Embed documents
            print("Embedding documents...")
            doc_embeddings = await provider.aembed_documents(documents)
            print(f"Created {len(doc_embeddings)} document embeddings")
            print(f"Each embedding has {len(doc_embeddings[0])} dimensions")

            # Embed query
            print("\nEmbedding query...")
            query_embedding = await provider.aembed_query(query)
            print(f"Query embedding has {len(query_embedding)} dimensions")

            # Simple similarity search (cosine similarity)
            print("\nFinding most similar document to query...")
            similarities = []

            for i, doc_emb in enumerate(doc_embeddings):
                # Calculate cosine similarity
                dot_product = sum(a * b for a, b in zip(query_embedding, doc_emb, strict=False))
                norm_query = sum(x * x for x in query_embedding) ** 0.5
                norm_doc = sum(x * x for x in doc_emb) ** 0.5
                similarity = dot_product / (norm_query * norm_doc)
                similarities.append((similarity, i, documents[i]))

            # Sort by similarity (highest first)
            similarities.sort(reverse=True)

            print(f"Query: '{query}'")
            print("Most similar documents:")
            for similarity, _idx, doc in similarities[:2]:
                print(f"  {similarity:.4f}: {doc}")

        except ImportError as e:
            print(f"Provider {provider_name} not available: {e}")
        except Exception as e:
            print(f"Error with {provider_name}: {e}")

        print()


def demo_sync_usage():
    """Demonstrate synchronous usage (separate from async)."""
    print("--- Synchronous Usage ---")
    try:
        provider = create_embedding_provider("anthropic")

        # Sync methods
        sync_embedding = provider.embed_query("Synchronous embedding example")
        print(f"Sync query embedding: {len(sync_embedding)} dimensions")

        sync_doc_embeddings = provider.embed_documents(["Document 1", "Document 2"])
        print(f"Sync document embeddings: {len(sync_doc_embeddings)} embeddings")

    except Exception as e:
        print(f"Sync usage error: {e}")


if __name__ == "__main__":
    # Run async example
    asyncio.run(main())

    # Run sync example separately
    print()
    demo_sync_usage()
