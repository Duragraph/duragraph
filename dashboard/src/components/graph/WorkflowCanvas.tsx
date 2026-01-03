import { useCallback, useRef } from "react"
import {
  ReactFlow,
  Background,
  Controls,
  MiniMap,
  Panel,
  type ReactFlowInstance,
} from "@xyflow/react"
import "@xyflow/react/dist/style.css"
import {
  useWorkflowStore,
  type WorkflowNode,
  type WorkflowNodeType,
  type WorkflowEdge,
} from "@/stores/workflow"
import { nodeTypes, edgeTypes } from "./index"
import { NodePalette } from "./NodePalette"
import { useThemeStore } from "@/stores/theme"

let nodeId = 0
const getNodeId = () => `node_${nodeId++}`

export function WorkflowCanvas() {
  const reactFlowWrapper = useRef<HTMLDivElement>(null)
  const reactFlowInstance = useRef<ReactFlowInstance<
    WorkflowNode,
    WorkflowEdge
  > | null>(null)
  const theme = useThemeStore((state) => state.theme)

  const {
    nodes,
    edges,
    onNodesChange,
    onEdgesChange,
    onConnect,
    addNode,
    setSelectedNode,
  } = useWorkflowStore()

  const onInit = useCallback(
    (instance: ReactFlowInstance<WorkflowNode, WorkflowEdge>) => {
      reactFlowInstance.current = instance
    },
    []
  )

  const onSelectionChange = useCallback(
    ({ nodes: selectedNodes }: { nodes: WorkflowNode[] }) => {
      if (selectedNodes.length === 1) {
        setSelectedNode(selectedNodes[0].id)
      } else {
        setSelectedNode(null)
      }
    },
    [setSelectedNode]
  )

  const onDragOver = useCallback((event: React.DragEvent) => {
    event.preventDefault()
    event.dataTransfer.dropEffect = "move"
  }, [])

  const onDrop = useCallback(
    (event: React.DragEvent) => {
      event.preventDefault()

      const type = event.dataTransfer.getData(
        "application/reactflow"
      ) as WorkflowNodeType
      if (!type || !reactFlowInstance.current || !reactFlowWrapper.current) {
        return
      }

      const bounds = reactFlowWrapper.current.getBoundingClientRect()
      const position = reactFlowInstance.current.screenToFlowPosition({
        x: event.clientX - bounds.left,
        y: event.clientY - bounds.top,
      })

      const newNode: WorkflowNode = {
        id: getNodeId(),
        type,
        position,
        data: {
          label: type.charAt(0).toUpperCase() + type.slice(1),
          type,
        },
      }

      addNode(newNode)
    },
    [addNode]
  )

  const getColorMode = () => {
    if (theme === "system") {
      return typeof window !== "undefined" &&
        window.matchMedia("(prefers-color-scheme: dark)").matches
        ? "dark"
        : "light"
    }
    return theme
  }

  return (
    <div ref={reactFlowWrapper} className="h-full w-full">
      <ReactFlow
        nodes={nodes}
        edges={edges}
        onNodesChange={onNodesChange}
        onEdgesChange={onEdgesChange}
        onConnect={onConnect}
        onInit={onInit}
        onSelectionChange={onSelectionChange}
        onDragOver={onDragOver}
        onDrop={onDrop}
        nodeTypes={nodeTypes}
        edgeTypes={edgeTypes}
        defaultEdgeOptions={{
          type: "workflow",
        }}
        colorMode={getColorMode()}
        snapToGrid
        snapGrid={[16, 16]}
        fitView
        deleteKeyCode={["Backspace", "Delete"]}
        multiSelectionKeyCode={["Shift"]}
      >
        <Background gap={16} size={1} />
        <Controls />
        <MiniMap
          nodeStrokeWidth={3}
          zoomable
          pannable
          className="!bg-background !border-border"
        />
        <Panel position="top-left" className="!m-0">
          <NodePalette />
        </Panel>
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
