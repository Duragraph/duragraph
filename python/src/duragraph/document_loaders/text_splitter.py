"""Text chunking utilities for splitting documents."""

import re
from abc import ABC, abstractmethod
from collections.abc import Callable

from duragraph.vectorstores import Document


class TextSplitter(ABC):
    """Abstract base class for text splitters."""

    def __init__(
        self,
        chunk_size: int = 1000,
        chunk_overlap: int = 100,
        length_function: Callable[[str], int] | None = None,
        keep_separator: bool = False,
        add_start_index: bool = False,
    ):
        """Initialize text splitter.

        Args:
            chunk_size: Maximum size of each chunk.
            chunk_overlap: Overlap between chunks.
            length_function: Function to measure text length.
            keep_separator: Whether to keep separator in chunks.
            add_start_index: Whether to add start index to metadata.
        """
        self._chunk_size = chunk_size
        self._chunk_overlap = chunk_overlap
        self._length_function = length_function or len
        self._keep_separator = keep_separator
        self._add_start_index = add_start_index

    @abstractmethod
    def split_text(self, text: str) -> list[str]:
        """Split text into chunks.

        Args:
            text: Text to split.

        Returns:
            List of text chunks.
        """

    def split_documents(self, documents: list[Document]) -> list[Document]:
        """Split documents into smaller chunks.

        Args:
            documents: Documents to split.

        Returns:
            List of document chunks.
        """
        texts, metadatas = [], []
        for doc in documents:
            texts.append(doc.page_content)
            metadatas.append(doc.metadata)

        return self.create_documents(texts, metadatas)

    def create_documents(
        self, texts: list[str], metadatas: list[dict] | None = None
    ) -> list[Document]:
        """Create documents from texts and metadata.

        Args:
            texts: List of texts to create documents from.
            metadatas: Optional list of metadata for each text.

        Returns:
            List of documents.
        """
        _metadatas = metadatas or [{}] * len(texts)
        documents = []

        for i, text in enumerate(texts):
            index = 0
            for chunk in self.split_text(text):
                metadata = _metadatas[i].copy()

                if self._add_start_index:
                    metadata["start_index"] = index

                # Add chunk information
                metadata["chunk_index"] = len(documents)
                metadata["total_chunks"] = None  # Will be updated later

                documents.append(Document(page_content=chunk, metadata=metadata))
                index += len(chunk) + (self._chunk_overlap if len(documents) > 1 else 0)

        # Update total_chunks count
        for doc in documents:
            doc.metadata["total_chunks"] = len(documents)

        return documents

    def _merge_splits(self, splits: list[str], separator: str) -> list[str]:
        """Merge splits into chunks of appropriate size."""
        separator_len = self._length_function(separator)

        docs = []
        current_doc = []
        total = 0

        for split in splits:
            _len = self._length_function(split)

            # If adding this split would exceed chunk size, finalize current chunk
            if total + _len + (separator_len if len(current_doc) > 0 else 0) > self._chunk_size:
                if current_doc:
                    doc = self._join_docs(current_doc, separator)
                    if doc is not None:
                        docs.append(doc)

                    # Start new chunk with overlap
                    while total > self._chunk_overlap or (
                        total + _len + (separator_len if len(current_doc) > 0 else 0)
                        > self._chunk_size
                        and total > 0
                    ):
                        total -= self._length_function(current_doc[0]) + (
                            separator_len if len(current_doc) > 1 else 0
                        )
                        current_doc = current_doc[1:]

            current_doc.append(split)
            total += _len + (separator_len if len(current_doc) > 1 else 0)

        # Add remaining content
        doc = self._join_docs(current_doc, separator)
        if doc is not None:
            docs.append(doc)

        return docs

    def _join_docs(self, docs: list[str], separator: str) -> str | None:
        """Join documents with separator."""
        text = separator.join(docs)
        text = text.strip()
        if text == "":
            return None
        return text


class CharacterTextSplitter(TextSplitter):
    """Split text by character count."""

    def __init__(self, separator: str = "\n\n", **kwargs):
        """Initialize character text splitter.

        Args:
            separator: Separator to split on.
            **kwargs: Arguments passed to TextSplitter.
        """
        super().__init__(**kwargs)
        self._separator = separator

    def split_text(self, text: str) -> list[str]:
        """Split text by separator and merge to appropriate chunk size."""
        splits = text.split(self._separator) if self._separator else list(text)
        return self._merge_splits(splits, self._separator)


class RecursiveCharacterTextSplitter(TextSplitter):
    """Recursively split text using a hierarchy of separators."""

    def __init__(
        self, separators: list[str] | None = None, is_separator_regex: bool = False, **kwargs
    ):
        """Initialize recursive character text splitter.

        Args:
            separators: List of separators to try in order.
            is_separator_regex: Whether separators are regex patterns.
            **kwargs: Arguments passed to TextSplitter.
        """
        super().__init__(**kwargs)
        self._separators = separators or ["\n\n", "\n", " ", ""]
        self._is_separator_regex = is_separator_regex

    def split_text(self, text: str) -> list[str]:
        """Split text recursively using separators."""
        return self._split_text(text, self._separators)

    def _split_text(self, text: str, separators: list[str]) -> list[str]:
        """Recursively split text."""
        # Get appropriate separator
        separator = separators[-1]
        new_separators = []
        for i, _s in enumerate(separators):
            _separator = _s if self._is_separator_regex else re.escape(_s)
            if _s == "":
                separator = _s
                break
            if re.search(_separator, text):
                separator = _s
                new_separators = separators[i + 1 :]
                break

        # Split by separator
        _separator = separator if self._is_separator_regex else re.escape(separator)
        splits = _split_text_with_regex(text, _separator, self._keep_separator)

        # Recursively split large chunks
        good_splits = []
        for split in splits:
            if self._length_function(split) < self._chunk_size:
                good_splits.append(split)
            else:
                if new_separators:
                    other_info = self._split_text(split, new_separators)
                    good_splits.extend(other_info)
                else:
                    good_splits.append(split)

        # Merge splits to appropriate size
        return self._merge_splits(good_splits, separator)


