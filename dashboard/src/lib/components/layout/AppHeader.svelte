<script lang="ts">
	import {
		Header,
		HeaderNav,
		HeaderNavItem,
		HeaderGlobalAction,
		HeaderUtilities
	} from 'carbon-components-svelte';
	import { Asleep, Light, Logout } from 'carbon-icons-svelte';
	import { theme, auth } from '$lib/stores';
	import { goto } from '$app/navigation';

	let { isSideNavOpen = $bindable(false) } = $props();
	let isDark = $state(false);

	// Subscribe to theme for toggle button
	theme.subscribe((state) => {
		isDark = state.isDark;
	});

	function toggleTheme() {
		theme.toggleTheme();
	}

	function handleLogout() {
		auth.logout();
		goto('/login');
	}
</script>

<Header company="DuraGraph" platformName="Agent Builder" bind:isSideNavOpen>
	<div slot="skip-to-content"></div>

	<HeaderNav>
		<HeaderNavItem href="/" text="Home" />
		<HeaderNavItem href="/assistants" text="Assistants" />
		<HeaderNavItem href="/threads" text="Threads" />
		<HeaderNavItem href="/runs" text="Runs" />
		<HeaderNavItem href="/builder" text="Builder" />
	</HeaderNav>

	<HeaderUtilities>
		<HeaderGlobalAction
			aria-label="Toggle theme"
			icon={isDark ? Asleep : Light}
			on:click={toggleTheme}
		/>
		<HeaderGlobalAction aria-label="Logout" icon={Logout} on:click={handleLogout} />
	</HeaderUtilities>
</Header>
