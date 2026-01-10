"""DuraGraph CLI entry point."""

import argparse
import asyncio
import importlib.util
import inspect
import json
import sys
import time
import traceback
from pathlib import Path
from typing import Any

import httpx


def main() -> int:
    """Main CLI entry point."""
    parser = argparse.ArgumentParser(
        prog="duragraph",
        description="DuraGraph Python SDK CLI",
    )
    subparsers = parser.add_subparsers(dest="command", help="Available commands")

    # init command
    init_parser = subparsers.add_parser("init", help="Initialize a new DuraGraph project")
    init_parser.add_argument("name", help="Project name")
    init_parser.add_argument(
        "--template",
        choices=["minimal", "full", "chatbot", "tools"],
        default="minimal",
        help="Project template",
    )

    # dev command
    dev_parser = subparsers.add_parser("dev", help="Run in development mode with hot reload")
    dev_parser.add_argument(
        "file",
        nargs="?",
        default="src/agent.py",
        help="Python file containing the graph",
    )
    dev_parser.add_argument(
        "--port",
        type=int,
        default=8000,
        help="Port for local server",
    )
    dev_parser.add_argument(
        "--control-plane",
        default="http://localhost:8081",
        help="Control plane URL for worker connection",
    )

    # deploy command
    deploy_parser = subparsers.add_parser("deploy", help="Deploy to control plane")
    deploy_parser.add_argument(
        "file",
        help="Python file containing the graph",
    )
    deploy_parser.add_argument(
        "--control-plane",
        required=True,
        help="Control plane URL",
    )
    deploy_parser.add_argument(
        "--worker-name",
        help="Name for the worker (default: auto-generated)",
    )
    deploy_parser.add_argument(
        "--capabilities",
        nargs="*",
        help="Worker capabilities (e.g., openai anthropic tools)",
    )

    # visualize command
    viz_parser = subparsers.add_parser("visualize", help="Visualize a graph")
    viz_parser.add_argument("file", help="Python file containing the graph")
    viz_parser.add_argument(
        "--output",
        "-o",
        help="Output file (default: stdout)",
    )
    viz_parser.add_argument(
        "--format",
        choices=["mermaid", "dot", "json"],
        default="mermaid",
        help="Output format",
    )
    viz_parser.add_argument(
        "--graph",
        help="Specific graph class to visualize (default: auto-detect)",
    )

    args = parser.parse_args()

    if args.command is None:
        parser.print_help()
        return 0

    try:
        if args.command == "init":
            return cmd_init(args.name, args.template)
        elif args.command == "dev":
            return cmd_dev(args.file, args.port, args.control_plane)
        elif args.command == "deploy":
            return cmd_deploy(args.file, args.control_plane, args.worker_name, args.capabilities)
        elif args.command == "visualize":
            return cmd_visualize(args.file, args.output, args.format, args.graph)
    except KeyboardInterrupt:
        print("\nInterrupted by user")
        return 130
    except Exception as e:
        print(f"Error: {e}")
        if "--debug" in sys.argv:
            traceback.print_exc()
        return 1

    return 0


