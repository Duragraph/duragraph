# DuraGraph Studio

Interactive UI for AI agent interaction, reasoning visualization, and human-in-the-loop workflows.

[![DuraGraph](https://img.shields.io/badge/DuraGraph-v0.3.0-blue)](https://github.com/Duragraph/duragraph)
[![License](https://img.shields.io/badge/License-Apache%202.0-green)](LICENSE)

## Overview

DuraGraph Studio is a React-based UI for interacting with AI agents powered by DuraGraph. Unlike the admin dashboard (for monitoring and management), Studio focuses on the **end-user experience** of working with agents.

### Key Features

- **Chat Interface** - Conversational interaction with streaming responses
- **Agent Reasoning** - Visualize agent thinking, steps, and tool calls in real-time
- **Human-in-the-Loop** - Handle approval workflows and user decisions
- **Run Inspector** - Debug and inspect agent execution

## Screenshots

*Coming soon*

## Quick Start

### Using Docker (Recommended)

```bash
docker run -p 3000:80 \
  -e VITE_DURAGRAPH_API_URL=http://localhost:8081 \
  ghcr.io/duragraph/duragraph-studio:latest
```

Open http://localhost:3000

### Local Development

```bash
# Install dependencies
pnpm install

# Start dev server
pnpm dev

# Build for production
pnpm build
```

## Configuration

| Environment Variable | Default | Description |
|---------------------|---------|-------------|
| `VITE_DURAGRAPH_API_URL` | `http://localhost:8081` | DuraGraph API URL |

## Tech Stack

- **Framework:** React 19
- **Build:** Vite
- **Language:** TypeScript
- **Styling:** TailwindCSS + shadcn/ui
- **State:** TanStack Query + Zustand
- **Routing:** TanStack Router

## Views

### Chat

Primary conversational interface with streaming responses, markdown rendering, and thread management.

### Agent Trace

Visualize agent reasoning like in Cursor - see steps appearing as the agent thinks, tool calls with inputs/outputs.

### Approvals

Handle human-in-the-loop workflows - approve actions, make choices, provide input when agents need human guidance.

### Run Inspector

Debug specific runs with full event timeline, state snapshots, and error details.

## Architecture

```
src/
├── components/
│   ├── ui/           # shadcn/ui primitives
│   ├── chat/         # Chat components
│   ├── agent/        # Reasoning visualization
│   └── approval/     # Human-in-the-loop
├── views/            # Page components
├── hooks/            # Custom React hooks
├── stores/           # Zustand stores
└── lib/              # Utilities
```

## Integration with DuraGraph

Studio connects to DuraGraph via:

- **REST API** - Create runs, fetch threads, submit approvals
- **SSE Streaming** - Real-time updates for agent execution

```typescript
// Example: Using the useChat hook
const { messages, sendMessage, isStreaming } = useChat({
  threadId: 'thread_123',
  assistantId: 'assistant_456',
})
```

## Development

```bash
# Install dependencies
pnpm install

# Run dev server with hot reload
pnpm dev

# Type checking
pnpm typecheck

# Linting
pnpm lint

# Build production bundle
pnpm build

# Preview production build
pnpm preview
```

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for development guidelines.

## Related Projects

- [DuraGraph](https://github.com/Duragraph/duragraph) - Control plane
- [DuraGraph Dashboard](https://github.com/Duragraph/duragraph/tree/main/dashboard) - Admin UI
- [DuraGraph Examples](https://github.com/Duragraph/duragraph-examples) - Example agents

## License

Apache 2.0 - See [LICENSE](LICENSE) for details.
