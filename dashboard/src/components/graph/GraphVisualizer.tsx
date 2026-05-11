import { useEffect, useMemo, useRef } from "react"
import {
  ReactFlow,
  Background,
  Controls,
  MarkerType,
  MiniMap,
  useEdgesState,
  useNodesState,
  type Node,
  type Edge,
} from "@xyflow/react"
import "@xyflow/react/dist/style.css"
import { nodeTypes, edgeTypes } from "./index"
import { layoutWithElk } from "./useElkLayout"
import { useThemeStore } from "@/stores/theme"
import type { Graph, GraphNode } from "@/types/entities"
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

// Convert API graph format to ReactFlow format. Nodes are emitted
// with a placeholder `position: {0, 0}` — ELK takes over and computes
// the real layout on mount via `useElkLayout` in the parent. This
// replaces an earlier hand-rolled BFS-into-grid placer which made
// the graph look like a stack of bricks (every node forced onto a
// row in lockstep). ELK's layered + NETWORK_SIMPLEX strategy
// produces softer column placement and the POLYLINE edge routing
// rounds off right-angle elbows.
function convertToReactFlowFormat(
  graph: Graph,
  nodeStatuses?: Record<string, ExecutionStatus>
): {
  nodes: Node<WorkflowNodeData, WorkflowNodeType>[]
  edges: Edge[]
} {
  const nodes: Node<WorkflowNodeData, WorkflowNodeType>[] = graph.nodes.map((node) => {
    const nodeType = mapNodeType(node.type)
    return {
      id: node.id,
      type: nodeType,
      position: node.position ?? { x: 0, y: 0 },
      data: {
        label: node.id,
        type: nodeType,
        config: node.config,
        status: mapExecutionStatus(nodeStatuses?.[node.id]),
      },
    }
  })

  const edges: Edge[] = graph.edges.map((edge, i) => ({
    // xyflow requires a non-undefined id; the engine's GraphEdgeResponse
    // typed it as optional. Fall back to a deterministic source-target
    // synthetic id so a graph response without explicit edge ids still
    // renders one edge per pair.
    id: edge.id ?? `${edge.source}->${edge.target}#${i}`,
    source: edge.source,
    target: edge.target,
    type: "workflow",
    label: edge.condition,
    animated: nodeStatuses?.[edge.source] === "running",
    // markerEnd is what xyflow uses to render the arrowhead. The
    // custom <WorkflowEdge> passes this prop straight to <BaseEdge>;
    // xyflow auto-injects matching <defs><marker/></defs> into the
    // canvas so we don't need to define the SVG marker ourselves.
    markerEnd: { type: MarkerType.ArrowClosed },
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

// (Hand-rolled BFS-into-grid placer was here; replaced by ELK via
// useElkLayout — see useElkLayout.ts for the layout config.)

export function GraphVisualizer({
  graph,
  nodeStatuses,
  className = "",
  showMiniMap = true,
  showControls = true,
}: GraphVisualizerProps) {
  const theme = useThemeStore((state) => state.theme)

  // Compute the bare ReactFlow shape from the API graph. The data
  // half of every node (status, label, config) is recomputed every
  // render — that's intentional, see the merge effect below.
  const { nodes: incoming, edges: incomingEdges } = useMemo(() => {
    if (!graph) {
      return { nodes: [], edges: [] }
    }
    return convertToReactFlowFormat(graph, nodeStatuses)
  }, [graph, nodeStatuses])

  // Hand node + edge state over to xyflow via its own hooks. xyflow's
  // `onNodesChange` (auto-wired by `useNodesState`) records every
  // user drag into this state, so positions persist across the
  // parent's poll-driven re-renders. Without this we were re-running
  // ELK on every render and obliterating the user's drag positions —
  // hence "frozen canvas".
  const [nodes, setNodes, onNodesChange] = useNodesState<
    Node<WorkflowNodeData, WorkflowNodeType>
  >([])
  const [edges, setEdges, onEdgesChange] = useEdgesState<Edge>([])

  // Track the topology so we know when to re-run ELK. Re-laying out
  // on data updates would also reset user drags; we only do it when
  // the graph's node/edge SHAPE changes (a new node appears, edge
  // added, etc.).
  const topologyRef = useRef<string>("")

  useEffect(() => {
    const key =
      incoming.map((n) => n.id).sort().join("|") +
      "::" +
      incomingEdges.map((e) => `${e.source}>${e.target}`).sort().join("|")

    if (key !== topologyRef.current) {
      // New topology — run ELK then commit.
      topologyRef.current = key
      let cancelled = false
      layoutWithElk(incoming, incomingEdges).then((laid) => {
        if (cancelled) return
        setNodes(laid)
        setEdges(incomingEdges)
      })
      return () => {
        cancelled = true
      }
    }

    // Same topology, only data changed (status badge, label) — merge
    // fresh `data` into the existing nodes WITHOUT touching their
    // (user-modified) positions. Edge data is currently static; we
    // still mirror so animated state updates with run progress.
    setNodes((prev) =>
      prev.map((n) => {
        const fresh = incoming.find((x) => x.id === n.id)
        return fresh ? { ...n, data: fresh.data, type: fresh.type } : n
      }),
    )
    setEdges((prev) =>
      prev.map((e) => {
        const fresh = incomingEdges.find((x) => x.id === e.id)
        return fresh ? { ...e, animated: fresh.animated, label: fresh.label } : e
      }),
    )
  }, [incoming, incomingEdges, setNodes, setEdges])

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
        onNodesChange={onNodesChange}
        onEdgesChange={onEdgesChange}
        nodeTypes={nodeTypes}
        edgeTypes={edgeTypes}
        colorMode={getColorMode()}
        fitView
        fitViewOptions={{ padding: 0.2 }}
        // nodesDraggable defaults true. Selection stays off so the
        // runs-graph view remains ergonomically read-only (no
        // accidental delete on Backspace).
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
      </ReactFlow>
    </div>
  )
}
