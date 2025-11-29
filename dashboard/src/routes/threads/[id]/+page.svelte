<script lang="ts">
	import { onMount } from 'svelte';
	import { page } from '$app/stores';
	import {
		Button,
		InlineNotification,
		SkeletonPlaceholder,
		TextArea,
		Tile,
		Tag
	} from 'carbon-components-svelte';
	import { ArrowLeft, Send, User, Bot } from 'carbon-icons-svelte';
	import { goto } from '$app/navigation';
	import { threads } from '$lib/stores';
	import { api } from '$lib/api/client';
	import AppLayout from '$lib/components/layout/AppLayout.svelte';
	import type { Thread, Message } from '$lib/api/types';

	let threadId = $state('');
	let thread = $state<Thread | null>(null);
	let messages = $state<Message[]>([]);
	let isLoading = $state(true);
	let isLoadingMessages = $state(true);
	let isSending = $state(false);
	let error = $state<string | null>(null);
	let newMessageContent = $state('');

	let messagesContainer = $state<HTMLElement | null>(null);

	onMount(() => {
		// Get thread ID from URL params
		const unsubscribe = page.subscribe(($page) => {
			const id = $page.params.id;
			if (id) {
				threadId = id;
				loadThread();
				loadMessages();
			}
		});

		return unsubscribe;
	});

	async function loadThread() {
		try {
			isLoading = true;
			thread = await threads.getById(threadId);
		} catch (err) {
			error = err instanceof Error ? err.message : 'Failed to load thread';
		} finally {
			isLoading = false;
		}
	}

	async function loadMessages() {
		try {
			isLoadingMessages = true;
			messages = await api.getMessages(threadId);

			// Scroll to bottom after messages load
			setTimeout(scrollToBottom, 100);
		} catch (err) {
			error = err instanceof Error ? err.message : 'Failed to load messages';
		} finally {
			isLoadingMessages = false;
		}
	}

	async function handleSendMessage(e: Event) {
		e.preventDefault();

		if (!newMessageContent.trim()) return;

		try {
			isSending = true;
			error = null;

			// Create message
			await api.createMessage(threadId, {
				role: 'user',
				content: newMessageContent
			});

			// Clear input
			newMessageContent = '';

			// Reload messages
			await loadMessages();
		} catch (err) {
			error = err instanceof Error ? err.message : 'Failed to send message';
		} finally {
			isSending = false;
		}
	}

	function scrollToBottom() {
		if (messagesContainer) {
			messagesContainer.scrollTop = messagesContainer.scrollHeight;
		}
	}

	function formatDate(dateString: string): string {
		return new Date(dateString).toLocaleString('en-US', {
			month: 'short',
			day: 'numeric',
			hour: '2-digit',
			minute: '2-digit'
		});
	}

	function getThreadName(thread: Thread | null): string {
		if (!thread) return 'Thread';
		return thread.metadata?.name || `Thread ${thread.id.slice(0, 8)}`;
	}

	function handleBack() {
		goto('/threads');
	}
</script>

