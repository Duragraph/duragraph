"""Example: Complete RAG (Retrieval-Augmented Generation) system using DuraGraph."""

import asyncio
import tempfile
from pathlib import Path
from typing import Any

from duragraph import Graph, entrypoint, llm_node, router_node, tool, tool_node
from duragraph.document_loaders import (
    DirectoryLoader,
    RecursiveCharacterTextSplitter,
    TextFileLoader,
)
from duragraph.embeddings import create_embedding_provider
from duragraph.vectorstores import Document, create_vectorstore

# Global knowledge base (would be persistent in real application)
KNOWLEDGE_BASE = None


@tool(description="Search the knowledge base for relevant information")
def search_knowledge_base(query: str, max_results: int = 5) -> str:
    """Search the knowledge base for information relevant to the query."""
    global KNOWLEDGE_BASE

    if KNOWLEDGE_BASE is None:
        return "Knowledge base not available. Please set up the knowledge base first."

    try:
        # Perform similarity search
        results = KNOWLEDGE_BASE.similarity_search(query, k=max_results)

        if not results:
            return "No relevant information found in the knowledge base."

        # Format results
        context_parts = []
        for i, doc in enumerate(results, 1):
            source = doc.metadata.get("source", "Unknown")
            content = doc.page_content.strip()
            context_parts.append(f"[{i}] Source: {source}\n{content}")

        return "\n\n".join(context_parts)

    except Exception as e:
        return f"Error searching knowledge base: {str(e)}"


@tool(description="Add new documents to the knowledge base")
def add_to_knowledge_base(content: str, source: str = "user_input") -> str:
    """Add new content to the knowledge base."""
    global KNOWLEDGE_BASE

    if KNOWLEDGE_BASE is None:
        return "Knowledge base not available."

    try:
        # Create document
        doc = Document(page_content=content, metadata={"source": source, "added_by": "user"})

        # Add to vector store
        KNOWLEDGE_BASE.add_documents([doc])

        return f"Successfully added document from '{source}' to knowledge base."

    except Exception as e:
        return f"Error adding to knowledge base: {str(e)}"


@tool(description="Get information about the knowledge base")
def get_knowledge_base_info() -> str:
    """Get statistics and information about the current knowledge base."""
    global KNOWLEDGE_BASE

    if KNOWLEDGE_BASE is None:
        return "Knowledge base not available."

    try:
        if hasattr(KNOWLEDGE_BASE, "get_document_count"):
            doc_count = KNOWLEDGE_BASE.get_document_count()
            return f"Knowledge base contains {doc_count} documents."
        else:
            return "Knowledge base is available but document count is unknown."
    except Exception as e:
        return f"Error getting knowledge base info: {str(e)}"


