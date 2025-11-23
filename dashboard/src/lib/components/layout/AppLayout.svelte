<script lang="ts">
	import { Content } from 'carbon-components-svelte';
	import AppHeader from './AppHeader.svelte';
	import AppSidebar from './AppSidebar.svelte';

	let { children } = $props();
	let isSideNavOpen = $state(false);
</script>

<AppHeader bind:isSideNavOpen />
<AppSidebar bind:isOpen={isSideNavOpen} />

<Content>
	{@render children?.()}
</Content>

<style>
	:global(.bx--content) {
		padding: 2rem;
		background: var(--cds-background);
		min-height: calc(100vh - 48px);
	}
</style>