def cmd_init(name: str, template: str) -> int:
    """Initialize a new project."""
    project_dir = Path(name)
    if project_dir.exists():
        print(f"Error: Directory '{name}' already exists")
        return 1

    project_dir.mkdir(parents=True)

    # Create basic structure
    (project_dir / "src").mkdir()
    (project_dir / "tests").mkdir()

    # Create different templates
    if template == "minimal":
        agent_content = '''"""Simple DuraGraph agent."""

from duragraph import Graph, llm_node, entrypoint


@Graph(id="simple_agent")
class SimpleAgent:
    """A simple example agent."""

    @entrypoint
    @llm_node(model="gpt-4o-mini")
    def process(self, state):
        """Process the input and generate a response."""
        return state


if __name__ == "__main__":
    agent = SimpleAgent()
    result = agent.run({"messages": [{"role": "user", "content": "Hello!"}]})
    print(f"Response: {result.output.get('response', 'No response')}")
'''
    elif template == "chatbot":
        agent_content = '''"""DuraGraph chatbot with conversation flow."""

from duragraph import Graph, llm_node, node, entrypoint


@Graph(id="chatbot_agent", description="A conversational AI chatbot")
class ChatbotAgent:
    """An intelligent chatbot that maintains conversation context."""

    @entrypoint
    @node()
    def prepare_input(self, state):
        """Prepare the input for processing."""
        if "messages" not in state:
            # Convert simple input to messages format
            user_input = state.get("input", state.get("message", "Hello"))
            state["messages"] = [{"role": "user", "content": user_input}]
        return state

    @llm_node(
        model="gpt-4o-mini",
        temperature=0.7,
        system_prompt="You are a helpful AI assistant. Be conversational and engaging.",
    )
    def generate_response(self, state):
        """Generate a response using the LLM."""
        return state

    @node()
    def format_output(self, state):
        """Format the output for the user."""
        messages = state.get("messages", [])
        if messages and messages[-1]["role"] == "assistant":
            state["response"] = messages[-1]["content"]
        return state

    # Define the conversation flow
    prepare_input >> generate_response >> format_output


if __name__ == "__main__":
    chatbot = ChatbotAgent()

    # Interactive mode
    print("DuraGraph Chatbot (type 'quit' to exit)")
    print("=" * 40)

    conversation = []
    while True:
        user_input = input("You: ").strip()
        if user_input.lower() in ("quit", "exit", "bye"):
            print("Goodbye!")
            break

        conversation.append({"role": "user", "content": user_input})
        result = chatbot.run({"messages": conversation})

        if "messages" in result.output:
            conversation = result.output["messages"]
            if conversation[-1]["role"] == "assistant":
                print(f"Bot: {conversation[-1]['content']}")
        else:
            print("Bot: I'm sorry, I couldn't process that.")
'''
    elif template == "tools":
        agent_content = '''"""DuraGraph agent with tool capabilities."""

from duragraph import Graph, llm_node, tool, entrypoint


# Define tools for the agent
@tool(description="Get the current weather for a city")
def get_weather(city: str) -> str:
    """Get weather information for a city."""
    # This would integrate with a real weather API
    return f"The weather in {city} is sunny with a temperature of 22°C"


@tool(description="Search for information on the internet")
def web_search(query: str, max_results: int = 3) -> str:
    """Search for information online."""
    # This would integrate with a real search API
    return f"Found {max_results} results for '{query}': Example result 1, Example result 2, Example result 3"


@tool(description="Calculate mathematical expressions safely")
def calculate(expression: str) -> str:
    """Calculate mathematical expressions."""
    try:
        # Simple safe evaluation
        allowed_chars = set("0123456789+-*/.()")
        if all(c in allowed_chars or c.isspace() for c in expression):
            result = eval(expression)
            return f"The result of {expression} is {result}"
        else:
            return "Error: Invalid characters in expression"
    except Exception as e:
        return f"Error calculating {expression}: {str(e)}"


@Graph(id="tool_agent", description="AI agent with tool capabilities")
class ToolAgent:
    """An AI agent that can use tools to help users."""

    @entrypoint
    @llm_node(
        model="gpt-4o-mini",
        tools=[get_weather, web_search, calculate],
        temperature=0.7,
        system_prompt="""You are a helpful AI assistant with access to tools:
        - get_weather: Get current weather for any city
        - web_search: Search the internet for information
        - calculate: Perform mathematical calculations

        Use these tools to help answer user questions accurately.
        Always explain what you're doing and why you're using specific tools.""",
    )
    def process_with_tools(self, state):
        """Process user requests with tool capabilities."""
        return state


if __name__ == "__main__":
    agent = ToolAgent()

    # Example interactions
    test_inputs = [
        "What's the weather like in Tokyo?",
        "Search for information about Python programming",
        "Calculate 15 * 23 + 7",
        "What's 2 + 2 times 3?",
    ]

    print("DuraGraph Tool Agent Demo")
    print("=" * 30)

    for user_input in test_inputs:
        print(f"\\nUser: {user_input}")
        result = agent.run({"messages": [{"role": "user", "content": user_input}]})
        print(f"Agent: {result.output.get('response', 'No response')}")
'''
    elif template == "full":
        agent_content = '''"""Full-featured DuraGraph agent with multiple nodes."""

from duragraph import Graph, llm_node, node, router_node, entrypoint, tool


@tool(description="Analyze sentiment of text")
def analyze_sentiment(text: str) -> str:
    """Analyze the sentiment of given text."""
    # Simple sentiment analysis (would use a real model in production)
    positive_words = ["good", "great", "excellent", "amazing", "wonderful", "love"]
    negative_words = ["bad", "terrible", "awful", "hate", "horrible", "worst"]

    text_lower = text.lower()
    positive_count = sum(word in text_lower for word in positive_words)
    negative_count = sum(word in text_lower for word in negative_words)

    if positive_count > negative_count:
        return "positive"
    elif negative_count > positive_count:
        return "negative"
    else:
        return "neutral"


@Graph(id="full_agent", description="A comprehensive agent with routing and tools")
class FullAgent:
    """A complete agent demonstrating various DuraGraph features."""

    @entrypoint
    @llm_node(
        model="gpt-4o-mini",
        temperature=0.3,
        system_prompt="Classify the user's intent as: greeting, question, request, or complaint"
    )
    def classify_intent(self, state):
        """Classify the user's intent."""
        return state

    @router_node()
    def route_by_intent(self, state):
        """Route based on classified intent."""
        response = state.get("response", "").lower()
        if "greeting" in response:
            return "handle_greeting"
        elif "question" in response:
            return "handle_question"
        elif "request" in response:
            return "handle_request"
        elif "complaint" in response:
            return "handle_complaint"
        else:
            return "handle_general"

    @llm_node(model="gpt-4o-mini", temperature=0.8)
    def handle_greeting(self, state):
        """Handle greetings warmly."""
        return state

    @llm_node(
        model="gpt-4o-mini",
        tools=[analyze_sentiment],
        system_prompt="You are a knowledgeable assistant. Use available tools to help answer questions."
    )
    def handle_question(self, state):
        """Handle questions with tool support."""
        return state

    @llm_node(model="gpt-4o-mini", temperature=0.5)
    def handle_request(self, state):
        """Handle user requests."""
        return state

    @llm_node(
        model="gpt-4o-mini",
        temperature=0.3,
        system_prompt="You are empathetic and focused on resolving complaints professionally."
    )
    def handle_complaint(self, state):
        """Handle complaints with empathy."""
        return state

    @llm_node(model="gpt-4o-mini")
    def handle_general(self, state):
        """Handle general interactions."""
        return state

    @node()
    def finalize_response(self, state):
        """Finalize the response before returning."""
        state["processed"] = True
        return state

    # Define the flow
    classify_intent >> route_by_intent

    # All handlers lead to finalization
    handle_greeting >> finalize_response
    handle_question >> finalize_response
    handle_request >> finalize_response
    handle_complaint >> finalize_response
    handle_general >> finalize_response


if __name__ == "__main__":
    agent = FullAgent()

    # Test different types of inputs
    test_cases = [
        "Hello there!",
        "What's the weather like today?",
        "Can you help me write an email?",
        "I'm really frustrated with this service",
        "Tell me about machine learning",
    ]

    for test_input in test_cases:
        print(f"\\nInput: {test_input}")
        result = agent.run({"messages": [{"role": "user", "content": test_input}]})
        print(f"Response: {result.output.get('response', 'No response')}")
        print(f"Processed: {result.output.get('processed', False)}")
'''

    # Write the agent file
    (project_dir / "src" / "agent.py").write_text(agent_content)

    # Create README
    readme_content = f"""# {name}

A DuraGraph agent project created with the `{template}` template.

## Getting Started

1. Install dependencies:
   ```bash
   uv sync
   ```

2. Run the agent locally:
   ```bash
   python src/agent.py
   ```

3. Start development mode with hot reload:
   ```bash
   duragraph dev
   ```

4. Deploy to a control plane:
   ```bash
   duragraph deploy src/agent.py --control-plane http://localhost:8081
   ```

5. Visualize the graph:
   ```bash
   duragraph visualize src/agent.py
   ```

## Project Structure

- `src/agent.py` - Main agent definition
- `tests/` - Test files
- `pyproject.toml` - Project configuration

## Next Steps

- Modify the agent in `src/agent.py`
- Add tests in the `tests/` directory
- Deploy to production using `duragraph deploy`
"""
    (project_dir / "README.md").write_text(readme_content)

    # Create pyproject.toml
    dependencies = ["duragraph"]
    if template in ("tools", "full"):
        dependencies.extend(["duragraph[openai]"])

    pyproject_content = f'''[project]
name = "{name}"
version = "0.1.0"
description = "DuraGraph agent created with {template} template"
dependencies = {dependencies}

[build-system]
requires = ["hatchling"]
build-backend = "hatchling.build"

[tool.duragraph]
control_plane = "http://localhost:8081"
'''
    (project_dir / "pyproject.toml").write_text(pyproject_content)

    # Create basic test file
    test_content = f'''"""Tests for the {name} agent."""

import pytest
from src.agent import *


def test_agent_creation():
    """Test that the agent can be created."""
    # Import the first Graph class found
    import src.agent as agent_module

    for name in dir(agent_module):
        obj = getattr(agent_module, name)
        if (hasattr(obj, '__class__') and
            hasattr(obj.__class__, '__name__') and
            'Graph' in str(obj.__class__)):
            # Found a graph class, instantiate it
            graph = obj
            assert graph is not None
            return

    # If no graph found, create a simple test
    assert True  # Placeholder test


def test_agent_run():
    """Test that the agent can run."""
    # This test would be customized based on the template
    assert True  # Placeholder test
'''
    (project_dir / "tests" / "test_agent.py").write_text(test_content)

    print(f"✅ Created new DuraGraph project: {name}")
    print(f"📋 Template: {template}")
    print("\n📚 Next steps:")
    print(f"  cd {name}")
    print("  uv sync")
    print("  python src/agent.py")
    print("  duragraph dev")
    return 0


