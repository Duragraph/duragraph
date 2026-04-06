"""DuraGraph CLI entry point."""

import argparse
import asyncio
import importlib.util
import inspect
import json
import os
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
        description="DuraGraph - AI Workflow Orchestration CLI",
    )
    parser.add_argument("--debug", action="store_true", help="Enable debug output")
    subparsers = parser.add_subparsers(dest="command", help="Available commands")

    # init command
    init_parser = subparsers.add_parser("init", help="Initialize a new DuraGraph project")
    init_parser.add_argument("name", help="Project name")
    init_parser.add_argument(
        "--template",
        choices=["minimal", "standard", "chatbot", "tools", "full"],
        default="standard",
        help="Project template (default: standard)",
    )

    # dev command
    dev_parser = subparsers.add_parser("dev", help="Run graph locally with hot reload")
    dev_parser.add_argument(
        "file",
        nargs="?",
        default="src/agent.py",
        help="Python file containing the graph (default: src/agent.py)",
    )
    dev_parser.add_argument(
        "--port",
        type=int,
        default=8000,
        help="Local server port (default: 8000)",
    )
    dev_parser.add_argument(
        "--control-plane",
        default="http://localhost:8081",
        help="Control plane URL (default: http://localhost:8081)",
    )
    dev_parser.add_argument(
        "--no-reload",
        action="store_true",
        help="Disable hot reload",
    )

    # compile command
    compile_parser = subparsers.add_parser(
        "compile", help="Generate graph IR JSON from a Python module"
    )
    compile_parser.add_argument("file", help="Python file containing the graph")
    compile_parser.add_argument(
        "--output",
        "-o",
        help="Output file (default: stdout)",
    )
    compile_parser.add_argument(
        "--graph",
        help="Specific graph class to compile (default: all)",
    )

    # deploy command
    deploy_parser = subparsers.add_parser("deploy", help="Deploy graph to control plane")
    deploy_parser.add_argument("file", help="Python file containing the graph")
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
    deploy_parser.add_argument(
        "--nats-url",
        help="NATS URL for JetStream task delivery",
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
        help="Output format (default: mermaid)",
    )
    viz_parser.add_argument(
        "--graph",
        help="Specific graph class to visualize (default: auto-detect)",
    )

    # status command
    status_parser = subparsers.add_parser("status", help="Show deployment status")
    status_parser.add_argument(
        "--control-plane",
        default=None,
        help="Control plane URL (default: from config or http://localhost:8081)",
    )

    # logs command
    logs_parser = subparsers.add_parser("logs", help="Stream logs from deployed workers")
    logs_parser.add_argument(
        "--control-plane",
        default=None,
        help="Control plane URL",
    )
    logs_parser.add_argument(
        "--follow",
        "-f",
        action="store_true",
        help="Follow log output",
    )
    logs_parser.add_argument(
        "--worker",
        help="Filter logs by worker name",
    )

    # login command
    login_parser = subparsers.add_parser("login", help="Authenticate with control plane")
    login_parser.add_argument(
        "--url",
        help="Control plane URL",
    )
    login_parser.add_argument(
        "--token",
        help="API token (alternative to browser auth)",
    )

    # config command
    config_parser = subparsers.add_parser("config", help="Manage CLI configuration")
    config_subparsers = config_parser.add_subparsers(dest="config_command")

    config_set_parser = config_subparsers.add_parser("set", help="Set a config value")
    config_set_parser.add_argument("key", help="Configuration key")
    config_set_parser.add_argument("value", help="Configuration value")

    config_get_parser = config_subparsers.add_parser("get", help="Get a config value")
    config_get_parser.add_argument("key", help="Configuration key")

    config_subparsers.add_parser("list", help="List all configuration")

    args = parser.parse_args()

    if args.command is None:
        parser.print_help()
        return 0

    try:
        if args.command == "init":
            return cmd_init(args.name, args.template)
        elif args.command == "dev":
            return cmd_dev(args.file, args.port, args.control_plane, not args.no_reload)
        elif args.command == "compile":
            return cmd_compile(args.file, args.output, args.graph)
        elif args.command == "deploy":
            return cmd_deploy(
                args.file, args.control_plane, args.worker_name, args.capabilities, args.nats_url
            )
        elif args.command == "visualize":
            return cmd_visualize(args.file, args.output, args.format, args.graph)
        elif args.command == "status":
            return cmd_status(args.control_plane)
        elif args.command == "logs":
            return cmd_logs(args.control_plane, args.follow, args.worker)
        elif args.command == "login":
            return cmd_login(args.url, args.token)
        elif args.command == "config":
            return cmd_config(args)
    except KeyboardInterrupt:
        print("\nInterrupted by user")
        return 130
    except Exception as e:
        print(f"Error: {e}")
        if args.debug:
            traceback.print_exc()
        return 1

    return 0


