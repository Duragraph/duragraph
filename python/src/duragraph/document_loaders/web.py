"""Web and URL document loaders."""

from typing import Any
from urllib.parse import urljoin

from duragraph.document_loaders.base import BaseDocumentLoader
from duragraph.vectorstores import Document


class WebPageLoader(BaseDocumentLoader):
    """Load documents from web pages."""

    def __init__(
        self,
        url: str,
        extract_text: bool = True,
        include_links: bool = False,
        timeout: float = 30.0,
        headers: dict[str, str] | None = None,
        **kwargs: Any,
    ):
        """Initialize web page loader.

        Args:
            url: URL to load.
            extract_text: Whether to extract text from HTML.
            include_links: Whether to include links in metadata.
            timeout: Request timeout in seconds.
            headers: HTTP headers to include.
            **kwargs: Additional configuration.
        """
        super().__init__(**kwargs)
        self.url = url
        self.extract_text = extract_text
        self.include_links = include_links
        self.timeout = timeout
        self.headers = headers or {}

    def load(self) -> list[Document]:
        """Load the web page as a document."""
        import httpx

        # Set default headers
        default_headers = {
            "User-Agent": "DuraGraph Document Loader 1.0",
            "Accept": "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8",
        }
        default_headers.update(self.headers)

        # Fetch the page
        try:
            with httpx.Client(timeout=self.timeout, headers=default_headers) as client:
                response = client.get(self.url)
                response.raise_for_status()
        except Exception as e:
            raise RuntimeError(f"Failed to fetch {self.url}: {e}") from e

        # Process content
        if self.extract_text and "text/html" in response.headers.get("content-type", ""):
            content, links = self._extract_text_from_html(response.text)
        else:
            content = response.text
            links = []

        # Build metadata
        metadata = {
            "source": self.url,
            "url": self.url,
            "title": self._extract_title(response.text)
            if "text/html" in response.headers.get("content-type", "")
            else None,
            "content_type": response.headers.get("content-type"),
            "status_code": response.status_code,
            "encoding": response.encoding,
        }

        if self.include_links and links:
            metadata["links"] = links

        return [Document(page_content=content, metadata=metadata)]

    def _extract_text_from_html(self, html: str) -> tuple[str, list[str]]:
        """Extract text content and links from HTML."""
        try:
            from bs4 import BeautifulSoup
        except ImportError as e:
            raise ImportError(
                "beautifulsoup4 required for HTML parsing: pip install beautifulsoup4"
            ) from e

        soup = BeautifulSoup(html, "html.parser")

        # Remove script and style elements
        for element in soup(["script", "style", "nav", "header", "footer"]):
            element.decompose()

        # Extract text
        text = soup.get_text()
        # Clean up whitespace
        lines = (line.strip() for line in text.splitlines())
        chunks = (phrase.strip() for line in lines for phrase in line.split("  "))
        text = "\n".join(chunk for chunk in chunks if chunk)

        # Extract links
        links = []
        if self.include_links:
            for link in soup.find_all("a", href=True):
                href = link["href"]
                # Convert relative URLs to absolute
                if href.startswith(("http://", "https://")):
                    absolute_url = href
                else:
                    absolute_url = urljoin(self.url, href)

                links.append(
                    {
                        "url": absolute_url,
                        "text": link.get_text(strip=True),
                        "title": link.get("title"),
                    }
                )

        return text, links

    def _extract_title(self, html: str) -> str | None:
        """Extract title from HTML."""
        try:
            from bs4 import BeautifulSoup

            soup = BeautifulSoup(html, "html.parser")
            title_tag = soup.find("title")
            return title_tag.get_text(strip=True) if title_tag else None
        except ImportError:
            # Fallback: simple regex
            import re

            match = re.search(r"<title[^>]*>(.*?)</title>", html, re.IGNORECASE | re.DOTALL)
            return match.group(1).strip() if match else None


