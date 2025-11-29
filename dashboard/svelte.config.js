import { mdsvex } from 'mdsvex';
import adapter from '@sveltejs/adapter-static';
import { vitePreprocess } from '@sveltejs/vite-plugin-svelte';
import { optimizeImports } from 'carbon-preprocess-svelte';

/** @type {import('@sveltejs/kit').Config} */
const config = {
	// Consult https://svelte.dev/docs/kit/integrations
	// for more information about preprocessors
	// IMPORTANT: vitePreprocess must come BEFORE optimizeImports
	preprocess: [vitePreprocess(), optimizeImports(), mdsvex()],
	kit: {
		adapter: adapter({
			strict: false
		}),
		alias: {
			'@/*': './src/lib/*'
		}
	},
	extensions: ['.svelte', '.svx']
};

export default config;