def cmd_dev(file: str, port: int, control_plane: str) -> int:
    """Run in development mode with hot reload."""
    file_path = Path(file)
    if not file_path.exists():
        print(f"Error: File '{file}' not found")
        return 1

    print("🚀 Starting DuraGraph development mode...")
    print(f"📁 File: {file}")
    print(f"🌐 Port: {port}")
    print(f"🎯 Control plane: {control_plane}")
    print("📝 Press Ctrl+C to stop")
    print("=" * 50)

    try:
        return asyncio.run(_run_dev_server(file_path, port, control_plane))
    except KeyboardInterrupt:
        print("\n👋 Development server stopped")
        return 0


async def _run_dev_server(file_path: Path, port: int, control_plane: str) -> int:
    """Run the development server with hot reload."""
    import watchfiles

    from duragraph.worker import Worker

    worker = None

    async def start_worker():
        """Start or restart the worker."""
        nonlocal worker

        if worker:
            print("🔄 Restarting worker...")
            await worker._graceful_shutdown()

        # Import the graph from file
        try:
            spec = importlib.util.spec_from_file_location("user_agent", file_path)
            if spec is None or spec.loader is None:
                print(f"❌ Could not load {file_path}")
                return None

            module = importlib.util.module_from_spec(spec)
            spec.loader.exec_module(module)

            # Find Graph classes in the module
            graphs = []
            for name in dir(module):
                obj = getattr(module, name)
                if hasattr(obj, "_get_definition") and callable(
                    getattr(obj, "_get_definition", None)
                ):
                    graphs.append(obj)

            if not graphs:
                print(f"❌ No Graph classes found in {file_path}")
                return None

            # Create worker
            worker = Worker(
                control_plane_url=control_plane,
                name=f"dev-worker-{port}",
                capabilities=["dev", "local"],
                poll_interval=1.0,
                heartbeat_interval=10.0,
            )

            # Register all graphs
            for graph in graphs:
                definition = graph._get_definition()
                print(f"📋 Registered graph: {definition.graph_id}")
                worker.register_graph(definition, graph.run)

            # Start worker in background
            print(f"✅ Worker started with {len(graphs)} graph(s)")
            return worker

        except Exception as e:
            print(f"❌ Error loading {file_path}: {e}")
            return None

    # Initial start
    worker = await start_worker()
    if not worker:
        return 1

    # Start worker task
    worker_task = asyncio.create_task(worker.arun())

    # Watch for file changes
    print(f"👀 Watching {file_path} for changes...")

    try:
        async for changes in watchfiles.awatch(file_path.parent):
            # Check if our target file changed
            for _change_type, changed_path in changes:
                if Path(changed_path).name == file_path.name:
                    print(f"\n📝 File changed: {changed_path}")
                    worker = await start_worker()
                    if worker:
                        # Cancel old task and start new one
                        worker_task.cancel()
                        try:
                            await worker_task
                        except asyncio.CancelledError:
                            pass
                        worker_task = asyncio.create_task(worker.arun())
                    break
    except asyncio.CancelledError:
        if worker:
            await worker._graceful_shutdown()
        worker_task.cancel()
        return 0


