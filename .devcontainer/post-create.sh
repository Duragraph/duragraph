#!/bin/bash
set -e

echo "🚀 Setting up DuraGraph development environment..."

# Install Go tools
echo "📦 Installing Go tools..."
go install golang.org/x/tools/gopls@latest
go install github.com/go-delve/delve/cmd/dlv@latest
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
go install golang.org/x/tools/cmd/goimports@latest

# Install project dependencies
echo "📦 Installing Go dependencies..."
go mod download

# Install Node.js dependencies
echo "📦 Installing dashboard dependencies..."
[ -d dashboard ] && (cd dashboard && pnpm install)

echo "📦 Installing website dependencies..."
[ -d website ] && (cd website && pnpm install)

echo "📦 Installing docs dependencies..."
[ -d docs ] && (cd docs && pnpm install)

# Install Playwright browsers for E2E testing
echo "🎭 Installing Playwright browsers..."
if [ -d dashboard ] && [ -f dashboard/package.json ]; then
    if grep -q "@playwright/test" dashboard/package.json; then
        echo "Installing Playwright browsers for dashboard E2E tests..."
        cd dashboard && pnpm exec playwright install --with-deps chromium firefox webkit
        cd ..
        echo "✅ Playwright browsers installed"
    else
        echo "ℹ️  Playwright not found in dashboard, skipping browser installation"
    fi
fi

# Setup pre-commit hooks
echo "🪝 Setting up pre-commit hooks..."
~/.local/bin/pre-commit install
~/.local/bin/pre-commit install --hook-type commit-msg

# Setup git config for better experience
git config --global init.defaultBranch main
git config --global pull.rebase true
git config --global fetch.prune true

# Configure git user from environment variables
if [ -n "$GIT_USER_NAME" ]; then
    echo "📝 Configuring git user: $GIT_USER_NAME"
    git config --global user.name "$GIT_USER_NAME"
fi

if [ -n "$GIT_USER_EMAIL" ]; then
    echo "📧 Configuring git email: $GIT_USER_EMAIL"
    git config --global user.email "$GIT_USER_EMAIL"
fi

# Import GPG key and configure git signing
if [ -n "$GPG_PRIVATE_KEY" ]; then
    echo "🔐 Importing GPG private key..."

    # Create GPG directory if it doesn't exist
    mkdir -p ~/.gnupg
    chmod 700 ~/.gnupg

    # Import the private key (expects base64 encoded key)
    echo "$GPG_PRIVATE_KEY" | base64 -d | gpg --batch --import

    # Trust the key ultimately (non-interactive)
    if [ -n "$GPG_KEY_ID" ]; then
        echo "🔑 Configuring GPG key ID: $GPG_KEY_ID"

        # Set ultimate trust for the key
        echo -e "5\ny\n" | gpg --command-fd 0 --expert --edit-key "$GPG_KEY_ID" trust

        # Configure git to use this key for signing
        git config --global user.signingkey "$GPG_KEY_ID"
        git config --global commit.gpgsign true
        git config --global tag.gpgsign true

        # Configure GPG to use TTY (needed for devcontainer)
        echo "export GPG_TTY=\$(tty)" >> ~/.bashrc
        echo "export GPG_TTY=\$(tty)" >> ~/.zshrc

        echo "✅ GPG key imported and git signing enabled"
    else
        echo "⚠️  GPG_KEY_ID not set, skipping git signing configuration"
    fi
else
    echo "ℹ️  GPG_PRIVATE_KEY not set, skipping GPG import"
fi

# Setup Act configuration
echo "🎬 Setting up Act (GitHub Actions local runner)..."
# task act:setup 2>/dev/null || echo "⚠️  Run 'task act:setup' manually to configure Act"

# Verify Act installation
if command -v act &> /dev/null; then
    echo "✅ Act installed: $(act --version)"
else
    echo "⚠️  Act installation failed, run 'task act:install' to retry"
fi

echo ""
echo "✅ Development environment ready!"
echo ""
echo "💡 PostgreSQL and NATS are already running via devcontainer!"
echo ""
echo "Quick commands:"
echo "  task up          - Start all services"
echo "  task dev         - Run API server in dev mode"
echo "  task dashboard:dev - Run dashboard dev server"
echo "  task docs:build  - Build docs + website"
echo "  task test        - Run all tests"
echo ""
echo "GitHub Actions (local testing with Act):"
echo "  task act:setup   - Setup Act configuration & secrets"
echo "  task act:list    - List all workflows"
echo "  task act:ci      - Run CI workflow locally"
echo "  task conformance - Run LangGraph conformance tests"
echo ""
echo "  task --list      - See all available tasks"
