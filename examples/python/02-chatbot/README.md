# Chatbot with Memory

A conversational chatbot that maintains conversation history across multiple runs using thread_id.

## What This Example Demonstrates

- **Conversation memory** - Maintains message history per thread
- **Thread-based context** - Each thread_id has independent conversation state
- **Multi-node workflow** - Load history → Add message → Generate response → Save
- **Stateful interactions** - Build context from previous messages in the conversation

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

3. **Start a conversation** (in another terminal):
   ```bash
   # First message in thread "alice"
   curl -X POST http://localhost:8081/api/v1/runs \
     -H "Content-Type: application/json" \
     -d '{
       "assistant_id": "chatbot",
       "thread_id": "alice",
       "input": {"input": "Hello!", "thread_id": "alice"}
     }'
   
   # Continue the conversation
   curl -X POST http://localhost:8081/api/v1/runs \
     -H "Content-Type: application/json" \
     -d '{
       "assistant_id": "chatbot",
       "thread_id": "alice",
       "input": {"input": "What is your name?", "thread_id": "alice"}
     }'
   
   # Third message - shows conversation context
   curl -X POST http://localhost:8081/api/v1/runs \
     -H "Content-Type: application/json" \
     -d '{
       "assistant_id": "chatbot",
       "thread_id": "alice",
       "input": {"input": "How many messages have we exchanged?", "thread_id": "alice"}
     }'
   ```

4. **Start a different conversation** (separate thread):
   ```bash
   curl -X POST http://localhost:8081/api/v1/runs \
     -H "Content-Type: application/json" \
     -d '{
       "assistant_id": "chatbot",
       "thread_id": "bob",
       "input": {"input": "Hello!", "thread_id": "bob"}
     }'
   ```

## Expected Output

Worker output:
```
Connecting to DuraGraph at http://localhost:8081
Starting Chatbot Worker with Conversation Memory
Worker registered. Waiting for runs...

[load_history] Thread: alice, Messages: 0
[add_user_message] User: Hello!
[generate_response] Assistant: Hello! How can I help you today?
[save_response] Saved to thread: alice

[load_history] Thread: alice, Messages: 2
[add_user_message] User: What is your name?
[generate_response] Assistant: I'm DuraGraph Bot, your helpful assistant!
[save_response] Saved to thread: alice

[load_history] Thread: alice, Messages: 4
[add_user_message] User: How many messages have we exchanged?
[generate_response] Assistant: I understand you said: 'How many messages have we exchanged?'. This is message #5 in our conversation. How can I help further?
[save_response] Saved to thread: alice

[load_history] Thread: bob, Messages: 0
[add_user_message] User: Hello!
[generate_response] Assistant: Hello! How can I help you today?
[save_response] Saved to thread: bob
```

## Code Walkthrough

### Conversation Store

```python
class ConversationStore:
    """Stores conversation history by thread_id."""
    
    def __init__(self):
        self._store: dict[str, list[dict[str, str]]] = defaultdict(list)
    
    def get_messages(self, thread_id: str) -> list[dict[str, str]]:
        """Retrieve conversation history for a thread."""
        return self._store[thread_id].copy()
```

- Simple in-memory store using `defaultdict`
- Each thread_id has its own message list
- **Production note:** Replace with Redis, PostgreSQL, or persistent storage

### Graph Workflow

```python
@Graph
class ChatbotWithMemory:
    @node
    async def load_history(self, state: dict) -> dict:
        """Load conversation history from store."""
        thread_id = state.get("thread_id", "default")
        state["messages"] = conversation_store.get_messages(thread_id)
        return state
    
    @node
    async def add_user_message(self, state: dict) -> dict:
        """Add user's new message to conversation."""
        state["messages"].append({
            "role": "user",
            "content": state.get("input", "")
        })
        return state
    
    @node
    async def generate_response(self, state: dict) -> dict:
        """Generate response using conversation context."""
        # Access conversation history
        messages = state.get("messages", [])
        # Generate response based on history
        state["response"] = generate_ai_response(messages)
        return state
    
    @node
    async def save_response(self, state: dict) -> dict:
        """Persist response to conversation store."""
        conversation_store.add_message(
            state["thread_id"], 
            "assistant", 
            state["response"]
        )
        return state
```

**Node Flow:**
1. `load_history` - Retrieve existing messages for this thread
2. `add_user_message` - Append new user message to history
3. `generate_response` - Create AI response using full conversation context
4. `save_response` - Persist the conversation update

### Key Concepts

