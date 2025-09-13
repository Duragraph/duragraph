# Python SDK

The Duragraph Python SDK provides a clean, Pythonic interface for building and managing AI workflows.

!!! info "Development Status"
    **Status**: In Development (Q1 2024)  
    **Implementation**: Python Eino adapter worker  
    **Tracking**: [GitHub Issue #4](https://github.com/adwiteeymauriya/duragraph/issues/4)

## Installation

```bash
# Available in Q1 2024
pip install duragraph-sdk
```

For now, use the REST API directly or the LangGraph SDK with Duragraph's compatible endpoints.

## Quick Start

```python
from duragraph import DuragraphClient, Workflow

# Initialize client
client = DuragraphClient(
    base_url="http://localhost:8080",
    api_key="your-api-key"  # Optional for local development
)

# Create a simple workflow
workflow = Workflow()
workflow.add_step("greeting", {
    "type": "llm_call",
    "model": "gpt-3.5-turbo",
    "messages": [
        {"role": "user", "content": "Generate a creative greeting"}
    ]
})

# Execute workflow
run = client.create_run(
    assistant_id="my-assistant",
    thread_id="conversation-1",
    workflow=workflow
)

# Stream results
for event in client.stream_events(run.id):
    print(f"📦 {event.type}: {event.data}")
```

## Core Classes

### DuragraphClient

Main entry point for interacting with Duragraph.

```python
from duragraph import DuragraphClient

client = DuragraphClient(
    base_url="https://your-duragraph-instance.com",
    api_key="your-api-key",
    timeout=30.0,
    max_retries=3
)
```

**Methods:**

- `create_assistant(name, config)` - Create a new assistant
- `create_thread(metadata=None)` - Start a new conversation thread  
- `create_run(assistant_id, thread_id, **kwargs)` - Execute a workflow
- `stream_events(run_id)` - Stream real-time events
- `get_run(run_id)` - Get run status and results

### Workflow

Define complex multi-step workflows with branching logic.

```python
from duragraph import Workflow, Step

workflow = Workflow()

# Add sequential steps
workflow.add_step("research", Step.llm_call(
    model="gpt-4",
    messages=[{"role": "user", "content": "Research {topic}"}],
    output_key="research_results"
))

workflow.add_step("summarize", Step.llm_call(
    model="gpt-3.5-turbo", 
    messages=[
        {"role": "user", "content": "Summarize: {research_results}"}
    ],
    depends_on=["research"]
))

# Add conditional branching
workflow.add_conditional(
    "check_quality",
    condition=lambda state: len(state["research_results"]) > 100,
    if_true="detailed_analysis",
    if_false="simple_summary"
)
```

### Step Types

Built-in step types for common operations:

```python
from duragraph import Step

# LLM calls
step = Step.llm_call(
    model="gpt-4",
    messages=[...],
    temperature=0.7,
    max_tokens=1000
)

# Tool calls  
step = Step.tool_call(
    tool_name="web_search",
    arguments={"query": "{search_term}"}
)

# Human-in-the-loop
step = Step.human_input(
    prompt="Please review and approve:",
    input_schema={"type": "string"}
)

# Custom functions
step = Step.function_call(
    function=my_custom_function,
    arguments={"param1": "{dynamic_value}"}
)
```

## Advanced Usage

### Error Handling

```python
from duragraph import DuragraphError, WorkflowError

try:
    run = client.create_run(
        assistant_id="my-assistant",
        thread_id="thread-1",
        workflow=workflow
    )
    
    result = run.wait_for_completion(timeout=300)
    
except WorkflowError as e:
    print(f"Workflow failed: {e.message}")
    print(f"Failed step: {e.step_name}")
    print(f"Error details: {e.details}")
    
except DuragraphError as e:
    print(f"API error: {e}")
```

### Async Support

```python
import asyncio
from duragraph import AsyncDuragraphClient

async def main():
    client = AsyncDuragraphClient(base_url="http://localhost:8080")
    
    # Create assistant and thread
    assistant = await client.create_assistant("My Assistant")
    thread = await client.create_thread()
    
    # Run workflow asynchronously
    run = await client.create_run(
        assistant_id=assistant.id,
        thread_id=thread.id,
        workflow=workflow
    )
    
    # Stream events
    async for event in client.stream_events(run.id):
        print(f"Event: {event}")

asyncio.run(main())
```

### State Management

```python
from duragraph import Workflow, StateSchema

# Define state schema
schema = StateSchema({
    "user_input": {"type": "string", "required": True},
    "analysis": {"type": "object"},
    "final_result": {"type": "string"}
})

workflow = Workflow(state_schema=schema)

# Access state in steps
workflow.add_step("analyze", Step.llm_call(
    model="gpt-4",
    messages=[
        {"role": "user", "content": "Analyze: {user_input}"}
    ],
    output_key="analysis"
))

# Transform state
workflow.add_step("transform", Step.function_call(
    function=lambda state: {
        "final_result": f"Analysis: {state['analysis']['summary']}"
    }
))
```

### Custom Tools

```python
from duragraph import Tool

# Define custom tool
@Tool.register("web_search")
def web_search(query: str, max_results: int = 5) -> dict:
    """Search the web for information."""
    # Your web search implementation
    results = search_web(query, limit=max_results)
    return {"results": results, "query": query}

# Use in workflow
workflow.add_step("search", Step.tool_call(
    tool_name="web_search", 
    arguments={
        "query": "{search_query}",
        "max_results": 10
    }
))
```

### Streaming and Real-time

```python
# Stream events with filtering
for event in client.stream_events(run.id):
    if event.type == "step_start":
        print(f"🚀 Starting: {event.step_name}")
    elif event.type == "llm_token":
        print(event.token, end="", flush=True)
    elif event.type == "step_complete":
        print(f"✅ Completed: {event.step_name}")
    elif event.type == "error":
        print(f"❌ Error: {event.message}")

# Server-sent events with custom handlers
client.on("llm_token", lambda token: print(token, end=""))
client.on("step_complete", lambda step: log.info(f"Step done: {step}"))

run = client.create_run(assistant_id="...", thread_id="...")
client.start_streaming(run.id)
```

## Configuration

### Environment Variables

```bash
# Set default configuration
export DURAGRAPH_BASE_URL="https://your-instance.com"
export DURAGRAPH_API_KEY="your-api-key"
export DURAGRAPH_TIMEOUT="30"
export DURAGRAPH_MAX_RETRIES="3"
```

```python
# Client will automatically use environment variables
client = DuragraphClient()  # Uses env vars
```

### Logging

```python
import logging
from duragraph import configure_logging

# Enable debug logging
configure_logging(level=logging.DEBUG)

# Custom logger
logger = logging.getLogger("duragraph")
logger.setLevel(logging.INFO)
```

## Examples

### Multi-Agent Research Pipeline

```python
from duragraph import Workflow, Step

# Define research workflow
research_workflow = Workflow()

# Step 1: Initial research
research_workflow.add_step("research", Step.llm_call(
    model="gpt-4",
    messages=[{"role": "user", "content": "Research topic: {topic}"}],
    output_key="raw_research"
))

# Step 2: Fact-checking
research_workflow.add_step("fact_check", Step.tool_call(
    tool_name="web_search",
    arguments={"query": "fact check {raw_research}"},
    depends_on=["research"]
))

# Step 3: Writing
research_workflow.add_step("write", Step.llm_call(
    model="gpt-4",
    messages=[
        {"role": "system", "content": "You are a technical writer."},
        {"role": "user", "content": "Write an article based on: {raw_research}\nFact-check results: {fact_check}"}
    ],
    depends_on=["fact_check"]
))

# Execute
run = client.create_run(
    assistant_id="research-assistant",
    thread_id="research-session",
    workflow=research_workflow,
    inputs={"topic": "Latest developments in AI safety"}
)
```

### Human-in-the-Loop Approval

```python
# Workflow with human approval
approval_workflow = Workflow()

approval_workflow.add_step("draft", Step.llm_call(
    model="gpt-4",
    messages=[{"role": "user", "content": "Draft email: {request}"}]
))

approval_workflow.add_step("review", Step.human_input(
    prompt="Please review this email draft:",
    input_schema={
        "type": "object",
        "properties": {
            "approved": {"type": "boolean"},
            "feedback": {"type": "string"}
        }
    },
    depends_on=["draft"]
))

# Conditional next step
approval_workflow.add_conditional(
    "check_approval",
    condition=lambda state: state["review"]["approved"],
    if_true="send_email",
    if_false="revise_draft"
)
```

## Migration from LangGraph

The Duragraph Python SDK is designed to be familiar to LangGraph users:

```python
# LangGraph style (also works with Duragraph)
from langgraph_sdk import get_client
client = get_client(url="http://localhost:8080")

# Native Duragraph style
from duragraph import DuragraphClient
client = DuragraphClient(base_url="http://localhost:8080")
```

Both approaches work! The native SDK provides additional Duragraph-specific features and better type safety.

## API Reference

Full API documentation: [https://duragraph.ai/sdk/python](https://duragraph.ai/sdk/python)

## Contributing

The Python SDK is open source! Contribute at [GitHub](https://github.com/adwiteeymauriya/duragraph/tree/main/sdk/python).

```bash
git clone https://github.com/adwiteeymauriya/duragraph
cd duragraph/sdk/python
pip install -e ".[dev]"
pytest tests/
```
