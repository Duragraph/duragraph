<script lang="ts">
	import {
		Form,
		FormGroup,
		TextInput,
		PasswordInput,
		Button,
		InlineNotification
	} from 'carbon-components-svelte';
	import { auth } from '$lib/stores';
	import { goto } from '$app/navigation';
	import { onMount } from 'svelte';

	let email = $state('');
	let password = $state('');
	let isLoading = $state(false);
	let error = $state<string | null>(null);

	onMount(() => {
		// Check if already authenticated
		const unsubscribe = auth.subscribe((state) => {
			if (state.isAuthenticated) {
				goto('/');
			}
		});
		return unsubscribe;
	});

	async function handleLogin(e: Event) {
		e.preventDefault();
		isLoading = true;
		error = null;

		const success = await auth.login(email, password);

		if (success) {
			goto('/');
		} else {
			const unsubscribe = auth.subscribe((state) => {
				error = state.error;
				isLoading = state.isLoading;
			});
			unsubscribe();
		}
	}
</script>

<div class="login-container">
	<div class="login-box">
		<div class="login-header">
			<h1>DuraGraph</h1>
			<p>Agent Builder Dashboard</p>
		</div>

		{#if error}
			<InlineNotification
				kind="error"
				title="Login Failed"
				subtitle={error}
				on:close={() => {
					error = null;
				}}
			/>
		{/if}

		<Form on:submit={handleLogin}>
			<FormGroup legendText="">
				<TextInput
					labelText="Email"
					placeholder="Enter your email"
					bind:value={email}
					required
					disabled={isLoading}
				/>
			</FormGroup>

			<FormGroup legendText="">
				<PasswordInput
					labelText="Password"
					placeholder="Enter your password"
					bind:value={password}
					required
					disabled={isLoading}
				/>
			</FormGroup>

			<Button type="submit" disabled={isLoading} style="width: 100%; margin-top: 1rem;">
				{isLoading ? 'Logging in...' : 'Login'}
			</Button>
		</Form>

		<div class="login-footer">
			<p>
				For development, you can use any email/password combination. Authentication is currently
				mocked.
			</p>
		</div>
	</div>
</div>

<style>
	.login-container {
		display: flex;
		justify-content: center;
		align-items: center;
		min-height: 100vh;
		background: var(--cds-background);
		padding: 2rem;
	}

	.login-box {
		background: var(--cds-layer-01);
		padding: 3rem;
		border-radius: 8px;
		box-shadow: 0 2px 8px rgba(0, 0, 0, 0.1);
		max-width: 450px;
		width: 100%;
	}

	.login-header {
		text-align: center;
		margin-bottom: 2rem;
	}

	.login-header h1 {
		font-size: 2.5rem;
		font-weight: 600;
		margin-bottom: 0.5rem;
		color: var(--cds-text-primary);
	}

	.login-header p {
		font-size: 1rem;
		color: var(--cds-text-secondary);
	}

	.login-footer {
		margin-top: 2rem;
		padding-top: 1rem;
		border-top: 1px solid var(--cds-border-subtle);
	}

	.login-footer p {
		font-size: 0.875rem;
		color: var(--cds-text-secondary);
		text-align: center;
	}
</style>
