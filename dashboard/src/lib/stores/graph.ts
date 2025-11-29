// Graph Store for SvelteFlow

import { writable } from 'svelte/store';
import type { Node, Edge } from '@xyflow/svelte';
import type { Graph, SaveGraphRequest } from '$lib/api/types';
import { api } from '$lib/api/client';

interface GraphHistory {
	nodes: Node[];
	edges: Edge[];
}

interface GraphState {
	nodes: Node[];
	edges: Edge[];
	selectedNodes: string[];
	clipboard: { nodes: Node[]; edges: Edge[] } | null;
	history: GraphHistory[];
	historyIndex: number;
	isEditing: boolean;
	isSaving: boolean;
	lastSaved: Date | null;
	error: string | null;
}

function createGraphStore() {
	const { subscribe, set, update } = writable<GraphState>({
		nodes: [],
		edges: [],
		selectedNodes: [],
		clipboard: null,
		history: [],
		historyIndex: -1,
		isEditing: false,
		isSaving: false,
		lastSaved: null,
		error: null
	});

	let autoSaveTimeout: ReturnType<typeof setTimeout> | null = null;

	return {
		subscribe,

		setNodes: (nodes: Node[]) => {
			update((state) => ({ ...state, nodes }));
		},

		setEdges: (edges: Edge[]) => {
			update((state) => ({ ...state, edges }));
		},

		addNode: (node: Node) => {
			update((state) => {
				const newState = { ...state, nodes: [...state.nodes, node] };
				return newState;
			});
		},

		updateNode: (id: string, data: Record<string, any>) => {
			update((state) => ({
				...state,
				nodes: state.nodes.map((n) => (n.id === id ? { ...n, data: { ...n.data, ...data } } : n))
			}));
		},

		deleteNode: (id: string) => {
			update((state) => ({
				...state,
				nodes: state.nodes.filter((n) => n.id !== id),
				edges: state.edges.filter((e) => e.source !== id && e.target !== id)
			}));
		},

		deleteSelectedNodes: () => {
			update((state) => {
				const selectedIds = new Set(state.selectedNodes);
				return {
					...state,
					nodes: state.nodes.filter((n) => !selectedIds.has(n.id)),
					edges: state.edges.filter((e) => !selectedIds.has(e.source) && !selectedIds.has(e.target)),
					selectedNodes: []
				};
			});
		},

		setSelectedNodes: (nodeIds: string[]) => {
			update((state) => ({ ...state, selectedNodes: nodeIds }));
		},

		copySelected: () => {
			update((state) => {
				const selectedIds = new Set(state.selectedNodes);
				const selectedNodes = state.nodes.filter((n) => selectedIds.has(n.id));
				const selectedEdges = state.edges.filter(
					(e) => selectedIds.has(e.source) && selectedIds.has(e.target)
				);
				return {
					...state,
					clipboard: { nodes: selectedNodes, edges: selectedEdges }
				};
			});
		},

		paste: (offset = { x: 50, y: 50 }) => {
			update((state) => {
				if (!state.clipboard) return state;

				const idMap = new Map<string, string>();
				const newNodes = state.clipboard.nodes.map((node) => {
					const newId = `${node.id}_copy_${Date.now()}`;
					idMap.set(node.id, newId);
					return {
						...node,
						id: newId,
						position: {
							x: node.position.x + offset.x,
							y: node.position.y + offset.y
						},
						selected: false
					};
				});

				const newEdges = state.clipboard.edges.map((edge) => ({
					...edge,
					id: `${edge.id}_copy_${Date.now()}`,
					source: idMap.get(edge.source) || edge.source,
					target: idMap.get(edge.target) || edge.target
				}));

				return {
					...state,
					nodes: [...state.nodes, ...newNodes],
					edges: [...state.edges, ...newEdges],
					selectedNodes: newNodes.map((n) => n.id)
				};
			});
		},

		addToHistory: () => {
			update((state) => {
				const history = state.history.slice(0, state.historyIndex + 1);
				history.push({ nodes: state.nodes, edges: state.edges });
				return {
					...state,
					history,
					historyIndex: history.length - 1
				};
			});
		},

		undo: () => {
			update((state) => {
				if (state.historyIndex <= 0) return state;
				const newIndex = state.historyIndex - 1;
				const snapshot = state.history[newIndex];
				return {
					...state,
					nodes: snapshot.nodes,
					edges: snapshot.edges,
					historyIndex: newIndex
				};
			});
		},

		redo: () => {
			update((state) => {
				if (state.historyIndex >= state.history.length - 1) return state;
				const newIndex = state.historyIndex + 1;
				const snapshot = state.history[newIndex];
				return {
					...state,
					nodes: snapshot.nodes,
					edges: snapshot.edges,
					historyIndex: newIndex
				};
			});
		},

		clear: () => {
			update((state) => ({
				...state,
				nodes: [],
				edges: [],
				selectedNodes: [],
				history: [],
				historyIndex: -1
			}));
		},

		loadGraph: (graph: Graph) => {
			const nodes: Node[] = graph.nodes.map((n) => ({
				id: n.id,
				type: n.type,
				position: n.position || { x: 0, y: 0 },
				data: n.config || {}
			}));

			const edges: Edge[] = graph.edges.map((e) => ({
				id: e.id,
				source: e.source,
				target: e.target,
				sourceHandle: e.sourceHandle,
				targetHandle: e.targetHandle
			}));

			update((state) => ({
				...state,
				nodes,
				edges,
				history: [{ nodes, edges }],
				historyIndex: 0
			}));
		},

		saveGraph: async (assistantId: string, name: string, description?: string): Promise<Graph | null> => {
			update((state) => ({ ...state, isSaving: true, error: null }));

			try {
				let state: GraphState = {} as GraphState;
				const unsubscribe = subscribe((s) => (state = s));
				unsubscribe();

				const request: SaveGraphRequest = {
					assistant_id: assistantId,
					name,
					description: description || '',
					nodes: state.nodes.map((n) => ({
						id: n.id,
						type: n.type as any,
						config: n.data,
						position: n.position
					})),
					edges: state.edges.map((e) => ({
						id: e.id,
						source: e.source,
						target: e.target,
						sourceHandle: e.sourceHandle || undefined,
						targetHandle: e.targetHandle || undefined
					}))
				};

				const saved = await api.saveGraph(request);

				update((s) => ({
					...s,
					isSaving: false,
					lastSaved: new Date(),
					error: null
				}));

				return saved;
			} catch (error: any) {
				update((s) => ({
					...s,
					isSaving: false,
					error: error.message || 'Failed to save graph'
				}));
				return null;
			}
		},

		scheduleAutoSave: (assistantId: string, name: string) => {
			if (autoSaveTimeout) {
				clearTimeout(autoSaveTimeout);
			}

			autoSaveTimeout = setTimeout(() => {
				// Auto-save logic
				let state: GraphState = {} as GraphState;
				const unsubscribe = subscribe((s) => (state = s));
				unsubscribe();

				if (state.nodes.length > 0) {
					// Silent save in background
					api.saveGraph({
						assistant_id: assistantId,
						name,
						nodes: state.nodes.map((n) => ({
							id: n.id,
							type: n.type as any,
							config: n.data,
							position: n.position
						})),
						edges: state.edges.map((e) => ({
							id: e.id,
							source: e.source,
							target: e.target
						}))
					}).catch(console.error);
				}
			}, 3000); // 3 second debounce
		},

		clearError: () => {
			update((state) => ({ ...state, error: null }));
		}
	};
}

export const graph = createGraphStore();
