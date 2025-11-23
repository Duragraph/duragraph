// Authentication Store

import { writable } from 'svelte/store';
import { api } from '$lib/api/client';

interface User {
	id: string;
	email: string;
	name?: string;
}

interface AuthState {
	user: User | null;
	token: string | null;
	isAuthenticated: boolean;
	isLoading: boolean;
	error: string | null;
}

function createAuthStore() {
	const { subscribe, set, update } = writable<AuthState>({
		user: null,
		token: null,
		isAuthenticated: false,
		isLoading: false,
		error: null
	});

	// Initialize - check if token exists in localStorage
	if (typeof window !== 'undefined') {
		const token = localStorage.getItem('auth_token');
		if (token) {
			api.setAuthToken(token);
			// TODO: Validate token with backend
			// For now, just set as authenticated
			update((state) => ({
				...state,
				token,
				isAuthenticated: true
			}));
		}
	}

	return {
		subscribe,

		login: async (email: string, password: string) => {
			update((state) => ({ ...state, isLoading: true, error: null }));

			try {
				// TODO: Replace with actual login endpoint when backend implements it
				// For now, mock login for development
				const mockToken = 'mock-jwt-token';
				const mockUser = { id: '1', email, name: email.split('@')[0] };

				api.setAuthToken(mockToken);

				update((state) => ({
					...state,
					user: mockUser,
					token: mockToken,
					isAuthenticated: true,
					isLoading: false,
					error: null
				}));

				return true;
			} catch (error: any) {
				update((state) => ({
					...state,
					isLoading: false,
					error: error.message || 'Login failed'
				}));
				return false;
			}
		},

		logout: () => {
			api.setAuthToken(null);
			set({
				user: null,
				token: null,
				isAuthenticated: false,
				isLoading: false,
				error: null
			});
		},

		clearError: () => {
			update((state) => ({ ...state, error: null }));
		}
	};
}

export const auth = createAuthStore();
