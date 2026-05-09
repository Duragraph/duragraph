# REST API Client Reference

The `client` package provides a Go client for the DuraGraph control plane REST API. It covers all endpoints for managing assistants, threads, runs, the key-value store, and cron jobs.

## Installation

```bash
go get github.com/duragraph/duragraph-go
```

## Quick Start

```go
package main

import (
    "context"
    "fmt"
    "log"

    "github.com/duragraph/duragraph-go/client"
)

func main() {
    c := client.New("http://localhost:8081")

    // Create an assistant
    assistant, err := c.CreateAssistant(context.Background(), client.CreateAssistantRequest{
        GraphID: "chatbot",
        Name:    "My Agent",
    })
    if err != nil {
        log.Fatal(err)
    }

    // Create a thread
    thread, err := c.CreateThread(context.Background())
    if err != nil {
        log.Fatal(err)
    }

    // Start a run
    run, err := c.CreateRun(context.Background(), thread.ID, client.CreateRunRequest{
        AssistantID: assistant.ID,
        Input:       map[string]any{"message": "Hello!"},
    })
    if err != nil {
        log.Fatal(err)
    }
    fmt.Printf("Run %s status: %s\n", run.ID, run.Status)
}
```

## Authentication

```go
c := client.New("http://localhost:8081", client.WithAPIKey("sk-your-key"))
```

The key is sent via the `Authorization: Bearer <key>` header.

## Custom HTTP Client

```go
c := client.New("http://localhost:8081", client.WithHTTPClient(&http.Client{
    Timeout: 60 * time.Second,
}))
```

---

## Assistants

Assistants represent configured AI agents backed by a graph definition.

### CreateAssistant

```go
assistant, err := c.CreateAssistant(ctx, client.CreateAssistantRequest{
    GraphID:     "my_graph",
    Name:        "My Agent",
    Description: "A helpful chatbot",
    Config:      map[string]any{"temperature": 0.7},
    Metadata:    map[string]any{"team": "support"},
})
```

### GetAssistant

```go
assistant, err := c.GetAssistant(ctx, "assistant-id")
```

### ListAssistants

```go
assistants, err := c.ListAssistants(ctx)
```

### SearchAssistants

```go
results, err := c.SearchAssistants(ctx, client.SearchAssistantsRequest{
    GraphID: "chatbot",
    Limit:   10,
})
```

### UpdateAssistant

```go
updated, err := c.UpdateAssistant(ctx, "assistant-id", client.UpdateAssistantRequest{
    Name: "Updated Name",
})
```

### DeleteAssistant

```go
err := c.DeleteAssistant(ctx, "assistant-id")
```

---

## Threads

Threads represent conversation sessions that maintain state across runs.

### CreateThread

```go
// Without metadata
thread, err := c.CreateThread(ctx)

// With metadata
thread, err := c.CreateThread(ctx, client.CreateThreadRequest{
    Metadata: map[string]any{"user_id": "u-123"},
})
```

### GetThread

```go
thread, err := c.GetThread(ctx, "thread-id")
```

### ListThreads

```go
threads, err := c.ListThreads(ctx)
```

### SearchThreads

```go
results, err := c.SearchThreads(ctx, client.SearchThreadsRequest{
    Status: "idle",
    Limit:  20,
})
```

### DeleteThread

```go
err := c.DeleteThread(ctx, "thread-id")
```

---

## Thread State

Access and modify thread state directly for human-in-the-loop or debugging.

### GetThreadState

```go
state, err := c.GetThreadState(ctx, "thread-id")
fmt.Println(state.Values) // current state values
fmt.Println(state.Next)   // next nodes to execute
```

**ThreadState fields:**

