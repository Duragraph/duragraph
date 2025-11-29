<script lang="ts">
	import '../app.css';
	import { Theme } from 'carbon-components-svelte';
	import { theme, type CarbonTheme } from '$lib/stores/theme';
	import { onMount } from 'svelte';

	let { children } = $props();
	let currentTheme = $state<CarbonTheme>('white');

	// Subscribe to theme store
	onMount(() => {
		const unsubscribe = theme.subscribe((state) => {
			currentTheme = state.theme;
		});
		return unsubscribe;
	});
</script>

<svelte:head>
	<title>DuraGraph - Agent Builder</title>
	<meta name="description" content="Build and manage AI agents with DuraGraph" />
</svelte:head>

<!-- Carbon Theme Component -->
<Theme bind:theme={currentTheme} persist persistKey="carbon-theme" />

{@render children?.()}
