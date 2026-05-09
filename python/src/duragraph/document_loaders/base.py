"""Base classes for document loaders."""

from abc import ABC, abstractmethod
from collections.abc import AsyncIterator, Iterator
from typing import Any

from duragraph.vectorstores import Document


class DocumentLoader(ABC):
    """Abstract base class for document loaders."""

    @abstractmethod
    def load(self) -> list[Document]:
        """Load documents synchronously.

        Returns:
            List of loaded documents.
        """

    @abstractmethod
    async def aload(self) -> list[Document]:
        """Load documents asynchronously.

        Returns:
            List of loaded documents.
        """

    def lazy_load(self) -> Iterator[Document]:
        """Load documents lazily as an iterator.

        Default implementation loads all documents and yields them.
        Subclasses can override for true lazy loading.

        Yields:
            Documents one by one.
        """
        documents = self.load()
        yield from documents

    async def alazy_load(self) -> AsyncIterator[Document]:
        """Load documents lazily as an async iterator.

        Default implementation loads all documents and yields them.
        Subclasses can override for true async lazy loading.

        Yields:
            Documents one by one.
        """
        documents = await self.aload()
        for doc in documents:
            yield doc


class BaseDocumentLoader(DocumentLoader):
    """Base implementation with common functionality."""

    def __init__(self, **kwargs: Any):
        """Initialize loader with configuration."""
        self.config = kwargs

    async def aload(self) -> list[Document]:
        """Default async implementation calls sync load."""
        import asyncio

        return await asyncio.get_event_loop().run_in_executor(None, self.load)
