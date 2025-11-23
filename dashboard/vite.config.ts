import { sveltekit } from '@sveltejs/kit/vite';
import { optimizeCss } from 'carbon-preprocess-svelte';
import { defineConfig } from 'vite';

export default defineConfig({
	plugins: [sveltekit(), optimizeCss()],
	server: {
		proxy: {
			'/api': {
				target: 'http://localhost:8081',
				changeOrigin: true
			}
		}
	}
});
