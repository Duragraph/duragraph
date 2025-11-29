<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import { page } from '$app/stores';
	import {
		Button,
		InlineNotification,
		SkeletonPlaceholder,
		Tile,
		Tag,
		CodeSnippet,
		Accordion,
		AccordionItem,
		StructuredList,
		StructuredListHead,
		StructuredListRow,
		StructuredListCell,
		StructuredListBody
	} from 'carbon-components-svelte';
	import { ArrowLeft, Renew, StopOutline } from 'carbon-icons-svelte';
	import { goto } from '$app/navigation';
	import { runs } from '$lib/stores';
	import { api } from '$lib/api/client';
	import AppLayout from '$lib/components/layout/AppLayout.svelte';
	import type { Run, RunStatus } from '$lib/api/types';

	let runId = $state('');
	let run = $state<Run | null>(null);
	let isLoading = $state(true);
	let error = $state<string | null>(null);
	let streamEvents = $state<any[]>([]);
	let eventSource: EventSource | null = null;

	onMount(() => {
		// Get run ID from URL params
		const unsubscribe = page.subscribe(($page) => {
			const id = $page.params.id;
			if (id) {
				runId = id;
				loadRun();
				startStreaming();
			}
		});

		return unsubscribe;
	});

	onDestroy(() => {
		// Clean up event source
		if (eventSource) {
			eventSource.close();
		}
	});

	async function loadRun() {
		try {
			isLoading = true;
			run = await runs.getById(runId);
		} catch (err) {
			error = err instanceof Error ? err.message : 'Failed to load run';
		} finally {
			isLoading = false;
		}
	}

	function startStreaming() {
		if (eventSource) {
			eventSource.close();
		}

		streamEvents = [];
		eventSource = api.streamRun(runId);

		eventSource.onmessage = (event) => {
			try {
				const data = JSON.parse(event.data);
				streamEvents = [...streamEvents, data];

				// Update run status if we get a status update
				if (data.event === 'thread.run.completed' || data.event === 'thread.run.failed') {
					loadRun();
				}
			} catch (err) {
				console.error('Failed to parse SSE event:', err);
			}
		};

		eventSource.onerror = (err) => {
			console.error('SSE error:', err);
			if (eventSource) {
				eventSource.close();
			}
		};
	}

	async function handleCancel() {
		try {
			await api.cancelRun(runId);
			await loadRun();
		} catch (err) {
			error = err instanceof Error ? err.message : 'Failed to cancel run';
		}
	}

	function handleBack() {
		goto('/runs');
	}

	function formatDate(dateString: string): string {
		return new Date(dateString).toLocaleString('en-US', {
			year: 'numeric',
			month: 'short',
			day: 'numeric',
			hour: '2-digit',
			minute: '2-digit',
			second: '2-digit'
		});
	}

	function getStatusTagType(status: RunStatus): 'red' | 'green' | 'blue' | 'cyan' | 'gray' {
		switch (status) {
			case 'completed':
				return 'green';
			case 'failed':
			case 'cancelled':
			case 'expired':
				return 'red';
			case 'in_progress':
				return 'blue';
			case 'requires_action':
				return 'cyan';
			default:
				return 'gray';
		}
	}
</script>

