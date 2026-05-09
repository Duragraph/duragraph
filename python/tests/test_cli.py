"""Tests for the DuraGraph CLI."""

import json
import os
from unittest.mock import MagicMock, patch

import pytest

from duragraph.cli.main import (
    _discover_graphs,
    _generate_json,
    _generate_mermaid,
    _load_module_from_file,
    _resolve_control_plane,
    cmd_compile,
    cmd_config,
    cmd_init,
    cmd_visualize,
    main,
)


class TestMain:
    def test_no_command_returns_zero(self):
        with patch("sys.argv", ["duragraph"]):
            assert main() == 0

    def test_unknown_command(self):
        with patch("sys.argv", ["duragraph", "nonexistent"]):
            with pytest.raises(SystemExit):
                main()


class TestCmdInit:
    def test_init_minimal(self, tmp_path):
        os.chdir(tmp_path)
        result = cmd_init("test-project", "minimal")
        assert result == 0

        project_dir = tmp_path / "test-project"
        assert project_dir.exists()
        assert (project_dir / "src" / "agent.py").exists()
        assert (project_dir / "pyproject.toml").exists()
        assert (project_dir / "README.md").exists()
        assert (project_dir / "tests" / "test_agent.py").exists()
        assert (project_dir / ".gitignore").exists()

        agent_content = (project_dir / "src" / "agent.py").read_text()
        assert "@Graph(id=" in agent_content
        assert "@entrypoint" in agent_content
        assert "@node()" in agent_content

    def test_init_standard(self, tmp_path):
        os.chdir(tmp_path)
        result = cmd_init("std-project", "standard")
        assert result == 0
        agent_content = (tmp_path / "std-project" / "src" / "agent.py").read_text()
        assert "standard_agent" in agent_content
        assert "@llm_node" in agent_content

    def test_init_chatbot(self, tmp_path):
        os.chdir(tmp_path)
        result = cmd_init("chat-project", "chatbot")
        assert result == 0
        content = (tmp_path / "chat-project" / "src" / "agent.py").read_text()
        assert "chatbot" in content.lower()

    def test_init_tools(self, tmp_path):
        os.chdir(tmp_path)
        result = cmd_init("tools-project", "tools")
        assert result == 0
        content = (tmp_path / "tools-project" / "src" / "agent.py").read_text()
        assert "@tool" in content

    def test_init_full(self, tmp_path):
        os.chdir(tmp_path)
        result = cmd_init("full-project", "full")
        assert result == 0
        content = (tmp_path / "full-project" / "src" / "agent.py").read_text()
        assert "@router_node" in content

    def test_init_existing_directory_fails(self, tmp_path):
        os.chdir(tmp_path)
        (tmp_path / "existing").mkdir()
        result = cmd_init("existing", "minimal")
        assert result == 1


class TestLoadModule:
    def test_load_valid_module(self, tmp_path):
        graph_file = tmp_path / "test_graph.py"
        graph_file.write_text("""
from duragraph import Graph, node, entrypoint

@Graph(id="test_graph")
class TestGraph:
    @entrypoint
    @node()
    def start(self, state):
        return {"result": "ok"}
""")
        module = _load_module_from_file(graph_file)
        assert hasattr(module, "TestGraph")

    def test_load_nonexistent_file_raises(self, tmp_path):
        with pytest.raises((RuntimeError, FileNotFoundError)):
            _load_module_from_file(tmp_path / "nonexistent.py")


class TestDiscoverGraphs:
    def test_discover_graph_class(self, tmp_path):
        graph_file = tmp_path / "test_graph.py"
        graph_file.write_text("""
from duragraph import Graph, node, entrypoint

@Graph(id="test_graph")
class TestGraph:
    @entrypoint
    @node()
    def start(self, state):
        return {"result": "ok"}
""")
        module = _load_module_from_file(graph_file)
        graphs = _discover_graphs(module)
        assert len(graphs) == 1
        assert graphs[0][0] == "TestGraph"

    def test_discover_multiple_graphs(self, tmp_path):
        graph_file = tmp_path / "multi.py"
        graph_file.write_text("""
from duragraph import Graph, node, entrypoint

@Graph(id="graph_a")
class GraphA:
    @entrypoint
    @node()
    def start(self, state):
        return state

@Graph(id="graph_b")
class GraphB:
    @entrypoint
    @node()
    def start(self, state):
        return state
""")
        module = _load_module_from_file(graph_file)
        graphs = _discover_graphs(module)
        assert len(graphs) == 2


