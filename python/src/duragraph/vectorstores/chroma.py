"""ChromaDB vector store integration."""

import uuid
from typing import Any

from duragraph.vectorstores.base import Document, VectorStore


class ChromaVectorStore(VectorStore):
    """ChromaDB vector store implementation."""

    def __init__(
        self,
        collection_name: str = "duragraph_collection",
        embedding_function: Any = None,
        persist_directory: str | None = None,
        client_settings: dict[str, Any] | None = None,
        **kwargs: Any,
    ):
        """Initialize ChromaDB vector store.

        Args:
            collection_name: Name of the Chroma collection.
            embedding_function: Function to generate embeddings from text.
            persist_directory: Directory to persist Chroma database.
            client_settings: Additional Chroma client settings.
            **kwargs: Additional configuration.
        """
        super().__init__(embedding_function, **kwargs)
        self.collection_name = collection_name
        self.persist_directory = persist_directory
        self.client_settings = client_settings or {}

        self._client = None
        self._collection = None

    def _get_client(self):
        """Get or create ChromaDB client."""
        if self._client is None:
            try:
                import chromadb
                from chromadb.config import Settings
            except ImportError as e:
                raise ImportError("ChromaDB not found. Install with: pip install chromadb") from e

            if self.persist_directory:
                # Persistent client
                self._client = chromadb.PersistentClient(
                    path=self.persist_directory, settings=Settings(**self.client_settings)
                )
            else:
                # In-memory client
                self._client = chromadb.Client(settings=Settings(**self.client_settings))

        return self._client

    def _get_collection(self):
        """Get or create ChromaDB collection."""
        if self._collection is None:
            client = self._get_client()

            # Create embedding function for Chroma
            chroma_embedding_function = None
            if self.embedding_function:
                if hasattr(self.embedding_function, "embed_documents"):
                    # LangChain-style embedding function
                    chroma_embedding_function = self.embedding_function
                else:
                    # Custom embedding function wrapper
                    class CustomEmbeddingFunction:
                        def __init__(self, func):
                            self.func = func

                        def __call__(self, input):
                            if hasattr(self.func, "embed_documents"):
                                return self.func.embed_documents(input)
                            elif hasattr(self.func, "embed_query"):
                                if isinstance(input, list):
                                    return [self.func.embed_query(text) for text in input]
                                else:
                                    return [self.func.embed_query(input)]
                            else:
                                # Assume it's a callable
                                if isinstance(input, list):
                                    return [self.func(text) for text in input]
                                else:
                                    return [self.func(input)]

                    chroma_embedding_function = CustomEmbeddingFunction(self.embedding_function)

            try:
                # Try to get existing collection
                self._collection = client.get_collection(
                    name=self.collection_name, embedding_function=chroma_embedding_function
                )
            except Exception:
                # Create new collection
                self._collection = client.create_collection(
                    name=self.collection_name, embedding_function=chroma_embedding_function
                )

        return self._collection

    async def aadd_documents(
        self, documents: list[Document], ids: list[str] | None = None, **kwargs: Any
    ) -> list[str]:
        """Add documents to ChromaDB asynchronously."""
        if ids is None:
            ids = [str(uuid.uuid4()) for _ in documents]

        if len(ids) != len(documents):
            raise ValueError("Number of ids must match number of documents")

        collection = self._get_collection()

        # Prepare data for Chroma
        texts = [doc.page_content for doc in documents]
        metadatas = [doc.metadata for doc in documents]

        # Add to collection
        collection.add(
            ids=ids,
            documents=texts,
            metadatas=metadatas,
        )

        return ids

    async def asimilarity_search(
        self, query: str, k: int = 4, filter: dict[str, Any] | None = None, **kwargs: Any
    ) -> list[Document]:
        """Search for similar documents asynchronously."""
        results = await self.asimilarity_search_with_score(query, k, filter, **kwargs)
        return [doc for doc, _ in results]

    async def asimilarity_search_with_score(
        self, query: str, k: int = 4, filter: dict[str, Any] | None = None, **kwargs: Any
    ) -> list[tuple[Document, float]]:
        """Search for similar documents with scores asynchronously."""
        collection = self._get_collection()

        # Convert filter to Chroma format
        where_filter = None
        if filter:
            where_filter = self._convert_filter(filter)

        # Query collection
        results = collection.query(
            query_texts=[query],
            n_results=k,
            where=where_filter,
            include=["documents", "metadatas", "distances"],
        )

        # Convert results
        documents = []
        distances = results["distances"][0] if results["distances"] else []
        docs = results["documents"][0] if results["documents"] else []
        metadatas = results["metadatas"][0] if results["metadatas"] else []

        for i, doc_text in enumerate(docs):
            metadata = metadatas[i] if i < len(metadatas) else {}
            distance = distances[i] if i < len(distances) else 0.0

            # Convert distance to similarity score (Chroma uses distance)
            # Assume cosine distance: similarity = 1 - distance
            similarity = max(0.0, 1.0 - distance)

            document = Document(page_content=doc_text, metadata=metadata)
            documents.append((document, similarity))

        return documents

    async def asimilarity_search_with_score_by_vector(
        self,
        embedding: list[float],
        k: int = 4,
        filter: dict[str, Any] | None = None,
        **kwargs: Any,
    ) -> list[tuple[Document, float]]:
        """Search for similar documents by embedding with scores asynchronously."""
        collection = self._get_collection()

        # Convert filter to Chroma format
        where_filter = None
        if filter:
            where_filter = self._convert_filter(filter)

        # Query collection with embedding
        results = collection.query(
            query_embeddings=[embedding],
            n_results=k,
            where=where_filter,
            include=["documents", "metadatas", "distances"],
        )

        # Convert results
        documents = []
        distances = results["distances"][0] if results["distances"] else []
        docs = results["documents"][0] if results["documents"] else []
        metadatas = results["metadatas"][0] if results["metadatas"] else []

        for i, doc_text in enumerate(docs):
            metadata = metadatas[i] if i < len(metadatas) else {}
            distance = distances[i] if i < len(distances) else 0.0

            # Convert distance to similarity score
            similarity = max(0.0, 1.0 - distance)

            document = Document(page_content=doc_text, metadata=metadata)
            documents.append((document, similarity))

        return documents

    def _convert_filter(self, filter: dict[str, Any]) -> dict[str, Any]:
        """Convert generic filter to ChromaDB format."""
        chroma_filter = {}

        for key, value in filter.items():
            if isinstance(value, dict):
                # Handle complex filters
                for op, filter_value in value.items():
                    if op == "$eq":
                        chroma_filter[key] = {"$eq": filter_value}
                    elif op == "$ne":
                        chroma_filter[key] = {"$ne": filter_value}
                    elif op == "$in":
                        chroma_filter[key] = {"$in": filter_value}
                    elif op == "$nin":
                        chroma_filter[key] = {"$nin": filter_value}
                    elif op in ["$gt", "$gte", "$lt", "$lte"]:
                        chroma_filter[key] = {op: filter_value}
                    # Add other operators as needed
            else:
                # Simple equality
                chroma_filter[key] = value

        return chroma_filter

    async def adelete(
        self,
        ids: list[str] | None = None,
        filter: dict[str, Any] | None = None,
        **kwargs: Any,
    ) -> bool:
        """Delete documents from ChromaDB asynchronously."""
        collection = self._get_collection()

        try:
            if ids:
                # Delete by IDs
                collection.delete(ids=ids)
            elif filter:
                # Delete by filter
                where_filter = self._convert_filter(filter)
                collection.delete(where=where_filter)
            else:
                # Clear all - reset collection
                client = self._get_client()
                client.delete_collection(self.collection_name)
                self._collection = None  # Will be recreated on next access

            return True
        except Exception:
            return False

    def get_collection_info(self) -> dict[str, Any]:
        """Get information about the ChromaDB collection."""
        collection = self._get_collection()
        return {
            "name": collection.name,
            "count": collection.count(),
            "metadata": collection.metadata,
        }
