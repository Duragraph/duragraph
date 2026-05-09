"""Tests for document loaders and text splitters."""

import tempfile
from pathlib import Path

import pytest

from duragraph.document_loaders import (
    CharacterTextSplitter,
    CodeTextSplitter,
    MarkdownTextSplitter,
    ParagraphTextSplitter,
    RecursiveCharacterTextSplitter,
)
from duragraph.vectorstores import Document


# Test text splitters
class TestTextSplitters:
    """Tests for text splitter classes."""

    def test_character_text_splitter(self):
        """Test basic character text splitter."""
        text = "This is paragraph one.\n\nThis is paragraph two.\n\nThis is paragraph three."

        splitter = CharacterTextSplitter(chunk_size=50, chunk_overlap=10, separator="\n\n")

        chunks = splitter.split_text(text)

        assert len(chunks) >= 2
        assert all(isinstance(chunk, str) for chunk in chunks)
        assert all(len(chunk) <= 60 for chunk in chunks)  # Allow some flexibility

    def test_recursive_character_text_splitter(self):
        """Test recursive character text splitter."""
        text = """
        # Chapter 1

        This is the first chapter with some content.

        ## Section 1.1

        This is a subsection with more content.

        # Chapter 2

        This is the second chapter.
        """

        splitter = RecursiveCharacterTextSplitter(
            chunk_size=100, chunk_overlap=20, separators=["\n# ", "\n## ", "\n\n", "\n", " "]
        )

        chunks = splitter.split_text(text)

        assert len(chunks) >= 2
        assert all(isinstance(chunk, str) for chunk in chunks)

    def test_paragraph_text_splitter(self):
        """Test paragraph text splitter."""
        text = """
        This is the first paragraph. It contains multiple sentences.

        This is the second paragraph. It also contains multiple sentences.

        This is the third paragraph.
        """

        splitter = ParagraphTextSplitter(chunk_size=80, chunk_overlap=10)
        chunks = splitter.split_text(text.strip())

        assert len(chunks) >= 1
        assert all(isinstance(chunk, str) for chunk in chunks)

    def test_markdown_text_splitter(self):
        """Test Markdown text splitter."""
        markdown_text = """
        # Main Title

        This is the introduction paragraph.

        ## Subsection

        Here is some content in a subsection.

        ```python
        def example():
            return "code block"
        ```

        More content after the code block.
        """

        splitter = MarkdownTextSplitter(chunk_size=150, chunk_overlap=20)
        chunks = splitter.split_text(markdown_text.strip())

        assert len(chunks) >= 1
        assert all(isinstance(chunk, str) for chunk in chunks)

    def test_code_text_splitter(self):
        """Test code text splitter."""
        python_code = '''
def function_one():
    """First function."""
    return "one"

def function_two():
    """Second function."""
    return "two"

class ExampleClass:
    """Example class."""

    def method_one(self):
        return "method"
'''

        splitter = CodeTextSplitter(language="python", chunk_size=100, chunk_overlap=10)
        chunks = splitter.split_text(python_code.strip())

        assert len(chunks) >= 1
        assert all(isinstance(chunk, str) for chunk in chunks)

    def test_split_documents(self):
        """Test splitting documents."""
        documents = [
            Document(
                page_content="This is a long document that needs to be split into smaller chunks for processing.",
                metadata={"source": "test.txt", "id": 1},
            ),
            Document(
                page_content="Another document with different content that also needs chunking.",
                metadata={"source": "test2.txt", "id": 2},
            ),
        ]

        splitter = CharacterTextSplitter(chunk_size=30, chunk_overlap=5)
        split_docs = splitter.split_documents(documents)

        assert len(split_docs) >= len(documents)
        assert all(isinstance(doc, Document) for doc in split_docs)
        assert all("chunk_index" in doc.metadata for doc in split_docs)
        assert all("total_chunks" in doc.metadata for doc in split_docs)

    def test_create_documents(self):
        """Test creating documents from texts."""
        texts = ["First text to be processed.", "Second text that will also be processed."]
        metadatas = [{"source": "text1"}, {"source": "text2"}]

        splitter = CharacterTextSplitter(chunk_size=20, chunk_overlap=3)
        documents = splitter.create_documents(texts, metadatas)

        assert len(documents) >= len(texts)
        assert all(isinstance(doc, Document) for doc in documents)
        assert all(doc.metadata.get("source") in ["text1", "text2"] for doc in documents)


