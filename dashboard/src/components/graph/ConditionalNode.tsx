import { memo } from "react"
import { Position, type NodeProps, type Node } from "@xyflow/react"
import { GitBranch } from "lucide-react"
import {
  BaseNode,
  BaseNodeHeader,
  BaseNodeHeaderTitle,
  BaseNodeContent,
} from "@/components/base-node"
import { BaseHandle } from "@/components/base-handle"
import { LabeledHandle } from "@/components/labeled-handle"
import { NodeStatusIndicator } from "@/components/node-status-indicator"
import type { WorkflowNodeData, WorkflowNodeType } from "@/stores/workflow"

type ConditionalNodeProps = NodeProps<Node<WorkflowNodeData, WorkflowNodeType>>

export const ConditionalNode = memo(function ConditionalNode({
  data,
  selected,
}: ConditionalNodeProps) {
  const condition = (data.config?.condition as string) || "condition"

  return (
    <NodeStatusIndicator status={data.status}>
      <BaseNode
        className="min-w-[200px] border-amber-200 bg-amber-50 dark:border-amber-800 dark:bg-amber-950"
        data-selected={selected}
      >
        <BaseHandle
          type="target"
          position={Position.Top}
          className="!bg-amber-400 !border-amber-500"
        />
        <BaseNodeHeader className="border-b border-amber-200 dark:border-amber-800">
          <div className="flex items-center gap-2">
            <div className="flex h-6 w-6 items-center justify-center rounded-md bg-amber-100 dark:bg-amber-900">
              <GitBranch className="h-3.5 w-3.5 text-amber-600 dark:text-amber-400" />
            </div>
            <BaseNodeHeaderTitle className="text-amber-700 dark:text-amber-300">
              {data.label || "Conditional"}
            </BaseNodeHeaderTitle>
          </div>
        </BaseNodeHeader>
        <BaseNodeContent className="text-xs text-amber-600 dark:text-amber-400">
          <div className="flex items-center gap-1">
            <span className="font-mono">{condition}</span>
          </div>
        </BaseNodeContent>
        <div className="flex justify-between px-3 pb-2">
          <LabeledHandle
            type="source"
            position={Position.Bottom}
            id="true"
            title="True"
            className="!relative !transform-none"
            handleClassName="!bg-green-400 !border-green-500"
            labelClassName="text-xs text-green-600 dark:text-green-400"
          />
          <LabeledHandle
            type="source"
            position={Position.Bottom}
            id="false"
            title="False"
            className="!relative !transform-none"
            handleClassName="!bg-red-400 !border-red-500"
            labelClassName="text-xs text-red-600 dark:text-red-400"
          />
        </div>
      </BaseNode>
    </NodeStatusIndicator>
  )
})