@Graph(id="rag_agent", description="Advanced RAG system with document management")
class RAGAgent:
    """A comprehensive RAG (Retrieval-Augmented Generation) agent."""

    @entrypoint
    @llm_node(
        model="gpt-4o-mini",
        temperature=0.1,
        system_prompt="""You are a helpful AI assistant with access to a knowledge base.

        Analyze the user's query and determine the appropriate action:
        - "search": The user is asking for information that might be in the knowledge base
        - "add": The user wants to add information to the knowledge base
        - "info": The user wants information about the knowledge base itself
        - "direct": The user is asking a general question that doesn't require the knowledge base

        Respond with just the action type.""",
    )
    def classify_intent(self, state: dict[str, Any]) -> dict[str, Any]:
        """Classify what the user wants to do."""
        user_query = state.get("user_query", "")
        messages = [{"role": "user", "content": f"Classify this query: {user_query}"}]
        state["messages"] = messages
        return state

    @router_node()
    def route_by_intent(self, state: dict[str, Any]) -> str:
        """Route based on the classified intent."""
        messages = state.get("messages", [])
        if messages and messages[-1]["role"] == "assistant":
            intent = messages[-1]["content"].lower().strip()

            if "search" in intent:
                return "search_and_answer"
            elif "add" in intent:
                return "add_to_kb"
            elif "info" in intent:
                return "kb_info"
            else:
                return "direct_answer"

        # Default to search if unclear
        return "search_and_answer"

    @llm_node(
        model="gpt-4o-mini",
        tools=[search_knowledge_base],
        temperature=0.3,
        system_prompt="""You are a helpful AI assistant. When answering questions:

        1. First search the knowledge base for relevant information
        2. Use the search results to provide accurate, well-sourced answers
        3. If the knowledge base doesn't contain relevant information, say so clearly
        4. Always cite your sources when using information from the knowledge base
        5. Provide comprehensive answers that directly address the user's question

        Format your response clearly and include source citations.""",
    )
    def search_and_answer(self, state: dict[str, Any]) -> dict[str, Any]:
        """Search the knowledge base and provide a comprehensive answer."""
        user_query = state.get("user_query", "")
        messages = [{"role": "user", "content": user_query}]
        state["messages"] = messages
        return state

    @llm_node(
        model="gpt-4o-mini",
        tools=[add_to_knowledge_base],
        system_prompt="""You help users add information to the knowledge base.

        Extract the content the user wants to add and use the add_to_knowledge_base tool.
        Provide a clear confirmation of what was added.""",
    )
    def add_to_kb(self, state: dict[str, Any]) -> dict[str, Any]:
        """Add user-provided information to the knowledge base."""
        user_query = state.get("user_query", "")
        messages = [{"role": "user", "content": user_query}]
        state["messages"] = messages
        return state

    @tool_node(timeout=10.0)
    def kb_info(self, state: dict[str, Any]) -> dict[str, Any]:
        """Get and format knowledge base information."""
        info = get_knowledge_base_info()

        response = f"Knowledge Base Information:\n{info}"
        state["response"] = response

        # Format as messages
        user_query = state.get("user_query", "")
        state["messages"] = [
            {"role": "user", "content": user_query},
            {"role": "assistant", "content": response},
        ]

        return state

    @llm_node(
        model="gpt-4o-mini",
        temperature=0.7,
        system_prompt="""You are a helpful AI assistant. Answer the user's question directly
        without using the knowledge base. Be helpful and informative.""",
    )
    def direct_answer(self, state: dict[str, Any]) -> dict[str, Any]:
        """Provide a direct answer without using the knowledge base."""
        user_query = state.get("user_query", "")
        messages = [{"role": "user", "content": user_query}]
        state["messages"] = messages
        return state

    @tool_node()
    def format_response(self, state: dict[str, Any]) -> dict[str, Any]:
        """Format the final response for the user."""
        messages = state.get("messages", [])

        if messages and messages[-1]["role"] == "assistant":
            response = messages[-1]["content"]
        else:
            response = state.get("response", "I apologize, but I couldn't process your request.")

        state["final_response"] = response
        return state

    # Define the conversation flow
    classify_intent >> route_by_intent

    # All paths converge on formatting
    search_and_answer >> format_response
    add_to_kb >> format_response
    kb_info >> format_response
    direct_answer >> format_response


async def setup_knowledge_base() -> None:
    """Set up the knowledge base with sample documents."""
    global KNOWLEDGE_BASE

    print("Setting up knowledge base...")

    # Create embedding provider
    embedding_provider = create_embedding_provider("anthropic")

    # Create vector store
    KNOWLEDGE_BASE = create_vectorstore("memory", embedding_function=embedding_provider)

    # Create sample documents in a temporary directory
    with tempfile.TemporaryDirectory() as temp_dir:
        temp_path = Path(temp_dir)

        # Create sample knowledge documents
        docs_content = {
            "duragraph_overview.txt": """
            DuraGraph is an enterprise-ready AI workflow orchestration platform. It provides:

            - Event-sourced architecture for complete audit trails
            - Built-in support for human-in-the-loop workflows
            - Comprehensive observability and monitoring
            - LangGraph Cloud compatibility for seamless migration
            - Production-grade reliability and scalability

            DuraGraph uses Domain-Driven Design patterns with CQRS and event sourcing
            to ensure reliable, observable AI workflow execution.
            """,
            "installation_guide.txt": """
            Installing DuraGraph:

            1. Install the Python SDK:
               pip install duragraph

            2. For additional features:
               pip install duragraph[all]  # All optional dependencies
               pip install duragraph[openai]  # OpenAI support
               pip install duragraph[anthropic]  # Anthropic support

            3. Verify installation:
               python -c "import duragraph; print('DuraGraph installed successfully')"

            4. Create your first agent using the CLI:
               duragraph init my-agent --template minimal
            """,
            "vector_stores.txt": """
            DuraGraph Vector Store Support:

            DuraGraph includes built-in support for vector databases:

            - InMemoryVectorStore: For development and testing
            - ChromaVectorStore: For persistent storage with ChromaDB
            - Custom implementations: Extend VectorStore base class

            Vector stores integrate seamlessly with embedding providers
            to enable semantic search and retrieval-augmented generation.

            Example usage:
            from duragraph.vectorstores import create_vectorstore
            store = create_vectorstore("memory", embedding_function=embedder)
            """,
            "tool_system.txt": """
            DuraGraph Tool System:

            The @tool decorator enables function calling in LLM nodes:

            @tool(description="Search for information")
            def search_api(query: str) -> str:
                return search_results

            @llm_node(model="gpt-4o-mini", tools=[search_api])
            def process_with_tools(self, state):
                return state

            Tools are automatically converted to JSON schemas for LLM function calling.
            The tool registry manages execution and error handling.
            """,
            "deployment.txt": """
            Deploying DuraGraph Agents:

            Local Development:
            - duragraph dev agent.py  # Hot reload development
            - python agent.py  # Direct execution

            Production Deployment:
            - duragraph deploy agent.py --control-plane http://server:8081
            - Use worker.serve() method in code
            - Deploy with Docker containers
            - Connect to DuraGraph control plane

            The control plane handles orchestration, scaling, and monitoring.
            Workers register capabilities and process assigned tasks.
            """,
        }

        # Write documents to files
        for filename, content in docs_content.items():
            (temp_path / filename).write_text(content.strip())

        # Load documents using DuraGraph document loaders
        loader = DirectoryLoader(temp_path, glob="*.txt", loader_cls=TextFileLoader)

        documents = loader.load()
        print(f"Loaded {len(documents)} documents")

        # Split documents into chunks
        splitter = RecursiveCharacterTextSplitter(chunk_size=300, chunk_overlap=50)

        split_docs = splitter.split_documents(documents)
        print(f"Split into {len(split_docs)} chunks")

        # Add to vector store
        doc_ids = await KNOWLEDGE_BASE.aadd_documents(split_docs)
        print(f"Added {len(doc_ids)} document chunks to knowledge base")

    print("✅ Knowledge base setup complete!")


