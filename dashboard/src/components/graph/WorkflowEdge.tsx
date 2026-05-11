import { memo } from "react"
import {
  getBezierPath,
  type EdgeProps,
  EdgeLabelRenderer,
  BaseEdge,
} from "@xyflow/react"
import { X } from "lucide-react"
import { Button } from "@/components/ui/button"
import { useWorkflowStore } from "@/stores/workflow"

export const WorkflowEdge = memo(function WorkflowEdge({
  id,
  sourceX,
  sourceY,
  targetX,
  targetY,
  sourcePosition,
  targetPosition,
  label,
  selected,
  markerEnd,
}: EdgeProps) {
  const deleteEdge = useWorkflowStore((state) => state.deleteEdge)

  // Bezier path = curved, organic flow between nodes. The previous
  // getSmoothStepPath produced right-angle "brick" elbows which
  // looked rigid even when the layout itself was good.
  const [edgePath, labelX, labelY] = getBezierPath({
    sourceX,
    sourceY,
    sourcePosition,
    targetX,
    targetY,
    targetPosition,
    curvature: 0.25,
  })

  // Pass-through pattern (xyflow idiom): the parent <ReactFlow /> sets
  // each edge's `markerEnd` to `{type: MarkerType.ArrowClosed}` and
  // xyflow auto-injects the matching <defs><marker/></defs> into the
  // canvas. The custom edge just forwards the prop. The previous
  // hardcoded `markerEnd="url(#arrow)"` pointed at an SVG marker that
  // was never defined anywhere — which is why edges had no arrowhead.
  return (
    <>
      <BaseEdge
        id={id}
        path={edgePath}
        markerEnd={markerEnd}
        style={{
          strokeWidth: 2,
          stroke: selected ? "var(--primary)" : "var(--muted-foreground)",
        }}
      />

      {label && (
        <EdgeLabelRenderer>
          <div
            style={{
              position: "absolute",
              transform: `translate(-50%, -50%) translate(${labelX}px, ${labelY}px)`,
              pointerEvents: "all",
            }}
            className="rounded-md border bg-background px-2 py-0.5 text-xs font-medium"
          >
            {label}
          </div>
        </EdgeLabelRenderer>
      )}
      {selected && (
        <EdgeLabelRenderer>
          <div
            style={{
              position: "absolute",
              transform: `translate(-50%, -50%) translate(${labelX}px, ${labelY}px)`,
              pointerEvents: "all",
            }}
          >
            <Button
              size="icon"
              variant="destructive"
              className="h-6 w-6"
              onClick={() => deleteEdge(id)}
            >
              <X className="h-3 w-3" />
            </Button>
          </div>
        </EdgeLabelRenderer>
      )}
    </>
  )
})
