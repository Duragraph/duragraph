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

// LLMNode is also the fallback for engine-returned `function`-type
// nodes (mapNodeType in GraphVisualizer collapses unknown types
// into this one). So this component's colour scheme drives the
// "default node" appearance for the run-inspector graph. Painted
// with `var(--primary)` (DuraGraph orange) so the graph matches
// the brand instead of shadcn's stock purple ramp.
export const LLMNode = memo(function LLMNode({
  data,
  selected,
}: LLMNodeProps) {
  const model = (data.config?.model as string) || ""

  return (
    <NodeStatusIndicator status={data.status}>
      <BaseNode
        className="min-w-[180px] border-primary/40 bg-primary/10 dark:border-primary/30 dark:bg-primary/15"
        data-selected={selected}
      >
        <BaseHandle
          type="target"
          position={Position.Top}
          className="!bg-primary !border-primary"
        />
        <BaseNodeHeader className="border-b border-primary/30">
          <div className="flex items-center gap-2">
            <div className="flex h-6 w-6 items-center justify-center rounded-md bg-primary/20 dark:bg-primary/25">
              <Brain className="h-3.5 w-3.5 text-primary" />
            </div>
            <BaseNodeHeaderTitle className="text-primary">
              {data.label || "Node"}
            </BaseNodeHeaderTitle>
          </div>
        </BaseNodeHeader>
        {model && (
          <BaseNodeContent className="text-xs text-foreground/70">
            <div className="flex items-center gap-1">
              <span className="font-medium">Model:</span>
              <span className="font-mono">{model}</span>
            </div>
          </BaseNodeContent>
        )}
        <BaseHandle
          type="source"
          position={Position.Bottom}
          className="!bg-primary !border-primary"
        />
      </BaseNode>
    </NodeStatusIndicator>
  )
})