class SitemapLoader(BaseDocumentLoader):
    """Load documents from URLs listed in a sitemap."""

    def __init__(
        self,
        sitemap_url: str,
        filter_urls: list[str] | None = None,
        exclude_urls: list[str] | None = None,
        max_pages: int = 100,
        page_loader_kwargs: dict[str, Any] | None = None,
        **kwargs: Any,
    ):
        """Initialize sitemap loader.

        Args:
            sitemap_url: URL of the sitemap.
            filter_urls: List of URL patterns to include.
            exclude_urls: List of URL patterns to exclude.
            max_pages: Maximum number of pages to load.
            page_loader_kwargs: Arguments for WebPageLoader.
            **kwargs: Additional configuration.
        """
        super().__init__(**kwargs)
        self.sitemap_url = sitemap_url
        self.filter_urls = filter_urls or []
        self.exclude_urls = exclude_urls or []
        self.max_pages = max_pages
        self.page_loader_kwargs = page_loader_kwargs or {}

    def load(self) -> list[Document]:
        """Load documents from sitemap URLs."""
        import httpx

        # Fetch sitemap
        try:
            with httpx.Client(timeout=30.0) as client:
                response = client.get(self.sitemap_url)
                response.raise_for_status()
        except Exception as e:
            raise RuntimeError(f"Failed to fetch sitemap {self.sitemap_url}: {e}") from e

        # Parse sitemap
        urls = self._parse_sitemap(response.text)

        # Filter URLs
        filtered_urls = self._filter_urls(urls)

        # Limit number of URLs
        if len(filtered_urls) > self.max_pages:
            filtered_urls = filtered_urls[: self.max_pages]

        # Load documents from each URL
        documents = []
        for url in filtered_urls:
            try:
                loader = WebPageLoader(url, **self.page_loader_kwargs)
                page_docs = loader.load()
                documents.extend(page_docs)
            except Exception as e:
                print(f"Warning: Failed to load {url}: {e}")
                continue

        return documents

    def _parse_sitemap(self, xml_content: str) -> list[str]:
        """Parse sitemap XML to extract URLs."""
        try:
            import xml.etree.ElementTree as ET
        except ImportError as e:
            raise ImportError("xml.etree.ElementTree required for sitemap parsing") from e

        urls = []

        try:
            root = ET.fromstring(xml_content)

            # Handle different sitemap namespaces
            namespaces = {"sitemap": "http://www.sitemaps.org/schemas/sitemap/0.9"}

            # Try to find URL elements
            for url_elem in root.findall(".//sitemap:url", namespaces):
                loc_elem = url_elem.find("sitemap:loc", namespaces)
                if loc_elem is not None and loc_elem.text:
                    urls.append(loc_elem.text.strip())

            # If no URLs found with namespace, try without
            if not urls:
                for url_elem in root.findall(".//url"):
                    loc_elem = url_elem.find("loc")
                    if loc_elem is not None and loc_elem.text:
                        urls.append(loc_elem.text.strip())

            # Handle sitemap index (points to other sitemaps)
            import httpx

            for sitemap_elem in root.findall(".//sitemap:sitemap", namespaces):
                loc_elem = sitemap_elem.find("sitemap:loc", namespaces)
                if loc_elem is not None and loc_elem.text:
                    # Recursively load sub-sitemap
                    try:
                        sub_loader = SitemapLoader(
                            loc_elem.text.strip(),
                            self.filter_urls,
                            self.exclude_urls,
                            self.max_pages - len(urls),
                            self.page_loader_kwargs,
                        )
                        sub_urls = sub_loader._parse_sitemap(httpx.get(loc_elem.text.strip()).text)
                        urls.extend(sub_urls)
                    except Exception:
                        continue

        except ET.ParseError as e:
            raise ValueError(f"Invalid sitemap XML: {e}") from e

        return urls

    def _filter_urls(self, urls: list[str]) -> list[str]:
        """Filter URLs based on include/exclude patterns."""
        filtered = []

        for url in urls:
            # Check exclusions first
            excluded = False
            for pattern in self.exclude_urls:
                if pattern in url:
                    excluded = True
                    break

            if excluded:
                continue

            # Check inclusions
            if self.filter_urls:
                included = False
                for pattern in self.filter_urls:
                    if pattern in url:
                        included = True
                        break

                if not included:
                    continue

            filtered.append(url)

        return filtered


class URLListLoader(BaseDocumentLoader):
    """Load documents from a list of URLs."""

    def __init__(
        self,
        urls: list[str],
        max_concurrent: int = 5,
        page_loader_kwargs: dict[str, Any] | None = None,
        **kwargs: Any,
    ):
        """Initialize URL list loader.

        Args:
            urls: List of URLs to load.
            max_concurrent: Maximum concurrent requests.
            page_loader_kwargs: Arguments for WebPageLoader.
            **kwargs: Additional configuration.
        """
        super().__init__(**kwargs)
        self.urls = urls
        self.max_concurrent = max_concurrent
        self.page_loader_kwargs = page_loader_kwargs or {}

    def load(self) -> list[Document]:
        """Load documents from all URLs."""
        documents = []

        for url in self.urls:
            try:
                loader = WebPageLoader(url, **self.page_loader_kwargs)
                page_docs = loader.load()
                documents.extend(page_docs)
            except Exception as e:
                print(f"Warning: Failed to load {url}: {e}")
                continue

        return documents

    async def aload(self) -> list[Document]:
        """Load documents asynchronously with concurrency control."""
        import asyncio

        semaphore = asyncio.Semaphore(self.max_concurrent)
        documents = []

        async def load_url(url: str) -> list[Document]:
            async with semaphore:
                try:
                    # Create async version of WebPageLoader
                    return await self._load_url_async(url)
                except Exception as e:
                    print(f"Warning: Failed to load {url}: {e}")
                    return []

        # Load all URLs concurrently
        tasks = [load_url(url) for url in self.urls]
        results = await asyncio.gather(*tasks, return_exceptions=True)

        # Collect documents
        for result in results:
            if isinstance(result, list):
                documents.extend(result)

        return documents

    async def _load_url_async(self, url: str) -> list[Document]:
        """Load a single URL asynchronously."""
        import httpx

        headers = {
            "User-Agent": "DuraGraph Document Loader 1.0",
            "Accept": "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8",
        }
        headers.update(self.page_loader_kwargs.get("headers", {}))

        timeout = self.page_loader_kwargs.get("timeout", 30.0)

        async with httpx.AsyncClient(timeout=timeout, headers=headers) as client:
            response = await client.get(url)
            response.raise_for_status()

        # Use WebPageLoader to process the content
        loader = WebPageLoader(url, **self.page_loader_kwargs)

        # Mock the response for processing
        if loader.extract_text and "text/html" in response.headers.get("content-type", ""):
            content, links = loader._extract_text_from_html(response.text)
        else:
            content = response.text
            links = []

        metadata = {
            "source": url,
            "url": url,
            "title": loader._extract_title(response.text)
            if "text/html" in response.headers.get("content-type", "")
            else None,
            "content_type": response.headers.get("content-type"),
            "status_code": response.status_code,
            "encoding": response.encoding,
        }

        if loader.include_links and links:
            metadata["links"] = links

        return [Document(page_content=content, metadata=metadata)]
