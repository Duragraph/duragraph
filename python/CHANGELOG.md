# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.2.1] - 2026-05-01

### Removed
- Broken provider stubs: `CohereEmbeddingProvider`, `OllamaEmbeddingProvider`,
  `QdrantVectorStore`, `WeaviateVectorStore`, `PgVectorStore`, `PineconeVectorStore`.
  Their imports referenced names that don't exist (`EmbeddingProvider` from
  `duragraph.vectorstores.base`), so they silently exposed as `None` rather than
  the class. Tracked in roadmap for v0.8.

### Fixed
- `duragraph.__version__` now matches the installed package version
  (was stale at `0.1.0` while pyproject was already at `0.2.0`).

## [0.2.0] - 2026-04-12

### Changed
- Drop Python 3.10 support; minimum is now Python 3.11.

### Added
- REST API client (`DuraGraphClient`, `AsyncDuraGraphClient`).

## [0.1.0] - 2024-12-29

### Added
- Initial package structure
- `@Graph` class decorator for defining workflows
- Node decorators: `@llm_node`, `@tool_node`, `@router_node`, `@human_node`, `@node`
- `@entrypoint` decorator for marking graph entry points
- Edge definitions with `>>` operator support
- `Worker` class for control plane integration
- `PromptStore` client for prompt management
- `@prompt` decorator for attaching prompts to nodes
- CLI commands: `init`, `dev`, `deploy`, `visualize`
- Type definitions: `State`, `Message`, `HumanMessage`, `AIMessage`, `ToolMessage`, `Event`, `RunResult`
- GitHub Actions CI/CD for PyPI publishing
- Lefthook git hooks for pre-commit and commit message validation
- Conventional commits enforcement
- Apache 2.0 license
- PEP 561 type hints support (py.typed marker)

[Unreleased]: https://github.com/duragraph/duragraph-python/compare/v0.2.1...HEAD
[0.2.1]: https://github.com/duragraph/duragraph-python/releases/tag/v0.2.1
[0.2.0]: https://github.com/duragraph/duragraph-python/releases/tag/v0.2.0
[0.1.0]: https://github.com/duragraph/duragraph-python/releases/tag/v0.1.0
