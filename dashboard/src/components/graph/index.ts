import type { NodeTypes, EdgeTypes } from "@xyflow/react"
import { StartNode } from "./StartNode"
import { EndNode } from "./EndNode"
import { LLMNode } from "./LLMNode"
import { ToolNode } from "./ToolNode"
import { ConditionalNode } from "./ConditionalNode"
import { HumanNode } from "./HumanNode"
import { WorkflowEdge } from "./WorkflowEdge"

export const nodeTypes: NodeTypes = {
  start: StartNode,
  end: EndNode,
  llm: LLMNode,
  tool: ToolNode,
  conditional: ConditionalNode,
  human: HumanNode,
}

export const edgeTypes: EdgeTypes = {
  workflow: WorkflowEdge,
}

export { StartNode, EndNode, LLMNode, ToolNode, ConditionalNode, HumanNode }
export { WorkflowEdge }
