import { memo } from "react"
import { Position, type NodeProps, type Node } from "@xyflow/react"
import { Brain } from "lucide-react"
import {
  BaseNode,
  BaseNodeHeader,
  BaseNodeHeaderTitle,
  BaseNodeContent,
} from "@/components/base-node"
import { BaseHandle } from "@/components/base-handle"
import { NodeStatusIndicator } from "@/components/node-status-indicator"
import type { WorkflowNodeData, WorkflowNodeType } from "@/stores/workflow"

type LLMNodeProps = NodeProps<Node<WorkflowNodeData, WorkflowNodeType>>

export const LLMNode = memo(function LLMNode({
  data,
  selected,
}: LLMNodeProps) {
  const model = (data.config?.model as string) || "gpt-4"

  return (
    <NodeStatusIndicator status={data.status}>
      <BaseNode
        className="min-w-[180px] border-purple-200 bg-purple-50 dark:border-purple-800 dark:bg-purple-950"
        data-selected={selected}
      >
        <BaseHandle
          type="target"
          position={Position.Top}
          className="!bg-purple-400 !border-purple-500"
        />
        <BaseNodeHeader className="border-b border-purple-200 dark:border-purple-800">
          <div className="flex items-center gap-2">
            <div className="flex h-6 w-6 items-center justify-center rounded-md bg-purple-100 dark:bg-purple-900">
              <Brain className="h-3.5 w-3.5 text-purple-600 dark:text-purple-400" />
            </div>
            <BaseNodeHeaderTitle className="text-purple-700 dark:text-purple-300">
              {data.label || "LLM"}
            </BaseNodeHeaderTitle>
          </div>
        </BaseNodeHeader>
        <BaseNodeContent className="text-xs text-purple-600 dark:text-purple-400">
          <div className="flex items-center gap-1">
            <span className="font-medium">Model:</span>
            <span className="font-mono">{model}</span>
          </div>
        </BaseNodeContent>
        <BaseHandle
          type="source"
          position={Position.Bottom}
          className="!bg-purple-400 !border-purple-500"
        />
      </BaseNode>
    </NodeStatusIndicator>
  )
})
