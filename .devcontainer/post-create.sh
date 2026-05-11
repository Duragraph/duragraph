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

# Configure git user from environment variables (optional)
# Only set if provided - allows contributors to use their own git config
if [ -n "$GIT_USER_NAME" ]; then
    echo "📝 Configuring git user: $GIT_USER_NAME"
    git config --global user.name "$GIT_USER_NAME"
else
    echo "ℹ️  GIT_USER_NAME not set, using your existing git config"
fi

if [ -n "$GIT_USER_EMAIL" ]; then
    echo "📧 Configuring git email: $GIT_USER_EMAIL"
    git config --global user.email "$GIT_USER_EMAIL"
else
    echo "ℹ️  GIT_USER_EMAIL not set, using your existing git config"
fi

# Import GPG key and configure git signing (OPTIONAL - only for maintainers)
# Contributors working on forks do NOT need GPG signing enabled
# This only affects your devcontainer, not the repository requirements
if [ -n "$GPG_PRIVATE_KEY" ] && [ -n "$GPG_KEY_ID" ]; then
    echo "🔐 Importing GPG private key..."

    # Create GPG directory if it doesn't exist
    mkdir -p ~/.gnupg
    chmod 700 ~/.gnupg

    # Import the private key (expects base64 encoded key)
    # Use error handling to prevent script failure
    if echo "$GPG_PRIVATE_KEY" | base64 -d | gpg --batch --import 2>/dev/null; then
        echo "🔑 Configuring GPG key ID: $GPG_KEY_ID"

        # Set ultimate trust for the key (non-interactive)
        # Suppress errors if key is already trusted
        echo -e "5\ny\n" | gpg --command-fd 0 --expert --edit-key "$GPG_KEY_ID" trust 2>/dev/null || true

        # Configure git to use this key for signing LOCALLY (in devcontainer only)
        git config --global user.signingkey "$GPG_KEY_ID"

        # Only enable automatic signing if explicitly requested
        # This allows you to sign commits when pushing to main repo
        # but doesn't force signing for all commits (e.g., when testing)
        if [ "$ENABLE_GPG_SIGNING" = "true" ]; then
            git config --global commit.gpgsign true
            git config --global tag.gpgsign true
            echo "✅ GPG key imported and automatic commit signing enabled"
        else
            echo "✅ GPG key imported (use 'git commit -S' to sign commits manually)"
            echo "ℹ️  Set ENABLE_GPG_SIGNING=true to enable automatic signing"
        fi

        # Configure GPG to use TTY (needed for devcontainer)
        # Only add if not already present to maintain idempotency
        grep -q "export GPG_TTY" ~/.bashrc 2>/dev/null || echo "export GPG_TTY=\$(tty)" >> ~/.bashrc
        grep -q "export GPG_TTY" ~/.zshrc 2>/dev/null || echo "export GPG_TTY=\$(tty)" >> ~/.zshrc
    else
        echo "⚠️  Failed to import GPG key - check that GPG_PRIVATE_KEY is base64 encoded correctly"
        echo "ℹ️  Continuing without GPG signing (not required for development)"
    fi
elif [ -n "$GPG_PRIVATE_KEY" ] || [ -n "$GPG_KEY_ID" ]; then
    echo "⚠️  Both GPG_PRIVATE_KEY and GPG_KEY_ID must be set for GPG signing"
    echo "ℹ️  Continuing without GPG signing (not required for development)"
else
    echo "ℹ️  GPG not configured (optional - only needed for maintainers pushing to main repo)"
fi

# Authenticate GitHub CLI (optional - for maintainers)
if [ -n "$GH_PAT" ]; then
    echo "🔑 Authenticating GitHub CLI..."
    echo "$GH_PAT" | gh auth login --with-token
    echo "✅ GitHub CLI authenticated"
else
    echo "ℹ️  GH_PAT not set, skipping GitHub CLI authentication"
fi

echo ""
echo "✅ Development environment ready!"
echo ""
echo "💡 PostgreSQL and NATS are running via devcontainer sidecars."
echo "   Or run 'duragraph dev' for an embedded Postgres + NATS one-shot."
echo ""
echo "Quick commands:"
echo "  duragraph dev          - Engine + dashboard on :8081 (embedded data plane)"
echo "  task dev               - Engine against the sidecar Postgres + NATS"
echo "  task dashboard:dev     - Vite dev server on :3303"
echo "  task gen:types         - Regenerate dashboard types from Go DTOs"
echo "  task test              - Run all tests"
echo "  task test:conformance  - API conformance suite"
echo "  task --list            - See all available tasks"