| Field      | Type                | Description                       |
|------------|---------------------|-----------------------------------|
| `Values`   | `map[string]any`    | Current state values              |
| `Next`     | `[]string`          | Next nodes to execute             |
| `Metadata` | `map[string]any`    | State metadata                    |
| `Config`   | `map[string]any`    | Configuration                     |
| `Tasks`    | `[]map[string]any`  | Pending tasks                     |
| `CreatedAt`| `string`            | ISO 8601 timestamp                |
| `ParentID` | `string`            | Parent checkpoint ID              |

### UpdateThreadState

```go
state, err := c.UpdateThreadState(ctx, "thread-id", client.UpdateThreadStateRequest{
    Values: map[string]any{"approved": true},
    AsNode: "human_review",  // inject as if from this node
})
```

### GetThreadHistory

```go
history, err := c.GetThreadHistory(ctx, "thread-id")
for _, checkpoint := range history {
    fmt.Printf("%s: %v\n", checkpoint.CreatedAt, checkpoint.Values)
}
```

---

## Runs

Runs represent a single execution of a graph within a thread.

### CreateRun

```go
run, err := c.CreateRun(ctx, "thread-id", client.CreateRunRequest{
    AssistantID: "assistant-id",
    Input:       map[string]any{"message": "Hello"},
    Config:      map[string]any{"temperature": 0.5},
    Metadata:    map[string]any{"source": "api"},
})
```

### GetRun

```go
run, err := c.GetRun(ctx, "thread-id", "run-id")
```

### ListRuns

```go
runs, err := c.ListRuns(ctx, "thread-id")
```

### WaitForRun

Poll until a run reaches a terminal state:

```go
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
defer cancel()

run, err := c.WaitForRun(ctx, "thread-id", "run-id", 500*time.Millisecond)
if err != nil {
    log.Fatal(err)
}
fmt.Println(run.Status) // "completed", "failed", or "canceled"
```

### CancelRun

```go
err := c.CancelRun(ctx, "thread-id", "run-id")
```

---

## Store

The key-value store provides persistent storage organized by hierarchical namespaces.

### PutStoreItem

```go
err := c.PutStoreItem(ctx, client.PutStoreItemRequest{
    Namespace: []string{"users", "preferences"},
    Key:       "user-123",
    Value:     map[string]any{"theme": "dark", "language": "en"},
})
```

### GetStoreItem

```go
item, err := c.GetStoreItem(ctx, []string{"users", "preferences"}, "user-123")
fmt.Println(item.Value) // {"theme": "dark", "language": "en"}
```

### DeleteStoreItem

```go
err := c.DeleteStoreItem(ctx, []string{"users", "preferences"}, "user-123")
```

### SearchStore

```go
items, err := c.SearchStore(ctx, client.SearchStoreRequest{
    Namespace: []string{"users"},
    Query:     "dark",
    Limit:     50,
})
```

### ListNamespaces

```go
namespaces, err := c.ListNamespaces(ctx, client.ListNamespacesRequest{
    Prefix: []string{"users"},
    Limit:  100,
})
// Returns: [["users", "preferences"], ["users", "settings"]]
```

---

## Crons

Schedule recurring runs using cron expressions.

### CreateCron

```go
cron, err := c.CreateCron(ctx, client.CreateCronRequest{
    AssistantID: "assistant-id",
    Schedule:    "0 */6 * * *", // every 6 hours
    ThreadID:    "thread-id",   // optional
    Input:       map[string]any{"task": "summarize"},
    Metadata:    map[string]any{"owner": "ops"},
})
```

### DeleteCron

```go
err := c.DeleteCron(ctx, "cron-id")
```

### SearchCrons

```go
crons, err := c.SearchCrons(ctx, client.SearchCronsRequest{
    AssistantID: "assistant-id",
    Limit:       20,
})
```

---

## Error Handling

All methods return `*APIError` on non-2xx responses:

```go
assistant, err := c.GetAssistant(ctx, "nonexistent")
if err != nil {
    var apiErr *client.APIError
    if errors.As(err, &apiErr) {
        fmt.Printf("Status %d: %s\n", apiErr.StatusCode, apiErr.Body)
    }
}
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

```go
package main

