# Tool Use

Demonstrates how to define and use tools with DuraGraph's `@tool` decorator and `@tool_node()`.

## What This Example Demonstrates

- Defining tools with the `@tool` decorator and automatic JSON Schema generation
- Using `@tool_node()` to create a node that executes tools
- Combining tool results with graph state for final responses
- Registering tools in the global `ToolRegistry`

## Prerequisites

- DuraGraph control plane running at `http://localhost:8081`
- Python 3.11+

## Quick Start

1. **Run the example:**

   > Always use `uv`. Never `pip install`, never `python -m venv`, never `source .venv/bin/activate`.

   ```bash
   DURAGRAPH_URL=http://localhost:18081 PYTHONUNBUFFERED=1 \
     uv run --with-editable ../../../python \
     python main.py
   ```

3. **Trigger a run** (in another terminal):
   ```bash
   curl -X POST http://localhost:8081/api/v1/runs \
     -H "Content-Type: application/json" \
     -d '{
       "assistant_id": "tool_use_agent",
       "thread_id": "test-thread",
       "input": {"input": "What is the weather in Tokyo?"}
     }'
   ```

## Architecture

```
prepare → run_tools → synthesize
```

- **prepare** — Formats user input and initializes tracking state
- **run_tools** — Routes to the appropriate tool based on the query
- **synthesize** — Combines tool results into a human-readable response

## Tools Defined

| Tool | Description |
|------|-------------|
| `get_weather` | Returns simulated weather data for a city |
| `search_documents` | Keyword search over a sample document set |
| `calculate` | Evaluates a mathematical expression |

## Production Usage

In production with an LLM provider configured, use `@llm_node(tools=[get_weather, search_documents])` to let the LLM decide which tools to call. The executor will automatically resolve tool calls and feed results back to the LLM.

## Configuration

| Environment Variable | Default | Description |
|---------------------|---------|-------------|
| `DURAGRAPH_URL` | `http://localhost:8081` | Control plane URL |

## Next Steps

- [07-evals](../07-evals) — Test and evaluate graph outputs
- [04-multi-agent](../04-multi-agent) — Compose multiple agents