async def main():
    """Run the RAG agent demonstration."""

    print("=== DuraGraph RAG Agent Demo ===\n")

    # Set up knowledge base
    await setup_knowledge_base()

    # Create the RAG agent
    rag_agent = RAGAgent()

    # Test queries that demonstrate different capabilities
    test_queries = [
        # Search queries
        "What is DuraGraph and what are its key features?",
        "How do I install DuraGraph with all optional dependencies?",
        "Tell me about the vector store support in DuraGraph",
        "How does the tool system work?",
        # Knowledge base info query
        "What information is available in the knowledge base?",
        # Add to knowledge base
        "Add this to the knowledge base: DuraGraph supports async execution with asyncio for improved performance in I/O-bound operations.",
        # Direct answer (no KB needed)
        "What is the capital of France?",
        # Search for newly added info
        "Tell me about async execution in DuraGraph",
    ]

    print("🤖 Starting RAG agent interactions...\n")

    for i, query in enumerate(test_queries, 1):
        print(f"Query {i}: {query}")
        print("-" * 60)

        try:
            # Run the agent
            result = await rag_agent.arun({"user_query": query})

            # Get the response
            response = result.output.get("final_response", "No response generated")
            print(f"Agent: {response}")

            # Show execution path
            nodes_executed = result.nodes_executed
            print(f"📊 Execution path: {' → '.join(nodes_executed)}")

        except Exception as e:
            print(f"❌ Error: {e}")

        print("\n" + "=" * 80 + "\n")

    # Demonstrate streaming capability
    print("🔄 Demonstrating streaming response...\n")

    query = "How do I deploy a DuraGraph agent to production?"
    print(f"Query: {query}")
    print("-" * 60)
    print("Streaming response:")

    async for event in rag_agent.stream({"user_query": query}):
        if event.type == "node_completed":
            print(f"✅ Completed: {event.node_id}")
        elif event.type == "run_completed":
            response = event.data.get("output", {}).get("final_response", "")
            print(f"\nFinal response: {response}")


def demonstrate_knowledge_base_management():
    """Demonstrate knowledge base management features."""

    print("\n=== Knowledge Base Management Demo ===\n")

    # Show current state
    info = get_knowledge_base_info()
    print(f"Current state: {info}")

    # Add some content
    new_content = """
    DuraGraph CLI Commands:

    - duragraph init <name>: Create new project
    - duragraph dev <file>: Development mode with hot reload
    - duragraph deploy <file>: Deploy to control plane
    - duragraph visualize <file>: Generate graph visualizations

    Each command supports various options for customization.
    """

    result = add_to_knowledge_base(new_content, "cli_documentation")
    print(f"Add result: {result}")

    # Search for the new content
    search_result = search_knowledge_base("CLI commands", max_results=2)
    print(f"Search result:\n{search_result}")


if __name__ == "__main__":
    # Run the main demo
    asyncio.run(main())

    # Run knowledge base management demo
    demonstrate_knowledge_base_management()

    print("\n🎉 RAG Agent Demo Complete!")
    print("\nThis example demonstrates:")
    print("✅ Intelligent query routing based on intent")
    print("✅ Vector similarity search with embeddings")
    print("✅ Tool-based knowledge base management")
    print("✅ Multiple response strategies")
    print("✅ Async execution and streaming")
    print("✅ Document loading and chunking")
    print("✅ Comprehensive error handling")
    print("\nThe RAG agent showcases how DuraGraph enables")
    print("sophisticated AI applications with reliable orchestration!")
