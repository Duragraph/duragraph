# Hello World

The simplest possible DuraGraph example - a worker with a basic two-node graph.

## What This Example Demonstrates

- Creating a graph using the `@Graph` decorator
- Defining nodes with the `@node` decorator
- Registering a worker with the DuraGraph control plane
- Running a worker that listens for and executes runs

## Prerequisites

- DuraGraph control plane running at `http://localhost:8081`
- Python 3.11+

## Quick Start

1. **Install dependencies:**
   ```bash
   pip install -r requirements.txt
   ```

2. **Run the worker:**
   ```bash
   python main.py
   ```

3. **Trigger a run** (in another terminal):
   ```bash
   curl -X POST http://localhost:8081/api/v1/runs \
     -H "Content-Type: application/json" \
     -d '{
       "assistant_id": "hello-world",
       "thread_id": "test-thread",
       "input": {"name": "DuraGraph User"}
     }'
   ```

## Expected Output

Worker output:
```
Connecting to DuraGraph at http://localhost:8081
Worker registered. Waiting for runs...
[greet] Generated: Hello, DuraGraph User!
[farewell] Generated: Goodbye! Thanks for using DuraGraph.
```

## Code Walkthrough

### Graph Definition

```python
@Graph
class HelloWorld:
    @node
    async def greet(self, state: dict) -> dict:
        name = state.get("name", "World")
        state["greeting"] = f"Hello, {name}!"
        return state
```

- `@Graph` marks a class as a workflow graph
- `@node` marks methods as executable nodes
- Each node receives state, modifies it, and returns the updated state

### Worker Setup

```python
worker = Worker(
    control_plane_url="http://localhost:8081",
    name="hello-world-worker",
)
worker.register_graph(HelloWorld)
worker.run()
```

- Worker connects to the DuraGraph control plane
- Registers its available graphs
- Polls for work and executes runs

## Configuration

| Environment Variable | Default | Description |
|---------------------|---------|-------------|
| `DURAGRAPH_URL` | `http://localhost:8081` | Control plane URL |

## Next Steps

- [02-chatbot](../02-chatbot) - Add conversation memory
- [06-tool-use](../06-tool-use) - Call external tools