<AppLayout>
	<div class="run-detail-page">
		<div class="page-header">
			<Button kind="ghost" icon={ArrowLeft} on:click={handleBack}>Back to Runs</Button>
			<h1>Run Details</h1>
		</div>

		{#if error}
			<InlineNotification
				kind="error"
				title="Error"
				subtitle={error}
				on:close={() => (error = null)}
			/>
		{/if}

		{#if isLoading}
			<SkeletonPlaceholder style="height: 600px; width: 100%;" />
		{:else if run}
			<div class="run-container">
				<!-- Run Overview -->
				<Tile class="overview-tile">
					<div class="overview-header">
						<h2>Overview</h2>
						<div class="actions">
							{#if run.status === 'in_progress' || run.status === 'queued'}
								<Button kind="danger-tertiary" size="small" icon={StopOutline} on:click={handleCancel}>
									Cancel Run
								</Button>
							{/if}
							<Button kind="tertiary" size="small" icon={Renew} on:click={loadRun}>
								Refresh
							</Button>
						</div>
					</div>

					<div class="overview-grid">
						<div class="info-item">
							<span class="label">Status</span>
							<Tag type={getStatusTagType(run.status)}>{run.status}</Tag>
						</div>
						<div class="info-item">
							<span class="label">Run ID</span>
							<code class="id">{run.id}</code>
						</div>
						<div class="info-item">
							<span class="label">Assistant ID</span>
							<code class="id">{run.assistant_id}</code>
						</div>
						<div class="info-item">
							<span class="label">Thread ID</span>
							<code class="id">{run.thread_id}</code>
						</div>
						<div class="info-item">
							<span class="label">Model</span>
							<span>{run.model}</span>
						</div>
						<div class="info-item">
							<span class="label">Created</span>
							<span>{formatDate(run.created_at)}</span>
						</div>
						{#if run.started_at}
							<div class="info-item">
								<span class="label">Started</span>
								<span>{formatDate(run.started_at)}</span>
							</div>
						{/if}
						{#if run.completed_at}
							<div class="info-item">
								<span class="label">Completed</span>
								<span>{formatDate(run.completed_at)}</span>
							</div>
						{/if}
					</div>

					{#if run.instructions}
						<div class="instructions">
							<h3>Instructions</h3>
							<p>{run.instructions}</p>
						</div>
					{/if}
				</Tile>

				<!-- Stream Events -->
				<Tile class="events-tile">
					<h2>Stream Events</h2>
					{#if streamEvents.length === 0}
						<p class="empty-state">No events yet. Events will appear here as the run progresses.</p>
					{:else}
						<Accordion>
							{#each streamEvents as event, index}
								<AccordionItem>
									<svelte:fragment slot="title">
										<div class="event-title">
											<Tag size="sm">{event.event}</Tag>
											<span class="event-index">Event {index + 1}</span>
										</div>
									</svelte:fragment>
									<CodeSnippet type="multi" code={JSON.stringify(event, null, 2)} />
								</AccordionItem>
							{/each}
						</Accordion>
					{/if}
				</Tile>

				<!-- Required Actions -->
				{#if run.required_action}
					<Tile class="actions-tile">
						<h2>Required Actions</h2>
						<Tag type="cyan">Action Required</Tag>

						{#if run.required_action.type === 'submit_tool_outputs'}
							<div class="tool-calls">
								<h3>Tool Calls</h3>
								<StructuredList>
									<StructuredListHead>
										<StructuredListRow head>
											<StructuredListCell head>Function</StructuredListCell>
											<StructuredListCell head>Arguments</StructuredListCell>
										</StructuredListRow>
									</StructuredListHead>
									<StructuredListBody>
										{#each run.required_action.submit_tool_outputs.tool_calls as toolCall}
											<StructuredListRow>
												<StructuredListCell>
													{toolCall.function.name}
												</StructuredListCell>
												<StructuredListCell>
													<CodeSnippet
														type="single"
														code={toolCall.function.arguments}
													/>
												</StructuredListCell>
											</StructuredListRow>
										{/each}
									</StructuredListBody>
								</StructuredList>
							</div>
						{/if}
					</Tile>
				{/if}

				<!-- Metadata -->
				{#if run.metadata && Object.keys(run.metadata).length > 0}
					<Tile class="metadata-tile">
						<h2>Metadata</h2>
						<StructuredList>
							<StructuredListBody>
								{#each Object.entries(run.metadata) as [key, value]}
									<StructuredListRow>
										<StructuredListCell>{key}</StructuredListCell>
										<StructuredListCell>{value}</StructuredListCell>
									</StructuredListRow>
								{/each}
							</StructuredListBody>
						</StructuredList>
					</Tile>
				{/if}
			</div>
		{/if}
	</div>
</AppLayout>

<style>
	.run-detail-page {
		padding: 2rem;
		max-width: 1400px;
	}

	.page-header {
		margin-bottom: 2rem;
	}

	.page-header h1 {
		font-size: 2rem;
		font-weight: 400;
		margin: 1rem 0 0.5rem 0;
	}

	.run-container {
		display: flex;
		flex-direction: column;
		gap: 1.5rem;
	}

	.overview-header {
		display: flex;
		justify-content: space-between;
		align-items: center;
		margin-bottom: 1.5rem;
	}

	.overview-header h2 {
		font-size: 1.5rem;
		font-weight: 400;
		margin: 0;
	}

	.actions {
		display: flex;
		gap: 0.5rem;
	}

	.overview-grid {
		display: grid;
		grid-template-columns: repeat(auto-fit, minmax(250px, 1fr));
		gap: 1.5rem;
		margin-bottom: 1.5rem;
	}

	.info-item {
		display: flex;
		flex-direction: column;
		gap: 0.5rem;
	}

	.info-item .label {
		color: var(--cds-text-secondary);
		font-size: 0.75rem;
		text-transform: uppercase;
		letter-spacing: 0.5px;
	}

	.id {
		font-family: 'IBM Plex Mono', monospace;
		font-size: 0.875rem;
		background: var(--cds-layer-02);
		padding: 0.25rem 0.5rem;
		border-radius: 2px;
		word-break: break-all;
	}

	.instructions {
		margin-top: 1.5rem;
		padding-top: 1.5rem;
		border-top: 1px solid var(--cds-border-subtle);
	}

	.instructions h3 {
		font-size: 1rem;
		font-weight: 600;
		margin-bottom: 0.5rem;
	}

	.instructions p {
		line-height: 1.6;
		white-space: pre-wrap;
	}

	.events-tile h2,
	.actions-tile h2,
	.metadata-tile h2 {
		font-size: 1.5rem;
		font-weight: 400;
		margin-bottom: 1rem;
	}

	.empty-state {
		color: var(--cds-text-secondary);
		font-style: italic;
		padding: 2rem;
		text-align: center;
	}

	.event-title {
		display: flex;
		gap: 1rem;
		align-items: center;
	}

	.event-index {
		color: var(--cds-text-secondary);
		font-size: 0.875rem;
	}

	.tool-calls {
		margin-top: 1rem;
	}

	.tool-calls h3 {
		font-size: 1rem;
		font-weight: 600;
		margin-bottom: 0.75rem;
	}
</style>