# ---------------------------------------------------------------------------
# Graph discovery helpers
# ---------------------------------------------------------------------------


def _load_module_from_file(file_path: Path) -> Any:
    """Load a Python module from a file path."""
    spec = importlib.util.spec_from_file_location("user_graph", file_path)
    if spec is None or spec.loader is None:
        raise RuntimeError(f"Could not load module from {file_path}")
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


def _discover_graphs(module: Any) -> list[tuple[str, Any]]:
    """Find all @Graph-decorated classes in a module and return (name, instance) pairs."""
    graphs: list[tuple[str, Any]] = []
    for name in dir(module):
        obj = getattr(module, name)
        if (
            inspect.isclass(obj)
            and hasattr(obj, "_get_definition")
            and callable(getattr(obj, "_get_definition", None))
        ):
            try:
                instance = obj()
                graphs.append((name, instance))
            except Exception as e:
                print(f"  Warning: Could not instantiate {name}: {e}")
    return graphs


# ---------------------------------------------------------------------------
# Commands
# ---------------------------------------------------------------------------


def cmd_init(name: str, template: str) -> int:
    """Initialize a new project."""
    project_dir = Path(name)
    if project_dir.exists():
        print(f"Error: Directory '{name}' already exists")
        return 1

    project_dir.mkdir(parents=True)
    (project_dir / "src").mkdir()
    (project_dir / "tests").mkdir()

    templates: dict[str, str] = {
        "minimal": _template_minimal(),
        "standard": _template_standard(),
        "chatbot": _template_chatbot(),
        "tools": _template_tools(),
        "full": _template_full(),
    }

    agent_content = templates[template]
    (project_dir / "src" / "agent.py").write_text(agent_content)

    dependencies = '["duragraph"]'
    if template in ("tools", "full"):
        dependencies = '["duragraph", "duragraph[openai]"]'

    pyproject_content = f"""[project]
name = "{name}"
version = "0.1.0"
description = "DuraGraph agent - {template} template"
requires-python = ">=3.10"
dependencies = {dependencies}

[build-system]
requires = ["hatchling"]
build-backend = "hatchling.build"

[tool.duragraph]
control_plane = "http://localhost:8081"
"""
    (project_dir / "pyproject.toml").write_text(pyproject_content)

    readme_content = f"""# {name}

A DuraGraph agent project created with the `{template}` template.

## Getting Started

```bash
cd {name}
uv sync
python src/agent.py        # Run locally
duragraph dev              # Dev mode with hot reload
duragraph visualize src/agent.py  # See the graph
```
"""
    (project_dir / "README.md").write_text(readme_content)

    test_content = f'''"""Tests for {name} agent."""


def test_agent_import():
    """Test that the agent module can be imported."""
    import importlib.util

    spec = importlib.util.spec_from_file_location("agent", "src/agent.py")
    assert spec is not None
'''
    (project_dir / "tests" / "test_agent.py").write_text(test_content)

    (project_dir / ".gitignore").write_text(
        "__pycache__/\n*.pyc\n.venv/\n.env\ndist/\n*.egg-info/\n"
    )

    print(f"Created new DuraGraph project: {name}")
    print(f"Template: {template}")
    print()
    print("Next steps:")
    print(f"  cd {name}")
    print("  uv sync")
    print("  python src/agent.py")
    print("  duragraph dev")
    return 0


