"""Example: Using document loaders and text splitters in DuraGraph."""

import asyncio
import tempfile
from pathlib import Path

from duragraph.document_loaders import (
    CharacterTextSplitter,
    CodeTextSplitter,
    CSVLoader,
    DirectoryLoader,
    JSONLoader,
    MarkdownTextSplitter,
    ParagraphTextSplitter,
    RecursiveCharacterTextSplitter,
    TextFileLoader,
)
from duragraph.embeddings import create_embedding_provider
from duragraph.vectorstores import create_vectorstore


def create_sample_files(temp_dir: Path) -> None:
    """Create sample files for testing."""

    # Create a text file
    (temp_dir / "article.txt").write_text(
        """
DuraGraph: The Future of AI Workflow Orchestration

DuraGraph represents a revolutionary approach to building and deploying AI workflows.
Unlike traditional solutions, DuraGraph combines the flexibility of modern AI frameworks
with the reliability and observability requirements of production systems.

Key Features:
- Event-sourced architecture for complete audit trails
- Built-in support for human-in-the-loop workflows
- Comprehensive observability and monitoring
- LangGraph Cloud compatibility for seamless migration

Getting Started:
To begin using DuraGraph, install the Python SDK and create your first workflow.
The decorator-based API makes it easy to define complex multi-step processes while
maintaining clean separation of concerns.

DuraGraph is designed for teams that need to move beyond prototype AI applications
and build robust, scalable systems that can handle real-world production workloads.
    """.strip()
    )

    # Create a Markdown file
    (temp_dir / "documentation.md").write_text(
        """
# DuraGraph Documentation

## Installation

```bash
pip install duragraph
```

## Quick Start

```python
from duragraph import Graph, llm_node, entrypoint

@Graph(id="simple_agent")
class SimpleAgent:
    @entrypoint
    @llm_node(model="gpt-4o-mini")
    def process(self, state):
        return state
```

## Advanced Features

### Vector Stores

DuraGraph includes built-in support for vector databases:

- In-memory storage for development
- ChromaDB integration for persistent storage
- Custom vector store implementations

### Document Processing

The document loader system supports:

1. Text files
2. CSV data
3. JSON documents
4. Web pages
5. Directory scanning

Each loader can be combined with text splitters for optimal chunking.
    """.strip()
    )

    # Create a CSV file
    csv_content = """title,content,category,author
Getting Started with DuraGraph,"Learn the basics of DuraGraph workflow orchestration. This guide covers installation setup and your first agent.",tutorial,engineering
Advanced Vector Search,"Explore sophisticated vector search techniques using DuraGraph's embedding providers and vector stores.",advanced,data-science
Production Deployment,"Best practices for deploying DuraGraph agents in production environments with monitoring and scaling.",operations,devops
Human-in-the-Loop Workflows,"Implement workflows that seamlessly integrate human review and approval steps.",feature,product
API Reference Guide,"Complete reference for the DuraGraph Python SDK including all decorators and utilities.",reference,documentation"""

    (temp_dir / "articles.csv").write_text(csv_content)

    # Create a JSON file
    json_content = """[
  {
    "id": 1,
    "title": "Building Reliable AI Systems",
    "content": "Reliability is crucial for AI systems in production. This article explores patterns and practices for building robust AI workflows that can handle failures gracefully.",
    "tags": ["reliability", "production", "best-practices"],
    "published": "2024-01-01"
  },
  {
    "id": 2,
    "title": "Event Sourcing for AI Workflows",
    "content": "Event sourcing provides a powerful foundation for AI workflow orchestration. Learn how DuraGraph uses event sourcing to enable complete auditability and replay capabilities.",
    "tags": ["event-sourcing", "architecture", "observability"],
    "published": "2024-01-15"
  },
  {
    "id": 3,
    "title": "Scaling AI Workflows",
    "content": "As your AI applications grow, scaling becomes essential. This guide covers strategies for horizontal scaling, load balancing, and resource optimization in DuraGraph.",
    "tags": ["scaling", "performance", "architecture"],
    "published": "2024-02-01"
  }
]"""

    (temp_dir / "blog_posts.json").write_text(json_content)

    # Create a Python code file
    (temp_dir / "example_agent.py").write_text('''
"""Example DuraGraph agent demonstrating best practices."""

from duragraph import Graph, llm_node, tool_node, router_node, entrypoint, tool


@tool(description="Search for information in the knowledge base")
def search_knowledge_base(query: str, max_results: int = 5) -> str:
    """Search the knowledge base for relevant information."""
    # This would integrate with a real search system
    return f"Found {max_results} results for '{query}'"


@tool(description="Format response with proper structure")
def format_response(content: str, format_type: str = "markdown") -> str:
    """Format the response content."""
    if format_type == "markdown":
        return f"**Response:**\\n\\n{content}"
    return content


@Graph(id="knowledge_agent", description="Agent with knowledge base access")
class KnowledgeAgent:
    """An intelligent agent that can search and format responses."""

    @entrypoint
    @llm_node(model="gpt-4o-mini", temperature=0.1)
    def classify_query(self, state):
        """Classify the type of user query."""
        return state

    @router_node()
    def route_query(self, state):
        """Route based on query classification."""
        query_type = state.get("query_type", "general")
        if "search" in query_type.lower():
            return "search_and_respond"
        else:
            return "direct_response"

    @llm_node(model="gpt-4o-mini", tools=[search_knowledge_base])
    def search_and_respond(self, state):
        """Search knowledge base and generate response."""
        return state

    @llm_node(model="gpt-4o-mini")
    def direct_response(self, state):
        """Generate direct response without search."""
        return state

    @tool_node()
    def format_output(self, state):
        """Format the final response."""
        response = state.get("response", "")
        formatted = format_response(response, "markdown")
        state["formatted_response"] = formatted
        return state

    # Define the flow
    classify_query >> route_query
    search_and_respond >> format_output
    direct_response >> format_output


if __name__ == "__main__":
    agent = KnowledgeAgent()

    # Test the agent
    test_queries = [
        "What is DuraGraph?",
        "Search for information about scaling AI systems",
        "How do I install the Python SDK?",
    ]

    for query in test_queries:
        print(f"\\nQuery: {query}")
        result = agent.run({"user_query": query})
        print(f"Response: {result.output.get('formatted_response', 'No response')}")
    ''')


