<script lang="ts">
	import { SideNav, SideNavItems, SideNavLink, SideNavDivider } from 'carbon-components-svelte';
	import { page } from '$app/stores';

	let { isOpen = $bindable(false) } = $props();
	let currentPath = $state('');

	// Subscribe to page store
	page.subscribe((p) => {
		currentPath = p.url.pathname;
	});
</script>

<SideNav bind:isOpen>
	<SideNavItems>
		<SideNavLink text="Dashboard" href="/" isSelected={currentPath === '/'} />

		<SideNavDivider />

		<SideNavLink
			text="Assistants"
			href="/assistants"
			isSelected={currentPath.startsWith('/assistants')}
		/>
		<SideNavLink text="Threads" href="/threads" isSelected={currentPath.startsWith('/threads')} />
		<SideNavLink text="Runs" href="/runs" isSelected={currentPath.startsWith('/runs')} />

		<SideNavDivider />

		<SideNavLink
			text="Graph Builder"
			href="/builder"
			isSelected={currentPath.startsWith('/builder')}
		/>
		<SideNavLink
			text="Legacy Graph"
			href="/graph"
			isSelected={currentPath.startsWith('/graph')}
		/>
	</SideNavItems>
</SideNav>
