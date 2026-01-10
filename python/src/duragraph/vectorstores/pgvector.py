"""PostgreSQL pgvector store integration."""

import json
import uuid
from collections.abc import Sequence
from typing import Any

from duragraph.vectorstores.base import Document, EmbeddingProvider, SearchResult, VectorStore

try:
    import psycopg
    from psycopg.rows import dict_row

    HAS_PSYCOPG = True
except ImportError:
    HAS_PSYCOPG = False
    psycopg = None  # type: ignore


class PgVectorStore(VectorStore):
    """PostgreSQL pgvector store.

    Uses the pgvector extension for vector similarity search in PostgreSQL.
    Works with standard PostgreSQL, Neon, Supabase, and other Postgres providers.

    Example:
        ```python
        from duragraph.vectorstores import PgVectorStore
        from duragraph.embeddings import OpenAIEmbeddings

        # Standard PostgreSQL
        store = PgVectorStore(
            connection_string="postgresql://user:pass@localhost/db",
            table="documents",
            embedding_provider=OpenAIEmbeddings(),
        )

        # Neon serverless
        store = PgVectorStore(
            connection_string="postgresql://user:pass@xxx.neon.tech/db",
            table="documents",
            embedding_provider=OpenAIEmbeddings(),
        )

        # Add documents
        docs = [Document(content="Hello world", metadata={"type": "greeting"})]
        await store.add(docs)

        # Search
        results = await store.search("greeting", k=5)
        ```
    """

    def __init__(
        self,
        connection_string: str,
        table: str = "documents",
        *,
        embedding_provider: EmbeddingProvider | None = None,
        dimension: int | None = None,
        distance_strategy: str = "cosine",
        **kwargs: Any,
    ) -> None:
        """Initialize pgvector store.

        Args:
            connection_string: PostgreSQL connection string.
            table: Table name for storing documents.
            embedding_provider: Embedding provider for automatic embedding.
            dimension: Vector dimension (required for table creation).
            distance_strategy: Distance strategy (cosine, l2, inner_product).
            **kwargs: Additional configuration.
        """
        if not HAS_PSYCOPG:
            raise ImportError(
                "psycopg package not installed. "
                "Install with: pip install duragraph-python[pgvector]"
            )

        super().__init__(**kwargs)

        self._connection_string = connection_string
        self._table = table
        self._embedding_provider = embedding_provider
        self._dimension = dimension
        self._distance_strategy = distance_strategy

        # Distance operator map
        self._distance_ops = {
            "cosine": "<=>",  # Cosine distance
            "l2": "<->",  # Euclidean distance
            "inner_product": "<#>",  # Negative inner product
        }

        # Initialize connection pool
        self._conn: psycopg.Connection | None = None

    async def _get_connection(self) -> psycopg.AsyncConnection:
        """Get or create async connection."""
        if self._conn is None or self._conn.closed:
            self._conn = await psycopg.AsyncConnection.connect(
                self._connection_string,
                row_factory=dict_row,
            )
            await self._ensure_table()
        return self._conn

    async def _ensure_table(self) -> None:
        """Ensure the table and pgvector extension exist."""
        conn = self._conn
        if not conn:
            return

        # Determine dimension
        dim = self._dimension
        if not dim and self._embedding_provider:
            dim = self._embedding_provider.dimension
        if not dim:
            raise ValueError(
                "dimension required to create table. "
                "Provide dimension or use an embedding provider with known dimension."
            )

        async with conn.cursor() as cur:
            # Create pgvector extension
            await cur.execute("CREATE EXTENSION IF NOT EXISTS vector")

            # Create table
            await cur.execute(
                f"""
                CREATE TABLE IF NOT EXISTS {self._table} (
                    id TEXT PRIMARY KEY,
                    content TEXT NOT NULL,
                    metadata JSONB DEFAULT '{{}}',
                    embedding vector({dim})
                )
                """
            )

            # Create index for similarity search
            index_name = f"{self._table}_embedding_idx"
            op_class = {
                "cosine": "vector_cosine_ops",
                "l2": "vector_l2_ops",
                "inner_product": "vector_ip_ops",
            }.get(self._distance_strategy, "vector_cosine_ops")

            await cur.execute(
                f"""
                CREATE INDEX IF NOT EXISTS {index_name}
                ON {self._table}
                USING ivfflat (embedding {op_class})
                WITH (lists = 100)
                """
            )

            await conn.commit()

    async def add(
        self,
        documents: Sequence[Document],
        *,
        embeddings: Sequence[Sequence[float]] | None = None,
        ids: Sequence[str] | None = None,
    ) -> list[str]:
        """Add documents to pgvector."""
        if not documents:
            return []

        conn = await self._get_connection()

        # Generate IDs if not provided
        doc_ids = list(ids) if ids else [doc.id or str(uuid.uuid4()) for doc in documents]

        # Generate embeddings if needed
        if embeddings is None:
            if not self._embedding_provider:
                raise ValueError("embeddings required when no embedding_provider is configured")
            contents = [doc.content for doc in documents]
            embedding_list = await self._embedding_provider.embed(contents)
        else:
            embedding_list = [list(e) for e in embeddings]

        # Insert documents
        async with conn.cursor() as cur:
            for doc_id, doc, emb in zip(doc_ids, documents, embedding_list, strict=True):
                embedding_str = f"[{','.join(str(x) for x in emb)}]"

                await cur.execute(
                    f"""
                    INSERT INTO {self._table} (id, content, metadata, embedding)
                    VALUES (%s, %s, %s, %s::vector)
                    ON CONFLICT (id) DO UPDATE SET
                        content = EXCLUDED.content,
                        metadata = EXCLUDED.metadata,
                        embedding = EXCLUDED.embedding
                    """,
                    (doc_id, doc.content, json.dumps(doc.metadata), embedding_str),
                )

            await conn.commit()

        return doc_ids

    async def search(
        self,
        query: str | list[float],
        *,
        k: int = 10,
        filter: dict[str, Any] | None = None,
    ) -> list[SearchResult]:
        """Search for similar documents in pgvector."""
        conn = await self._get_connection()

        # Get query embedding
        if isinstance(query, str):
            if not self._embedding_provider:
                raise ValueError("embedding_provider required for text queries")
            query_embedding = await self._embedding_provider.embed_query(query)
        else:
            query_embedding = query

        embedding_str = f"[{','.join(str(x) for x in query_embedding)}]"
        distance_op = self._distance_ops.get(self._distance_strategy, "<=>")

        # Build query with optional filter
        where_clause = ""
        params: list[Any] = [embedding_str]

        if filter:
            conditions = []
            for key, value in filter.items():
                conditions.append(f"metadata->>'{key}' = %s")
                params.append(str(value))
            where_clause = "WHERE " + " AND ".join(conditions)

        params.append(k)

        async with conn.cursor() as cur:
            await cur.execute(
                f"""
                SELECT id, content, metadata,
                       embedding {distance_op} %s::vector AS distance
                FROM {self._table}
                {where_clause}
                ORDER BY embedding {distance_op} %s::vector
                LIMIT %s
                """,
                [embedding_str] + params,
            )

            rows = await cur.fetchall()

        # Convert to SearchResult
        search_results = []
        for row in rows:
            # Convert distance to similarity score
            distance = row["distance"]
            if self._distance_strategy == "cosine":
                score = 1.0 - distance  # Cosine distance is 1 - similarity
            else:
                score = 1.0 / (1.0 + distance)

            metadata = (
                row["metadata"]
                if isinstance(row["metadata"], dict)
                else json.loads(row["metadata"] or "{}")
            )

            search_results.append(
                SearchResult(
                    document=Document(
                        content=row["content"],
                        metadata=metadata,
                        id=row["id"],
                    ),
                    score=score,
                    id=row["id"],
                )
            )

        return search_results

    async def delete(
        self,
        ids: Sequence[str] | None = None,
        *,
        filter: dict[str, Any] | None = None,
    ) -> int:
        """Delete documents from pgvector."""
        if not ids and not filter:
            return 0

        conn = await self._get_connection()

        async with conn.cursor() as cur:
            if ids:
                await cur.execute(
                    f"DELETE FROM {self._table} WHERE id = ANY(%s)",
                    (list(ids),),
                )
            elif filter:
                conditions = []
                params = []
                for key, value in filter.items():
                    conditions.append(f"metadata->>'{key}' = %s")
                    params.append(str(value))

                await cur.execute(
                    f"DELETE FROM {self._table} WHERE {' AND '.join(conditions)}",
                    params,
                )

            count = cur.rowcount
            await conn.commit()

        return count

    async def get(self, ids: Sequence[str]) -> list[Document]:
        """Get documents by ID from pgvector."""
        if not ids:
            return []

        conn = await self._get_connection()

        async with conn.cursor() as cur:
            await cur.execute(
                f"SELECT id, content, metadata, embedding FROM {self._table} WHERE id = ANY(%s)",
                (list(ids),),
            )

            rows = await cur.fetchall()

        documents = []
        for row in rows:
            metadata = (
                row["metadata"]
                if isinstance(row["metadata"], dict)
                else json.loads(row["metadata"] or "{}")
            )

            # Parse embedding from string if needed
            embedding = None
            if row["embedding"]:
                if isinstance(row["embedding"], str):
                    # Parse "[1.0, 2.0, ...]" format
                    embedding = [float(x) for x in row["embedding"].strip("[]").split(",")]
                else:
                    embedding = list(row["embedding"])

            documents.append(
                Document(
                    content=row["content"],
                    metadata=metadata,
                    id=row["id"],
                    embedding=embedding,
                )
            )

        return documents

    async def clear(self) -> int:
        """Clear all documents from pgvector table."""
        conn = await self._get_connection()

        async with conn.cursor() as cur:
            await cur.execute(f"SELECT COUNT(*) as count FROM {self._table}")
            result = await cur.fetchone()
            count = result["count"] if result else 0

            await cur.execute(f"TRUNCATE TABLE {self._table}")
            await conn.commit()

        return count

    async def count(self) -> int:
        """Get the number of documents in the table."""
        conn = await self._get_connection()

        async with conn.cursor() as cur:
            await cur.execute(f"SELECT COUNT(*) as count FROM {self._table}")
            result = await cur.fetchone()

        return result["count"] if result else 0

    async def close(self) -> None:
        """Close the database connection."""
        if self._conn and not self._conn.closed:
            await self._conn.close()

    async def __aenter__(self) -> "PgVectorStore":
        """Async context manager entry."""
        await self._get_connection()
        return self

    async def __aexit__(self, exc_type: Any, exc_val: Any, exc_tb: Any) -> None:
        """Async context manager exit."""
        await self.close()
