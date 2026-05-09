# REST API Client Reference

The DuraGraph Python SDK includes synchronous and asynchronous REST API clients for interacting with a DuraGraph control plane server. These clients are compatible with the LangGraph Cloud API.

## Installation

```bash
uv add duragraph
# or
pip install duragraph
```

The client requires `httpx` which is included as a dependency.

## Quick Start

```python
from duragraph import DuraGraphClient

with DuraGraphClient("http://localhost:8081") as client:
    # Create an assistant
    assistant = client.create_assistant("My Agent", graph_id="chatbot")

    # Create a thread
    thread = client.create_thread()

    # Start a run
    run = client.create_run(
        thread["thread_id"],
        assistant["assistant_id"],
        input={"messages": [{"role": "user", "content": "Hello!"}]},
    )
    print(run)
```

### Async Usage

```python
import asyncio
from duragraph import AsyncDuraGraphClient

async def main():
    async with AsyncDuraGraphClient("http://localhost:8081") as client:
        assistant = await client.create_assistant("My Agent", graph_id="chatbot")
        thread = await client.create_thread()
        run = await client.create_run(
            thread["thread_id"],
            assistant["assistant_id"],
            input={"messages": [{"role": "user", "content": "Hello!"}]},
        )
        print(run)

asyncio.run(main())
```

## Authentication

Pass an API key to authenticate requests:

```python
client = DuraGraphClient(
    "http://localhost:8081",
    api_key="your-api-key",
)
```

The key is sent via the `X-Api-Key` header on every request.

## Configuration

| Parameter  | Type           | Default                  | Description                          |
|------------|----------------|--------------------------|--------------------------------------|
| `base_url` | `str`          | `"http://localhost:8081"` | DuraGraph server URL                |
| `api_key`  | `str \| None`  | `None`                   | API key for authentication           |
| `timeout`  | `float`        | `30.0`                   | Request timeout in seconds           |

---

## Assistants

Assistants represent configured AI agents backed by a graph definition.

### create_assistant

Create a new assistant.

```python
assistant = client.create_assistant(
    "My Agent",
    graph_id="chatbot",
    description="A helpful chatbot",
    model="gpt-4o-mini",
    instructions="You are a helpful assistant.",
    metadata={"team": "support"},
    config={"temperature": 0.7},
)
```

**Parameters:**

| Name           | Type                    | Required | Description                     |
|----------------|-------------------------|----------|---------------------------------|
| `name`         | `str`                   | Yes      | Display name                    |
| `graph_id`     | `str \| None`           | No       | Graph definition ID             |
| `description`  | `str \| None`           | No       | Human-readable description      |
| `model`        | `str \| None`           | No       | Default LLM model               |
| `instructions` | `str \| None`           | No       | System instructions             |
| `metadata`     | `dict[str, Any] \| None`| No      | Arbitrary metadata              |
| `config`       | `dict[str, Any] \| None`| No      | Runtime configuration           |

**Returns:** `dict[str, Any]` with `assistant_id`, `name`, `graph_id`, `created_at`, `updated_at`.

### get_assistant

```python
assistant = client.get_assistant("assistant-uuid")
```

### list_assistants

```python
assistants = client.list_assistants(limit=50, offset=0)
```

### search_assistants

```python
results = client.search_assistants(
    graph_id="chatbot",
    metadata={"team": "support"},
    limit=10,
    offset=0,
)
```

### update_assistant

```python
updated = client.update_assistant("assistant-uuid", name="New Name", model="gpt-4o")
```

Accepts any assistant fields as keyword arguments.

### delete_assistant

```python
client.delete_assistant("assistant-uuid")
```

---

## Threads

Threads represent conversation sessions that maintain state across multiple runs.

### create_thread

```python
thread = client.create_thread(metadata={"user_id": "u-123"})
```

### get_thread

```python
thread = client.get_thread("thread-uuid")
```

### list_threads

```python
threads = client.list_threads(limit=20, offset=0)
```

### search_threads

```python
results = client.search_threads(
    status="idle",
    metadata={"user_id": "u-123"},
    limit=10,
)
```

### update_thread

```python
updated = client.update_thread("thread-uuid", metadata={"label": "vip"})
```

### delete_thread

```python
client.delete_thread("thread-uuid")
```

---

## Thread State

Access and modify the state of a thread directly.

### get_thread_state

```python
state = client.get_thread_state("thread-uuid")
print(state["values"])
```

**Returns:** `dict` with `values`, `next`, `metadata`, `config`, `tasks`, `created_at`.

### update_thread_state

Inject state into a thread, optionally as if it came from a specific node:

```python
state = client.update_thread_state(
    "thread-uuid",
    values={"messages": [{"role": "user", "content": "Resume here"}]},
    as_node="human_input",
)
```

### get_thread_history

Retrieve the checkpoint history of a thread:

```python
history = client.get_thread_history("thread-uuid", limit=20)
for checkpoint in history:
    print(checkpoint["created_at"], checkpoint["values"])
```

---

## Runs

Runs represent a single execution of a graph within a thread.

### create_run

```python
run = client.create_run(
    "thread-uuid",
    "assistant-uuid",
    input={"messages": [{"role": "user", "content": "Hello"}]},
    config={"temperature": 0.5},
    metadata={"source": "api"},
    multitask_strategy="enqueue",
    interrupt_before=["human_review"],
    interrupt_after=["tool_call"],
)
```

**Parameters:**

