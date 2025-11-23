FROM mcr.microsoft.com/devcontainers/base:noble

# Versions
ARG GO_VERSION=1.25.4
ARG NODE_VERSION=22.12
ARG PYTHON_VERSION=3.12

# Install common utilities
RUN apt-get update && apt-get install -y \
    build-essential \
    curl \
    git \
    jq \
    yq \
    make \
    unzip \
    wget \
    zsh \
    postgresql-client \
    && rm -rf /var/lib/apt/lists/*

# Install Go (detect architecture)
RUN ARCH=$(dpkg --print-architecture) && \
    if [ "$ARCH" = "arm64" ]; then GOARCH="arm64"; else GOARCH="amd64"; fi && \
    curl -fsSL "https://go.dev/dl/go${GO_VERSION}.linux-${GOARCH}.tar.gz" | tar -C /usr/local -xzf -
ENV PATH="/usr/local/go/bin:${PATH}"
ENV GOPATH="/go"
ENV PATH="${GOPATH}/bin:${PATH}"

# Install Go tools (CGO_ENABLED=0 to avoid gcc issues)
RUN CGO_ENABLED=0 go install golang.org/x/tools/gopls@latest && \
    CGO_ENABLED=0 go install github.com/go-delve/delve/cmd/dlv@latest && \
    CGO_ENABLED=0 go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest && \
    CGO_ENABLED=0 go install golang.org/x/tools/cmd/goimports@latest

# Create directories for vscode user first
RUN mkdir -p /home/vscode/.local/bin /home/vscode/.local/share /home/vscode/.cache \
    && chown -R vscode:vscode /home/vscode/.local /home/vscode/.cache

# Switch to vscode user for all tooling installs
USER vscode

# Install fnm (Fast Node Manager)
RUN curl -fsSL https://fnm.vercel.app/install | bash -s -- --skip-shell
ENV PATH="/home/vscode/.local/share/fnm:${PATH}"

# Install Node.js and pnpm
RUN eval "$(fnm env)" && fnm install ${NODE_VERSION} && fnm default ${NODE_VERSION}
RUN eval "$(fnm env)" && npm install -g pnpm@9

# Install uv
RUN curl -LsSf https://astral.sh/uv/install.sh | sh
ENV PATH="/home/vscode/.local/bin:${PATH}"

# Install Python via uv
RUN uv python install ${PYTHON_VERSION}

# Install Python tools via uv
RUN uv tool install pre-commit --with pre-commit-uv && \
    uv tool install ruff && \
    uv tool install black

# Switch back to root for remaining setup
USER root

# Install Task (go-task)
RUN sh -c "$(curl --location https://taskfile.dev/install.sh)" -- -d -b /usr/local/bin

# Install GitHub CLI
RUN curl -fsSL https://cli.github.com/packages/githubcli-archive-keyring.gpg | dd of=/usr/share/keyrings/githubcli-archive-keyring.gpg && \
    chmod go+r /usr/share/keyrings/githubcli-archive-keyring.gpg && \
    echo "deb [arch=$(dpkg --print-architecture) signed-by=/usr/share/keyrings/githubcli-archive-keyring.gpg] https://cli.github.com/packages stable main" | tee /etc/apt/sources.list.d/github-cli.list > /dev/null && \
    apt-get update && apt-get install -y gh && \
    rm -rf /var/lib/apt/lists/*

# Install Act (nektos/act) for local GitHub Actions testing
RUN ARCH=$(dpkg --print-architecture) && \
    if [ "$ARCH" = "arm64" ]; then ACT_ARCH="arm64"; else ACT_ARCH="x86_64"; fi && \
    curl -s https://api.github.com/repos/nektos/act/releases/latest | \
    grep "browser_download_url.*Linux_${ACT_ARCH}.tar.gz" | \
    cut -d '"' -f 4 | \
    wget -qi - && \
    tar xzf act_Linux_${ACT_ARCH}.tar.gz -C /usr/local/bin act && \
    rm act_Linux_${ACT_ARCH}.tar.gz && \
    chmod +x /usr/local/bin/act

# Set zsh as default shell
RUN chsh -s /usr/bin/zsh vscode

# Create go directory and set permissions
RUN mkdir -p /go && chown -R vscode:vscode /go

# Setup PATH for vscode user (include fnm path)
ENV FNM_DIR="/home/vscode/.local/share/fnm"
ENV PATH="/home/vscode/.local/share/fnm:/home/vscode/.local/bin:/usr/local/go/bin:/go/bin:${PATH}"

USER vscode

# Install Claude CLI via npm (requires fnm eval to access npm)
RUN eval "$(fnm env)" && npm install -g @anthropic-ai/claude-code

WORKDIR /workspace

# Source fnm for node/npm/pnpm availability
RUN echo 'eval "$(fnm env)"' >> ~/.zshrc && \
    echo 'eval "$(fnm env)"' >> ~/.bashrc
