<script lang="ts">
	import { onMount } from 'svelte';
	import { page } from '$app/stores';
	import {
		Form,
		TextInput,
		TextArea,
		Select,
		SelectItem,
		Button,
		InlineNotification,
		Grid,
		Row,
		Column,
		StructuredList,
		StructuredListHead,
		StructuredListRow,
		StructuredListCell,
		StructuredListBody,
		Modal,
		SkeletonPlaceholder
	} from 'carbon-components-svelte';
	import { Add, TrashCan, ArrowLeft, Save } from 'carbon-icons-svelte';
	import { goto } from '$app/navigation';
	import { assistants } from '$lib/stores';
	import AppLayout from '$lib/components/layout/AppLayout.svelte';
	import type { Assistant, Tool, UpdateAssistantRequest } from '$lib/api/types';

	let assistantId = $state('');
	let assistant = $state<Assistant | null>(null);
	let isLoading = $state(true);
	let isSubmitting = $state(false);
	let error = $state<string | null>(null);
	let success = $state(false);

	// Form fields
	let name = $state('');
	let model = $state('gpt-4-turbo-preview');
	let description = $state('');
	let instructions = $state('');
	let tools = $state<Tool[]>([]);
	let metadata = $state<Record<string, string>>({});

	// Tool modal
	let toolModalOpen = $state(false);
	let newToolType = $state<'code_interpreter' | 'retrieval' | 'function'>('function');
	let newToolFunction = $state({
		name: '',
		description: '',
		parameters: '{}'
	});

	// Metadata modal
	let metadataModalOpen = $state(false);
	let newMetadataKey = $state('');
	let newMetadataValue = $state('');

	const modelOptions = [
		{ id: 'gpt-4-turbo-preview', text: 'GPT-4 Turbo' },
		{ id: 'gpt-4', text: 'GPT-4' },
		{ id: 'gpt-3.5-turbo', text: 'GPT-3.5 Turbo' },
		{ id: 'claude-3-opus', text: 'Claude 3 Opus' },
		{ id: 'claude-3-sonnet', text: 'Claude 3 Sonnet' }
	];

	onMount(() => {
		// Get assistant ID from URL params
		const unsubscribe = page.subscribe(($page) => {
			const id = $page.params.id;
			if (id) {
				assistantId = id;
				loadAssistant();
			}
		});

		return unsubscribe;
	});

	async function loadAssistant() {
		try {
			isLoading = true;
			assistant = await assistants.getById(assistantId);

			if (assistant) {
				// Populate form fields
				name = assistant.name;
				model = assistant.model;
				description = assistant.description || '';
				instructions = assistant.instructions || '';
				tools = assistant.tools || [];
				metadata = assistant.metadata || {};
			}
		} catch (err) {
			error = err instanceof Error ? err.message : 'Failed to load assistant';
		} finally {
			isLoading = false;
		}
	}

	async function handleSubmit(e: Event) {
		e.preventDefault();
		error = null;
		isSubmitting = true;

		try {
			const updateData: UpdateAssistantRequest = {
				name,
				model,
				description: description || undefined,
				instructions: instructions || undefined,
				tools: tools.length > 0 ? tools : undefined,
				metadata: Object.keys(metadata).length > 0 ? metadata : undefined
			};

			await assistants.update(assistantId, updateData);
			success = true;

			// Redirect after short delay
			setTimeout(() => {
				goto('/assistants');
			}, 1500);
		} catch (err) {
			error = err instanceof Error ? err.message : 'Failed to update assistant';
		} finally {
			isSubmitting = false;
		}
	}

	function handleAddTool() {
		if (newToolType === 'function') {
			try {
				const parameters = JSON.parse(newToolFunction.parameters);
				tools = [
					...tools,
					{
						type: 'function',
						function: {
							name: newToolFunction.name,
							description: newToolFunction.description,
							parameters
						}
					}
				];
				resetToolForm();
				toolModalOpen = false;
			} catch (err) {
				error = 'Invalid JSON for function parameters';
			}
		} else {
			tools = [...tools, { type: newToolType }];
			resetToolForm();
			toolModalOpen = false;
		}
	}

	function handleRemoveTool(index: number) {
		tools = tools.filter((_, i) => i !== index);
	}

	function handleAddMetadata() {
		if (newMetadataKey && newMetadataValue) {
			metadata = { ...metadata, [newMetadataKey]: newMetadataValue };
			newMetadataKey = '';
			newMetadataValue = '';
			metadataModalOpen = false;
		}
	}

	function handleRemoveMetadata(key: string) {
		const { [key]: _, ...rest } = metadata;
		metadata = rest;
	}

	function resetToolForm() {
		newToolType = 'function';
		newToolFunction = { name: '', description: '', parameters: '{}' };
	}

	function handleCancel() {
		goto('/assistants');
	}