def cmd_deploy(
    file: str, control_plane: str, worker_name: str | None, capabilities: list[str] | None
) -> int:
    """Deploy to control plane."""
    file_path = Path(file)
    if not file_path.exists():
        print(f"Error: File '{file}' not found")
        return 1

    print(f"🚀 Deploying to {control_plane}...")

    try:
        return asyncio.run(_deploy_agent(file_path, control_plane, worker_name, capabilities or []))
    except Exception as e:
        print(f"❌ Deployment failed: {e}")
        return 1


async def _deploy_agent(
    file_path: Path, control_plane: str, worker_name: str | None, capabilities: list[str]
) -> int:
    """Deploy the agent to control plane."""
    from duragraph.worker import Worker

    # Import the graph from file
    try:
        spec = importlib.util.spec_from_file_location("user_agent", file_path)
        if spec is None or spec.loader is None:
            print(f"❌ Could not load {file_path}")
            return 1

        module = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(module)

        # Find Graph classes
        graphs = []
        for name in dir(module):
            obj = getattr(module, name)
            if (
                inspect.isclass(obj)
                and hasattr(obj, "_get_definition")
                and callable(getattr(obj, "_get_definition", None))
            ):
                # This is a class decorated with @Graph, instantiate it
                try:
                    instance = obj()
                    graphs.append((name, instance))
                except Exception as e:
                    print(f"Warning: Could not instantiate {name}: {e}")
                    continue

        if not graphs:
            print(f"❌ No Graph classes found in {file_path}")
            return 1

        print(f"📋 Found {len(graphs)} graph(s): {', '.join(name for name, _ in graphs)}")

        # Test connection to control plane
        async with httpx.AsyncClient(timeout=10.0) as client:
            try:
                response = await client.get(f"{control_plane}/health")
                if response.status_code != 200:
                    print(f"❌ Control plane health check failed: {response.status_code}")
                    return 1
                print("✅ Control plane is healthy")
            except Exception as e:
                print(f"❌ Cannot connect to control plane: {e}")
                return 1

        # Create and configure worker
        worker = Worker(
            control_plane_url=control_plane,
            name=worker_name or f"worker-{int(time.time())}",
            capabilities=capabilities,
        )

        # Register all graphs
        for _graph_name, graph in graphs:
            definition = graph._get_definition()
            print(f"📝 Registering graph: {definition.graph_id}")
            worker.register_graph(definition, graph.run)

        print(f"🎯 Starting worker: {worker.name}")
        print(f"🔧 Capabilities: {capabilities}")
        print("📝 Press Ctrl+C to stop")

        # Run worker
        await worker.arun()

    except KeyboardInterrupt:
        print("\n👋 Worker stopped")
        return 0
    except Exception as e:
        print(f"❌ Error during deployment: {e}")
        return 1

    return 0