**Thread Isolation:**
- Each `thread_id` maintains separate conversation state
- Threads don't interfere with each other
- Perfect for multi-user scenarios

**Conversation Context:**
- Full message history available to response generation
- Can reference previous messages
- Build context-aware responses

**State Flow:**
```
input: {"input": "Hello", "thread_id": "alice"}
  ↓
state: {"thread_id": "alice", "messages": []}
  ↓
state: {"thread_id": "alice", "messages": [{"role": "user", "content": "Hello"}]}
  ↓
state: {"thread_id": "alice", "messages": [...], "response": "Hello! How can I help?"}
  ↓
output: {"messages": [...], "response": "Hello! How can I help?"}
```

## Production Considerations

### Persistent Storage

Replace in-memory store with real persistence:

```python
# Redis example
import redis
import json

class RedisConversationStore:
    def __init__(self):
        self.redis = redis.Redis(host='localhost', port=6379, db=0)
    
    def get_messages(self, thread_id: str) -> list[dict[str, str]]:
        data = self.redis.get(f"thread:{thread_id}")
        return json.loads(data) if data else []
    
    def add_message(self, thread_id: str, role: str, content: str):
        messages = self.get_messages(thread_id)
        messages.append({"role": role, "content": content})
        self.redis.set(f"thread:{thread_id}", json.dumps(messages))
```

### LLM Integration

Replace rule-based responses with real LLM:

```python
from duragraph import llm_node

@Graph
class ChatbotWithMemory:
    @node
    async def load_history(self, state: dict) -> dict:
        # Same as before
        pass
    
    @node
    async def add_user_message(self, state: dict) -> dict:
        # Same as before
        pass
    
    @llm_node(
        model="gpt-4o-mini",
        temperature=0.7,
        system_prompt="You are a helpful assistant.",
    )
    def generate_response(self, state: dict) -> dict:
        # llm_node automatically uses state["messages"]
        return state
    
    @node
    async def save_response(self, state: dict) -> dict:
        # Same as before
        pass
```

### Message Limit

Prevent unbounded memory growth:

```python
def get_messages(self, thread_id: str) -> list[dict[str, str]]:
    """Get last 50 messages to limit context size."""
    messages = self._store[thread_id]
    return messages[-50:]  # Keep most recent 50 messages
```

## Configuration

| Environment Variable | Default | Description |
|---------------------|---------|-------------|
| `DURAGRAPH_URL` | `http://localhost:8081` | Control plane URL |

## Testing Script

Create `test_chatbot.sh`:

```bash
#!/bin/bash
THREAD_ID="test-$(date +%s)"

echo "Testing chatbot with thread: $THREAD_ID"

# Message 1
echo "Message 1: Hello"
curl -X POST http://localhost:8081/api/v1/runs \
  -H "Content-Type: application/json" \
  -d "{\"assistant_id\": \"chatbot\", \"thread_id\": \"$THREAD_ID\", \"input\": {\"input\": \"Hello\", \"thread_id\": \"$THREAD_ID\"}}"

sleep 2

# Message 2
echo "Message 2: What's your name?"
curl -X POST http://localhost:8081/api/v1/runs \
  -H "Content-Type: application/json" \
  -d "{\"assistant_id\": \"chatbot\", \"thread_id\": \"$THREAD_ID\", \"input\": {\"input\": \"What is your name?\", \"thread_id\": \"$THREAD_ID\"}}"

sleep 2

# Message 3
echo "Message 3: Goodbye"
curl -X POST http://localhost:8081/api/v1/runs \
  -H "Content-Type: application/json" \
  -d "{\"assistant_id\": \"chatbot\", \"thread_id\": \"$THREAD_ID\", \"input\": {\"input\": \"Goodbye\", \"thread_id\": \"$THREAD_ID\"}}"
```

## Next Steps

- [03-rag-agent](../03-rag-agent) - Add retrieval-augmented generation
- [05-human-in-loop](../05-human-in-loop) - Add approval workflows
- [06-tool-use](../06-tool-use) - Call external APIs and tools

## Troubleshooting

**Worker not receiving runs:**
- Ensure DuraGraph control plane is running
- Check `DURAGRAPH_URL` environment variable
- Verify `assistant_id` matches in curl commands

**Memory not persisting:**
- Ensure you're using the same `thread_id` in consecutive runs
- Worker restart will clear in-memory store (use persistent storage in production)

**State not flowing correctly:**
- Ensure `thread_id` is included in the input payload
- Check worker logs for state transitions between nodes
