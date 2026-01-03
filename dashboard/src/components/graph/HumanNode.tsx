import { memo } from "react"
import { Position, type NodeProps, type Node } from "@xyflow/react"
import { User } from "lucide-react"
import {
  BaseNode,
  BaseNodeHeader,
  BaseNodeHeaderTitle,
  BaseNodeContent,
} from "@/components/base-node"
import { BaseHandle } from "@/components/base-handle"
import { NodeStatusIndicator } from "@/components/node-status-indicator"
import type { WorkflowNodeData, WorkflowNodeType } from "@/stores/workflow"

type HumanNodeProps = NodeProps<Node<WorkflowNodeData, WorkflowNodeType>>

export const HumanNode = memo(function HumanNode({
  data,
  selected,
}: HumanNodeProps) {
  const prompt = (data.config?.prompt as string) || "Awaiting human input..."

  return (
    <NodeStatusIndicator status={data.status}>
      <BaseNode
        className="min-w-[200px] border-pink-200 bg-pink-50 dark:border-pink-800 dark:bg-pink-950"
        data-selected={selected}
      >
        <BaseHandle
          type="target"
          position={Position.Top}
          className="!bg-pink-400 !border-pink-500"
        />
        <BaseNodeHeader className="border-b border-pink-200 dark:border-pink-800">
          <div className="flex items-center gap-2">
            <div className="flex h-6 w-6 items-center justify-center rounded-md bg-pink-100 dark:bg-pink-900">
              <User className="h-3.5 w-3.5 text-pink-600 dark:text-pink-400" />
            </div>
            <BaseNodeHeaderTitle className="text-pink-700 dark:text-pink-300">
              {data.label || "Human Input"}
            </BaseNodeHeaderTitle>
          </div>
        </BaseNodeHeader>
        <BaseNodeContent className="text-xs text-pink-600 dark:text-pink-400">
          <p className="line-clamp-2">{prompt}</p>
        </BaseNodeContent>
        <BaseHandle
          type="source"
          position={Position.Bottom}
          className="!bg-pink-400 !border-pink-500"
        />
      </BaseNode>
    </NodeStatusIndicator>
  )
})