def cmd_visualize(file: str, output: str | None, format: str, graph: str | None) -> int:
    """Visualize a graph."""
    file_path = Path(file)
    if not file_path.exists():
        print(f"Error: File '{file}' not found")
        return 1

    try:
        # Import the graph from file
        spec = importlib.util.spec_from_file_location("user_graph", file_path)
        if spec is None or spec.loader is None:
            print(f"Error: Could not load {file}")
            return 1

        module = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(module)

        # Find Graph classes
        graphs = []
        for name in dir(module):
            obj = getattr(module, name)
            if (
                inspect.isclass(obj)
                and hasattr(obj, "_get_definition")
                and callable(getattr(obj, "_get_definition", None))
            ):
                # This is a class decorated with @Graph, instantiate it
                try:
                    instance = obj()
                    graphs.append((name, instance))
                except Exception as e:
                    print(f"Warning: Could not instantiate {name}: {e}")
                    continue

        if not graphs:
            print(f"Error: No Graph classes found in {file}")
            return 1

        # Select which graph to visualize
        if graph:
            selected_graph = None
            for name, obj in graphs:
                if name == graph:
                    selected_graph = obj
                    break
            if not selected_graph:
                print(
                    f"Error: Graph '{graph}' not found. Available: {', '.join(name for name, _ in graphs)}"
                )
                return 1
        else:
            if len(graphs) == 1:
                selected_graph = graphs[0][1]
            else:
                print(f"Multiple graphs found: {', '.join(name for name, _ in graphs)}")
                print("Please specify which graph to visualize with --graph")
                return 1

        # Get graph definition
        definition = selected_graph._get_definition()

        # Generate visualization
        if format == "mermaid":
            visualization = _generate_mermaid(definition)
        elif format == "dot":
            visualization = _generate_dot(definition)
        elif format == "json":
            visualization = _generate_json(definition)
        else:
            print(f"Error: Unsupported format '{format}'")
            return 1

        # Output result
        if output:
            Path(output).write_text(visualization)
            print(f"✅ Visualization saved to {output}")
        else:
            print(visualization)

        return 0

    except Exception as e:
        print(f"Error visualizing graph: {e}")
        return 1


