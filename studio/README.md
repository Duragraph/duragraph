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

## Demo cheat sheet

For a live walkthrough against the local-dev stack from
[`duragraph-examples`](../duragraph-examples). Assumes:

- Control plane on host port `18081` (per the `duragraph-dev-app` container)
- Studio dev server on host port `13300`
- Chatbot worker running with `OPENROUTER_API_KEY` set, model `openai/gpt-4o-mini`

### SSH port forwarding (run on your laptop)

```bash
ssh -L 13300:localhost:13300 \
    -L 18081:localhost:18081 \
    vps-host
```

Then open `http://localhost:13300` in your browser.

### Health and topology

```bash
# Control plane health
curl -s http://localhost:18081/health | jq

# All registered workers (graph_id is the routing key)
curl -s http://localhost:18081/api/v1/workers \
  | jq '.workers[] | {worker_id, status, graphs}'

# All assistants (named, configured uses of a graph)
curl -s http://localhost:18081/api/v1/assistants \
  | jq '.assistants[] | {assistant_id, name, graph_id: .metadata.graph_id}'
```

### Trigger a chatbot run from the CLI (LangChain-shape input)

```bash
CP=http://localhost:18081
ASSISTANT_ID=<paste from /api/v1/assistants>
THREAD_ID=$(curl -s -X POST $CP/api/v1/threads \
  -H "Content-Type: application/json" -d '{}' | jq -r .thread_id)

RUN_ID=$(curl -s -X POST "$CP/api/v1/threads/$THREAD_ID/runs" \
  -H "Content-Type: application/json" \
  -d "{\"assistant_id\":\"$ASSISTANT_ID\",
       \"input\":{\"messages\":[{\"role\":\"user\",\"content\":\"In one sentence, what does DuraGraph do?\"}]}}" \
  | jq -r .run_id)

# Watch it complete
sleep 6
curl -s "$CP/api/v1/runs/$RUN_ID" \
  | jq '{status, model: .output.model, provider: .output.provider,
         response: .output.response, usage: .output.usage}'
```

Expected: `status: "completed"`, real LLM response, token usage populated.

### Inspect the event-sourced trail (proves it's not just an HTTP echo)

```bash
docker exec duragraph-dev-postgres psql -U duragraph -d duragraph \
  -c "SELECT event_type, occurred_at FROM events
      WHERE aggregate_id='$RUN_ID' ORDER BY id;"
```

You should see `RunCreated → RunStarted → RunCompleted` with timestamps.

### Show the outbox (events durably shipped to NATS)

```bash
docker exec duragraph-dev-postgres psql -U duragraph -d duragraph \
  -c "SELECT event_type, published, attempts FROM outbox
      WHERE aggregate_id='$RUN_ID' ORDER BY id;"
```

All rows should be `published = t`, `attempts = 0`.

### Live worker logs (proves user code runs in the worker, not the control plane)

```bash
tail -f /tmp/chatbot.log     # OpenRouter calls + node-event posts
tail -f /tmp/tool-use.log    # Wikipedia / wttr.in HTTP calls
tail -f /tmp/rag-agent.log   # RAG retrieval pipeline
```

### Switch the LLM provider without changing code

The chatbot worker is wired through OpenRouter's OpenAI-compatible endpoint. To
route the same code to a different upstream model, set
`OPENROUTER_MODEL` and restart the worker:

```bash
OPENROUTER_MODEL=anthropic/claude-3.5-sonnet uv run --with-editable ../duragraph-python --with openai python main.py
# or anything from https://openrouter.ai/models
```

No code change. That's a strong architectural talking point: the worker is
provider-agnostic.

### Talking points

- **Workers register graph capabilities; assistants are configured uses of a
  graph.** Many-to-many between assistants and workers, both decoupled from the
  graph definition itself.
- **Runs go through the Run aggregate's state machine** — `queued → running →
  completed`. Try cancelling a completed run to demo illegal-transition
  rejection (`409 Conflict`).
- **Every state change is an event.** The `events` table is append-only; the
  `outbox` table bridges the synchronous DB transaction to the asynchronous
  NATS bus.
- **User code runs in the Python worker process**, never on the Go control
  plane. The control plane orchestrates dispatch, leases, and persistence; the
  worker owns the `@node` method bodies.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for development guidelines.

## Related Projects

- [DuraGraph](https://github.com/Duragraph/duragraph) - Control plane
- [DuraGraph Dashboard](https://github.com/Duragraph/duragraph/tree/main/dashboard) - Admin UI
- [DuraGraph Examples](https://github.com/Duragraph/duragraph-examples) - Example agents

## License

Apache 2.0 - See [LICENSE](LICENSE) for details.
