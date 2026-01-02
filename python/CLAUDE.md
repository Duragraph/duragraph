# CLAUDE.md

This file provides guidance to Claude Code when working with this repository.

## Git Workflow

**IMPORTANT: The `main` branch is protected. All changes MUST go through pull requests.**

### Rules

1. **NEVER push directly to `main`** - Always create a feature branch
2. **ALWAYS create a PR** for any changes
3. **Use conventional commits** for commit messages

### Branch Naming

```
feat/short-description    # New features
fix/short-description     # Bug fixes
refactor/short-description # Code refactoring
docs/short-description    # Documentation
test/short-description    # Test improvements
chore/short-description   # Maintenance
```

### Commit Messages

Follow conventional commits:

```
feat: add streaming support to Graph class
fix: resolve worker connection timeout
refactor: extract prompt loader module
docs: add CLI usage examples
test: add integration tests for LLM nodes
chore: update dependencies
```

### PR Workflow

```bash
# 1. Create feature branch
git checkout -b feat/my-feature

# 2. Make changes and commit
git add .
git commit -m "feat: description"

# 3. Push branch
git push -u origin feat/my-feature

# 4. Create PR
gh pr create --title "feat: description" --body "..."
```

## Git Worktree

Use git worktree for parallel development on multiple features.

### Creating a Worktree

```bash
# Create worktree for a new feature branch
git worktree add ../worktrees/duragraph-python-feat-streaming feat/streaming-api

# List all worktrees
git worktree list
```

### Symlink Untracked Files

Gitignored files (CLAUDE.md, specs/) are NOT copied to worktrees. Use symlinks:

```bash
cd ../worktrees/duragraph-python-feat-streaming

# Symlink CLAUDE.md
ln -s /home/qwe/platform/duragraph-org/duragraph-python/CLAUDE.md CLAUDE.md

# Symlink specs folder
ln -s /home/qwe/platform/duragraph-org/duragraph-python/specs specs
```

### Merge and Cleanup

After feature is complete and PR is merged:

```bash
# From main worktree
cd /home/qwe/platform/duragraph-org/duragraph-python

# Remove the worktree (deletes the directory)
git worktree remove ../worktrees/duragraph-python-feat-streaming

# Delete the local branch if merged
git branch -d feat/streaming-api

# Prune stale worktree references
git worktree prune
```

**Note:** Symlinked files remain in the original location and are not affected by worktree removal.

## Project Overview

DuraGraph Python SDK for building AI agents with decorators, deploying to a control plane, and getting full observability.

### Tech Stack

- **Language:** Python 3.10+
- **Build:** uv / pip
- **Testing:** pytest
- **Linting:** ruff
- **Type Checking:** mypy

### Directory Structure

```
duragraph-python/
├── src/duragraph/       # Main package
│   ├── graph/           # Graph definition
│   ├── worker/          # Worker runtime
│   ├── prompts/         # Prompt management
│   └── cli/             # CLI commands
├── tests/               # Test suite
├── examples/            # Usage examples
├── docs/                # Documentation
└── specs/               # Internal specs (gitignored)
```

## Development Commands

```bash
# Install dependencies
uv sync

# Run tests
uv run pytest

# Run tests with coverage
uv run pytest --cov=duragraph

# Lint
uv run ruff check .

# Format
uv run ruff format .

# Type check
uv run mypy src/

# Build package
uv build

# Run example
uv run python examples/hello_world.py
```

## Key Concepts

- **Graph** - Decorator-based workflow definition
- **Nodes** - LLM, tool, router, human-in-loop
- **Worker** - Connects to control plane
- **Streaming** - SSE event stream support