def demonstrate_text_splitters():
    """Demonstrate different text splitter options."""

    print("=== Text Splitter Demonstrations ===\n")

    # Sample text for splitting
    sample_text = """
    DuraGraph is an enterprise-ready AI workflow orchestration platform. It provides developers with powerful tools for building reliable, observable, and scalable AI applications.

    The platform combines event sourcing architecture with modern AI frameworks to deliver production-grade capabilities. Key features include comprehensive audit trails, built-in human-in-the-loop support, and seamless LangGraph compatibility.

    Getting started with DuraGraph is straightforward. Install the Python SDK, define your workflow using decorators, and deploy to your preferred infrastructure. The platform handles the complexity of coordination, error handling, and observability automatically.
    """

    # Character-based splitting
    print("1. Character Text Splitter:")
    char_splitter = CharacterTextSplitter(chunk_size=200, chunk_overlap=20, separator="\n\n")
    char_chunks = char_splitter.split_text(sample_text.strip())

    for i, chunk in enumerate(char_chunks):
        print(f"   Chunk {i + 1} ({len(chunk)} chars): {chunk[:50]}...")

    # Recursive splitting
    print("\n2. Recursive Character Text Splitter:")
    recursive_splitter = RecursiveCharacterTextSplitter(chunk_size=150, chunk_overlap=15)
    recursive_chunks = recursive_splitter.split_text(sample_text.strip())

    for i, chunk in enumerate(recursive_chunks):
        print(f"   Chunk {i + 1} ({len(chunk)} chars): {chunk[:50]}...")

    # Paragraph splitting
    print("\n3. Paragraph Text Splitter:")
    paragraph_splitter = ParagraphTextSplitter(chunk_size=300, chunk_overlap=25)
    paragraph_chunks = paragraph_splitter.split_text(sample_text.strip())

    for i, chunk in enumerate(paragraph_chunks):
        print(f"   Chunk {i + 1} ({len(chunk)} chars): {chunk[:50]}...")


def demonstrate_file_loaders():
    """Demonstrate file-based document loaders."""

    print("\n=== File Loader Demonstrations ===\n")

    with tempfile.TemporaryDirectory() as temp_dir:
        temp_path = Path(temp_dir)
        create_sample_files(temp_path)

        # 1. Text file loader
        print("1. Text File Loader:")
        text_loader = TextFileLoader(temp_path / "article.txt")
        text_docs = text_loader.load()

        print(f"   Loaded {len(text_docs)} document(s)")
        print(f"   Content length: {len(text_docs[0].page_content)} characters")
        print(f"   Metadata keys: {list(text_docs[0].metadata.keys())}")

        # 2. Directory loader
        print("\n2. Directory Loader:")
        dir_loader = DirectoryLoader(temp_path, glob="*.txt", loader_cls=TextFileLoader)
        dir_docs = dir_loader.load()

        print(f"   Found {len(dir_docs)} text file(s)")
        for doc in dir_docs:
            print(f"   - {doc.metadata['file_name']}: {len(doc.page_content)} chars")

        # 3. CSV loader
        print("\n3. CSV Loader:")
        csv_loader = CSVLoader(
            temp_path / "articles.csv",
            content_columns=["title", "content"],
            metadata_columns=["category", "author"],
        )
        csv_docs = csv_loader.load()

        print(f"   Loaded {len(csv_docs)} articles from CSV")
        for i, doc in enumerate(csv_docs[:2]):  # Show first 2
            print(
                f"   - Article {i + 1}: {doc.metadata.get('category', 'N/A')} by {doc.metadata.get('author', 'N/A')}"
            )

        # 4. JSON loader
        print("\n4. JSON Loader:")
        json_loader = JSONLoader(
            temp_path / "blog_posts.json",
            content_key="content",
            metadata_keys=["id", "title", "tags", "published"],
        )
        json_docs = json_loader.load()

        print(f"   Loaded {len(json_docs)} blog posts from JSON")
        for doc in json_docs:
            tags = doc.metadata.get("tags", [])
            print(f"   - {doc.metadata.get('title', 'Untitled')}: {len(tags)} tags")