def cmd_dev(file: str, port: int, control_plane: str, reload: bool) -> int:
    """Run in development mode with hot reload."""
    file_path = Path(file)
    if not file_path.exists():
        print(f"Error: File '{file}' not found")
        return 1

    print("Starting DuraGraph development server...")
    print(f"  File: {file}")
    print(f"  Control plane: {control_plane}")
    if reload:
        print("  Hot reload: enabled")
    print()

    try:
        return asyncio.run(_run_dev_server(file_path, port, control_plane, reload))
    except KeyboardInterrupt:
        print("\nDevelopment server stopped")
        return 0


async def _run_dev_server(file_path: Path, port: int, control_plane: str, reload: bool) -> int:
    """Run the development server with optional hot reload."""
    from duragraph.worker import Worker

    worker: Worker | None = None

    async def start_worker() -> Worker | None:
        nonlocal worker
        if worker:
            print("Reloading...")
            await worker._graceful_shutdown()

        try:
            module = _load_module_from_file(file_path)
            graphs = _discover_graphs(module)

            if not graphs:
                print(f"Error: No @Graph classes found in {file_path}")
                return None

            worker = Worker(
                control_plane_url=control_plane,
                name=f"dev-worker-{port}",
                capabilities=["dev", "local"],
                poll_interval=1.0,
                heartbeat_interval=10.0,
            )

            for _graph_name, graph_instance in graphs:
                definition = graph_instance._get_definition()
                worker.register_graph(definition, instance=graph_instance)
                print(f"  Registered graph: {definition.graph_id}")

            print(f"Worker ready with {len(graphs)} graph(s)")
            return worker

        except Exception as e:
            print(f"Error loading {file_path}: {e}")
            traceback.print_exc()
            return None

    worker = await start_worker()
    if not worker:
        return 1

    worker_task = asyncio.create_task(worker.arun())

    if not reload:
        try:
            await worker_task
        except asyncio.CancelledError:
            pass
        return 0

    import watchfiles

    print(f"Watching {file_path.parent} for changes...")
    print("Press Ctrl+C to stop\n")

    try:
        async for changes in watchfiles.awatch(file_path.parent):
            for _change_type, changed_path in changes:
                if changed_path.endswith(".py"):
                    print(f"\nFile changed: {Path(changed_path).name}")
                    worker = await start_worker()
                    if worker:
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

    return 0


def cmd_compile(file: str, output: str | None, graph_name: str | None) -> int:
    """Generate graph IR JSON from a Python module."""
    file_path = Path(file)
    if not file_path.exists():
        print(f"Error: File '{file}' not found")
        return 1

    try:
        module = _load_module_from_file(file_path)
        graphs = _discover_graphs(module)

        if not graphs:
            print(f"Error: No @Graph classes found in {file}")
            return 1

        if graph_name:
            selected = [(n, g) for n, g in graphs if n == graph_name]
            if not selected:
                available = ", ".join(n for n, _ in graphs)
                print(f"Error: Graph '{graph_name}' not found. Available: {available}")
                return 1
            graphs = selected

        if len(graphs) == 1:
            definition = graphs[0][1]._get_definition()
            ir = definition.to_ir()
        else:
            ir = {
                "graphs": [g._get_definition().to_ir() for _, g in graphs],
            }

        result = json.dumps(ir, indent=2)

        if output:
            Path(output).write_text(result)
            print(f"Compiled {len(graphs)} graph(s) to {output}")
        else:
            print(result)

        return 0

    except Exception as e:
        print(f"Error compiling graph: {e}")
        return 1


def cmd_deploy(
    file: str,
    control_plane: str,
    worker_name: str | None,
    capabilities: list[str] | None,
    nats_url: str | None,
) -> int:
    """Deploy to control plane."""
    file_path = Path(file)
    if not file_path.exists():
        print(f"Error: File '{file}' not found")
        return 1

    print(f"Deploying to {control_plane}...")

    try:
        return asyncio.run(
            _deploy_agent(file_path, control_plane, worker_name, capabilities or [], nats_url)
        )
    except KeyboardInterrupt:
        print("\nWorker stopped")
        return 0
    except Exception as e:
        print(f"Deployment failed: {e}")
        return 1