</script>

<AppLayout>
	<div class="edit-assistant-page">
		<div class="page-header">
			<Button kind="ghost" icon={ArrowLeft} on:click={handleCancel}>Back to Assistants</Button>
			<h1>Edit Assistant</h1>
			{#if assistant}
				<p>Editing: {assistant.name}</p>
			{/if}
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
				subtitle="Assistant updated successfully. Redirecting..."
				hideCloseButton
			/>
		{/if}

		{#if isLoading}
			<SkeletonPlaceholder style="height: 600px; width: 100%;" />
		{:else if assistant}
			<Form on:submit={handleSubmit}>
				<Grid>
					<Row>
						<Column lg={8}>
							<TextInput
								labelText="Name"
								placeholder="My Assistant"
								bind:value={name}
								required
								helperText="A unique name for your assistant"
							/>
						</Column>
						<Column lg={8}>
							<Select labelText="Model" bind:selected={model} required>
								{#each modelOptions as option}
									<SelectItem value={option.id} text={option.text} />
								{/each}
							</Select>
						</Column>
					</Row>

					<Row>
						<Column lg={16}>
							<TextArea
								labelText="Description"
								placeholder="Describe what this assistant does..."
								bind:value={description}
								rows={2}
							/>
						</Column>
					</Row>

					<Row>
						<Column lg={16}>
							<TextArea
								labelText="Instructions"
								placeholder="You are a helpful assistant that..."
								bind:value={instructions}
								rows={6}
								helperText="System instructions that define the assistant's behavior and personality"
							/>
						</Column>
					</Row>

					<!-- Tools Section -->
					<Row>
						<Column lg={16}>
							<div class="section">
								<div class="section-header">
									<h3>Tools</h3>
									<Button size="small" icon={Add} on:click={() => (toolModalOpen = true)}>
										Add Tool
									</Button>
								</div>

								{#if tools.length > 0}
									<StructuredList>
										<StructuredListHead>
											<StructuredListRow head>
												<StructuredListCell head>Type</StructuredListCell>
												<StructuredListCell head>Details</StructuredListCell>
												<StructuredListCell head>Actions</StructuredListCell>
											</StructuredListRow>
										</StructuredListHead>
										<StructuredListBody>
											{#each tools as tool, index}
												<StructuredListRow>
													<StructuredListCell>{tool.type}</StructuredListCell>
													<StructuredListCell>
														{#if tool.type === 'function' && tool.function}
															<strong>{tool.function.name}</strong><br />
															<span class="text-secondary">{tool.function.description}</span>
														{:else}
															-
														{/if}
													</StructuredListCell>
													<StructuredListCell>
														<Button
															kind="danger-ghost"
															size="small"
															icon={TrashCan}
															iconDescription="Remove"
															on:click={() => handleRemoveTool(index)}
														/>
													</StructuredListCell>
												</StructuredListRow>
											{/each}
										</StructuredListBody>
									</StructuredList>
								{:else}
									<p class="empty-state">No tools added yet</p>
								{/if}
							</div>
						</Column>
					</Row>

					<!-- Metadata Section -->
					<Row>
						<Column lg={16}>
							<div class="section">
								<div class="section-header">
									<h3>Metadata</h3>
									<Button size="small" icon={Add} on:click={() => (metadataModalOpen = true)}>
										Add Metadata
									</Button>
								</div>

								{#if Object.keys(metadata).length > 0}
									<StructuredList>
										<StructuredListHead>
											<StructuredListRow head>
												<StructuredListCell head>Key</StructuredListCell>
												<StructuredListCell head>Value</StructuredListCell>
												<StructuredListCell head>Actions</StructuredListCell>
											</StructuredListRow>
										</StructuredListHead>
										<StructuredListBody>
											{#each Object.entries(metadata) as [key, value]}
												<StructuredListRow>
													<StructuredListCell>{key}</StructuredListCell>
													<StructuredListCell>{value}</StructuredListCell>
													<StructuredListCell>
														<Button
															kind="danger-ghost"
															size="small"
															icon={TrashCan}
															iconDescription="Remove"
															on:click={() => handleRemoveMetadata(key)}
														/>
													</StructuredListCell>
												</StructuredListRow>
											{/each}
										</StructuredListBody>
									</StructuredList>
								{:else}
									<p class="empty-state">No metadata added yet</p>
								{/if}
							</div>
						</Column>
					</Row>

					<!-- Form Actions -->
					<Row>
						<Column lg={16}>
							<div class="form-actions">
								<Button kind="secondary" on:click={handleCancel}>Cancel</Button>
								<Button type="submit" icon={Save} disabled={isSubmitting || !name || !model}>
									{isSubmitting ? 'Saving...' : 'Save Changes'}
								</Button>
							</div>
						</Column>
					</Row>
				</Grid>
			</Form>

			<!-- Add Tool Modal -->
			<Modal
				bind:open={toolModalOpen}
				modalHeading="Add Tool"
				primaryButtonText="Add"
				secondaryButtonText="Cancel"
				on:click:button--secondary={() => (toolModalOpen = false)}
				on:click:button--primary={handleAddTool}
			>
				<Select labelText="Tool Type" bind:selected={newToolType}>
					<SelectItem value="function" text="Function" />
					<SelectItem value="code_interpreter" text="Code Interpreter" />
					<SelectItem value="retrieval" text="Retrieval" />
				</Select>

				{#if newToolType === 'function'}
					<TextInput
						labelText="Function Name"
						placeholder="get_weather"
						bind:value={newToolFunction.name}
						required
					/>
					<TextArea
						labelText="Description"
						placeholder="Get the current weather for a location"
						bind:value={newToolFunction.description}
						rows={2}
					/>
					<TextArea
						labelText="Parameters (JSON)"
						placeholder="JSON schema for parameters"
						bind:value={newToolFunction.parameters}
						rows={4}
						helperText="JSON Schema for function parameters"
					/>
				{/if}
			</Modal>

			<!-- Add Metadata Modal -->
			<Modal
				bind:open={metadataModalOpen}
				modalHeading="Add Metadata"
				primaryButtonText="Add"
				secondaryButtonText="Cancel"
				on:click:button--secondary={() => (metadataModalOpen = false)}
				on:click:button--primary={handleAddMetadata}
			>
				<TextInput labelText="Key" placeholder="category" bind:value={newMetadataKey} required />
				<TextInput
					labelText="Value"
					placeholder="customer-support"
					bind:value={newMetadataValue}
					required
				/>
			</Modal>
		{/if}
	</div>
</AppLayout>

<style>
	.edit-assistant-page {
		padding: 2rem;
		max-width: 1200px;
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

	.section {
		margin: 2rem 0;
		padding: 1.5rem;
		background: var(--cds-layer-01);
		border: 1px solid var(--cds-border-subtle);
	}

	.section-header {
		display: flex;
		justify-content: space-between;
		align-items: center;
		margin-bottom: 1rem;
	}

	.section-header h3 {
		font-size: 1.25rem;
		font-weight: 400;
		margin: 0;
	}

	.empty-state {
		color: var(--cds-text-secondary);
		font-style: italic;
		padding: 1rem;
		text-align: center;
	}

	.form-actions {
		display: flex;
		gap: 1rem;
		justify-content: flex-end;
		margin-top: 2rem;
		padding-top: 2rem;
		border-top: 1px solid var(--cds-border-subtle);
	}

	.text-secondary {
		color: var(--cds-text-secondary);
		font-size: 0.875rem;
	}
</style>
