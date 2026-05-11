import { create } from 'zustand'
import type { EditorNode, EditorEdge, EditorNodeType, GraphDefinitionLocal as GraphDefinition } from '@/types/entities'

let nextNodeId = 1
let nextEdgeId = 1

interface EditorState {
  nodes: EditorNode[]
  edges: EditorEdge[]
  selectedNodeId: string | null
  selectedEdgeId: string | null
  graphName: string
  graphDescription: string
  connectingFrom: string | null
  isDirty: boolean

  addNode: (type: EditorNodeType, x: number, y: number) => void
  removeNode: (id: string) => void
  updateNode: (id: string, updates: Partial<EditorNode>) => void
  moveNode: (id: string, x: number, y: number) => void
  selectNode: (id: string | null) => void
  selectEdge: (id: string | null) => void
  setConnectingFrom: (id: string | null) => void
  addEdge: (source: string, target: string, label?: string) => void
  removeEdge: (id: string) => void
  setGraphMeta: (name: string, description: string) => void
  setEntrypoint: (id: string) => void
  loadGraph: (def: GraphDefinition) => void
  toDefinition: () => GraphDefinition
  clear: () => void
}

export const useEditorStore = create<EditorState>((set, get) => ({
  nodes: [],
  edges: [],
  selectedNodeId: null,
  selectedEdgeId: null,
  graphName: 'Untitled Graph',
  graphDescription: '',
  connectingFrom: null,
  isDirty: false,

  addNode: (type, x, y) => {
    const id = `node_${nextNodeId++}`
    const label = `${type}_${id}`
    const node: EditorNode = { id, type, label, x, y, config: {}, isEntrypoint: get().nodes.length === 0 }
    set((s) => ({ nodes: [...s.nodes, node], isDirty: true, selectedNodeId: id }))
  },

  removeNode: (id) =>
    set((s) => ({
      nodes: s.nodes.filter((n) => n.id !== id),
      edges: s.edges.filter((e) => e.source !== id && e.target !== id),
      selectedNodeId: s.selectedNodeId === id ? null : s.selectedNodeId,
      isDirty: true,
    })),

  updateNode: (id, updates) =>
    set((s) => ({
      nodes: s.nodes.map((n) => (n.id === id ? { ...n, ...updates } : n)),
      isDirty: true,
    })),

  moveNode: (id, x, y) =>
    set((s) => ({
      nodes: s.nodes.map((n) => (n.id === id ? { ...n, x, y } : n)),
      isDirty: true,
    })),

  selectNode: (id) => set({ selectedNodeId: id, selectedEdgeId: null }),
  selectEdge: (id) => set({ selectedEdgeId: id, selectedNodeId: null }),
  setConnectingFrom: (id) => set({ connectingFrom: id }),

  addEdge: (source, target, label) => {
    if (source === target) return
    const exists = get().edges.some((e) => e.source === source && e.target === target)
    if (exists) return
    const id = `edge_${nextEdgeId++}`
    set((s) => ({ edges: [...s.edges, { id, source, target, label }], isDirty: true }))
  },

  removeEdge: (id) =>
    set((s) => ({
      edges: s.edges.filter((e) => e.id !== id),
      selectedEdgeId: s.selectedEdgeId === id ? null : s.selectedEdgeId,
      isDirty: true,
    })),

  setGraphMeta: (name, description) => set({ graphName: name, graphDescription: description, isDirty: true }),

  setEntrypoint: (id) =>
    set((s) => ({
      nodes: s.nodes.map((n) => ({ ...n, isEntrypoint: n.id === id })),
      isDirty: true,
    })),

  loadGraph: (def) => {
    nextNodeId = def.nodes.length + 1
    nextEdgeId = def.edges.length + 1
    set({
      nodes: def.nodes,
      edges: def.edges,
      graphName: def.name,
      graphDescription: def.description,
      isDirty: false,
      selectedNodeId: null,
      selectedEdgeId: null,
    })
  },

  toDefinition: () => {
    const s = get()
    return {
      id: s.graphName.toLowerCase().replace(/\s+/g, '_'),
      name: s.graphName,
      description: s.graphDescription,
      nodes: s.nodes,
      edges: s.edges,
    }
  },

  clear: () =>
    set({
      nodes: [],
      edges: [],
      selectedNodeId: null,
      selectedEdgeId: null,
      graphName: 'Untitled Graph',
      graphDescription: '',
      connectingFrom: null,
      isDirty: false,
    }),
}))
