import { create } from "zustand"
import {
  type Node,
  type Edge,
  type OnNodesChange,
  type OnEdgesChange,
  type OnConnect,
  applyNodeChanges,
  applyEdgeChanges,
  addEdge,
} from "@xyflow/react"
import type { NodeStatus } from "@/components/node-status-indicator"

export type WorkflowNodeType =
  | "start"
  | "end"
  | "llm"
  | "tool"
  | "conditional"
  | "human"

export interface WorkflowNodeData extends Record<string, unknown> {
  label: string
  type: WorkflowNodeType
  config?: Record<string, unknown>
  status?: NodeStatus
  output?: unknown
  error?: string
}

export type WorkflowNode = Node<WorkflowNodeData, WorkflowNodeType>
export type WorkflowEdge = Edge

interface WorkflowState {
  nodes: WorkflowNode[]
  edges: WorkflowEdge[]
  selectedNodeId: string | null
  isRunning: boolean
}

interface WorkflowActions {
  setNodes: (nodes: WorkflowNode[]) => void
  setEdges: (edges: WorkflowEdge[]) => void
  onNodesChange: OnNodesChange<WorkflowNode>
  onEdgesChange: OnEdgesChange<WorkflowEdge>
  onConnect: OnConnect
  setSelectedNode: (nodeId: string | null) => void
  updateNodeStatus: (nodeId: string, status: NodeStatus) => void
  updateNodeData: (
    nodeId: string,
    data: Partial<WorkflowNodeData>
  ) => void
  addNode: (node: WorkflowNode) => void
  deleteNode: (nodeId: string) => void
  deleteEdge: (edgeId: string) => void
  reset: () => void
  setRunning: (isRunning: boolean) => void
}

const initialState: WorkflowState = {
  nodes: [],
  edges: [],
  selectedNodeId: null,
  isRunning: false,
}

export const useWorkflowStore = create<WorkflowState & WorkflowActions>(
  (set, get) => ({
    ...initialState,

    setNodes: (nodes) => set({ nodes }),
    setEdges: (edges) => set({ edges }),

    onNodesChange: (changes) => {
      set({
        nodes: applyNodeChanges(changes, get().nodes),
      })
    },

    onEdgesChange: (changes) => {
      set({
        edges: applyEdgeChanges(changes, get().edges),
      })
    },

    onConnect: (connection) => {
      set({
        edges: addEdge(
          {
            ...connection,
            type: "workflow",
          },
          get().edges
        ),
      })
    },

    setSelectedNode: (nodeId) => set({ selectedNodeId: nodeId }),

    updateNodeStatus: (nodeId, status) => {
      set({
        nodes: get().nodes.map((node) =>
          node.id === nodeId
            ? { ...node, data: { ...node.data, status } }
            : node
        ),
      })
    },

    updateNodeData: (nodeId, data) => {
      set({
        nodes: get().nodes.map((node) =>
          node.id === nodeId
            ? { ...node, data: { ...node.data, ...data } }
            : node
        ),
      })
    },

    addNode: (node) => {
      set({
        nodes: [...get().nodes, node],
      })
    },

    deleteNode: (nodeId) => {
      set({
        nodes: get().nodes.filter((n) => n.id !== nodeId),
        edges: get().edges.filter(
          (e) => e.source !== nodeId && e.target !== nodeId
        ),
        selectedNodeId:
          get().selectedNodeId === nodeId ? null : get().selectedNodeId,
      })
    },

    deleteEdge: (edgeId) => {
      set({
        edges: get().edges.filter((e) => e.id !== edgeId),
      })
    },

    reset: () => set(initialState),

    setRunning: (isRunning) => set({ isRunning }),
  })
)
