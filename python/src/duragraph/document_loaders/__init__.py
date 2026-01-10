"""Document loaders and text processing utilities for DuraGraph."""

from duragraph.document_loaders.base import BaseDocumentLoader, DocumentLoader
from duragraph.document_loaders.text_splitter import (
    CharacterTextSplitter,
    CodeTextSplitter,
    MarkdownTextSplitter,
    ParagraphTextSplitter,
    RecursiveCharacterTextSplitter,
    TextSplitter,
)

# Optional imports - define as None first
CSVLoader = None
DirectoryLoader = None
JSONLoader = None
TextFileLoader = None
SitemapLoader = None
URLListLoader = None
WebPageLoader = None
TokenTextSplitter = None

# Import file loaders
try:
    from duragraph.document_loaders.file import CSVLoader as _CSVLoader
    from duragraph.document_loaders.file import DirectoryLoader as _DirectoryLoader
    from duragraph.document_loaders.file import JSONLoader as _JSONLoader
    from duragraph.document_loaders.file import TextFileLoader as _TextFileLoader

    CSVLoader = _CSVLoader
    DirectoryLoader = _DirectoryLoader
    JSONLoader = _JSONLoader
    TextFileLoader = _TextFileLoader
    _has_file_loaders = True
except ImportError:
    _has_file_loaders = False

# Import web loaders
try:
    from duragraph.document_loaders.web import SitemapLoader as _SitemapLoader
    from duragraph.document_loaders.web import URLListLoader as _URLListLoader
    from duragraph.document_loaders.web import WebPageLoader as _WebPageLoader

    SitemapLoader = _SitemapLoader
    URLListLoader = _URLListLoader
    WebPageLoader = _WebPageLoader
    _has_web_loaders = True
except ImportError:
    _has_web_loaders = False

# Import token splitter if available
try:
    from duragraph.document_loaders.text_splitter import (
        TokenTextSplitter as _TokenTextSplitter,
    )

    TokenTextSplitter = _TokenTextSplitter
    _has_token_splitter = True
except ImportError:
    _has_token_splitter = False

__all__ = [
    # Base classes
    "DocumentLoader",
    "BaseDocumentLoader",
    # Text splitters
    "TextSplitter",
    "CharacterTextSplitter",
    "RecursiveCharacterTextSplitter",
    "ParagraphTextSplitter",
    "MarkdownTextSplitter",
    "CodeTextSplitter",
]

# Add file loaders if available
if _has_file_loaders:
    __all__.extend(
        [
            "TextFileLoader",
            "DirectoryLoader",
            "CSVLoader",
            "JSONLoader",
        ]
    )

# Add web loaders if available
if _has_web_loaders:
    __all__.extend(
        [
            "WebPageLoader",
            "SitemapLoader",
            "URLListLoader",
        ]
    )

# Add token splitter if available
if _has_token_splitter:
    __all__.extend(
        [
            "TokenTextSplitter",
        ]
    )
