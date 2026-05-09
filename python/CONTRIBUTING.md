# Contributing to DuraGraph Python SDK

Thank you for your interest in contributing to DuraGraph! This document provides guidelines and conventions for contributing.

## Getting Started

1. Fork the repository
2. Clone your fork:
   ```bash
   git clone https://github.com/YOUR_USERNAME/duragraph-python.git
   cd duragraph-python
   ```

3. Install dependencies with uv:
   ```bash
   uv sync --all-extras
   ```

4. Install git hooks:
   ```bash
   lefthook install
   ```

## Development Workflow

### Running Tests

```bash
# Run all tests
pytest tests/

# Run with coverage
pytest tests/ --cov=duragraph --cov-report=html

# Run specific test
pytest tests/test_graph.py -v
```

### Linting & Formatting

```bash
# Check linting
ruff check src/

# Auto-fix linting issues
ruff check src/ --fix

# Format code
ruff format src/

# Type checking
mypy src/duragraph
```

## Commit Convention

We use [Conventional Commits](https://www.conventionalcommits.org/). All commit messages must follow this format:

```
<type>(<scope>): <description>

[optional body]

[optional footer(s)]
```

### Commit Types

| Type       | Description                                          | Example                                    |
|------------|------------------------------------------------------|--------------------------------------------|
| `feat`     | New feature                                          | `feat(graph): add subgraph support`        |
| `fix`      | Bug fix                                              | `fix(worker): handle connection timeout`   |
| `docs`     | Documentation only                                   | `docs: update installation guide`          |
| `style`    | Code style (formatting, semicolons, etc.)            | `style: fix indentation in nodes.py`       |
| `refactor` | Code change that neither fixes a bug nor adds feature| `refactor(cli): simplify command parsing`  |
| `perf`     | Performance improvement                              | `perf(graph): optimize node lookup`        |
| `test`     | Adding or updating tests                             | `test(worker): add registration tests`     |
| `build`    | Build system or dependencies                         | `build: update httpx to 0.28`              |
| `ci`       | CI/CD configuration                                  | `ci: add Python 3.13 to test matrix`       |
| `chore`    | Maintenance tasks                                    | `chore: update .gitignore`                 |
| `revert`   | Revert a previous commit                             | `revert: feat(graph): add subgraph support`|

### Scopes

Common scopes for this project:

- `graph` - Graph decorator and execution
- `nodes` - Node decorators (llm_node, tool_node, etc.)
- `edges` - Edge definitions
- `worker` - Worker and control plane integration
- `prompts` - Prompt store and management
- `cli` - Command-line interface
- `types` - Type definitions
- `deps` - Dependencies

### Examples

```bash
# Feature
git commit -m "feat(graph): add async streaming support"

# Bug fix with issue reference
git commit -m "fix(worker): prevent duplicate registration

Fixes #123"

# Breaking change
git commit -m "feat(nodes)!: rename @llm to @llm_node

BREAKING CHANGE: The @llm decorator has been renamed to @llm_node for clarity."

# Documentation
git commit -m "docs(readme): add streaming example"
```

## Pull Request Guidelines

### Before Submitting

- [ ] Code passes all tests (`pytest tests/`)
- [ ] Code passes linting (`ruff check src/`)
- [ ] Code is formatted (`ruff format src/`)
- [ ] Commit messages follow convention
- [ ] Documentation is updated if needed
- [ ] CHANGELOG.md is updated for user-facing changes

### PR Title

PR titles should also follow the commit convention:

```
feat(graph): add subgraph support
```

### PR Labels

PRs are automatically labeled based on files changed:

| Label          | Files                              |
|----------------|------------------------------------|
| `core`         | `src/duragraph/graph.py`, `nodes.py`, `edges.py` |
| `worker`       | `src/duragraph/worker/`            |
| `prompts`      | `src/duragraph/prompts/`           |
| `cli`          | `src/duragraph/cli/`               |
| `tests`        | `tests/`                           |
| `docs`         | `*.md`, `docs/`                    |
| `ci`           | `.github/`                         |
| `dependencies` | `pyproject.toml`, `*.lock`         |

### Review Process

1. All PRs require at least one approval
2. CI must pass (lint, test, build)
3. Maintainers may request changes
4. Squash merge is preferred for clean history

## Releasing

Releases are handled by maintainers:

1. Update version in `pyproject.toml`
2. Update `CHANGELOG.md`
3. Create a GitHub release with tag `vX.Y.Z`
4. GitHub Actions automatically publishes to PyPI

## Code of Conduct

Be respectful and inclusive. We follow the [Contributor Covenant](https://www.contributor-covenant.org/).

## Questions?

- Open a [GitHub Discussion](https://github.com/duragraph/duragraph-python/discussions)
- Check existing [Issues](https://github.com/duragraph/duragraph-python/issues)
