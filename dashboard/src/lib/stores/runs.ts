// Runs Store

import { writable } from 'svelte/store';
import { api } from '$lib/api/client';
import type { Run, CreateRunRequest, SubmitToolOutputsRequest } from '$lib/api/types';

interface RunsState {
	items: Run[];
	currentRun: Run | null;
	isLoading: boolean;
	error: string | null;
}

function createRunsStore() {
	const { subscribe, set, update } = writable<RunsState>({
		items: [],
		currentRun: null,
		isLoading: false,
		error: null
	});

	return {
		subscribe,

		load: async (threadId?: string) => {
			update((state) => ({ ...state, isLoading: true, error: null }));
			try {
				const items = await api.getRuns(threadId);
				update((state) => ({ ...state, items, isLoading: false, error: null }));
			} catch (error: any) {
				update((state) => ({
					...state,
					isLoading: false,
					error: error.message || 'Failed to load runs'
				}));
			}
		},

		fetchAll: async () => {
			update((state) => ({ ...state, isLoading: true, error: null }));
			try {
				const items = await api.getRuns();
				update((state) => ({ ...state, items, isLoading: false, error: null }));
			} catch (error: any) {
				update((state) => ({
					...state,
					isLoading: false,
					error: error.message || 'Failed to load runs'
				}));
			}
		},

		getById: async (id: string): Promise<Run | null> => {
			try {
				const run = await api.getRun(id);
				return run;
			} catch (error: any) {
				update((state) => ({
					...state,
					error: error.message || 'Failed to load run'
				}));
				return null;
			}
		},

		loadRun: async (id: string) => {
			update((state) => ({ ...state, isLoading: true, error: null }));
			try {
				const run = await api.getRun(id);
				update((state) => ({ ...state, currentRun: run, isLoading: false, error: null }));
				return run;
			} catch (error: any) {
				update((state) => ({
					...state,
					isLoading: false,
					error: error.message || 'Failed to load run'
				}));
				return null;
			}
		},

		create: async (data: CreateRunRequest): Promise<Run | null> => {
			update((state) => ({ ...state, isLoading: true, error: null }));
			try {
				const created = await api.createRun(data);
				update((state) => ({
					items: [created, ...state.items],
					currentRun: created,
					isLoading: false,
					error: null
				}));
				return created;
			} catch (error: any) {
				update((state) => ({
					...state,
					isLoading: false,
					error: error.message || 'Failed to create run'
				}));
				return null;
			}
		},

		submitToolOutputs: async (runId: string, data: SubmitToolOutputsRequest): Promise<boolean> => {
			update((state) => ({ ...state, isLoading: true, error: null }));
			try {
				await api.submitToolOutputs(runId, data);
				// Reload the run to get updated status
				const updated = await api.getRun(runId);
				update((state) => ({
					items: state.items.map((r) => (r.id === runId ? updated : r)),
					currentRun: state.currentRun?.id === runId ? updated : state.currentRun,
					isLoading: false,
					error: null
				}));
				return true;
			} catch (error: any) {
				update((state) => ({
					...state,
					isLoading: false,
					error: error.message || 'Failed to submit tool outputs'
				}));
				return false;
			}
		},

		updateRunStatus: (runId: string, updatedRun: Run) => {
			update((state) => ({
				...state,
				items: state.items.map((r) => (r.id === runId ? updatedRun : r)),
				currentRun: state.currentRun?.id === runId ? updatedRun : state.currentRun
			}));
		},

		clearCurrentRun: () => {
			update((state) => ({ ...state, currentRun: null }));
		},

		clearError: () => {
			update((state) => ({ ...state, error: null }));
		}
	};
}

export const runs = createRunsStore();
