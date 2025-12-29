"""DuraGraph CLI entry point."""

import argparse
import sys
from pathlib import Path


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
        choices=["minimal", "full"],
        default="minimal",
        help="Project template",
    )

    # dev command
    dev_parser = subparsers.add_parser("dev", help="Run in development mode")
    dev_parser.add_argument(
        "--port",
        type=int,
        default=8000,
        help="Port for local server",
    )
    dev_parser.add_argument(
        "--reload",
        action="store_true",
        help="Enable auto-reload",
    )

    # deploy command
    deploy_parser = subparsers.add_parser("deploy", help="Deploy to control plane")
    deploy_parser.add_argument(
        "--control-plane",
        required=True,
        help="Control plane URL",
    )
    deploy_parser.add_argument(
        "--graph",
        help="Specific graph to deploy (default: all)",
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

    args = parser.parse_args()

    if args.command is None:
        parser.print_help()
        return 0

    if args.command == "init":
        return cmd_init(args.name, args.template)
    elif args.command == "dev":
        return cmd_dev(args.port, args.reload)
    elif args.command == "deploy":
        return cmd_deploy(args.control_plane, args.graph)
    elif args.command == "visualize":
        return cmd_visualize(args.file, args.output, args.format)

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

    # Create main agent file
    agent_content = '''"""Example DuraGraph agent."""

from duragraph import Graph, llm_node, entrypoint


@Graph(id="example_agent")
class ExampleAgent:
    """A simple example agent."""

    @entrypoint
    @llm_node(model="gpt-4o-mini")
    def process(self, state):
        """Process the input."""
        return state


if __name__ == "__main__":
    agent = ExampleAgent()
    result = agent.run({"message": "Hello, world!"})
    print(result)
'''
    (project_dir / "src" / "agent.py").write_text(agent_content)

    # Create pyproject.toml
    pyproject_content = f'''[project]
name = "{name}"
version = "0.1.0"
dependencies = ["duragraph"]

[tool.duragraph]
control_plane = "http://localhost:8081"
'''
    (project_dir / "pyproject.toml").write_text(pyproject_content)

    print(f"Created new DuraGraph project: {name}")
    print("\nNext steps:")
    print(f"  cd {name}")
    print("  uv sync")
    print("  duragraph dev")
    return 0


def cmd_dev(port: int, reload: bool) -> int:
    """Run in development mode."""
    print(f"Starting development server on port {port}...")
    print("Development mode not yet implemented")
    return 1


def cmd_deploy(control_plane: str, graph: str | None) -> int:
    """Deploy to control plane."""
    print(f"Deploying to {control_plane}...")
    print("Deployment not yet implemented")
    return 1


def cmd_visualize(file: str, output: str | None, format: str) -> int:
    """Visualize a graph."""
    print(f"Visualizing {file}...")
    print("Visualization not yet implemented")
    return 1


if __name__ == "__main__":
    sys.exit(main())