def demonstrate_code_splitting():
    """Demonstrate code-specific text splitting."""

    print("\n=== Code Text Splitter Demonstration ===\n")

    with tempfile.TemporaryDirectory() as temp_dir:
        temp_path = Path(temp_dir)
        create_sample_files(temp_path)

        # Load Python code file
        code_loader = TextFileLoader(temp_path / "example_agent.py")
        code_docs = code_loader.load()

        print("Original code file:")
        print(f"   Length: {len(code_docs[0].page_content)} characters")

        # Split with code-aware splitter
        code_splitter = CodeTextSplitter(language="python", chunk_size=500, chunk_overlap=50)

        code_chunks = code_splitter.split_documents(code_docs)

        print(f"\nSplit into {len(code_chunks)} code chunks:")
        for i, chunk in enumerate(code_chunks):
            lines = chunk.page_content.split("\n")
            first_line = next((line for line in lines if line.strip()), "").strip()
            print(f"   Chunk {i + 1} ({len(chunk.page_content)} chars): {first_line[:60]}...")


def demonstrate_markdown_splitting():
    """Demonstrate Markdown-aware text splitting."""

    print("\n=== Markdown Text Splitter Demonstration ===\n")

    with tempfile.TemporaryDirectory() as temp_dir:
        temp_path = Path(temp_dir)
        create_sample_files(temp_path)

        # Load Markdown file
        md_loader = TextFileLoader(temp_path / "documentation.md")
        md_docs = md_loader.load()

        print("Original Markdown file:")
        print(f"   Length: {len(md_docs[0].page_content)} characters")

        # Split with Markdown-aware splitter
        md_splitter = MarkdownTextSplitter(chunk_size=300, chunk_overlap=30)

        md_chunks = md_splitter.split_documents(md_docs)

        print(f"\nSplit into {len(md_chunks)} Markdown chunks:")
        for i, chunk in enumerate(md_chunks):
            lines = chunk.page_content.split("\n")
            first_line = next((line for line in lines if line.strip()), "").strip()
            print(f"   Chunk {i + 1} ({len(chunk.page_content)} chars): {first_line[:50]}...")


async def demonstrate_integration_with_vectorstore():
    """Demonstrate integration with vector stores."""

    print("\n=== Integration with Vector Stores ===\n")

    with tempfile.TemporaryDirectory() as temp_dir:
        temp_path = Path(temp_dir)
        create_sample_files(temp_path)

        # Create embedding provider
        embedding_provider = create_embedding_provider("anthropic")

        # Create vector store
        vector_store = create_vectorstore("memory", embedding_function=embedding_provider)

        # Load and process all documents
        all_docs = []

        # Load from directory
        dir_loader = DirectoryLoader(
            temp_path,
            glob="*",
            exclude=["*.py"],  # Skip code files for this demo
            loader_cls=TextFileLoader,
        )
        all_docs.extend(dir_loader.load())

        # Load from CSV
        csv_loader = CSVLoader(
            temp_path / "articles.csv",
            content_columns=["title", "content"],
            metadata_columns=["category", "author"],
        )
        all_docs.extend(csv_loader.load())

        print(f"Loaded {len(all_docs)} documents from various sources")

        # Split documents for better vector search
        splitter = RecursiveCharacterTextSplitter(chunk_size=500, chunk_overlap=50)
        split_docs = splitter.split_documents(all_docs)

        print(f"Split into {len(split_docs)} chunks for vector storage")

        # Add to vector store
        doc_ids = await vector_store.aadd_documents(split_docs)
        print(f"Added {len(doc_ids)} document chunks to vector store")

        # Perform similarity search
        query = "How to get started with DuraGraph?"
        search_results = await vector_store.asimilarity_search(query, k=3)

        print(f"\nSearch results for: '{query}'")
        for i, doc in enumerate(search_results, 1):
            print(f"{i}. {doc.page_content[:100]}...")
            print(f"   Source: {doc.metadata.get('source', 'Unknown')}")
            print(
                f"   Chunk: {doc.metadata.get('chunk_index', '?')}/{doc.metadata.get('total_chunks', '?')}"
            )


def main():
    """Run all demonstrations."""

    print("DuraGraph Document Loaders and Text Splitters Demo")
    print("=" * 55)

    # Run demonstrations
    demonstrate_text_splitters()
    demonstrate_file_loaders()
    demonstrate_code_splitting()
    demonstrate_markdown_splitting()

    # Run async integration demo
    asyncio.run(demonstrate_integration_with_vectorstore())

    print("\n=== Summary ===")
    print("✅ Text splitters: Character, Recursive, Paragraph, Markdown, Code")
    print("✅ File loaders: Text, Directory, CSV, JSON")
    print("✅ Integration with vector stores and embeddings")
    print("✅ Async support for efficient processing")
    print("\nDocument loaders provide a flexible foundation for building")
    print("knowledge-based AI applications with DuraGraph!")


if __name__ == "__main__":
    main()
