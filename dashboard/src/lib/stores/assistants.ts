// Assistants Store

import { writable } from 'svelte/store';
import { api } from '$lib/api/client';
import type { Assistant, CreateAssistantRequest, UpdateAssistantRequest } from '$lib/api/types';

interface AssistantsState {
	items: Assistant[];
	isLoading: boolean;
	error: string | null;
}

function createAssistantsStore() {
	const { subscribe, set, update } = writable<AssistantsState>({
		items: [],
		isLoading: false,
		error: null
	});

	return {
		subscribe,

		load: async () => {
			update((state) => ({ ...state, isLoading: true, error: null }));
			try {
				const items = await api.getAssistants();
				set({ items, isLoading: false, error: null });
			} catch (error: any) {
				update((state) => ({
					...state,
					isLoading: false,
					error: error.message || 'Failed to load assistants'
				}));
			}
		},

		fetchAll: async () => {
			update((state) => ({ ...state, isLoading: true, error: null }));
			try {
				const items = await api.getAssistants();
				set({ items, isLoading: false, error: null });
			} catch (error: any) {
				update((state) => ({
					...state,
					isLoading: false,
					error: error.message || 'Failed to load assistants'
				}));
			}
		},

		getById: async (id: string): Promise<Assistant | null> => {
			try {
				const assistant = await api.getAssistant(id);
				return assistant;
			} catch (error: any) {
				update((state) => ({
					...state,
					error: error.message || 'Failed to load assistant'
				}));
				return null;
			}
		},

		create: async (data: CreateAssistantRequest): Promise<Assistant | null> => {
			update((state) => ({ ...state, isLoading: true, error: null }));
			try {
				const created = await api.createAssistant(data);
				update((state) => ({
					items: [...state.items, created],
					isLoading: false,
					error: null
				}));
				return created;
			} catch (error: any) {
				update((state) => ({
					...state,
					isLoading: false,
					error: error.message || 'Failed to create assistant'
				}));
				return null;
			}
		},

		update: async (id: string, data: UpdateAssistantRequest): Promise<Assistant | null> => {
			update((state) => ({ ...state, isLoading: true, error: null }));
			try {
				const updated = await api.updateAssistant(id, data);
				update((state) => ({
					items: state.items.map((a) => (a.id === id ? updated : a)),
					isLoading: false,
					error: null
				}));
				return updated;
			} catch (error: any) {
				update((state) => ({
					...state,
					isLoading: false,
					error: error.message || 'Failed to update assistant'
				}));
				return null;
			}
		},

		delete: async (id: string): Promise<boolean> => {
			update((state) => ({ ...state, isLoading: true, error: null }));
			try {
				await api.deleteAssistant(id);
				update((state) => ({
					items: state.items.filter((a) => a.id !== id),
					isLoading: false,
					error: null
				}));
				return true;
			} catch (error: any) {
				update((state) => ({
					...state,
					isLoading: false,
					error: error.message || 'Failed to delete assistant'
				}));
				return false;
			}
		},

		clearError: () => {
			update((state) => ({ ...state, error: null }));
		}
	};
}

export const assistants = createAssistantsStore();