class TestCmdCompile:
    def test_compile_single_graph(self, tmp_path):
        graph_file = tmp_path / "agent.py"
        graph_file.write_text("""
from duragraph import Graph, node, entrypoint

@Graph(id="compile_test")
class CompileTest:
    @entrypoint
    @node()
    def start(self, state):
        return state
""")
        output_file = tmp_path / "output.json"
        result = cmd_compile(str(graph_file), str(output_file), None)
        assert result == 0
        assert output_file.exists()
        ir = json.loads(output_file.read_text())
        assert ir["graph"]["id"] == "compile_test"

    def test_compile_nonexistent_file(self):
        result = cmd_compile("nonexistent.py", None, None)
        assert result == 1

    def test_compile_specific_graph(self, tmp_path):
        graph_file = tmp_path / "multi.py"
        graph_file.write_text("""
from duragraph import Graph, node, entrypoint

@Graph(id="graph_a")
class GraphA:
    @entrypoint
    @node()
    def start(self, state):
        return state

@Graph(id="graph_b")
class GraphB:
    @entrypoint
    @node()
    def start(self, state):
        return state
""")
        output_file = tmp_path / "a.json"
        result = cmd_compile(str(graph_file), str(output_file), "GraphA")
        assert result == 0
        ir = json.loads(output_file.read_text())
        assert ir["graph"]["id"] == "graph_a"

    def test_compile_unknown_graph_fails(self, tmp_path):
        graph_file = tmp_path / "agent.py"
        graph_file.write_text("""
from duragraph import Graph, node, entrypoint

@Graph(id="test")
class Test:
    @entrypoint
    @node()
    def start(self, state):
        return state
""")
        result = cmd_compile(str(graph_file), None, "NonExistent")
        assert result == 1


class TestCmdVisualize:
    def test_visualize_mermaid(self, tmp_path):
        graph_file = tmp_path / "agent.py"
        graph_file.write_text("""
from duragraph import Graph, node, entrypoint

@Graph(id="viz_test")
class VizTest:
    @entrypoint
    @node()
    def start(self, state):
        return state

    @node()
    def end(self, state):
        return state

    start >> end
""")
        output_file = tmp_path / "graph.mmd"
        result = cmd_visualize(str(graph_file), str(output_file), "mermaid", None)
        assert result == 0
        content = output_file.read_text()
        assert "flowchart TD" in content
        assert "start" in content
        assert "-->" in content

    def test_visualize_json(self, tmp_path):
        graph_file = tmp_path / "agent.py"
        graph_file.write_text("""
from duragraph import Graph, node, entrypoint

@Graph(id="json_test")
class JsonTest:
    @entrypoint
    @node()
    def start(self, state):
        return state
""")
        output_file = tmp_path / "graph.json"
        result = cmd_visualize(str(graph_file), str(output_file), "json", None)
        assert result == 0
        ir = json.loads(output_file.read_text())
        assert ir["graph"]["id"] == "json_test"

    def test_visualize_nonexistent_file(self):
        result = cmd_visualize("nonexistent.py", None, "mermaid", None)
        assert result == 1


class TestCmdConfig:
    def test_config_set_and_get(self, tmp_path):
        with patch("duragraph.cli.main._get_config_dir", return_value=tmp_path):
            args = MagicMock()
            args.config_command = "set"
            args.key = "control_plane"
            args.value = "http://example.com:8081"
            result = cmd_config(args)
            assert result == 0

            args.config_command = "get"
            args.key = "control_plane"
            result = cmd_config(args)
            assert result == 0

    def test_config_list(self, tmp_path):
        config_file = tmp_path / "config.json"
        config_file.write_text(json.dumps({"key1": "val1", "key2": "val2"}))

        with patch("duragraph.cli.main._get_config_dir", return_value=tmp_path):
            args = MagicMock()
            args.config_command = "list"
            result = cmd_config(args)
            assert result == 0

    def test_config_get_missing_key(self, tmp_path):
        config_file = tmp_path / "config.json"
        config_file.write_text("{}")

        with patch("duragraph.cli.main._get_config_dir", return_value=tmp_path):
            args = MagicMock()
            args.config_command = "get"
            args.key = "nonexistent"
            result = cmd_config(args)
            assert result == 1

    def test_config_no_subcommand(self):
        args = MagicMock()
        args.config_command = None
        result = cmd_config(args)
        assert result == 0


class TestResolveControlPlane:
    def test_explicit_url(self):
        result = _resolve_control_plane("http://example.com:8081/")
        assert result == "http://example.com:8081"

    def test_default_url(self, tmp_path):
        with patch("duragraph.cli.main._get_config_dir", return_value=tmp_path):
            result = _resolve_control_plane(None)
            assert result == "http://localhost:8081"

    def test_from_config(self, tmp_path):
        config_file = tmp_path / "config.json"
        config_file.write_text(json.dumps({"control_plane": "http://custom:9090"}))
        with patch("duragraph.cli.main._get_config_dir", return_value=tmp_path):
            result = _resolve_control_plane(None)
            assert result == "http://custom:9090"


class TestVisualizationGenerators:
    def test_mermaid_with_edges(self):
        definition = MagicMock()
        node_a = MagicMock()
        node_a.node_type = "function"
        node_a.config = {"is_entrypoint": True}
        node_b = MagicMock()
        node_b.node_type = "llm"
        node_b.config = {}

        definition.nodes = {"start": node_a, "respond": node_b}

        edge = MagicMock()
        edge.source = "start"
        edge.target = "respond"
        definition.edges = [edge]

        result = _generate_mermaid(definition)
        assert "flowchart TD" in result
        assert "start" in result
        assert "respond" in result
        assert "-->" in result

    def test_json_output(self):
        definition = MagicMock()
        definition.to_ir.return_value = {"graph": {"id": "test"}}
        result = _generate_json(definition)
        parsed = json.loads(result)
        assert parsed["graph"]["id"] == "test"
