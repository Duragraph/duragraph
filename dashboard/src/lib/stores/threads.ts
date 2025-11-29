// Threads Store

import { writable } from 'svelte/store';
import { api } from '$lib/api/client';
import type { Thread, CreateThreadRequest, UpdateThreadRequest, AddMessageRequest } from '$lib/api/types';

interface ThreadsState {
	items: Thread[];
	currentThread: Thread | null;
	isLoading: boolean;
	error: string | null;
}

function createThreadsStore() {
	const { subscribe, set, update } = writable<ThreadsState>({
		items: [],
		currentThread: null,
		isLoading: false,
		error: null
	});

	return {
		subscribe,

		load: async () => {
			update((state) => ({ ...state, isLoading: true, error: null }));
			try {
				const items = await api.getThreads();
				update((state) => ({ ...state, items, isLoading: false, error: null }));
			} catch (error: any) {
				update((state) => ({
					...state,
					isLoading: false,
					error: error.message || 'Failed to load threads'
				}));
			}
		},

		fetchAll: async () => {
			update((state) => ({ ...state, isLoading: true, error: null }));
			try {
				const items = await api.getThreads();
				update((state) => ({ ...state, items, isLoading: false, error: null }));
			} catch (error: any) {
				update((state) => ({
					...state,
					isLoading: false,
					error: error.message || 'Failed to load threads'
				}));
			}
		},

		getById: async (id: string): Promise<Thread | null> => {
			try {
				const thread = await api.getThread(id);
				return thread;
			} catch (error: any) {
				update((state) => ({
					...state,
					error: error.message || 'Failed to load thread'
				}));
				return null;
			}
		},

		loadThread: async (id: string) => {
			update((state) => ({ ...state, isLoading: true, error: null }));
			try {
				const thread = await api.getThread(id);
				update((state) => ({ ...state, currentThread: thread, isLoading: false, error: null }));
			} catch (error: any) {
				update((state) => ({
					...state,
					isLoading: false,
					error: error.message || 'Failed to load thread'
				}));
			}
		},

		create: async (data?: CreateThreadRequest): Promise<Thread | null> => {
			update((state) => ({ ...state, isLoading: true, error: null }));
			try {
				const created = await api.createThread(data);
				update((state) => ({
					items: [...state.items, created],
					currentThread: created,
					isLoading: false,
					error: null
				}));
				return created;
			} catch (error: any) {
				update((state) => ({
					...state,
					isLoading: false,
					error: error.message || 'Failed to create thread'
				}));
				return null;
			}
		},

		updateThread: async (id: string, data: UpdateThreadRequest): Promise<Thread | null> => {
			update((state) => ({ ...state, isLoading: true, error: null }));
			try {
				const updated = await api.updateThread(id, data);
				update((state) => ({
					items: state.items.map((t) => (t.id === id ? updated : t)),
					currentThread: state.currentThread?.id === id ? updated : state.currentThread,
					isLoading: false,
					error: null
				}));
				return updated;
			} catch (error: any) {
				update((state) => ({
					...state,
					isLoading: false,
					error: error.message || 'Failed to update thread'
				}));
				return null;
			}
		},

		addMessage: async (threadId: string, data: AddMessageRequest): Promise<Thread | null> => {
			update((state) => ({ ...state, isLoading: true, error: null }));
			try {
				const updated = await api.addMessage(threadId, data);
				update((state) => ({
					items: state.items.map((t) => (t.id === threadId ? updated : t)),
					currentThread: state.currentThread?.id === threadId ? updated : state.currentThread,
					isLoading: false,
					error: null
				}));
				return updated;
			} catch (error: any) {
				update((state) => ({
					...state,
					isLoading: false,
					error: error.message || 'Failed to add message'
				}));
				return null;
			}
		},

		delete: async (id: string): Promise<void> => {
			update((state) => ({ ...state, isLoading: true, error: null }));
			try {
				await api.deleteThread(id);
				update((state) => ({
					items: state.items.filter((t) => t.id !== id),
					currentThread: state.currentThread?.id === id ? null : state.currentThread,
					isLoading: false,
					error: null
				}));
			} catch (error: any) {
				update((state) => ({
					...state,
					isLoading: false,
					error: error.message || 'Failed to delete thread'
				}));
			}
		},

		clearCurrentThread: () => {
			update((state) => ({ ...state, currentThread: null }));
		},

		clearError: () => {
			update((state) => ({ ...state, error: null }));
		}
	};
}

export const threads = createThreadsStore();