import (
    "context"
    "fmt"
    "log"
    "time"

    "github.com/duragraph/duragraph-go/client"
)

func main() {
    ctx := context.Background()
    c := client.New("http://localhost:8081", client.WithAPIKey("sk-prod-key"))

    // Create assistant
    assistant, err := c.CreateAssistant(ctx, client.CreateAssistantRequest{
        GraphID: "support_v2",
        Name:    "Support Bot",
    })
    if err != nil {
        log.Fatal(err)
    }

    // Create thread
    thread, err := c.CreateThread(ctx, client.CreateThreadRequest{
        Metadata: map[string]any{"channel": "web"},
    })
    if err != nil {
        log.Fatal(err)
    }

    // Store user preferences
    _ = c.PutStoreItem(ctx, client.PutStoreItemRequest{
        Namespace: []string{"users", "prefs"},
        Key:       "u-42",
        Value:     map[string]any{"language": "en", "tier": "premium"},
    })

    // Start a run
    run, err := c.CreateRun(ctx, thread.ID, client.CreateRunRequest{
        AssistantID: assistant.ID,
        Input:       map[string]any{"message": "I need help with billing"},
    })
    if err != nil {
        log.Fatal(err)
    }

    // Wait for completion
    result, err := c.WaitForRun(ctx, thread.ID, run.ID, time.Second)
    if err != nil {
        log.Fatal(err)
    }
    fmt.Printf("Run completed with status: %s\n", result.Status)

    // Check thread state
    state, err := c.GetThreadState(ctx, thread.ID)
    if err != nil {
        log.Fatal(err)
    }
    fmt.Printf("Current state: %v\n", state.Values)

    // Schedule daily summary
    cron, _ := c.CreateCron(ctx, client.CreateCronRequest{
        AssistantID: assistant.ID,
        Schedule:    "0 9 * * *",
        Input:       map[string]any{"task": "daily_summary"},
    })

    // Clean up
    _ = c.DeleteCron(ctx, cron.CronID)
    _ = c.DeleteThread(ctx, thread.ID)
    _ = c.DeleteAssistant(ctx, assistant.ID)
}
```

## Type Reference

### Resource Types

| Type           | Description                              | Key Fields                                    |
|----------------|------------------------------------------|-----------------------------------------------|
| `Assistant`    | AI agent backed by a graph               | `ID`, `GraphID`, `Name`, `Config`, `Metadata` |
| `Thread`       | Conversation session                     | `ID`, `Metadata`, `CreatedAt`                 |
| `Run`          | Single graph execution                   | `ID`, `ThreadID`, `AssistantID`, `Status`     |
| `StoreItem`    | Key-value store entry                    | `Namespace`, `Key`, `Value`                   |
| `Cron`         | Scheduled recurring run                  | `CronID`, `AssistantID`, `Schedule`           |
| `ThreadState`  | Current thread state                     | `Values`, `Next`, `Metadata`, `Config`        |

### Request Types

| Type                       | Used By              |
|----------------------------|----------------------|
| `CreateAssistantRequest`   | `CreateAssistant`    |
| `UpdateAssistantRequest`   | `UpdateAssistant`    |
| `SearchAssistantsRequest`  | `SearchAssistants`   |
| `CreateThreadRequest`      | `CreateThread`       |
| `SearchThreadsRequest`     | `SearchThreads`      |
| `UpdateThreadStateRequest` | `UpdateThreadState`  |
| `CreateRunRequest`         | `CreateRun`          |
| `PutStoreItemRequest`      | `PutStoreItem`       |
| `SearchStoreRequest`       | `SearchStore`        |
| `ListNamespacesRequest`    | `ListNamespaces`     |
| `CreateCronRequest`        | `CreateCron`         |
| `SearchCronsRequest`       | `SearchCrons`        |
