import { memo } from "react"
import { Position, type NodeProps, type Node } from "@xyflow/react"
import { Play } from "lucide-react"
import { BaseNode, BaseNodeHeader, BaseNodeHeaderTitle } from "@/components/base-node"
import { BaseHandle } from "@/components/base-handle"
import { NodeStatusIndicator } from "@/components/node-status-indicator"
import type { WorkflowNodeData, WorkflowNodeType } from "@/stores/workflow"

type StartNodeProps = NodeProps<Node<WorkflowNodeData, WorkflowNodeType>>

export const StartNode = memo(function StartNode({
  data,
  selected,
}: StartNodeProps) {
  return (
    <NodeStatusIndicator status={data.status}>
      <BaseNode
        className="min-w-[120px] border-green-400 bg-green-50 dark:border-green-600 dark:bg-green-950"
        data-selected={selected}
      >
        <BaseNodeHeader className="border-b border-green-200 dark:border-green-800">
          <div className="flex items-center gap-2">
            <div className="flex h-6 w-6 items-center justify-center rounded-md bg-green-100 dark:bg-green-900">
              <Play className="h-3.5 w-3.5 text-green-600 dark:text-green-400" />
            </div>
            <BaseNodeHeaderTitle className="text-green-700 dark:text-green-300">
              {data.label || "Start"}
            </BaseNodeHeaderTitle>
          </div>
        </BaseNodeHeader>
        <BaseHandle
          type="source"
          position={Position.Bottom}
          className="!bg-green-500 !border-green-600"
        />
      </BaseNode>
    </NodeStatusIndicator>
  )
})