class TokenTextSplitter(TextSplitter):
    """Split text by token count using tiktoken."""

    def __init__(
        self,
        encoding_name: str = "cl100k_base",
        model_name: str | None = None,
        allowed_special: str | set | None = None,
        disallowed_special: str | set = "all",
        **kwargs,
    ):
        """Initialize token text splitter.

        Args:
            encoding_name: Name of tiktoken encoding.
            model_name: Model name to get encoding for.
            allowed_special: Special tokens to allow.
            disallowed_special: Special tokens to disallow.
            **kwargs: Arguments passed to TextSplitter.
        """
        super().__init__(**kwargs)

        try:
            import tiktoken
        except ImportError as e:
            raise ImportError("tiktoken required for token splitting: pip install tiktoken") from e

        if model_name:
            self._encoding = tiktoken.encoding_for_model(model_name)
        else:
            self._encoding = tiktoken.get_encoding(encoding_name)

        self._allowed_special = allowed_special if allowed_special is not None else set()
        self._disallowed_special = disallowed_special

        # Use token count for length function
        self._length_function = self._token_count

    def _token_count(self, text: str) -> int:
        """Count tokens in text."""
        return len(
            self._encoding.encode(
                text,
                allowed_special=self._allowed_special,
                disallowed_special=self._disallowed_special,
            )
        )

    def split_text(self, text: str) -> list[str]:
        """Split text by token count."""
        # Encode text to tokens
        tokens = self._encoding.encode(
            text,
            allowed_special=self._allowed_special,
            disallowed_special=self._disallowed_special,
        )

        chunks = []
        start = 0

        while start < len(tokens):
            # Calculate chunk end
            end = min(start + self._chunk_size, len(tokens))

            # Get chunk tokens
            chunk_tokens = tokens[start:end]

            # Decode back to text
            chunk_text = self._encoding.decode(chunk_tokens)
            chunks.append(chunk_text)

            # Move start position with overlap
            start = end - self._chunk_overlap
            if start <= 0:
                start = end

        return chunks


class ParagraphTextSplitter(RecursiveCharacterTextSplitter):
    """Split text by paragraphs with smart handling of different formats."""

    def __init__(self, **kwargs):
        """Initialize paragraph text splitter."""
        separators = [
            "\n\n\n",  # Multiple newlines
            "\n\n",  # Double newlines (paragraphs)
            "\n",  # Single newlines
            ".",  # Sentences
            "!",  # Exclamations
            "?",  # Questions
            ";",  # Semicolons
            ",",  # Commas
            " ",  # Spaces
            "",  # Characters
        ]
        super().__init__(separators=separators, **kwargs)


class MarkdownTextSplitter(RecursiveCharacterTextSplitter):
    """Split Markdown text while preserving structure."""

    def __init__(self, **kwargs):
        """Initialize Markdown text splitter."""
        separators = [
            "\n#{1,6} ",  # Headers
            "```\n\n```",  # Code blocks
            "\n\n",  # Paragraphs
            "\n",  # Lines
            " ",  # Words
            "",  # Characters
        ]
        super().__init__(separators=separators, is_separator_regex=True, **kwargs)


class CodeTextSplitter(RecursiveCharacterTextSplitter):
    """Split code while preserving logical structure."""

    def __init__(self, language: str = "python", **kwargs):
        """Initialize code text splitter.

        Args:
            language: Programming language.
            **kwargs: Arguments passed to RecursiveCharacterTextSplitter.
        """
        self.language = language.lower()
        separators = self._get_separators_for_language(language)
        super().__init__(separators=separators, **kwargs)

    def _get_separators_for_language(self, language: str) -> list[str]:
        """Get separators appropriate for programming language."""
        if language in ("python", "py"):
            return [
                "\nclass ",
                "\ndef ",
                "\n\ndef ",
                "\n\n",
                "\n",
                " ",
                "",
            ]
        elif language in ("javascript", "js", "typescript", "ts"):
            return [
                "\nfunction ",
                "\nconst ",
                "\nlet ",
                "\nvar ",
                "\nclass ",
                "\n\n",
                "\n",
                " ",
                "",
            ]
        elif language in ("java", "c", "cpp", "c++"):
            return [
                "\nclass ",
                "\nstruct ",
                "\npublic ",
                "\nprivate ",
                "\nprotected ",
                "\n\n",
                "\n",
                " ",
                "",
            ]
        else:
            # Default separators
            return ["\n\n", "\n", " ", ""]


def _split_text_with_regex(text: str, separator: str, keep_separator: bool) -> list[str]:
    """Split text using regex separator."""
    if separator:
        if keep_separator:
            # Keep separator at the end of chunks
            splits = re.split(f"({separator})", text)
            splits = [
                splits[i] + (splits[i + 1] if i + 1 < len(splits) else "")
                for i in range(0, len(splits), 2)
            ]
        else:
            splits = re.split(separator, text)
    else:
        splits = list(text)

    return [s for s in splits if s != ""]
