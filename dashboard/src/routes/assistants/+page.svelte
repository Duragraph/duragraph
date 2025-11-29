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
		Pagination
	} from 'carbon-components-svelte';
	import { Add, TrashCan, Edit } from 'carbon-icons-svelte';
	import { goto } from '$app/navigation';
	import { assistants } from '$lib/stores';
	import AppLayout from '$lib/components/layout/AppLayout.svelte';
	import type { Assistant } from '$lib/api/types';

	let isLoading = $state(true);
	let error = $state<string | null>(null);
	let assistantsList = $state<Assistant[]>([]);
	let filteredRows = $state<Assistant[]>([]);
	let searchValue = $state('');
	let deleteModalOpen = $state(false);
	let assistantToDelete = $state<Assistant | null>(null);

	// Pagination
	let pageSize = $state(10);
	let page = $state(1);

	// DataTable headers
	const headers: any = [
		{ key: 'name', value: 'Name' },
		{ key: 'model', value: 'Model' },
		{ key: 'description', value: 'Description' },
		{ key: 'created_at', value: 'Created' },
		{ key: 'actions', value: 'Actions', sort: false }
	];

	onMount(() => {
		loadAssistants();

		// Subscribe to store changes
		const unsubscribe = assistants.subscribe((state) => {
			assistantsList = state.items;
			filterAssistants();
			isLoading = state.isLoading;
			error = state.error;
		});

		return unsubscribe;
	});

	async function loadAssistants() {
		await assistants.fetchAll();
	}

	function filterAssistants() {
		if (!searchValue) {
			filteredRows = assistantsList;
		} else {
			const search = searchValue.toLowerCase();
			filteredRows = assistantsList.filter(
				(a) =>
					a.name.toLowerCase().includes(search) ||
					a.description?.toLowerCase().includes(search) ||
					a.model.toLowerCase().includes(search)
			);
		}
	}

	function handleSearch(e: Event) {
		searchValue = (e as CustomEvent).detail;
		filterAssistants();
	}

	function handleCreateNew() {
		goto('/assistants/new');
	}

	function handleEdit(assistant: Assistant) {
		goto(`/assistants/${assistant.id}/edit`);
	}

	function handleDeleteClick(assistant: Assistant) {
		assistantToDelete = assistant;
		deleteModalOpen = true;
	}

	async function handleDeleteConfirm() {
		if (assistantToDelete) {
			await assistants.delete(assistantToDelete.id);
			deleteModalOpen = false;
			assistantToDelete = null;
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

	// Paginated rows
	$effect(() => {
		const start = (page - 1) * pageSize;
		const end = start + pageSize;
		filteredRows = filteredRows.slice(start, end);
	});
</script>

<AppLayout>
	<div class="assistants-page">
		<div class="page-header">
			<h1>Assistants</h1>
			<p>Manage your AI assistants and their configurations</p>
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
			<DataTable {headers} rows={filteredRows}>
				<Toolbar>
					<ToolbarContent>
						<ToolbarSearch
							persistent
							placeholder="Search assistants..."
							bind:value={searchValue}
							on:change={handleSearch}
						/>
						<Button icon={Add} on:click={handleCreateNew}>Create Assistant</Button>
					</ToolbarContent>
				</Toolbar>

				<svelte:fragment slot="cell" let:row let:cell>
					{#if cell.key === 'created_at'}
						{formatDate(cell.value)}
					{:else if cell.key === 'description'}
						<span class="description">{cell.value || '-'}</span>
					{:else if cell.key === 'actions'}
						<div class="actions">
							<Button
								kind="ghost"
								size="small"
								icon={Edit}
								iconDescription="Edit"
								on:click={() => handleEdit(row)}
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
				/>
			{/if}
		{/if}

		<!-- Delete Confirmation Modal -->
		<Modal
			bind:open={deleteModalOpen}
			modalHeading="Delete Assistant"
			primaryButtonText="Delete"
			secondaryButtonText="Cancel"
			danger
			on:click:button--secondary={() => (deleteModalOpen = false)}
			on:click:button--primary={handleDeleteConfirm}
		>
			<p>
				Are you sure you want to delete the assistant <strong>{assistantToDelete?.name}</strong>?
				This action cannot be undone.
			</p>
		</Modal>
	</div>
</AppLayout>

<style>
	.assistants-page {
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

	.description {
		max-width: 300px;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
		display: block;
	}

	.actions {
		display: flex;
		gap: 0.5rem;
	}
</style>
