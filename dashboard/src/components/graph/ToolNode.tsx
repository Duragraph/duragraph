import { memo } from "react"
import { Position, type NodeProps, type Node } from "@xyflow/react"
import { Wrench } from "lucide-react"
import {
  BaseNode,
  BaseNodeHeader,
  BaseNodeHeaderTitle,
  BaseNodeContent,
} from "@/components/base-node"
import { BaseHandle } from "@/components/base-handle"
import { NodeStatusIndicator } from "@/components/node-status-indicator"
import type { WorkflowNodeData, WorkflowNodeType } from "@/stores/workflow"

type ToolNodeProps = NodeProps<Node<WorkflowNodeData, WorkflowNodeType>>

export const ToolNode = memo(function ToolNode({
  data,
  selected,
}: ToolNodeProps) {
  const toolName = (data.config?.tool as string) || "tool"

  return (
    <NodeStatusIndicator status={data.status}>
      <BaseNode
        className="min-w-[180px] border-blue-200 bg-blue-50 dark:border-blue-800 dark:bg-blue-950"
        data-selected={selected}
      >
        <BaseHandle
          type="target"
          position={Position.Top}
          className="!bg-blue-400 !border-blue-500"
        />
        <BaseNodeHeader className="border-b border-blue-200 dark:border-blue-800">
          <div className="flex items-center gap-2">
            <div className="flex h-6 w-6 items-center justify-center rounded-md bg-blue-100 dark:bg-blue-900">
              <Wrench className="h-3.5 w-3.5 text-blue-600 dark:text-blue-400" />
            </div>
            <BaseNodeHeaderTitle className="text-blue-700 dark:text-blue-300">
              {data.label || "Tool"}
            </BaseNodeHeaderTitle>
          </div>
        </BaseNodeHeader>
        <BaseNodeContent className="text-xs text-blue-600 dark:text-blue-400">
          <div className="flex items-center gap-1">
            <span className="font-medium">Function:</span>
            <span className="font-mono">{toolName}</span>
          </div>
        </BaseNodeContent>
        <BaseHandle
          type="source"
          position={Position.Bottom}
          className="!bg-blue-400 !border-blue-500"
        />
      </BaseNode>
    </NodeStatusIndicator>
  )
})