<AppLayout>
	<div class="thread-detail-page">
		<div class="page-header">
			<Button kind="ghost" icon={ArrowLeft} on:click={handleBack}>Back to Threads</Button>
			{#if thread}
				<h1>{getThreadName(thread)}</h1>
				<div class="thread-meta">
					<span>Thread ID: {thread.id}</span>
					{#if thread.assistant_id}
						<span>Assistant: {thread.assistant_id}</span>
					{/if}
					<span>Created: {formatDate(thread.created_at)}</span>
				</div>
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

		{#if isLoading}
			<SkeletonPlaceholder style="height: 600px; width: 100%;" />
		{:else if thread}
			<div class="thread-container">
				<!-- Messages Area -->
				<div class="messages-area" bind:this={messagesContainer}>
					{#if isLoadingMessages}
						<SkeletonPlaceholder style="height: 200px; width: 100%;" />
					{:else if messages.length === 0}
						<div class="empty-state">
							<p>No messages yet. Start the conversation!</p>
						</div>
					{:else}
						<div class="messages-list">
							{#each messages as message}
								<div class="message {message.role}">
									<div class="message-icon">
										{#if message.role === 'user'}
											<User size={20} />
										{:else if message.role === 'assistant'}
											<Bot size={20} />
										{/if}
									</div>
									<div class="message-content">
										<div class="message-header">
											<span class="message-role">
												{message.role === 'user' ? 'You' : 'Assistant'}
											</span>
											<span class="message-time">{formatDate(message.created_at)}</span>
										</div>
										<div class="message-body">
											{#if typeof message.content === 'string'}
												<p>{message.content}</p>
											{:else}
												{#each message.content as content}
													{#if content.type === 'text' && content.text}
														<p>{content.text.value}</p>
													{:else if content.type === 'image_file' && content.image_file}
														<div class="file-attachment">
															<Tag>Image: {content.image_file.file_id}</Tag>
														</div>
													{/if}
												{/each}
											{/if}
										</div>
										{#if message.file_ids && message.file_ids.length > 0}
											<div class="attachments">
												{#each message.file_ids as fileId}
													<Tag size="sm">File: {fileId}</Tag>
												{/each}
											</div>
										{/if}
									</div>
								</div>
							{/each}
						</div>
					{/if}
				</div>

				<!-- Message Input -->
				<div class="message-input">
					<form onsubmit={handleSendMessage}>
						<TextArea
							placeholder="Type your message..."
							bind:value={newMessageContent}
							rows={3}
							disabled={isSending}
						/>
						<div class="input-actions">
							<Button
								type="submit"
								icon={Send}
								disabled={isSending || !newMessageContent.trim()}
							>
								{isSending ? 'Sending...' : 'Send'}
							</Button>
						</div>
					</form>
				</div>
			</div>
		{/if}
	</div>
</AppLayout>

<style>
	.thread-detail-page {
		padding: 2rem;
		height: calc(100vh - 4rem);
		display: flex;
		flex-direction: column;
	}

	.page-header {
		margin-bottom: 1rem;
	}

	.page-header h1 {
		font-size: 1.75rem;
		font-weight: 400;
		margin: 1rem 0 0.5rem 0;
	}

	.thread-meta {
		display: flex;
		gap: 1.5rem;
		color: var(--cds-text-secondary);
		font-size: 0.875rem;
	}

	.thread-container {
		flex: 1;
		display: flex;
		flex-direction: column;
		background: var(--cds-layer-01);
		border: 1px solid var(--cds-border-subtle);
		overflow: hidden;
	}

	.messages-area {
		flex: 1;
		overflow-y: auto;
		padding: 1.5rem;
	}

	.messages-list {
		display: flex;
		flex-direction: column;
		gap: 1.5rem;
	}

	.message {
		display: flex;
		gap: 1rem;
		animation: fadeIn 0.3s ease-in;
	}

	@keyframes fadeIn {
		from {
			opacity: 0;
			transform: translateY(10px);
		}
		to {
			opacity: 1;
			transform: translateY(0);
		}
	}

	.message-icon {
		flex-shrink: 0;
		width: 2.5rem;
		height: 2.5rem;
		display: flex;
		align-items: center;
		justify-content: center;
		border-radius: 50%;
		background: var(--cds-layer-02);
	}

	.message.user .message-icon {
		background: var(--cds-interactive);
		color: white;
	}

	.message.assistant .message-icon {
		background: var(--cds-support-info);
		color: white;
	}

	.message-content {
		flex: 1;
		display: flex;
		flex-direction: column;
		gap: 0.5rem;
	}

	.message-header {
		display: flex;
		justify-content: space-between;
		align-items: center;
	}

	.message-role {
		font-weight: 600;
		font-size: 0.875rem;
	}

	.message-time {
		color: var(--cds-text-secondary);
		font-size: 0.75rem;
	}

	.message-body {
		line-height: 1.6;
	}

	.message-body p {
		margin: 0;
		white-space: pre-wrap;
	}

	.file-attachment {
		margin-top: 0.5rem;
	}

	.attachments {
		display: flex;
		gap: 0.5rem;
		flex-wrap: wrap;
		margin-top: 0.5rem;
	}

	.empty-state {
		height: 100%;
		display: flex;
		align-items: center;
		justify-content: center;
		color: var(--cds-text-secondary);
		font-style: italic;
	}

	.message-input {
		border-top: 1px solid var(--cds-border-subtle);
		padding: 1rem;
		background: var(--cds-layer-02);
	}

	.message-input form {
		display: flex;
		flex-direction: column;
		gap: 0.5rem;
	}

	.input-actions {
		display: flex;
		justify-content: flex-end;
	}
</style>
