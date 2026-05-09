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
style/short-description   # Styling changes
refactor/short-description # Code refactoring
docs/short-description    # Documentation
chore/short-description   # Maintenance
```

### Commit Messages

Follow conventional commits:

```
feat: add chat message streaming
fix: resolve SSE connection drop
style: update button hover states
refactor: extract useStream hook
docs: add API integration guide
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
git worktree add ../worktrees/duragraph-studio-feat-chat feat/chat-streaming

# List all worktrees
git worktree list
```

### Symlink Untracked Files

Gitignored files (CLAUDE.md, spec/) are NOT copied to worktrees. Use symlinks:

```bash
cd ../worktrees/duragraph-studio-feat-chat

# Symlink CLAUDE.md
ln -s /home/qwe/platform/duragraph-org/duragraph-studio/CLAUDE.md CLAUDE.md

# Symlink spec folder
ln -s /home/qwe/platform/duragraph-org/duragraph-studio/spec spec
```

### Merge and Cleanup

After feature is complete and PR is merged:

```bash
# From main worktree
cd /home/qwe/platform/duragraph-org/duragraph-studio

# Remove the worktree (deletes the directory)
git worktree remove ../worktrees/duragraph-studio-feat-chat

# Delete the local branch if merged
git branch -d feat/chat-streaming

# Prune stale worktree references
git worktree prune
```

**Note:** Symlinked files remain in the original location and are not affected by worktree removal.

## Project Overview

DuraGraph Studio is an interactive UI for AI agent interaction, reasoning visualization, and human-in-the-loop workflows.

### Tech Stack

- **Framework:** React 19
- **Build:** Vite
- **Language:** TypeScript
- **Styling:** TailwindCSS + shadcn/ui
- **State:** TanStack Query + Zustand
- **Routing:** TanStack Router

### Theme

Uses "Engineered Precision" theme matching the DuraGraph Dashboard:
- Coral/Orange primary (`#f97316`)
- Zero border radius (sharp, geometric)
- Space Grotesk + JetBrains Mono fonts
- Floating cards with shadows

### Directory Structure

```
src/
├── components/       # Reusable UI components
│   ├── ui/          # shadcn/ui primitives
│   ├── chat/        # Chat components
│   ├── agent/       # Reasoning visualization
│   └── approval/    # Human-in-the-loop
├── views/           # Page components
├── hooks/           # Custom React hooks
├── stores/          # Zustand stores
├── lib/             # Utilities
└── types/           # TypeScript types
```

## Development Commands

```bash
# Install dependencies
pnpm install

# Start dev server
pnpm dev

# Type checking
pnpm typecheck

# Linting
pnpm lint

# Build for production
pnpm build
```

## Key Features to Implement

1. **Chat Interface** - Conversational interaction with streaming
2. **Agent Trace** - Visualize reasoning steps and tool calls
3. **Approvals** - Human-in-the-loop workflows
4. **Run Inspector** - Debug agent execution

## API Integration

Studio connects to DuraGraph control plane via:
- REST API for CRUD operations
- SSE streaming for real-time updates

Default API URL: `http://localhost:8081`

## Docker

```bash
# Build image
docker build -t duragraph/studio .

# Run container
docker run -p 3000:80 -e VITE_DURAGRAPH_API_URL=http://localhost:8081 duragraph/studio
```
