import { memo } from "react"
import { Position, type NodeProps, type Node } from "@xyflow/react"
import { Square } from "lucide-react"
import { BaseNode, BaseNodeHeader, BaseNodeHeaderTitle } from "@/components/base-node"
import { BaseHandle } from "@/components/base-handle"
import { NodeStatusIndicator } from "@/components/node-status-indicator"
import type { WorkflowNodeData, WorkflowNodeType } from "@/stores/workflow"

type EndNodeProps = NodeProps<Node<WorkflowNodeData, WorkflowNodeType>>

export const EndNode = memo(function EndNode({
  data,
  selected,
}: EndNodeProps) {
  return (
    <NodeStatusIndicator status={data.status}>
      <BaseNode
        className="min-w-[120px] border-gray-400 bg-gray-50 dark:border-gray-600 dark:bg-gray-800"
        data-selected={selected}
      >
        <BaseHandle
          type="target"
          position={Position.Top}
          className="!bg-gray-400 !border-gray-500"
        />
        <BaseNodeHeader className="border-b border-gray-200 dark:border-gray-700">
          <div className="flex items-center gap-2">
            <div className="flex h-6 w-6 items-center justify-center rounded-md bg-gray-100 dark:bg-gray-700">
              <Square className="h-3.5 w-3.5 text-gray-600 dark:text-gray-400" />
            </div>
            <BaseNodeHeaderTitle className="text-gray-700 dark:text-gray-300">
              {data.label || "End"}
            </BaseNodeHeaderTitle>
          </div>
        </BaseNodeHeader>
      </BaseNode>
    </NodeStatusIndicator>
  )
})