# Test file loaders
class TestFileLoaders:
    """Tests for file-based document loaders."""

    def test_text_file_loader(self):
        """Test text file loader."""
        from duragraph.document_loaders.file import TextFileLoader

        # Create temporary file
        with tempfile.NamedTemporaryFile(mode="w", suffix=".txt", delete=False) as f:
            f.write("This is a test file.\nWith multiple lines.\n")
            temp_path = f.name

        try:
            loader = TextFileLoader(temp_path)
            documents = loader.load()

            assert len(documents) == 1
            assert isinstance(documents[0], Document)
            assert "This is a test file." in documents[0].page_content
            assert documents[0].metadata["source"] == temp_path
            assert "file_name" in documents[0].metadata

        finally:
            Path(temp_path).unlink()

    def test_directory_loader(self):
        """Test directory loader."""
        from duragraph.document_loaders.file import DirectoryLoader, TextFileLoader

        # Create temporary directory with files
        with tempfile.TemporaryDirectory() as temp_dir:
            temp_path = Path(temp_dir)

            # Create test files
            (temp_path / "file1.txt").write_text("Content of file 1")
            (temp_path / "file2.txt").write_text("Content of file 2")
            (temp_path / "file3.log").write_text("Log file content")

            # Test loading all txt files
            loader = DirectoryLoader(temp_path, glob="*.txt", loader_cls=TextFileLoader)
            documents = loader.load()

            assert len(documents) == 2  # Only .txt files
            assert all(isinstance(doc, Document) for doc in documents)

            contents = [doc.page_content for doc in documents]
            assert "Content of file 1" in contents
            assert "Content of file 2" in contents

    def test_csv_loader(self):
        """Test CSV loader."""
        from duragraph.document_loaders.file import CSVLoader

        # Create temporary CSV file
        csv_content = """name,description,category
Product A,A great product,electronics
Product B,Another product,clothing
Product C,Third product,electronics"""

        with tempfile.NamedTemporaryFile(mode="w", suffix=".csv", delete=False) as f:
            f.write(csv_content)
            temp_path = f.name

        try:
            loader = CSVLoader(
                temp_path, content_columns=["name", "description"], metadata_columns=["category"]
            )
            documents = loader.load()

            assert len(documents) == 3
            assert all(isinstance(doc, Document) for doc in documents)

            # Check content
            assert "Product A A great product" in documents[0].page_content
            assert documents[0].metadata["category"] == "electronics"

        finally:
            Path(temp_path).unlink()

    def test_json_loader(self):
        """Test JSON loader."""
        import json

        from duragraph.document_loaders.file import JSONLoader

        # Create temporary JSON file
        json_data = [
            {"title": "First Article", "content": "Content of first article", "author": "Alice"},
            {"title": "Second Article", "content": "Content of second article", "author": "Bob"},
        ]

        with tempfile.NamedTemporaryFile(mode="w", suffix=".json", delete=False) as f:
            json.dump(json_data, f)
            temp_path = f.name

        try:
            loader = JSONLoader(temp_path, content_key="content", metadata_keys=["title", "author"])
            documents = loader.load()

            assert len(documents) == 2
            assert all(isinstance(doc, Document) for doc in documents)

            assert "Content of first article" in documents[0].page_content
            assert documents[0].metadata["title"] == "First Article"
            assert documents[0].metadata["author"] == "Alice"

        finally:
            Path(temp_path).unlink()


# Test async loading
class TestAsyncLoading:
    """Tests for async document loading."""

    @pytest.mark.asyncio
    async def test_async_load(self):
        """Test async document loading."""
        from duragraph.document_loaders.file import TextFileLoader

        # Create temporary file
        with tempfile.NamedTemporaryFile(mode="w", suffix=".txt", delete=False) as f:
            f.write("Async test content")
            temp_path = f.name

        try:
            loader = TextFileLoader(temp_path)
            documents = await loader.aload()

            assert len(documents) == 1
            assert isinstance(documents[0], Document)
            assert "Async test content" in documents[0].page_content

        finally:
            Path(temp_path).unlink()

    @pytest.mark.asyncio
    async def test_lazy_load(self):
        """Test lazy loading."""
        from duragraph.document_loaders.file import DirectoryLoader, TextFileLoader

        # Create temporary directory with files
        with tempfile.TemporaryDirectory() as temp_dir:
            temp_path = Path(temp_dir)

            # Create test files
            (temp_path / "file1.txt").write_text("Content 1")
            (temp_path / "file2.txt").write_text("Content 2")

            loader = DirectoryLoader(temp_path, glob="*.txt", loader_cls=TextFileLoader)

            # Test sync lazy loading
            docs_sync = list(loader.lazy_load())
            assert len(docs_sync) == 2

            # Test async lazy loading
            docs_async = []
            async for doc in loader.alazy_load():
                docs_async.append(doc)

            assert len(docs_async) == 2
            assert all(isinstance(doc, Document) for doc in docs_async)


