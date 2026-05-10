"""DuraGraph Python SDK - Reliable AI Workflow Orchestration."""

from duragraph.client import AsyncDuraGraphClient, DuraGraphClient
from duragraph.edges import edge
from duragraph.graph import Graph
from duragraph.nodes import (
    entrypoint,
    human_node,
    llm_node,
    node,
    router_node,
    tool_node,
)
from duragraph.tools import get_global_registry, resolve_tool_calls, tool
from duragraph.types import AIMessage, HumanMessage, Message, State, StreamMode, ToolMessage

dspy_node = None

# Optional dependencies - define as None first, then import if available
create_embedding_provider = None
get_embedding_provider = None
create_vectorstore = None
get_vectorstore = None
TextFileLoader = None
DirectoryLoader = None
CharacterTextSplitter = None
RecursiveCharacterTextSplitter = None

# Import embeddings if available
try:
    from duragraph.embeddings import create_embedding_provider as _create_emb
    from duragraph.embeddings import get_provider as _get_emb

    create_embedding_provider = _create_emb
    get_embedding_provider = _get_emb
    _has_embeddings = True
except ImportError:
    _has_embeddings = False

# Import vectorstores if available
try:
    from duragraph.vectorstores import create_vectorstore as _create_vs
    from duragraph.vectorstores import get_vectorstore as _get_vs

    create_vectorstore = _create_vs
    get_vectorstore = _get_vs
    _has_vectorstores = True
except ImportError:
    _has_vectorstores = False

# Import document loaders if available
try:
    from duragraph.document_loaders import CharacterTextSplitter as _CharTextSplitter
    from duragraph.document_loaders import DirectoryLoader as _DirectoryLoader
    from duragraph.document_loaders import RecursiveCharacterTextSplitter as _RecCharTextSplitter
    from duragraph.document_loaders import TextFileLoader as _TextFileLoader

    TextFileLoader = _TextFileLoader
    DirectoryLoader = _DirectoryLoader
    CharacterTextSplitter = _CharTextSplitter
    RecursiveCharacterTextSplitter = _RecCharTextSplitter
    _has_document_loaders = True
except ImportError:
    _has_document_loaders = False

try:
    from duragraph.nodes import dspy_node as _dspy_node

    dspy_node = _dspy_node
    _has_dspy = True
except ImportError:
    _has_dspy = False

__version__ = "0.3.1"

__all__ = [
    # Client
    "DuraGraphClient",
    "AsyncDuraGraphClient",
    # Graph
    "Graph",
    # Node decorators
    "node",
    "llm_node",
    "tool_node",
    "router_node",
    "human_node",
    "entrypoint",
    # Edge
    "edge",
    # Tools
    "tool",
    "get_global_registry",
    "resolve_tool_calls",
    # Types
    "State",
    "StreamMode",
    "Message",
    "HumanMessage",
    "AIMessage",
    "ToolMessage",
]

if _has_dspy:
    __all__.append("dspy_node")

# Add embeddings to exports if available
if _has_embeddings:
    __all__.extend(
        [
            "create_embedding_provider",
            "get_embedding_provider",
        ]
    )

# Add vectorstores to exports if available
if _has_vectorstores:
    __all__.extend(
        [
            "create_vectorstore",
            "get_vectorstore",
        ]
    )

# Add document loaders to exports if available
if _has_document_loaders:
    __all__.extend(
        [
            "TextFileLoader",
            "DirectoryLoader",
            "CharacterTextSplitter",
            "RecursiveCharacterTextSplitter",
        ]
    )
