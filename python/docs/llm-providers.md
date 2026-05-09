# LLM Provider Integrations

## Overview

DuraGraph Python SDK now includes production-ready LLM provider integrations for OpenAI and Anthropic. The `@llm_node` decorator automatically calls the configured LLM provider during graph execution.

## Installation

```bash
# OpenAI support
uv add duragraph-python[openai]

# Anthropic support
uv add duragraph-python[anthropic]

# Both providers
uv add duragraph-python[openai,anthropic]
```

## Quick Start

### Using OpenAI

```python
import asyncio
from duragraph import Graph, llm_node, entrypoint

@Graph(id="my_agent")
class MyAgent:
    @entrypoint
    @llm_node(model="gpt-4o-mini", temperature=0.7)
    def process(self, state):
        return state

async def main():
    agent = MyAgent()
    result = await agent.arun({
        "messages": [{"role": "user", "content": "Hello!"}]
    })
    print(result.output["messages"][-1]["content"])

asyncio.run(main())
```

### Using Anthropic

```python
@Graph(id="claude_agent")
class ClaudeAgent:
    @entrypoint
    @llm_node(model="claude-3-5-sonnet-20241022", temperature=0.7)
    def process(self, state):
        return state
```

## Supported Models

### OpenAI
- **GPT-4o**: `gpt-4o`, `gpt-4o-mini`
- **GPT-4 Turbo**: `gpt-4-turbo`
- **GPT-4**: `gpt-4`
- **GPT-3.5**: `gpt-3.5-turbo`
- **o1 Reasoning**: `o1-preview`, `o1-mini`

### Anthropic
- **Claude 3.5**: `claude-3-5-sonnet-20241022`, `claude-3-5-haiku-20241022`
- **Claude 3**: `claude-3-opus`, `claude-3-sonnet`, `claude-3-haiku`

## Configuration

### Environment Variables

```bash
# OpenAI
export OPENAI_API_KEY=sk-...
export OPENAI_BASE_URL=https://api.openai.com/v1  # optional

# Anthropic  
export ANTHROPIC_API_KEY=sk-ant-...
export ANTHROPIC_BASE_URL=https://api.anthropic.com  # optional
```

### Programmatic Configuration

```python
from duragraph.llm import get_provider

# Explicit provider
provider = get_provider("gpt-4o-mini", api_key="sk-...", provider="openai")

# Auto-detected from model name
provider = get_provider("claude-3-sonnet", api_key="sk-ant-...")
```

## Advanced Features

### Tool Calling

```python
@llm_node(
    model="gpt-4o",
    tools=[
        {
            "type": "function",
            "function": {
                "name": "search_database",
                "description": "Search the customer database",
                "parameters": {
                    "type": "object",
                    "properties": {
                        "query": {"type": "string"}
                    }
                }
            }
        }
    ]
)
def agent_with_tools(self, state):
    return state
```

### System Prompts

```python
@llm_node(
    model="gpt-4o-mini",
    system_prompt="You are a helpful customer support agent. Be concise and friendly."
)
def support_agent(self, state):
    return state
```

### Streaming

Streaming is automatic when using `agent.stream()`:

```python
async for event in agent.stream({"messages": [...]}):
    if event.type == "node_completed":
        print(event.data["output"])
```

## State Management

The LLM providers integrate with DuraGraph state:

```python
# Messages in state
state = {
    "messages": [
        {"role": "user", "content": "Hello"},
        {"role": "assistant", "content": "Hi! How can I help?"},
        {"role": "user", "content": "What's the weather?"}
    ]
}

# LLM response is automatically appended
result = await agent.arun(state)
# result.output["messages"] now includes the assistant's response
```

## Examples

See the `examples/` directory:
- `chatbot_simple.py` - Basic OpenAI chatbot
- `chatbot_anthropic.py` - Claude-powered chatbot

## Testing

Run the test suite:

```bash
uv sync --extra openai --extra anthropic
uv run python -m pytest tests/test_llm.py -v
```

## Architecture

### Provider Registry

Providers are auto-registered on import. The registry automatically selects the correct provider based on model name:

```python
from duragraph.llm import get_provider

# Auto-detected: gpt-* → openai
provider = get_provider("gpt-4o-mini")

# Auto-detected: claude-* → anthropic  
provider = get_provider("claude-3-sonnet")
```

### Executor Integration

The `@llm_node` decorator is handled by `duragraph.executor.execute_llm_node()` during graph execution. This separates the LLM logic from the graph traversal logic.

## Roadmap

- **v0.3.0**: Streaming support for token-level responses
- **v0.3.0**: Tool execution integration
- **v0.4.0**: Gemini provider
- **v0.4.0**: Local model support (Ollama)

## Contributing

To add a new LLM provider:

1. Create `src/duragraph/llm/{provider}.py` implementing `LLMProvider`
2. Add to auto-registration in `registry.py`
3. Add tests in `tests/test_llm.py`
4. Update pyproject.toml with optional dependency
5. Add example in `examples/`