def _generate_mermaid(definition: Any) -> str:
    """Generate Mermaid diagram for graph."""
    lines = ["flowchart TD"]

    # Add nodes
    for node_name, node_meta in definition.nodes.items():
        node_type = node_meta.node_type
        if node_meta.config.get("is_entrypoint"):
            shape = f"{node_name}([{node_name}<br/>📥 {node_type}])"
        elif node_type == "llm":
            shape = f"{node_name}[{node_name}<br/>🤖 {node_type}]"
        elif node_type == "router":
            shape = f"{node_name}{{{{{node_name}<br/>🔀 {node_type}}}}}"
        elif node_type == "human":
            shape = f"{node_name}[{node_name}<br/>👤 {node_type}]"
        elif node_type == "tool":
            shape = f"{node_name}[{node_name}<br/>🔧 {node_type}]"
        else:
            shape = f"{node_name}[{node_name}<br/>⚙️ {node_type}]"

        lines.append(f"    {shape}")

    # Add edges
    for edge in definition.edges:
        if isinstance(edge.target, str):
            lines.append(f"    {edge.source} --> {edge.target}")
        elif isinstance(edge.target, dict):
            # Conditional edges
            for condition, target in edge.target.items():
                lines.append(f"    {edge.source} -->|{condition}| {target}")

    return "\n".join(lines)


def _generate_dot(definition: Any) -> str:
    """Generate DOT (Graphviz) diagram for graph."""
    lines = [f'digraph "{definition.graph_id}" {{']
    lines.append("    rankdir=TD;")
    lines.append("    node [shape=box, style=rounded];")

    # Add nodes with styling
    for node_name, node_meta in definition.nodes.items():
        node_type = node_meta.node_type

        # Determine node styling
        if node_meta.config.get("is_entrypoint"):
            style = "shape=ellipse, style=filled, fillcolor=lightgreen"
        elif node_type == "llm":
            style = "style=filled, fillcolor=lightblue"
        elif node_type == "router":
            style = "shape=diamond, style=filled, fillcolor=yellow"
        elif node_type == "human":
            style = "style=filled, fillcolor=pink"
        else:
            style = "style=filled, fillcolor=lightgray"

        label = f"{node_name}\\n({node_type})"
        lines.append(f'    {node_name} [label="{label}", {style}];')

    # Add edges
    for edge in definition.edges:
        if isinstance(edge.target, str):
            lines.append(f"    {edge.source} -> {edge.target};")
        elif isinstance(edge.target, dict):
            for condition, target in edge.target.items():
                lines.append(f'    {edge.source} -> {target} [label="{condition}"];')

    lines.append("}")
    return "\n".join(lines)


def _generate_json(definition: Any) -> str:
    """Generate JSON representation of graph."""
    return json.dumps(definition.to_ir(), indent=2)


if __name__ == "__main__":
    sys.exit(main())
