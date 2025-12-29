# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

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

[Unreleased]: https://github.com/duragraph/duragraph-python/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/duragraph/duragraph-python/releases/tag/v0.1.0
