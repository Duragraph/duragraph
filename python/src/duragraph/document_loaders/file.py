"""Text file document loader."""

import mimetypes
from pathlib import Path
from typing import Any

from duragraph.document_loaders.base import BaseDocumentLoader
from duragraph.vectorstores import Document


class TextFileLoader(BaseDocumentLoader):
    """Load documents from text files."""

    def __init__(
        self,
        file_path: str | Path,
        encoding: str = "utf-8",
        autodetect_encoding: bool = True,
        **kwargs: Any,
    ):
        """Initialize text file loader.

        Args:
            file_path: Path to the text file.
            encoding: File encoding (default: utf-8).
            autodetect_encoding: Whether to auto-detect encoding.
            **kwargs: Additional configuration.
        """
        super().__init__(**kwargs)
        self.file_path = Path(file_path)
        self.encoding = encoding
        self.autodetect_encoding = autodetect_encoding

    def load(self) -> list[Document]:
        """Load the text file as a document."""
        if not self.file_path.exists():
            raise FileNotFoundError(f"File not found: {self.file_path}")

        if not self.file_path.is_file():
            raise ValueError(f"Path is not a file: {self.file_path}")

        # Auto-detect encoding if requested
        encoding = self.encoding
        if self.autodetect_encoding:
            encoding = self._detect_encoding()

        # Read file content
        try:
            content = self.file_path.read_text(encoding=encoding)
        except UnicodeDecodeError as e:
            if self.autodetect_encoding and encoding != "utf-8":
                # Fallback to utf-8
                try:
                    content = self.file_path.read_text(encoding="utf-8")
                    encoding = "utf-8"
                except UnicodeDecodeError:
                    # Last resort: read with error handling
                    content = self.file_path.read_text(encoding="utf-8", errors="replace")
                    encoding = "utf-8"
            else:
                raise e

        # Create document with metadata
        metadata = {
            "source": str(self.file_path),
            "file_name": self.file_path.name,
            "file_path": str(self.file_path.absolute()),
            "file_size": self.file_path.stat().st_size,
            "encoding": encoding,
            "mime_type": mimetypes.guess_type(str(self.file_path))[0] or "text/plain",
        }

        document = Document(page_content=content, metadata=metadata)

        return [document]

    def _detect_encoding(self) -> str:
        """Detect file encoding."""
        try:
            import chardet

            # Read a sample of the file
            with open(self.file_path, "rb") as f:
                raw_data = f.read(10000)  # Read first 10KB

            result = chardet.detect(raw_data)
            detected_encoding = result.get("encoding", "utf-8")
            confidence = result.get("confidence", 0)

            # Only use detected encoding if confidence is high
            if confidence > 0.8 and detected_encoding:
                return detected_encoding
            else:
                return self.encoding

        except ImportError:
            # chardet not available, use default
            return self.encoding


class DirectoryLoader(BaseDocumentLoader):
    """Load documents from all files in a directory."""

    def __init__(
        self,
        path: str | Path,
        glob: str = "*",
        exclude: list[str] | None = None,
        recursive: bool = False,
        loader_cls: type = TextFileLoader,
        loader_kwargs: dict[str, Any] | None = None,
        **kwargs: Any,
    ):
        """Initialize directory loader.

        Args:
            path: Directory path.
            glob: Glob pattern for file matching.
            exclude: List of patterns to exclude.
            recursive: Whether to search recursively.
            loader_cls: Loader class for individual files.
            loader_kwargs: Arguments to pass to loader class.
            **kwargs: Additional configuration.
        """
        super().__init__(**kwargs)
        self.path = Path(path)
        self.glob = glob
        self.exclude = exclude or []
        self.recursive = recursive
        self.loader_cls = loader_cls
        self.loader_kwargs = loader_kwargs or {}

    def load(self) -> list[Document]:
        """Load all matching files in the directory."""
        if not self.path.exists():
            raise FileNotFoundError(f"Directory not found: {self.path}")

        if not self.path.is_dir():
            raise ValueError(f"Path is not a directory: {self.path}")

        # Find matching files
        if self.recursive:
            pattern = f"**/{self.glob}"
            files = list(self.path.glob(pattern))
        else:
            files = list(self.path.glob(self.glob))

        # Filter out excluded files
        filtered_files = []
        for file_path in files:
            if file_path.is_file():
                # Check against exclusion patterns
                excluded = False
                for pattern in self.exclude:
                    if file_path.match(pattern):
                        excluded = True
                        break

                if not excluded:
                    filtered_files.append(file_path)

        # Load documents from each file
        documents = []
        for file_path in filtered_files:
            try:
                loader = self.loader_cls(file_path, **self.loader_kwargs)
                file_docs = loader.load()
                documents.extend(file_docs)
            except Exception as e:
                # Log error but continue with other files
                print(f"Warning: Failed to load {file_path}: {e}")
                continue

        return documents


