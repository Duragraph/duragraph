<script lang="ts">
	import { Tile, Grid, Row, Column, Button, ClickableTile } from 'carbon-components-svelte';
	import AppLayout from '$lib/components/layout/AppLayout.svelte';
	import { goto } from '$app/navigation';
	import { onMount } from 'svelte';
	import { assistants, threads, runs } from '$lib/stores';

	let stats = $state({
		assistantsCount: 0,
		threadsCount: 0,
		runsCount: 0,
		activeRuns: 0
	});

	onMount(async () => {
		// Load data for stats
		await Promise.all([assistants.load(), threads.load(), runs.load()]);

		// Update stats from stores
		assistants.subscribe((state) => {
			stats.assistantsCount = state.items.length;
		});

		threads.subscribe((state) => {
			stats.threadsCount = state.items.length;
		});

		runs.subscribe((state) => {
			stats.runsCount = state.items.length;
			stats.activeRuns = state.items.filter((r) =>
				['queued', 'in_progress', 'requires_action'].includes(r.status)
			).length;
		});
	});
</script>

<AppLayout>
	<div class="page-container">
		<div class="page-header">
			<div>
				<h1>Dashboard</h1>
				<p>Welcome to DuraGraph Agent Builder</p>
			</div>
			<Button on:click={() => goto('/builder')}>Build New Agent</Button>
		</div>

		<!-- Stats Grid -->
		<Grid padding>
			<Row>
				<Column sm={4} md={4} lg={4}>
					<ClickableTile on:click={() => goto('/assistants')}>
						<div class="stat-tile">
							<p class="stat-label">Total Assistants</p>
							<p class="stat-value">{stats.assistantsCount}</p>
						</div>
					</ClickableTile>
				</Column>
				<Column sm={4} md={4} lg={4}>
					<ClickableTile on:click={() => goto('/threads')}>
						<div class="stat-tile">
							<p class="stat-label">Total Threads</p>
							<p class="stat-value">{stats.threadsCount}</p>
						</div>
					</ClickableTile>
				</Column>
				<Column sm={4} md={4} lg={4}>
					<ClickableTile on:click={() => goto('/runs')}>
						<div class="stat-tile">
							<p class="stat-label">Total Runs</p>
							<p class="stat-value">{stats.runsCount}</p>
						</div>
					</ClickableTile>
				</Column>
				<Column sm={4} md={4} lg={4}>
					<Tile>
						<div class="stat-tile">
							<p class="stat-label">Active Runs</p>
							<p class="stat-value active">{stats.activeRuns}</p>
						</div>
					</Tile>
				</Column>
			</Row>
		</Grid>

		<!-- Quick Actions -->
		<div class="section">
			<h2>Quick Actions</h2>
			<Grid padding>
				<Row>
					<Column sm={4} md={4} lg={4}>
						<ClickableTile on:click={() => goto('/assistants')}>
							<div class="action-tile">
								<h3>Create Assistant</h3>
								<p>Set up a new AI assistant with custom instructions and tools</p>
							</div>
						</ClickableTile>
					</Column>
					<Column sm={4} md={4} lg={4}>
						<ClickableTile on:click={() => goto('/threads')}>
							<div class="action-tile">
								<h3>New Thread</h3>
								<p>Start a new conversation thread for agent interactions</p>
							</div>
						</ClickableTile>
					</Column>
					<Column sm={4} md={4} lg={4}>
						<ClickableTile on:click={() => goto('/builder')}>
							<div class="action-tile">
								<h3>Build Workflow</h3>
								<p>Design complex agent workflows with the visual graph builder</p>
							</div>
						</ClickableTile>
					</Column>
				</Row>
			</Grid>
		</div>

		<!-- Getting Started -->
		<div class="section">
			<Tile>
				<h2>Getting Started</h2>
				<p style="margin-bottom: 1rem;">
					Welcome to DuraGraph! Here's how to get started with building AI agents:
				</p>
				<ol style="margin-left: 1.5rem;">
					<li>Create an <strong>Assistant</strong> with your desired AI model and instructions</li>
					<li>
						Design a <strong>Workflow</strong> using the visual graph builder to define agent behavior
					</li>
					<li>Create a <strong>Thread</strong> to start conversations</li>
					<li>Run your agent and monitor execution in real-time</li>
				</ol>
			</Tile>
		</div>
	</div>
</AppLayout>

<style>
	.page-header {
		margin-bottom: 2rem;
		display: flex;
		justify-content: space-between;
		align-items: flex-start;
	}

	.page-header h1 {
		font-size: 2rem;
		margin-bottom: 0.5rem;
	}

	.page-header p {
		color: var(--cds-text-secondary);
	}

	.stat-tile {
		padding: 1rem;
	}

	.stat-label {
		font-size: 0.875rem;
		color: var(--cds-text-secondary);
		margin-bottom: 0.5rem;
	}

	.stat-value {
		font-size: 2.5rem;
		font-weight: 600;
		color: var(--cds-text-primary);
	}

	.stat-value.active {
		color: var(--cds-support-success);
	}

	.action-tile {
		padding: 1rem;
	}

	.action-tile h3 {
		font-size: 1.25rem;
		margin-bottom: 0.5rem;
	}

	.action-tile p {
		font-size: 0.875rem;
		color: var(--cds-text-secondary);
	}

	.section {
		margin-top: 3rem;
	}

	.section h2 {
		font-size: 1.5rem;
		margin-bottom: 1rem;
	}

	ol {
		line-height: 1.8;
	}
</style>