async def _deploy_agent(
    file_path: Path,
    control_plane: str,
    worker_name: str | None,
    capabilities: list[str],
    nats_url: str | None,
) -> int:
    """Deploy the agent to control plane."""
    from duragraph.worker import Worker

    try:
        module = _load_module_from_file(file_path)
        graphs = _discover_graphs(module)

        if not graphs:
            print(f"Error: No @Graph classes found in {file_path}")
            return 1

        print(f"Found {len(graphs)} graph(s): {', '.join(n for n, _ in graphs)}")

        async with httpx.AsyncClient(timeout=10.0) as client:
            try:
                response = await client.get(f"{control_plane}/health")
                if response.status_code != 200:
                    print(f"Error: Control plane health check failed ({response.status_code})")
                    return 1
                print("Control plane is healthy")
            except Exception as e:
                print(f"Error: Cannot connect to control plane: {e}")
                return 1

        worker = Worker(
            control_plane_url=control_plane,
            name=worker_name or f"worker-{int(time.time())}",
            capabilities=capabilities,
            nats_url=nats_url,
        )

        for _graph_name, graph_instance in graphs:
            definition = graph_instance._get_definition()
            worker.register_graph(definition, instance=graph_instance)
            print(f"  Registered graph: {definition.graph_id}")

        print(f"Starting worker: {worker.name}")
        print("Press Ctrl+C to stop\n")
        await worker.arun()

    except Exception as e:
        print(f"Error during deployment: {e}")
        return 1

    return 0


def cmd_visualize(file: str, output: str | None, fmt: str, graph_name: str | None) -> int:
    """Visualize a graph."""
    file_path = Path(file)
    if not file_path.exists():
        print(f"Error: File '{file}' not found")
        return 1

    try:
        module = _load_module_from_file(file_path)
        graphs = _discover_graphs(module)

        if not graphs:
            print(f"Error: No @Graph classes found in {file}")
            return 1

        if graph_name:
            selected = [(n, g) for n, g in graphs if n == graph_name]
            if not selected:
                available = ", ".join(n for n, _ in graphs)
                print(f"Error: Graph '{graph_name}' not found. Available: {available}")
                return 1
            selected_graph = selected[0][1]
        elif len(graphs) == 1:
            selected_graph = graphs[0][1]
        else:
            names = ", ".join(n for n, _ in graphs)
            print(f"Multiple graphs found: {names}")
            print("Specify which graph with --graph")
            return 1

        definition = selected_graph._get_definition()

        generators = {
            "mermaid": _generate_mermaid,
            "dot": _generate_dot,
            "json": _generate_json,
        }

        visualization = generators[fmt](definition)

        if output:
            Path(output).write_text(visualization)
            print(f"Visualization saved to {output}")
        else:
            print(visualization)

        return 0

    except Exception as e:
        print(f"Error visualizing graph: {e}")
        return 1


def cmd_status(control_plane: str | None) -> int:
    """Show deployment status."""
    url = _resolve_control_plane(control_plane)
    print(f"Control plane: {url}\n")

    try:
        with httpx.Client(timeout=10.0) as client:
            response = client.get(f"{url}/health")
            if response.status_code == 200:
                print("Status: healthy")
                health = (
                    response.json()
                    if response.headers.get("content-type", "").startswith("application/json")
                    else {}
                )
                if health:
                    for key, value in health.items():
                        print(f"  {key}: {value}")
            else:
                print(f"Status: unhealthy ({response.status_code})")
                return 1

            try:
                workers_resp = client.get(f"{url}/api/v1/workers")
                if workers_resp.status_code == 200:
                    workers = workers_resp.json()
                    if isinstance(workers, list):
                        print(f"\nWorkers: {len(workers)}")
                        for w in workers:
                            name = w.get("name", w.get("id", "unknown"))
                            status = w.get("status", "unknown")
                            graphs = ", ".join(w.get("graph_ids", []))
                            print(f"  {name}: {status} [{graphs}]")
                    elif isinstance(workers, dict) and "workers" in workers:
                        worker_list = workers["workers"]
                        print(f"\nWorkers: {len(worker_list)}")
                        for w in worker_list:
                            name = w.get("name", w.get("id", "unknown"))
                            status = w.get("status", "unknown")
                            print(f"  {name}: {status}")
            except Exception:
                pass

    except httpx.ConnectError:
        print(f"Error: Cannot connect to {url}")
        return 1
    except Exception as e:
        print(f"Error: {e}")
        return 1

    return 0