# Test error handling
class TestErrorHandling:
    """Tests for error handling in document loaders."""

    def test_file_not_found(self):
        """Test handling of missing files."""
        from duragraph.document_loaders.file import TextFileLoader

        loader = TextFileLoader("nonexistent_file.txt")

        with pytest.raises(FileNotFoundError):
            loader.load()

    def test_invalid_directory(self):
        """Test handling of invalid directory."""
        from duragraph.document_loaders.file import DirectoryLoader

        loader = DirectoryLoader("nonexistent_directory")

        with pytest.raises(FileNotFoundError):
            loader.load()

    def test_invalid_csv(self):
        """Test handling of invalid CSV."""
        from duragraph.document_loaders.file import CSVLoader

        # Create invalid CSV file
        with tempfile.NamedTemporaryFile(mode="w", suffix=".csv", delete=False) as f:
            f.write("invalid,csv,content\nwith,missing\n")  # Inconsistent columns
            temp_path = f.name

        try:
            loader = CSVLoader(temp_path, content_columns=["nonexistent_column"])
            documents = loader.load()

            # Should handle gracefully - may return empty or skip invalid rows
            assert isinstance(documents, list)

        finally:
            Path(temp_path).unlink()


# Integration tests
class TestIntegration:
    """Integration tests for document loaders and text splitters."""

    def test_load_and_split_integration(self):
        """Test loading documents and then splitting them."""
        from duragraph.document_loaders.file import TextFileLoader

        # Create a longer test file
        long_content = "This is a very long document. " * 100

        with tempfile.NamedTemporaryFile(mode="w", suffix=".txt", delete=False) as f:
            f.write(long_content)
            temp_path = f.name

        try:
            # Load document
            loader = TextFileLoader(temp_path)
            documents = loader.load()

            assert len(documents) == 1
            assert len(documents[0].page_content) > 1000  # Long document

            # Split document (use space separator since content has no newlines)
            splitter = CharacterTextSplitter(chunk_size=200, chunk_overlap=20, separator=" ")
            split_docs = splitter.split_documents(documents)

            assert len(split_docs) > 1  # Should be split
            assert all(len(doc.page_content) <= 220 for doc in split_docs)  # Respects size limit
            assert all(
                doc.metadata["source"] == temp_path for doc in split_docs
            )  # Preserves metadata

        finally:
            Path(temp_path).unlink()

    def test_multiple_formats_integration(self):
        """Test loading multiple document formats."""
        from duragraph.document_loaders.file import CSVLoader, DirectoryLoader, TextFileLoader

        with tempfile.TemporaryDirectory() as temp_dir:
            temp_path = Path(temp_dir)

            # Create different file types
            (temp_path / "document.txt").write_text("Text document content")

            # Create CSV file
            csv_content = "title,content\nTest,CSV content"
            (temp_path / "data.csv").write_text(csv_content)

            # Load text files
            text_loader = DirectoryLoader(temp_path, glob="*.txt", loader_cls=TextFileLoader)
            text_docs = text_loader.load()

            # Load CSV files with custom loader
            csv_docs = CSVLoader(
                temp_path / "data.csv", content_columns=["content"], metadata_columns=["title"]
            ).load()

            # Combine all documents
            all_docs = text_docs + csv_docs

            assert len(all_docs) == 2
            assert any("Text document content" in doc.page_content for doc in all_docs)
            assert any("CSV content" in doc.page_content for doc in all_docs)

            # Split all documents uniformly
            splitter = RecursiveCharacterTextSplitter(chunk_size=50, chunk_overlap=5)
            split_docs = splitter.split_documents(all_docs)

            assert len(split_docs) >= len(all_docs)
            assert all(isinstance(doc, Document) for doc in split_docs)
