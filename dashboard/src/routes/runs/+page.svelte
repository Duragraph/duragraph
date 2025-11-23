<script lang="ts">
	import { onMount } from 'svelte';
	import {
		DataTable,
		Toolbar,
		ToolbarContent,
		ToolbarSearch,
		Button,
		InlineNotification,
		SkeletonPlaceholder,
		Pagination,
		Tag,
		Select,
		SelectItem
	} from 'carbon-components-svelte';
	import { Add, View } from 'carbon-icons-svelte';
	import { goto } from '$app/navigation';
	import { runs } from '$lib/stores';
	import AppLayout from '$lib/components/layout/AppLayout.svelte';
	import type { Run, RunStatus } from '$lib/api/types';

	let isLoading = $state(true);
	let error = $state<string | null>(null);
	let runsList = $state<Run[]>([]);
	let filteredRows = $state<Run[]>([]);
	let searchValue = $state('');
	let statusFilter = $state<RunStatus | 'all'>('all');

	// Pagination
	let pageSize = $state(10);
	let page = $state(1);
	let paginatedRows = $state<Run[]>([]);

	// DataTable headers
	const headers: any = [
		{ key: 'id', value: 'Run ID' },
		{ key: 'assistant_id', value: 'Assistant' },
		{ key: 'thread_id', value: 'Thread' },
		{ key: 'status', value: 'Status' },
		{ key: 'created_at', value: 'Created' },
		{ key: 'actions', value: 'Actions', sort: false }
	];

	const statusOptions = [
		{ id: 'all', text: 'All Statuses' },
		{ id: 'queued', text: 'Queued' },
		{ id: 'in_progress', text: 'In Progress' },
		{ id: 'requires_action', text: 'Requires Action' },
		{ id: 'cancelling', text: 'Cancelling' },
		{ id: 'cancelled', text: 'Cancelled' },
		{ id: 'failed', text: 'Failed' },
		{ id: 'completed', text: 'Completed' },
		{ id: 'expired', text: 'Expired' }
	];

	onMount(() => {
		loadRuns();

		// Subscribe to store changes
		const unsubscribe = runs.subscribe((state) => {
			runsList = state.items;
			filterRuns();
			isLoading = state.isLoading;
			error = state.error;
		});

		return unsubscribe;
	});

	async function loadRuns() {
		await runs.fetchAll();
	}

	function filterRuns() {
		let filtered = runsList;

		// Filter by status
		if (statusFilter !== 'all') {
			filtered = filtered.filter((r) => r.status === statusFilter);
		}

		// Filter by search
		if (searchValue) {
			const search = searchValue.toLowerCase();
			filtered = filtered.filter(
				(r) =>
					r.id.toLowerCase().includes(search) ||
					r.assistant_id.toLowerCase().includes(search) ||
					r.thread_id.toLowerCase().includes(search)
			);
		}

		filteredRows = filtered;
		updatePagination();
	}

	function updatePagination() {
		const start = (page - 1) * pageSize;
		const end = start + pageSize;
		paginatedRows = filteredRows.slice(start, end);
	}

	$effect(() => {
		updatePagination();
	});

	function handleSearch(e: Event) {
		searchValue = (e as CustomEvent).detail;
		page = 1;
		filterRuns();
	}

	function handleStatusFilterChange() {
		page = 1;
		filterRuns();
	}

	function handleCreateNew() {
		goto('/runs/new');
	}

	function handleView(run: Run) {
		goto(`/runs/${run.id}`);
	}

	function formatDate(dateString: string): string {
		return new Date(dateString).toLocaleDateString('en-US', {
			year: 'numeric',
			month: 'short',
			day: 'numeric',
			hour: '2-digit',
			minute: '2-digit'
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
	<div class="runs-page">
		<div class="page-header">
			<h1>Runs</h1>
			<p>Manage and monitor assistant runs</p>
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
			<SkeletonPlaceholder style="height: 400px; width: 100%;" />
		{:else}
			<div class="filters">
				<Select
					labelText="Filter by Status"
					bind:selected={statusFilter}
					on:change={handleStatusFilterChange}
				>
					{#each statusOptions as option}
						<SelectItem value={option.id} text={option.text} />
					{/each}
				</Select>
			</div>

			<DataTable headers={headers} rows={paginatedRows}>
				<Toolbar>
					<ToolbarContent>
						<ToolbarSearch
							persistent
							placeholder="Search runs..."
							bind:value={searchValue}
							on:change={handleSearch}
						/>
						<Button icon={Add} on:click={handleCreateNew}>Create Run</Button>
					</ToolbarContent>
				</Toolbar>

				<svelte:fragment slot="cell" let:row let:cell>
					{#if cell.key === 'id'}
						<code class="run-id">{cell.value.slice(0, 16)}...</code>
					{:else if cell.key === 'assistant_id' || cell.key === 'thread_id'}
						<code class="small-id">{cell.value.slice(0, 12)}...</code>
					{:else if cell.key === 'status'}
						<Tag type={getStatusTagType(cell.value)}>{cell.value}</Tag>
					{:else if cell.key === 'created_at'}
						{formatDate(cell.value)}
					{:else if cell.key === 'actions'}
						<div class="actions">
							<Button
								kind="ghost"
								size="small"
								icon={View}
								iconDescription="View"
								on:click={() => handleView(row)}
							/>
						</div>
					{:else}
						{cell.value}
					{/if}
				</svelte:fragment>
			</DataTable>

			{#if filteredRows.length > pageSize}
				<Pagination
					bind:pageSize
					bind:page
					totalItems={filteredRows.length}
					pageSizeInputDisabled
					on:update={updatePagination}
				/>
			{/if}
		{/if}
	</div>
</AppLayout>

<style>
	.runs-page {
		padding: 2rem;
	}

	.page-header {
		margin-bottom: 2rem;
	}

	.page-header h1 {
		font-size: 2rem;
		font-weight: 400;
		margin-bottom: 0.5rem;
	}

	.page-header p {
		color: var(--cds-text-secondary);
		font-size: 0.875rem;
	}

	.filters {
		margin-bottom: 1rem;
		max-width: 300px;
	}

	.run-id,
	.small-id {
		font-family: 'IBM Plex Mono', monospace;
		font-size: 0.75rem;
		background: var(--cds-layer-02);
		padding: 0.125rem 0.25rem;
		border-radius: 2px;
	}

	.actions {
		display: flex;
		gap: 0.5rem;
	}
</style>