class CSVLoader(BaseDocumentLoader):
    """Load documents from CSV files."""

    def __init__(
        self,
        file_path: str | Path,
        content_columns: list[str],
        metadata_columns: list[str] | None = None,
        delimiter: str = ",",
        encoding: str = "utf-8",
        **kwargs: Any,
    ):
        """Initialize CSV loader.

        Args:
            file_path: Path to CSV file.
            content_columns: Columns to use for document content.
            metadata_columns: Columns to include in metadata.
            delimiter: CSV delimiter.
            encoding: File encoding.
            **kwargs: Additional configuration.
        """
        super().__init__(**kwargs)
        self.file_path = Path(file_path)
        self.content_columns = content_columns
        self.metadata_columns = metadata_columns or []
        self.delimiter = delimiter
        self.encoding = encoding

    def load(self) -> list[Document]:
        """Load documents from CSV file."""
        import csv

        if not self.file_path.exists():
            raise FileNotFoundError(f"CSV file not found: {self.file_path}")

        documents = []

        with open(self.file_path, encoding=self.encoding) as f:
            reader = csv.DictReader(f, delimiter=self.delimiter)

            for row_idx, row in enumerate(reader):
                # Build content from specified columns
                content_parts = []
                for col in self.content_columns:
                    if col in row and row[col]:
                        content_parts.append(str(row[col]))

                if not content_parts:
                    continue  # Skip empty rows

                content = " ".join(content_parts)

                # Build metadata
                metadata = {
                    "source": str(self.file_path),
                    "row": row_idx,
                }

                # Add specified metadata columns
                for col in self.metadata_columns:
                    if col in row:
                        metadata[col] = row[col]

                # Add all other columns to metadata if not too many
                if len(row) <= 20:  # Arbitrary limit to avoid huge metadata
                    for col, value in row.items():
                        if col not in self.content_columns and col not in self.metadata_columns:
                            metadata[f"csv_{col}"] = value

                documents.append(Document(page_content=content, metadata=metadata))

        return documents


class JSONLoader(BaseDocumentLoader):
    """Load documents from JSON files."""

    def __init__(
        self,
        file_path: str | Path,
        content_key: str,
        metadata_keys: list[str] | None = None,
        json_path: str | None = None,
        **kwargs: Any,
    ):
        """Initialize JSON loader.

        Args:
            file_path: Path to JSON file.
            content_key: Key containing document content.
            metadata_keys: Keys to include in metadata.
            json_path: JSONPath expression for nested data.
            **kwargs: Additional configuration.
        """
        super().__init__(**kwargs)
        self.file_path = Path(file_path)
        self.content_key = content_key
        self.metadata_keys = metadata_keys or []
        self.json_path = json_path

    def load(self) -> list[Document]:
        """Load documents from JSON file."""
        import json

        if not self.file_path.exists():
            raise FileNotFoundError(f"JSON file not found: {self.file_path}")

        with open(self.file_path, encoding="utf-8") as f:
            data = json.load(f)

        # Handle JSON path if specified
        if self.json_path:
            try:
                import jsonpath_ng

                jsonpath_expr = jsonpath_ng.parse(self.json_path)
                matches = [match.value for match in jsonpath_expr.find(data)]
                data = matches
            except ImportError as e:
                raise ImportError(
                    "jsonpath-ng required for JSON path support: pip install jsonpath-ng"
                ) from e

        documents = []

        # Handle different data structures
        if isinstance(data, list):
            items = data
        elif isinstance(data, dict):
            items = [data]
        else:
            raise ValueError(f"Unsupported JSON structure: {type(data)}")

        for idx, item in enumerate(items):
            if not isinstance(item, dict):
                continue

            # Extract content
            if self.content_key not in item:
                continue

            content = str(item[self.content_key])

            # Build metadata
            metadata = {
                "source": str(self.file_path),
                "json_index": idx,
            }

            # Add specified metadata keys
            for key in self.metadata_keys:
                if key in item:
                    metadata[key] = item[key]

            documents.append(Document(page_content=content, metadata=metadata))

        return documents
