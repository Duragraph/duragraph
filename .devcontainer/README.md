# DuraGraph Development Container

Complete development environment for DuraGraph with all tools pre-installed.

## 🚀 What's Included

### Languages & Runtimes
- **Go 1.25.4** - API server development
- **Node.js 22.12** (via fnm) - Frontend development
- **Python 3.12** (via uv) - Testing & tooling

### Package Managers
- **pnpm 9** - Fast, efficient Node.js package manager
- **uv** - Ultra-fast Python package installer
- **go modules** - Go dependency management

### Development Tools
- **Task** - Task runner (replacement for Make)
- **Act** - Run GitHub Actions locally
- **GitHub CLI (gh)** - GitHub from the command line
- **Docker-in-Docker** - Build and run Docker containers
- **PostgreSQL client** - Database access
- **Git** - Version control

### Go Tools
- **gopls** - Go language server
- **delve (dlv)** - Go debugger
- **golangci-lint** - Go linter
- **goimports** - Import formatter

### Python Tools
- **pre-commit** - Git hooks framework
- **ruff** - Fast Python linter
- **black** - Python formatter
- **pytest** - Testing framework

### Shell
- **zsh** - Default shell with better experience
- **bash** - Available as alternative

## 📦 Services

The devcontainer includes these services via Docker Compose:

- **PostgreSQL 15** - Database (port 5432)
- **NATS JetStream** - Message broker (port 4222, monitor: 8222)

## 🔌 Port Forwarding

| Port | Service | Auto-forward |
|------|---------|--------------|
| 8080 | API Server | Notify |
| 8081 | API Server (alt) | Notify |
| 5173 | Dashboard (Svelte) | Notify |
| 3000 | Docs/Website | Notify |
| 5432 | PostgreSQL | Silent |
| 4222 | NATS | Silent |
| 8222 | NATS Monitor | Silent |

## 🎨 VS Code Extensions

### Go Development
- `golang.go` - Go language support
- `ms-azuretools.vscode-docker` - Docker support

### Python Development
- `ms-python.python` - Python support
- `ms-python.vscode-pylance` - Python language server

### Frontend Development
- `svelte.svelte-vscode` - Svelte support
- `dbaeumer.vscode-eslint` - ESLint
- `esbenp.prettier-vscode` - Prettier formatter
- `bradlc.vscode-tailwindcss` - Tailwind CSS

### Productivity
- `task.vscode-task` - Task runner integration
- `anthropic.claude-code` - Claude Code AI assistant
- `github.copilot` - GitHub Copilot
- `github.copilot-chat` - Copilot chat
- `eamodio.gitlens` - Git supercharged

### CI/CD & Testing
- `nektos.act` - Run GitHub Actions locally
- `github.vscode-github-actions` - GitHub Actions editor

### Code Quality
- `usernamehw.errorlens` - Inline error display
- `gruntfuggly.todo-tree` - TODO comments tree
- `streetsidesoftware.code-spell-checker` - Spell checker

## 🚀 Quick Start

### First Time Setup

The devcontainer runs `post-create.sh` automatically, which:
1. Installs Go dependencies
2. Installs Node.js dependencies (dashboard, website, docs)
3. Sets up pre-commit hooks
4. Configures Act for local GitHub Actions testing
5. Configures git defaults

### Common Commands

```bash
# Development
task dev              # Start API server
task dashboard:dev    # Start dashboard dev server
task website:dev      # Start website dev server

# Testing
task test             # Run all tests
task test:unit        # Run unit tests only
task conformance      # Run LangGraph conformance tests

# GitHub Actions (local)
task act:setup        # Setup Act configuration
task act:list         # List all workflows
task act:ci           # Run CI workflow locally
task act:job -- unit_go  # Run specific job

# Building
task build            # Build all components
task build:server     # Build Go binary
task build:dashboard  # Build dashboard for production
task docs:build       # Build docs + website

# Docker
task up               # Start all services
task down             # Stop all services
task logs             # View logs
task health           # Check service health

# Database
task db:psql          # Connect to PostgreSQL
task db:migrate       # Run migrations
task db:reset         # Reset database

# Code Quality
task lint             # Lint all code
task format           # Format all code
task pre-commit       # Run pre-commit checks

# Utilities
task --list           # Show all available tasks
task clean            # Clean build artifacts
```

## 🎬 Testing GitHub Actions Locally

The devcontainer includes **Act** for running GitHub Actions locally:

```bash
# Setup Act (creates .secrets and .env files)
task act:setup

# Edit secrets file (optional - for LLM API keys)
nano .github/workflows/.secrets

# List all workflows
task act:list

# Run full CI pipeline
task act:ci

# Run specific job
task act:job -- unit_go

# Dry run (preview what would run)
task act:ci:dry
```

See [README.act.md](../README.act.md) for full Act guide.

## 🔧 Configuration

### Environment Variables

The devcontainer sets these automatically:
- `DOCKER_BUILDKIT=1` - Use BuildKit for Docker builds
- `COMPOSE_DOCKER_CLI_BUILD=1` - Use BuildKit with Compose
- `PATH` - Includes Go, Node, Python, and tool binaries

### Git Configuration

Post-create script sets:
- Default branch: `main`
- Pull strategy: `rebase`
- Auto-prune on fetch

### Shell Configuration

- Default shell: `zsh`
- fnm (Fast Node Manager) auto-loaded in `.zshrc` and `.bashrc`
- Python via uv available in `~/.local/bin`

## 📂 Workspace Mounts

- `.claude` directory is mounted from host (`~/.claude` → `/home/vscode/.claude`)
  - Preserves Claude Code configuration across rebuilds

## 🔄 Rebuilding the Container

If you need to rebuild (e.g., to update tools):

```bash
# From VS Code
# 1. Open Command Palette (Cmd/Ctrl+Shift+P)
# 2. Type: "Dev Containers: Rebuild Container"

# Or manually
docker compose -f .devcontainer/docker-compose.yml down
docker compose -f .devcontainer/docker-compose.yml up -d --build
```

## 🐛 Troubleshooting

### Go modules not working
```bash
go mod download
go mod tidy
```

### Node/pnpm not found
```bash
eval "$(fnm env)"
fnm install 22.12
```

### Python not found
```bash
uv python install 3.12
```

### Act not working
```bash
# Verify installation
act --version

# Reinstall if needed
task act:install
```

### Docker socket issues
```bash
# Docker-in-Docker is enabled, socket should be available at:
ls -la /var/run/docker.sock

# If issues persist, rebuild container
```

### Port already in use
```bash
# Check what's using the port
sudo lsof -i :8080

# Or use different port in Taskfile.yml
```

## 📚 Resources

- [VS Code Dev Containers](https://code.visualstudio.com/docs/devcontainers/containers)
- [Task Documentation](https://taskfile.dev/)
- [Act Documentation](https://github.com/nektos/act)
- [fnm (Fast Node Manager)](https://github.com/Schniz/fnm)
- [uv (Python)](https://github.com/astral-sh/uv)

## 🆘 Support

- Check [Taskfile.yml](../Taskfile.yml) for available commands
- See [README.act.md](../README.act.md) for Act usage
- Review [.github/workflows/](../.github/workflows/) for CI/CD

---

**Happy coding!** 🚀