def cmd_logs(control_plane: str | None, follow: bool, worker_filter: str | None) -> int:
    """Stream logs from deployed workers."""
    url = _resolve_control_plane(control_plane)

    try:
        with httpx.Client(timeout=30.0) as client:
            params: dict[str, Any] = {}
            if worker_filter:
                params["worker"] = worker_filter

            endpoint = f"{url}/api/v1/workers/logs"

            if follow:
                print(f"Streaming logs from {url} (Ctrl+C to stop)...\n")
                with client.stream("GET", endpoint, params=params) as response:
                    if response.status_code != 200:
                        print(f"Error: {response.status_code}")
                        return 1
                    for line in response.iter_lines():
                        if line:
                            print(line)
            else:
                response = client.get(endpoint, params=params)
                if response.status_code == 200:
                    data = response.json()
                    if isinstance(data, list):
                        for entry in data:
                            ts = entry.get("timestamp", "")
                            level = entry.get("level", "INFO")
                            msg = entry.get("message", "")
                            worker = entry.get("worker", "")
                            prefix = f"[{worker}] " if worker else ""
                            print(f"{ts} {level} {prefix}{msg}")
                    else:
                        print(response.text)
                elif response.status_code == 404:
                    print("Logs endpoint not available on this control plane")
                    return 1
                else:
                    print(f"Error: {response.status_code}")
                    return 1

    except httpx.ConnectError:
        print(f"Error: Cannot connect to {url}")
        return 1
    except KeyboardInterrupt:
        print("\nStopped")
        return 0
    except Exception as e:
        print(f"Error: {e}")
        return 1

    return 0


def cmd_login(url: str | None, token: str | None) -> int:
    """Authenticate with control plane."""
    config_dir = _get_config_dir()
    config_dir.mkdir(parents=True, exist_ok=True)
    creds_file = config_dir / "credentials"

    if token:
        target_url = url or "http://localhost:8081"
        creds = {"url": target_url, "token": token}
        creds_file.write_text(json.dumps(creds, indent=2))
        print(f"Credentials saved for {target_url}")
        return 0

    if url:
        try:
            with httpx.Client(timeout=10.0) as client:
                response = client.get(f"{url}/health")
                if response.status_code == 200:
                    creds = {"url": url, "token": ""}
                    creds_file.write_text(json.dumps(creds, indent=2))
                    print(f"Connected to {url}")
                    print(f"Credentials saved to {creds_file}")
                    return 0
                else:
                    print(f"Error: Control plane returned {response.status_code}")
                    return 1
        except httpx.ConnectError:
            print(f"Error: Cannot connect to {url}")
            return 1

    print("Usage: duragraph login --url <control-plane-url> [--token <token>]")
    return 1


def cmd_config(args: Any) -> int:
    """Manage CLI configuration."""
    config_dir = _get_config_dir()
    config_file = config_dir / "config.json"

    if not hasattr(args, "config_command") or args.config_command is None:
        print("Usage: duragraph config <set|get|list>")
        return 0

    config: dict[str, str] = {}
    if config_file.exists():
        try:
            config = json.loads(config_file.read_text())
        except json.JSONDecodeError:
            config = {}

    if args.config_command == "set":
        config[args.key] = args.value
        config_dir.mkdir(parents=True, exist_ok=True)
        config_file.write_text(json.dumps(config, indent=2))
        print(f"{args.key} = {args.value}")
        return 0

    elif args.config_command == "get":
        value = config.get(args.key)
        if value is not None:
            print(f"{args.key} = {value}")
        else:
            print(f"Key '{args.key}' not set")
            return 1
        return 0

    elif args.config_command == "list":
        if not config:
            print("No configuration set")
        else:
            for key, value in sorted(config.items()):
                print(f"{key} = {value}")
        return 0

    return 0


# ---------------------------------------------------------------------------
# Config / credential helpers
# ---------------------------------------------------------------------------


def _get_config_dir() -> Path:
    """Get the DuraGraph config directory."""
    xdg = os.environ.get("XDG_CONFIG_HOME")
    if xdg:
        return Path(xdg) / "duragraph"
    return Path.home() / ".config" / "duragraph"


