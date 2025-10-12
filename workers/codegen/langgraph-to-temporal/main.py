#!/usr/bin/env python3
"""
LangGraph to Temporal Workflow Code Generator

Converts LangGraph workflow definitions to Temporal workflow code.
"""

import json
import argparse
import os
from pathlib import Path
from typing import Dict, List, Any
from dataclasses import dataclass


@dataclass
class LangGraphNode:
    id: str
    type: str  # "llm", "tool", "condition", "start", "end"
    config: Dict[str, Any]
    position: Dict[str, float] = None


@dataclass 
class LangGraphEdge:
    source: str
    target: str
    condition: Dict[str, Any] = None


@dataclass
class LangGraphSpec:
    name: str
    version: str
    description: str
    nodes: List[LangGraphNode]
    edges: List[LangGraphEdge]
    config: Dict[str, Any]


@dataclass
class TemporalWorkflow:
    package_name: str
    workflow_name: str
    activities: List[str]
    generated_code: str
    workflow_code: str
    activities_code: str


def load_langgraph_spec(file_path: str) -> LangGraphSpec:
    """Load LangGraph specification from JSON file."""
    with open(file_path, 'r') as f:
        data = json.load(f)
    
    nodes = [LangGraphNode(**node) for node in data.get('nodes', [])]
    edges = [LangGraphEdge(**edge) for edge in data.get('edges', [])]
    
    return LangGraphSpec(
        name=data.get('name', 'unnamed'),
        version=data.get('version', '1.0.0'),
        description=data.get('description', ''),
        nodes=nodes,
        edges=edges,
        config=data.get('config', {})
    )


def generate_temporal_workflow(spec: LangGraphSpec) -> TemporalWorkflow:
    """Generate Temporal workflow from LangGraph specification."""
    workflow_name = sanitize_name(spec.name)
    
    workflow = TemporalWorkflow(
        package_name="generated",
        workflow_name=workflow_name,
        activities=[],
        generated_code="",
        workflow_code="",
        activities_code=""
    )
    
    # Generate workflow code
    workflow.workflow_code = generate_workflow_code(spec, workflow)
    
    # Generate activities code
    workflow.activities_code = generate_activities_code(spec, workflow)
    
    # Combine all code
    workflow.generated_code = f'''"""
{spec.description}
Generated Temporal workflow from LangGraph specification.
"""

import asyncio
from typing import Any, Dict
from temporalio import workflow, activity
from datetime import timedelta


{workflow.workflow_code}


{workflow.activities_code}
'''
    
    return workflow


def generate_workflow_code(spec: LangGraphSpec, workflow: TemporalWorkflow) -> str:
    """Generate the main workflow function."""
    node_executions = generate_node_executions(spec.nodes)
    
    return f'''@workflow.defn
class {workflow.workflow_name}Workflow:
    """
    {spec.description}
    Generated from LangGraph specification.
    """
    
    @workflow.run
    async def run(self, input_data: Dict[str, Any]) -> Dict[str, Any]:
        """Execute the workflow."""
        self.logger = workflow.logger()
        self.logger.info(f"Starting workflow: {spec.name}")
        
        # TODO: Implement workflow logic based on nodes and edges
        # This is a basic template - extend based on LangGraph spec
        
        # Execute nodes in sequence (simplified)
        result = input_data
        {node_executions}
        
        self.logger.info("Workflow completed successfully")
        return result
'''


def generate_node_executions(nodes: List[LangGraphNode]) -> str:
    """Generate code for executing nodes."""
    executions = ""
    
    for node in nodes:
        if node.type == "llm":
            executions += f'''
        # Execute LLM node: {node.id}
        result = await workflow.execute_activity(
            llm_activity,
            result,
            start_to_close_timeout=timedelta(minutes=10),
        )'''
            
        elif node.type == "tool":
            executions += f'''
        # Execute Tool node: {node.id}
        result = await workflow.execute_activity(
            tool_activity,
            result,
            start_to_close_timeout=timedelta(minutes=5),
        )'''
    
    return executions


def generate_activities_code(spec: LangGraphSpec, workflow: TemporalWorkflow) -> str:
    """Generate activities code."""
    return '''# Generated activities

@activity.defn
async def llm_activity(input_data: Dict[str, Any]) -> Dict[str, Any]:
    """Execute LLM call activity."""
    logger = activity.logger()
    logger.info("Executing LLM activity", extra={"input": input_data})
    
    # TODO: Implement actual LLM call logic
    # Placeholder implementation
    return {
        "response": "Generated LLM response",
        "tokens": 42,
    }


@activity.defn  
async def tool_activity(input_data: Dict[str, Any]) -> Dict[str, Any]:
    """Execute tool call activity."""
    logger = activity.logger()
    logger.info("Executing tool activity", extra={"input": input_data})
    
    # TODO: Implement actual tool call logic
    # Placeholder implementation
    return {
        "result": "Tool execution result", 
        "status": "success",
    }
'''


def write_generated_files(output_dir: str, workflow: TemporalWorkflow) -> None:
    """Write generated files to output directory."""
    output_path = Path(output_dir)
    output_path.mkdir(parents=True, exist_ok=True)
    
    # Write main workflow file
    workflow_file = output_path / "workflow.py"
    with open(workflow_file, 'w') as f:
        f.write(workflow.generated_code)
    
    # Write pyproject.toml
    pyproject_content = f'''[tool.poetry]
name = "{workflow.package_name}"
version = "0.1.0"
description = "Generated Temporal workflow"
authors = ["DuraGraph Codegen"]

[tool.poetry.dependencies]
python = "^3.11"
temporalio = "^1.3.0"

[build-system]
requires = ["poetry-core"]
build-backend = "poetry.core.masonry.api"
'''
    
    pyproject_file = output_path / "pyproject.toml"
    with open(pyproject_file, 'w') as f:
        f.write(pyproject_content)


def sanitize_name(name: str) -> str:
    """Sanitize name to be a valid Python identifier."""
    result = ""
    for char in name:
        if char.isalnum():
            result += char
        elif char in " -_":
            result += "_"
    
    if not result:
        result = "GeneratedWorkflow"
    
    # Ensure it starts with a letter
    if result and result[0].isdigit():
        result = "W" + result
        
    return result.title().replace("_", "")


def main():
    parser = argparse.ArgumentParser(description="Generate Temporal workflow from LangGraph specification")
    parser.add_argument("--input", required=True, help="Input LangGraph specification file (JSON)")
    parser.add_argument("--output", default="./generated", help="Output directory for generated Temporal code")
    
    args = parser.parse_args()
    
    if not os.path.exists(args.input):
        print(f"❌ Input file not found: {args.input}")
        return 1
    
    try:
        # Load LangGraph specification
        spec = load_langgraph_spec(args.input)
        
        # Generate Temporal workflow
        workflow = generate_temporal_workflow(spec)
        
        # Write generated files
        write_generated_files(args.output, workflow)
        
        print(f"✅ Successfully generated Temporal workflow from {args.input}")
        print(f"📁 Output directory: {args.output}")
        
    except Exception as e:
        print(f"❌ Failed to generate workflow: {e}")
        return 1
    
    return 0


if __name__ == "__main__":
    exit(main())