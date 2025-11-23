<script lang="ts">
	import { onMount } from 'svelte';
	import {
		Form,
		TextInput,
		Select,
		SelectItem,
		Button,
		InlineNotification,
		Grid,
		Row,
		Column
	} from 'carbon-components-svelte';
	import { ArrowLeft } from 'carbon-icons-svelte';
	import { goto } from '$app/navigation';
	import { threads, assistants } from '$lib/stores';
	import AppLayout from '$lib/components/layout/AppLayout.svelte';
	import type { Assistant, CreateThreadRequest } from '$lib/api/types';

	let isSubmitting = $state(false);
	let error = $state<string | null>(null);
	let success = $state(false);
	let assistantsList = $state<Assistant[]>([]);

	// Form fields
	let name = $state('');
	let assistantId = $state('');
	let tags = $state('');

	onMount(() => {
		// Load assistants for selection
		assistants.fetchAll();

		const unsubscribe = assistants.subscribe((state) => {
			assistantsList = state.items;
		});

		return unsubscribe;
	});

	async function handleSubmit(e: Event) {
		e.preventDefault();
		error = null;
		isSubmitting = true;

		try {
			const threadData: CreateThreadRequest = {
				metadata: {
					name: name || undefined,
					tags: tags ? tags.split(',').map((t) => t.trim()) : undefined
				}
			};

			// Add assistant_id if selected
			if (assistantId) {
				// Note: The API might not support this directly, adjust based on actual API
				threadData.metadata = {
					...threadData.metadata,
					assistant_id: assistantId
				};
			}

			const newThread = await threads.create(threadData);

			if (newThread) {
				success = true;
				// Redirect to the new thread
				setTimeout(() => {
					goto(`/threads/${newThread.id}`);
				}, 1000);
			} else {
				throw new Error('Failed to create thread');
			}
		} catch (err) {
			error = err instanceof Error ? err.message : 'Failed to create thread';
		} finally {
			isSubmitting = false;
		}
	}

	function handleCancel() {
		goto('/threads');
	}
</script>

<AppLayout>
	<div class="new-thread-page">
		<div class="page-header">
			<Button kind="ghost" icon={ArrowLeft} on:click={handleCancel}>Back to Threads</Button>
			<h1>Create New Thread</h1>
			<p>Start a new conversation thread</p>
		</div>

		{#if error}
			<InlineNotification
				kind="error"
				title="Error"
				subtitle={error}
				on:close={() => (error = null)}
			/>
		{/if}

		{#if success}
			<InlineNotification
				kind="success"
				title="Success"
				subtitle="Thread created successfully. Redirecting..."
				hideCloseButton
			/>
		{/if}

		<Form on:submit={handleSubmit}>
			<Grid>
				<Row>
					<Column lg={16}>
						<TextInput
							labelText="Thread Name (Optional)"
							placeholder="Customer Support - Case #123"
							bind:value={name}
							helperText="A friendly name for this conversation thread"
						/>
					</Column>
				</Row>

				<Row>
					<Column lg={16}>
						<Select labelText="Assistant (Optional)" bind:selected={assistantId}>
							<SelectItem value="" text="No assistant" />
							{#each assistantsList as assistant}
								<SelectItem value={assistant.id} text={assistant.name} />
							{/each}
						</Select>
					</Column>
				</Row>

				<Row>
					<Column lg={16}>
						<TextInput
							labelText="Tags (Optional)"
							placeholder="support, urgent, billing"
							bind:value={tags}
							helperText="Comma-separated tags for organizing threads"
						/>
					</Column>
				</Row>

				<!-- Form Actions -->
				<Row>
					<Column lg={16}>
						<div class="form-actions">
							<Button kind="secondary" on:click={handleCancel}>Cancel</Button>
							<Button type="submit" disabled={isSubmitting}>
								{isSubmitting ? 'Creating...' : 'Create Thread'}
							</Button>
						</div>
					</Column>
				</Row>
			</Grid>
		</Form>
	</div>
</AppLayout>

<style>
	.new-thread-page {
		padding: 2rem;
		max-width: 800px;
	}

	.page-header {
		margin-bottom: 2rem;
	}

	.page-header h1 {
		font-size: 2rem;
		font-weight: 400;
		margin: 1rem 0 0.5rem 0;
	}

	.page-header p {
		color: var(--cds-text-secondary);
		font-size: 0.875rem;
	}

	.form-actions {
		display: flex;
		gap: 1rem;
		justify-content: flex-end;
		margin-top: 2rem;
		padding-top: 2rem;
		border-top: 1px solid var(--cds-border-subtle);
	}
</style>
