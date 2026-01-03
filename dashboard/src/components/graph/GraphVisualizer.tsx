import { useMemo } from "react"
import {
  ReactFlow,
  Background,
  Controls,
  MiniMap,
  type Node,
  type Edge,
} from "@xyflow/react"
import "@xyflow/react/dist/style.css"
import { nodeTypes, edgeTypes } from "./index"
import { useThemeStore } from "@/stores/theme"
import type { Graph, GraphNode, GraphEdge } from "@/types/entities"
import type { WorkflowNodeData, WorkflowNodeType } from "@/stores/workflow"
import type { NodeStatus } from "@/components/node-status-indicator"

export type ExecutionStatus = "idle" | "running" | "completed" | "error"

interface GraphVisualizerProps {
  graph: Graph | null
  nodeStatuses?: Record<string, ExecutionStatus>
  className?: string
  showMiniMap?: boolean
  showControls?: boolean
}

// Map execution status to NodeStatus used by the node components
function mapExecutionStatus(status?: ExecutionStatus): NodeStatus | undefined {
  if (!status || status === "idle") return undefined
  if (status === "running") return "loading"
  if (status === "completed") return "success"
  if (status === "error") return "error"
  return undefined
}

// Convert API graph format to ReactFlow format
function convertToReactFlowFormat(
  graph: Graph,
  nodeStatuses?: Record<string, ExecutionStatus>
): {
  nodes: Node<WorkflowNodeData, WorkflowNodeType>[]
  edges: Edge[]
} {
  // Auto-layout: arrange nodes in a vertical flow
  const nodePositions = calculateNodePositions(graph.nodes, graph.edges, graph.entry_point)

  const nodes: Node<WorkflowNodeData, WorkflowNodeType>[] = graph.nodes.map((node) => {
    const position = nodePositions.get(node.id) || { x: 0, y: 0 }
    const nodeType = mapNodeType(node.type)

    return {
      id: node.id,
      type: nodeType,
      position: node.position || position,
      data: {
        label: node.id,
        type: nodeType,
        config: node.config,
        status: mapExecutionStatus(nodeStatuses?.[node.id]),
      },
    }
  })

  const edges: Edge[] = graph.edges.map((edge) => ({
    id: edge.id,
    source: edge.source,
    target: edge.target,
    type: "workflow",
    label: edge.condition,
    animated: nodeStatuses?.[edge.source] === "running",
  }))

  return { nodes, edges }
}

function mapNodeType(apiType: GraphNode["type"]): WorkflowNodeType {
  switch (apiType) {
    case "llm":
      return "llm"
    case "tool":
      return "tool"
    case "conditional":
      return "conditional"
    case "human":
      return "human"
    case "start":
      return "start"
    case "end":
      return "end"
    default:
      return "llm"
  }
}

// Simple auto-layout algorithm for directed graphs
function calculateNodePositions(
  nodes: GraphNode[],
  edges: GraphEdge[],
  entryPoint: string
): Map<string, { x: number; y: number }> {
  const positions = new Map<string, { x: number; y: number }>()
  const visited = new Set<string>()
  const levels = new Map<string, number>()

  // Build adjacency list
  const adjacency = new Map<string, string[]>()
  nodes.forEach((node) => adjacency.set(node.id, []))
  edges.forEach((edge) => {
    const sources = adjacency.get(edge.source) || []
    sources.push(edge.target)
    adjacency.set(edge.source, sources)
  })

  // BFS to assign levels
  const queue: { id: string; level: number }[] = [{ id: entryPoint, level: 0 }]
  while (queue.length > 0) {
    const { id, level } = queue.shift()!
    if (visited.has(id)) continue
    visited.add(id)
    levels.set(id, level)

    const neighbors = adjacency.get(id) || []
    neighbors.forEach((neighbor) => {
      if (!visited.has(neighbor)) {
        queue.push({ id: neighbor, level: level + 1 })
      }
    })
  }

  // Handle nodes not reachable from entry point
  nodes.forEach((node) => {
    if (!levels.has(node.id)) {
      levels.set(node.id, 0)
    }
  })

  // Group nodes by level
  const levelGroups = new Map<number, string[]>()
  levels.forEach((level, nodeId) => {
    const group = levelGroups.get(level) || []
    group.push(nodeId)
    levelGroups.set(level, group)
  })

  // Calculate positions
  const nodeWidth = 200
  const nodeHeight = 120
  const horizontalGap = 50
  const verticalGap = 80

  levelGroups.forEach((nodeIds, level) => {
    const totalWidth = nodeIds.length * nodeWidth + (nodeIds.length - 1) * horizontalGap
    const startX = -totalWidth / 2

    nodeIds.forEach((nodeId, index) => {
      positions.set(nodeId, {
        x: startX + index * (nodeWidth + horizontalGap),
        y: level * (nodeHeight + verticalGap),
      })
    })
  })

  return positions
}

export function GraphVisualizer({
  graph,
  nodeStatuses,
  className = "",
  showMiniMap = true,
  showControls = true,
}: GraphVisualizerProps) {
  const theme = useThemeStore((state) => state.theme)

  const { nodes, edges } = useMemo(() => {
    if (!graph) {
      return { nodes: [], edges: [] }
    }
    return convertToReactFlowFormat(graph, nodeStatuses)
  }, [graph, nodeStatuses])

  const getColorMode = () => {
    if (theme === "system") {
      return typeof window !== "undefined" &&
        window.matchMedia("(prefers-color-scheme: dark)").matches
        ? "dark"
        : "light"
    }
    return theme
  }

  if (!graph) {
    return (
      <div className={`flex items-center justify-center h-full bg-muted/20 rounded-lg ${className}`}>
        <p className="text-muted-foreground text-sm">No graph data available</p>
      </div>
    )
  }

  return (
    <div className={`h-full w-full ${className}`}>
      <ReactFlow
        nodes={nodes}
        edges={edges}
        nodeTypes={nodeTypes}
        edgeTypes={edgeTypes}
        colorMode={getColorMode()}
        fitView
        fitViewOptions={{ padding: 0.2 }}
        nodesDraggable={false}
        nodesConnectable={false}
        elementsSelectable={false}
        panOnDrag
        zoomOnScroll
        preventScrolling={false}
      >
        <Background gap={16} size={1} />
        {showControls && <Controls showInteractive={false} />}
        {showMiniMap && (
          <MiniMap
            nodeStrokeWidth={3}
            zoomable
            pannable
            className="!bg-background !border-border"
          />
        )}
        <svg>
          <defs>
            <marker
              id="arrow"
              markerWidth="12"
              markerHeight="12"
              refX="10"
              refY="6"
              orient="auto"
            >
              <path
                d="M2,2 L10,6 L2,10 L4,6 Z"
                fill="hsl(var(--muted-foreground))"
              />
            </marker>
          </defs>
        </svg>
      </ReactFlow>
    </div>
  )
}
