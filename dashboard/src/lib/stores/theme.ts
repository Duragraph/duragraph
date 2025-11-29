// Theme Store for Carbon Design System

import { writable } from 'svelte/store';

export type CarbonTheme = 'white' | 'g10' | 'g80' | 'g90' | 'g100';

interface ThemeState {
	theme: CarbonTheme;
	isDark: boolean;
}

function createThemeStore() {
	// Load theme from localStorage or detect system preference
	const getInitialTheme = (): CarbonTheme => {
		if (typeof window === 'undefined') return 'white';

		const stored = localStorage.getItem('carbon-theme') as CarbonTheme;
		if (stored) return stored;

		// Check system preference
		const prefersDark = window.matchMedia('(prefers-color-scheme: dark)').matches;
		return prefersDark ? 'g100' : 'white';
	};

	const initialTheme = getInitialTheme();
	const isDarkTheme = (theme: CarbonTheme) => ['g80', 'g90', 'g100'].includes(theme);

	const { subscribe, set, update } = writable<ThemeState>({
		theme: initialTheme,
		isDark: isDarkTheme(initialTheme)
	});

	return {
		subscribe,
		setTheme: (theme: CarbonTheme) => {
			update((state) => {
				const newState = {
					theme,
					isDark: isDarkTheme(theme)
				};

				// Persist to localStorage
				if (typeof window !== 'undefined') {
					localStorage.setItem('carbon-theme', theme);
					// Set theme attribute on document
					document.documentElement.setAttribute('theme', theme);
				}

				return newState;
			});
		},
		toggleTheme: () => {
			update((state) => {
				const newTheme: CarbonTheme = state.isDark ? 'white' : 'g100';
				const newState: ThemeState = {
					theme: newTheme,
					isDark: isDarkTheme(newTheme)
				};

				// Persist to localStorage
				if (typeof window !== 'undefined') {
					localStorage.setItem('carbon-theme', newTheme);
					document.documentElement.setAttribute('theme', newTheme);
				}

				return newState;
			});
		}
	};
}

export const theme = createThemeStore();