def _resolve_control_plane(url: str | None) -> str:
    """Resolve control plane URL from argument, config, or default."""
    if url:
        return url.rstrip("/")

    config_file = _get_config_dir() / "config.json"
    if config_file.exists():
        try:
            config = json.loads(config_file.read_text())
            if "control_plane" in config:
                return config["control_plane"].rstrip("/")
        except json.JSONDecodeError:
            pass

    creds_file = _get_config_dir() / "credentials"
    if creds_file.exists():
        try:
            creds = json.loads(creds_file.read_text())
            if "url" in creds:
                return creds["url"].rstrip("/")
        except json.JSONDecodeError:
            pass

    return "http://localhost:8081"


# ---------------------------------------------------------------------------
# Visualization generators
# ---------------------------------------------------------------------------


def _generate_mermaid(definition: Any) -> str:
    """Generate Mermaid diagram for graph."""
    lines = ["flowchart TD"]

    for node_name, node_meta in definition.nodes.items():
        node_type = node_meta.node_type
        if node_meta.config.get("is_entrypoint"):
            shape = f"    {node_name}([{node_name} - {node_type}])"
        elif node_type == "llm":
            shape = f"    {node_name}[{node_name} - llm]"
        elif node_type == "router":
            shape = f"    {node_name}{{{{{node_name} - router}}}}"
        elif node_type == "human":
            shape = f"    {node_name}[/{node_name} - human/]"
        elif node_type == "tool":
            shape = f"    {node_name}[{node_name} - tool]"
        else:
            shape = f"    {node_name}[{node_name}]"

        lines.append(shape)

    for edge in definition.edges:
        if isinstance(edge.target, str):
            lines.append(f"    {edge.source} --> {edge.target}")
        elif isinstance(edge.target, dict):
            for condition, target in edge.target.items():
                lines.append(f"    {edge.source} -->|{condition}| {target}")

    return "\n".join(lines)


def _generate_dot(definition: Any) -> str:
    """Generate DOT (Graphviz) diagram for graph."""
    lines = [f'digraph "{definition.graph_id}" {{']
    lines.append("    rankdir=TD;")
    lines.append("    node [shape=box, style=rounded];")

    for node_name, node_meta in definition.nodes.items():
        node_type = node_meta.node_type
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


# ---------------------------------------------------------------------------
# Project templates
# ---------------------------------------------------------------------------


def _template_minimal() -> str:
    return '''"""Simple DuraGraph agent."""

from duragraph import Graph, node, entrypoint


@Graph(id="simple_agent")
class SimpleAgent:
    @entrypoint
    @node()
    def process(self, state):
        """Process the input and return a response."""
        message = state.get("messages", [{}])[-1].get("content", "")
        return {"response": f"Processed: {message}"}


if __name__ == "__main__":
    agent = SimpleAgent()
    result = agent.run({"messages": [{"role": "user", "content": "Hello!"}]})
    print(f"Response: {result.output.get('response')}")
'''


def _template_standard() -> str:
    return '''"""DuraGraph agent with input preparation and response formatting."""

from duragraph import Graph, node, llm_node, entrypoint


@Graph(id="standard_agent", description="Standard agent with prepare/respond/format flow")
class StandardAgent:
    @entrypoint
    @node()
    def prepare(self, state):
        """Prepare the input for processing."""
        if "messages" not in state:
            state["messages"] = [{"role": "user", "content": state.get("input", "")}]
        return state

    @llm_node(model="gpt-4o-mini")
    def respond(self, state):
        """Generate a response using the LLM."""
        return state

    @node()
    def format_output(self, state):
        """Extract the final response."""
        messages = state.get("messages", [])
        if messages and messages[-1].get("role") == "assistant":
            state["response"] = messages[-1]["content"]
        return state

    prepare >> respond >> format_output


if __name__ == "__main__":
    agent = StandardAgent()
    result = agent.run({"messages": [{"role": "user", "content": "Hello!"}]})
    print(f"Response: {result.output.get('response', 'No response')}")
'''


