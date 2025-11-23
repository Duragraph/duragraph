<script lang="ts">
	import { onMount } from 'svelte';
	import {
		Form,
		TextArea,
		Select,
		SelectItem,
		Button,
		InlineNotification,
		Grid,
		Row,
		Column,
		TextInput
	} from 'carbon-components-svelte';
	import { ArrowLeft } from 'carbon-icons-svelte';
	import { goto } from '$app/navigation';
	import { runs, assistants, threads } from '$lib/stores';
	import AppLayout from '$lib/components/layout/AppLayout.svelte';
	import type { Assistant, Thread, CreateRunRequest } from '$lib/api/types';

	let isSubmitting = $state(false);
	let error = $state<string | null>(null);
	let success = $state(false);
	let assistantsList = $state<Assistant[]>([]);
	let threadsList = $state<Thread[]>([]);

	// Form fields
	let assistantId = $state('');
	let threadId = $state('');
	let instructions = $state('');
	let additionalInstructions = $state('');

	onMount(() => {
		// Load assistants and threads for selection
		assistants.fetchAll();
		threads.fetchAll();

		const unsubscribeAssistants = assistants.subscribe((state) => {
			assistantsList = state.items;
		});

		const unsubscribeThreads = threads.subscribe((state) => {
			threadsList = state.items;
		});

		return () => {
			unsubscribeAssistants();
			unsubscribeThreads();
		};
	});

	async function handleSubmit(e: Event) {
		e.preventDefault();
		error = null;
		isSubmitting = true;

		try {
			const runData: CreateRunRequest = {
				assistant_id: assistantId,
				thread_id: threadId,
				instructions: instructions || undefined,
				additional_instructions: additionalInstructions || undefined
			};

			const newRun = await runs.create(runData);

			if (newRun) {
				success = true;
				// Redirect to the new run
				setTimeout(() => {
					goto(`/runs/${newRun.id}`);
				}, 1000);
			} else {
				throw new Error('Failed to create run');
			}
		} catch (err) {
			error = err instanceof Error ? err.message : 'Failed to create run';
		} finally {
			isSubmitting = false;
		}
	}

	function handleCancel() {
		goto('/runs');
	}

	function getThreadName(thread: Thread): string {
		return thread.metadata?.name || `Thread ${thread.id.slice(0, 8)}`;
	}
</script>

<AppLayout>
	<div class="new-run-page">
		<div class="page-header">
			<Button kind="ghost" icon={ArrowLeft} on:click={handleCancel}>Back to Runs</Button>
			<h1>Create New Run</h1>
			<p>Start a new assistant run on a thread</p>
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
				subtitle="Run created successfully. Redirecting..."
				hideCloseButton
			/>
		{/if}

		<Form on:submit={handleSubmit}>
			<Grid>
				<Row>
					<Column lg={8}>
						<Select labelText="Assistant *" bind:selected={assistantId} required>
							<SelectItem value="" text="Select an assistant" />
							{#each assistantsList as assistant}
								<SelectItem value={assistant.id} text={assistant.name} />
							{/each}
						</Select>
					</Column>
					<Column lg={8}>
						<Select labelText="Thread *" bind:selected={threadId} required>
							<SelectItem value="" text="Select a thread" />
							{#each threadsList as thread}
								<SelectItem value={thread.id} text={getThreadName(thread)} />
							{/each}
						</Select>
					</Column>
				</Row>

				<Row>
					<Column lg={16}>
						<TextArea
							labelText="Instructions (Optional)"
							placeholder="Override the default assistant instructions..."
							bind:value={instructions}
							rows={4}
							helperText="These instructions will override the assistant's default instructions for this run"
						/>
					</Column>
				</Row>

				<Row>
					<Column lg={16}>
						<TextArea
							labelText="Additional Instructions (Optional)"
							placeholder="Add extra context or instructions..."
							bind:value={additionalInstructions}
							rows={3}
							helperText="These instructions will be appended to the assistant's instructions for this run"
						/>
					</Column>
				</Row>

				<!-- Form Actions -->
				<Row>
					<Column lg={16}>
						<div class="form-actions">
							<Button kind="secondary" on:click={handleCancel}>Cancel</Button>
							<Button type="submit" disabled={isSubmitting || !assistantId || !threadId}>
								{isSubmitting ? 'Creating...' : 'Create Run'}
							</Button>
						</div>
					</Column>
				</Row>
			</Grid>
		</Form>

		<div class="help-section">
			<h3>What is a Run?</h3>
			<p>
				A Run represents an execution of an Assistant on a Thread. The Assistant uses its configuration
				and the Thread's Messages to perform tasks by calling models and tools. As part of a Run, the
				Assistant appends Messages to the Thread.
			</p>
		</div>
	</div>
</AppLayout>

<style>
	.new-run-page {
		padding: 2rem;
		max-width: 1000px;
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

	.help-section {
		margin-top: 3rem;
		padding: 1.5rem;
		background: var(--cds-layer-01);
		border-left: 3px solid var(--cds-interactive);
	}

	.help-section h3 {
		font-size: 1.125rem;
		font-weight: 600;
		margin-bottom: 0.75rem;
	}

	.help-section p {
		line-height: 1.6;
		color: var(--cds-text-secondary);
	}
</style>