| Name                 | Type                    | Required | Description                              |
|----------------------|-------------------------|----------|------------------------------------------|
| `thread_id`          | `str`                   | Yes      | Thread to run in                         |
| `assistant_id`       | `str`                   | Yes      | Assistant to use                         |
| `input`              | `dict \| None`          | No       | Input state                              |
| `config`             | `dict \| None`          | No       | Runtime configuration                    |
| `metadata`           | `dict \| None`          | No       | Run metadata                             |
| `multitask_strategy` | `str \| None`           | No       | `"reject"`, `"enqueue"`, `"rollback"`    |
| `interrupt_before`   | `list[str] \| None`     | No       | Pause before these nodes                 |
| `interrupt_after`    | `list[str] \| None`     | No       | Pause after these nodes                  |

**Returns:** `dict` with `run_id`, `thread_id`, `assistant_id`, `status`, `created_at`.

### create_stateless_run

Create a run without a thread (one-shot execution):

```python
run = client.create_stateless_run(
    "assistant-uuid",
    input={"query": "What is the weather?"},
)
```

### get_run

```python
run = client.get_run("run-uuid")
# or with thread context
run = client.get_run("run-uuid", thread_id="thread-uuid")
```

### list_runs

```python
runs = client.list_runs("thread-uuid")
```

### cancel_run

```python
client.cancel_run("thread-uuid", "run-uuid")
```

### wait_for_run

Create a run and block until it completes:

```python
result = client.wait_for_run(
    "assistant-uuid",
    input={"messages": [{"role": "user", "content": "Hello"}]},
)
print(result["status"])  # "success"
print(result["output"])
```

### join_run

Block until an existing run completes:

```python
result = client.join_run("thread-uuid", "run-uuid")
```

---

## Store

The key-value store provides persistent storage organized by hierarchical namespaces.

### put_store_item

```python
client.put_store_item(
    namespace=["users", "preferences"],
    key="user-123",
    value={"theme": "dark", "language": "en"},
    ttl_seconds=86400,  # expires in 24 hours
)
```

### get_store_item

```python
item = client.get_store_item(
    namespace=["users", "preferences"],
    key="user-123",
)
print(item["value"])  # {"theme": "dark", "language": "en"}
```

### delete_store_item

```python
client.delete_store_item(
    namespace=["users", "preferences"],
    key="user-123",
)
```

### search_store

Search items within a namespace:

```python
items = client.search_store(
    namespace_prefix=["users"],
    filter={"theme": "dark"},
    limit=50,
    offset=0,
)
```

### list_namespaces

```python
namespaces = client.list_namespaces(
    prefix=["users"],
    suffix=["preferences"],
    max_depth=3,
    limit=100,
)
# Returns: [["users", "preferences"], ["users", "settings"], ...]
```

---

## Crons

Schedule recurring runs using cron expressions.

### create_cron

```python
cron = client.create_cron(
    assistant_id="assistant-uuid",
    schedule="0 */6 * * *",  # every 6 hours
    thread_id="thread-uuid",  # optional: reuse thread
    payload={"task": "summarize_inbox"},
    metadata={"owner": "ops"},
)
print(cron["cron_id"])
```

**Parameters:**

| Name           | Type                    | Required | Description                     |
|----------------|-------------------------|----------|---------------------------------|
| `assistant_id` | `str`                   | Yes      | Assistant to run                |
| `schedule`     | `str`                   | Yes      | Cron expression (5-field)       |
| `thread_id`    | `str \| None`           | No       | Existing thread to run in       |
| `payload`      | `dict \| None`          | No       | Input passed to each run        |
| `metadata`     | `dict \| None`          | No       | Cron metadata                   |

### delete_cron

```python
client.delete_cron("cron-uuid")
```

### search_crons

```python
crons = client.search_crons(
    assistant_id="assistant-uuid",
    limit=20,
    offset=0,
)
```

---

## Error Handling

All client methods raise `httpx.HTTPStatusError` on non-2xx responses:

```python
import httpx

try:
    assistant = client.get_assistant("nonexistent-id")
except httpx.HTTPStatusError as e:
    print(f"Status {e.response.status_code}: {e.response.text}")
```

### Common Status Codes

| Code | Meaning                     |
|------|-----------------------------|
| 400  | Invalid request parameters  |
| 404  | Resource not found          |
| 409  | Conflict (e.g., duplicate)  |
| 422  | Validation error            |
| 500  | Internal server error       |

---

## Complete Example

End-to-end workflow using the sync client:

```python
from duragraph import DuraGraphClient

with DuraGraphClient("http://localhost:8081", api_key="sk-prod-key") as client:
    # Set up
    assistant = client.create_assistant("Support Bot", graph_id="support_v2")
    thread = client.create_thread(metadata={"channel": "web"})

    # Store user preferences
    client.put_store_item(
        namespace=["users", "prefs"],
        key="u-42",
        value={"language": "en", "tier": "premium"},
    )

    # Run conversation
    run = client.create_run(
        thread["thread_id"],
        assistant["assistant_id"],
        input={"messages": [{"role": "user", "content": "I need help with billing"}]},
    )

    # Check state
    state = client.get_thread_state(thread["thread_id"])
    print("Current state:", state["values"])

    # Review history
    history = client.get_thread_history(thread["thread_id"])
    print(f"Thread has {len(history)} checkpoints")

    # Schedule daily summary
    cron = client.create_cron(
        assistant["assistant_id"],
        "0 9 * * *",
        payload={"task": "daily_summary"},
    )

    # Clean up
    client.delete_cron(cron["cron_id"])
    client.delete_thread(thread["thread_id"])
    client.delete_assistant(assistant["assistant_id"])
```