def _template_chatbot() -> str:
    return '''"""DuraGraph chatbot with conversation flow."""

from duragraph import Graph, llm_node, node, entrypoint


@Graph(id="chatbot", description="Conversational AI chatbot")
class Chatbot:
    @entrypoint
    @node()
    def prepare(self, state):
        """Ensure messages list exists."""
        if "messages" not in state:
            state["messages"] = [
                {"role": "user", "content": state.get("input", "Hello")}
            ]
        return state

    @llm_node(
        model="gpt-4o-mini",
        temperature=0.7,
        system_prompt="You are a helpful AI assistant. Be conversational and engaging.",
    )
    def respond(self, state):
        """Generate a response using the LLM."""
        return state

    @node()
    def format_output(self, state):
        """Extract the assistant's response."""
        messages = state.get("messages", [])
        if messages and messages[-1].get("role") == "assistant":
            state["response"] = messages[-1]["content"]
        return state

    prepare >> respond >> format_output


if __name__ == "__main__":
    chatbot = Chatbot()
    print("DuraGraph Chatbot (type 'quit' to exit)")
    print("=" * 40)

    conversation = []
    while True:
        user_input = input("You: ").strip()
        if user_input.lower() in ("quit", "exit", "bye"):
            print("Goodbye!")
            break

        conversation.append({"role": "user", "content": user_input})
        result = chatbot.run({"messages": list(conversation)})

        if "messages" in result.output:
            conversation = result.output["messages"]
            if conversation[-1].get("role") == "assistant":
                print(f"Bot: {conversation[-1]['content']}")
        else:
            print("Bot: I could not process that.")
'''


def _template_tools() -> str:
    return '''"""DuraGraph agent with tool capabilities."""

from duragraph import Graph, llm_node, tool, entrypoint


@tool(description="Get the current weather for a city")
def get_weather(city: str) -> str:
    """Get weather information."""
    return f"The weather in {city} is sunny, 22C"


@tool(description="Calculate a mathematical expression")
def calculate(expression: str) -> str:
    """Calculate a math expression."""
    allowed = set("0123456789+-*/.()")
    if all(c in allowed or c.isspace() for c in expression):
        return str(eval(expression))
    return "Error: invalid expression"


@Graph(id="tool_agent", description="AI agent with tool capabilities")
class ToolAgent:
    @entrypoint
    @llm_node(
        model="gpt-4o-mini",
        tools=[get_weather, calculate],
        system_prompt="You are a helpful assistant with weather and calculation tools.",
    )
    def process(self, state):
        """Process user requests with tools."""
        return state


if __name__ == "__main__":
    agent = ToolAgent()
    result = agent.run({"messages": [{"role": "user", "content": "What is 15 * 23?"}]})
    print(f"Response: {result.output.get('response', 'No response')}")
'''


def _template_full() -> str:
    return '''"""Full-featured DuraGraph agent with routing."""

from duragraph import Graph, llm_node, node, router_node, entrypoint


@Graph(id="full_agent", description="Agent with intent classification and routing")
class FullAgent:
    @entrypoint
    @node()
    def classify(self, state):
        """Classify user intent."""
        msg = state.get("messages", [{}])[-1].get("content", "").lower()
        if any(w in msg for w in ["hello", "hi", "hey"]):
            state["intent"] = "greeting"
        elif "?" in msg:
            state["intent"] = "question"
        else:
            state["intent"] = "general"
        return state

    @router_node()
    def route(self, state):
        """Route based on intent."""
        intent = state.get("intent", "general")
        return f"handle_{intent}"

    @node()
    def handle_greeting(self, state):
        """Handle greetings."""
        state["response"] = "Hello! How can I help you today?"
        return state

    @llm_node(model="gpt-4o-mini")
    def handle_question(self, state):
        """Handle questions with LLM."""
        return state

    @node()
    def handle_general(self, state):
        """Handle general messages."""
        state["response"] = "I understand. How can I assist you further?"
        return state

    classify >> route
    route >> handle_greeting
    route >> handle_question
    route >> handle_general


if __name__ == "__main__":
    agent = FullAgent()
    for msg in ["Hello!", "What is Python?", "Thanks"]:
        result = agent.run({"messages": [{"role": "user", "content": msg}]})
        print(f"{msg} -> {result.output.get('response', 'No response')}")
'''


if __name__ == "__main__":
    sys.exit(main())
