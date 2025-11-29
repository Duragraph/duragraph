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
		Modal,
		Pagination,
		Tag
	} from 'carbon-components-svelte';
	import { Add, View, TrashCan } from 'carbon-icons-svelte';
	import { goto } from '$app/navigation';
	import { threads } from '$lib/stores';
	import AppLayout from '$lib/components/layout/AppLayout.svelte';
	import type { Thread } from '$lib/api/types';

	let isLoading = $state(true);
	let error = $state<string | null>(null);
	let threadsList = $state<Thread[]>([]);
	let filteredRows = $state<Thread[]>([]);
	let searchValue = $state('');
	let deleteModalOpen = $state(false);
	let threadToDelete = $state<Thread | null>(null);

	// Pagination
	let pageSize = $state(10);
	let page = $state(1);
	let paginatedRows = $state<Thread[]>([]);

	// DataTable headers
	const headers: any = [
		{ key: 'metadata', value: 'Name' },
		{ key: 'assistant_id', value: 'Assistant ID' },
		{ key: 'created_at', value: 'Created' },
		{ key: 'actions', value: 'Actions', sort: false }
	];

	onMount(() => {
		loadThreads();

		// Subscribe to store changes
		const unsubscribe = threads.subscribe((state) => {
			threadsList = state.items;
			filterThreads();
			isLoading = state.isLoading;
			error = state.error;
		});

		return unsubscribe;
	});

	async function loadThreads() {
		await threads.fetchAll();
	}

	function filterThreads() {
		if (!searchValue) {
			filteredRows = threadsList;
		} else {
			const search = searchValue.toLowerCase();
			filteredRows = threadsList.filter(
				(t) =>
					t.id.toLowerCase().includes(search) ||
					t.assistant_id?.toLowerCase().includes(search) ||
					JSON.stringify(t.metadata).toLowerCase().includes(search)
			);
		}
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
		page = 1; // Reset to first page on search
		filterThreads();
	}

	function handleCreateNew() {
		goto('/threads/new');
	}

	function handleView(thread: Thread) {
		goto(`/threads/${thread.id}`);
	}

	function handleDeleteClick(thread: Thread) {
		threadToDelete = thread;
		deleteModalOpen = true;
	}

	async function handleDeleteConfirm() {
		if (threadToDelete) {
			await threads.delete(threadToDelete.id);
			deleteModalOpen = false;
			threadToDelete = null;
		}
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

	function getThreadName(thread: Thread): string {
		return thread.metadata?.name || `Thread ${thread.id.slice(0, 8)}`;
	}
</script>

<AppLayout>
	<div class="threads-page">
		<div class="page-header">
			<h1>Threads</h1>
			<p>Manage conversation threads with assistants</p>
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
			<DataTable headers={headers} rows={paginatedRows}>
				<Toolbar>
					<ToolbarContent>
						<ToolbarSearch
							persistent
							placeholder="Search threads..."
							bind:value={searchValue}
							on:change={handleSearch}
						/>
						<Button icon={Add} on:click={handleCreateNew}>Create Thread</Button>
					</ToolbarContent>
				</Toolbar>

				<svelte:fragment slot="cell" let:row let:cell>
					{#if cell.key === 'metadata'}
						<div class="thread-name">
							{getThreadName(row)}
							{#if row.metadata?.tags}
								<div class="tags">
									{#each row.metadata.tags as tag}
										<Tag size="sm">{tag}</Tag>
									{/each}
								</div>
							{/if}
						</div>
					{:else if cell.key === 'assistant_id'}
						{cell.value || '-'}
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
							<Button
								kind="danger-ghost"
								size="small"
								icon={TrashCan}
								iconDescription="Delete"
								on:click={() => handleDeleteClick(row)}
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

		<!-- Delete Confirmation Modal -->
		<Modal
			bind:open={deleteModalOpen}
			modalHeading="Delete Thread"
			primaryButtonText="Delete"
			secondaryButtonText="Cancel"
			danger
			on:click:button--secondary={() => (deleteModalOpen = false)}
			on:click:button--primary={handleDeleteConfirm}
		>
			<p>
				Are you sure you want to delete this thread? This will delete all messages in the thread.
				This action cannot be undone.
			</p>
		</Modal>
	</div>
</AppLayout>

<style>
	.threads-page {
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

	.thread-name {
		display: flex;
		flex-direction: column;
		gap: 0.5rem;
	}

	.tags {
		display: flex;
		gap: 0.25rem;
		flex-wrap: wrap;
	}

	.actions {
		display: flex;
		gap: 0.5rem;
	}
</style>
