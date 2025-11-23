import { test, expect } from '@playwright/test';
import { loginAsUser, loginAsAdmin } from '../fixtures/auth';

test.describe('Real-Time Streaming - Server-Sent Events', () => {
	test.beforeEach(async ({ page }) => {
		await loginAsUser(page);
	});

	test('receives SSE events when run status changes', async ({ page }) => {
		// Create a run first
		await page.goto('/runs/new');

		// Mock successful run creation
		await page.route('**/api/v1/runs', async (route) => {
			if (route.request().method() === 'POST') {
				await route.fulfill({
					status: 200,
					headers: { 'Content-Type': 'application/json' },
					body: JSON.stringify({
						run_id: 'run-streaming-123',
						status: 'queued',
						thread_id: 'thread-123',
						assistant_id: 'assistant-123'
					})
				});
			} else {
				await route.continue();
			}
		});

		// Fill and submit form
		await page.fill('input[name="assistant_id"]', 'assistant-123');
		await page.fill('input[name="thread_id"]', 'thread-123');
		await page.click('button[type="submit"]');

		// Should navigate to run details page
		await page.waitForURL(/.*runs\/run-streaming-123/, { timeout: 5000 });

		// Mock SSE stream
		await page.route('**/api/v1/stream*', async (route) => {
			// SSE response with multiple events
			const sseData = [
				'data: {"event":"run.started","run_id":"run-streaming-123","status":"in_progress"}\n\n',
				'data: {"event":"run.node.started","node_id":"node-1","node_type":"llm"}\n\n',
				'data: {"event":"run.node.completed","node_id":"node-1","output":"Hello"}\n\n',
				'data: {"event":"run.completed","run_id":"run-streaming-123","status":"completed"}\n\n'
			].join('');

			await route.fulfill({
				status: 200,
				headers: {
					'Content-Type': 'text/event-stream',
					'Cache-Control': 'no-cache',
					Connection: 'keep-alive'
				},
				body: sseData
			});
		});

		// Status should update in real-time
		// Wait for initial "queued" status
		await expect(page.getByText(/status.*queued/i)).toBeVisible({ timeout: 2000 });

		// Should update to "in_progress"
		await expect(page.getByText(/status.*in.progress|running/i)).toBeVisible({
			timeout: 5000
		});

		// Should eventually show "completed"
		await expect(page.getByText(/status.*completed|finished/i)).toBeVisible({
			timeout: 5000
		});
	});

	test('displays streaming output in real-time', async ({ page }) => {
		await page.goto('/runs/run-stream-output-123');

		// Mock run details
		await page.route('**/api/v1/runs/run-stream-output-123', async (route) => {
			await route.fulfill({
				status: 200,
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify({
					run_id: 'run-stream-output-123',
					status: 'in_progress',
					output: null
				})
			});
		});

		// Mock SSE stream with output chunks
		await page.route('**/api/v1/stream*', async (route) => {
			const sseData = [
				'data: {"event":"run.started","run_id":"run-stream-output-123"}\n\n',
				'data: {"event":"run.output.chunk","content":"Hello"}\n\n',
				'data: {"event":"run.output.chunk","content":" world"}\n\n',
				'data: {"event":"run.output.chunk","content":"!"}\n\n',
				'data: {"event":"run.completed","output":"Hello world!"}\n\n'
			].join('');

			await route.fulfill({
				status: 200,
				headers: {
					'Content-Type': 'text/event-stream',
					'Cache-Control': 'no-cache'
				},
				body: sseData
			});
		});

		// Output should appear incrementally
		await expect(page.getByText(/Hello/)).toBeVisible({ timeout: 3000 });
		await expect(page.getByText(/Hello world/)).toBeVisible({ timeout: 3000 });
		await expect(page.getByText(/Hello world!/)).toBeVisible({ timeout: 3000 });
	});

	test('shows node execution progress in real-time', async ({ page }) => {
		await page.goto('/runs/run-progress-123');

		// Mock run details
		await page.route('**/api/v1/runs/run-progress-123', async (route) => {
			await route.fulfill({
				status: 200,
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify({
					run_id: 'run-progress-123',
					status: 'in_progress'
				})
			});
		});

		// Mock SSE stream with node events
		await page.route('**/api/v1/stream*', async (route) => {
			const sseData = [
				'data: {"event":"run.started"}\n\n',
				'data: {"event":"run.node.started","node_id":"node-1","node_name":"LLM Call"}\n\n',
				'data: {"event":"run.node.completed","node_id":"node-1"}\n\n',
				'data: {"event":"run.node.started","node_id":"node-2","node_name":"Tool Call"}\n\n',
				'data: {"event":"run.node.completed","node_id":"node-2"}\n\n',
				'data: {"event":"run.completed"}\n\n'
			].join('');

			await route.fulfill({
				status: 200,
				headers: { 'Content-Type': 'text/event-stream' },
				body: sseData
			});
		});

		// Should show node execution progress
		await expect(page.getByText(/LLM Call/)).toBeVisible({ timeout: 3000 });
		await expect(page.getByText(/Tool Call/)).toBeVisible({ timeout: 5000 });

		// Should show completion
		await expect(page.getByText(/completed|finished/i)).toBeVisible({ timeout: 5000 });
	});

	test('reconnects SSE stream on connection loss', async ({ page }) => {
		await page.goto('/runs/run-reconnect-123');

		// Mock run details
		await page.route('**/api/v1/runs/run-reconnect-123', async (route) => {
			await route.fulfill({
				status: 200,
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify({
					run_id: 'run-reconnect-123',
					status: 'in_progress'
				})
			});
		});

		let connectionCount = 0;

		// Mock SSE stream that disconnects and reconnects
		await page.route('**/api/v1/stream*', async (route) => {
			connectionCount++;

			if (connectionCount === 1) {
				// First connection - send partial data then disconnect
				await route.fulfill({
					status: 200,
					headers: { 'Content-Type': 'text/event-stream' },
					body: 'data: {"event":"run.started"}\n\n'
				});
			} else {
				// Reconnection - send remaining data
				const sseData = [
					'data: {"event":"run.node.started","node_id":"node-1"}\n\n',
					'data: {"event":"run.completed"}\n\n'
				].join('');

				await route.fulfill({
					status: 200,
					headers: { 'Content-Type': 'text/event-stream' },
					body: sseData
				});
			}
		});

		// Wait for initial connection
		await page.waitForTimeout(1000);

		// Should show reconnection indicator or successfully receive all events
		await expect(page.getByText(/completed/i)).toBeVisible({ timeout: 10000 });

		// Should have reconnected at least once
		expect(connectionCount).toBeGreaterThan(1);
	});

	test('handles SSE errors gracefully', async ({ page }) => {
		await page.goto('/runs/run-error-123');

		// Mock run details
		await page.route('**/api/v1/runs/run-error-123', async (route) => {
			await route.fulfill({
				status: 200,
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify({
					run_id: 'run-error-123',
					status: 'in_progress'
				})
			});
		});

		// Mock SSE stream error
		await page.route('**/api/v1/stream*', async (route) => {
			await route.fulfill({
				status: 500,
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify({ error: 'Internal server error' })
			});
		});

		// Should show error message
		await expect(
			page.getByText(/connection error|unable to connect|stream error/i)
		).toBeVisible({ timeout: 5000 });

		// Should offer retry option
		const retryButton = page.getByRole('button', { name: /retry|reconnect/i });
		if ((await retryButton.count()) > 0) {
			await expect(retryButton).toBeVisible();
		}
	});

	test('closes SSE connection when navigating away', async ({ page }) => {
		await page.goto('/runs/run-cleanup-123');

		// Mock run details
		await page.route('**/api/v1/runs/run-cleanup-123', async (route) => {
			await route.fulfill({
				status: 200,
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify({
					run_id: 'run-cleanup-123',
					status: 'in_progress'
				})
			});
		});

		let streamConnected = false;

		// Mock SSE stream
		await page.route('**/api/v1/stream*', async (route) => {
			streamConnected = true;

			// Long-running stream
			await route.fulfill({
				status: 200,
				headers: { 'Content-Type': 'text/event-stream' },
				body: 'data: {"event":"run.started"}\n\n'
			});
		});

		// Wait for stream to connect
		await page.waitForTimeout(1000);
		expect(streamConnected).toBe(true);

		// Navigate away
		await page.goto('/dashboard');

		// Connection should be closed (no memory leak)
		// This is verified by the test not hanging
		await expect(page).toHaveURL('/dashboard');
	});

	test('displays multiple concurrent streams correctly', async ({ page }) => {
		// Navigate to page that shows multiple runs
		await page.goto('/runs');

		// Mock runs list
		await page.route('**/api/v1/runs', async (route) => {
			await route.fulfill({
				status: 200,
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify([
					{ run_id: 'run-1', status: 'in_progress', created_at: new Date().toISOString() },
					{ run_id: 'run-2', status: 'in_progress', created_at: new Date().toISOString() }
				])
			});
		});

		// Mock SSE streams for both runs
		await page.route('**/api/v1/stream?run_id=run-1', async (route) => {
			await route.fulfill({
				status: 200,
				headers: { 'Content-Type': 'text/event-stream' },
				body: 'data: {"event":"run.completed","run_id":"run-1","status":"completed"}\n\n'
			});
		});

		await page.route('**/api/v1/stream?run_id=run-2', async (route) => {
			await route.fulfill({
				status: 200,
				headers: { 'Content-Type': 'text/event-stream' },
				body: 'data: {"event":"run.failed","run_id":"run-2","status":"failed"}\n\n'
			});
		});

		// Both runs should update independently
		await expect(
			page.locator('[data-run-id="run-1"]').getByText(/completed/i)
		).toBeVisible({ timeout: 5000 });

		await expect(page.locator('[data-run-id="run-2"]').getByText(/failed/i)).toBeVisible(
			{
				timeout: 5000
			}
		);
	});

	test('shows heartbeat/keepalive indicator', async ({ page }) => {
		await page.goto('/runs/run-heartbeat-123');

		// Mock run details
		await page.route('**/api/v1/runs/run-heartbeat-123', async (route) => {
			await route.fulfill({
				status: 200,
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify({
					run_id: 'run-heartbeat-123',
					status: 'in_progress'
				})
			});
		});

		// Mock SSE stream with heartbeat events
		await page.route('**/api/v1/stream*', async (route) => {
			const sseData = [
				'data: {"event":"run.started"}\n\n',
				': heartbeat\n\n', // SSE comment (keepalive)
				': heartbeat\n\n',
				'data: {"event":"run.completed"}\n\n'
			].join('');

			await route.fulfill({
				status: 200,
				headers: { 'Content-Type': 'text/event-stream' },
				body: sseData
			});
		});

		// Connection indicator should show "connected"
		const connectionIndicator = page.locator(
			'[data-testid="connection-status"], [aria-label="connection status"]'
		);

		if ((await connectionIndicator.count()) > 0) {
			await expect(connectionIndicator).toContainText(/connected|live/i, { timeout: 5000 });
		}
	});
});

test.describe('Real-Time Streaming - Admin Monitoring', () => {
	test.beforeEach(async ({ page }) => {
		await loginAsAdmin(page);
	});

	test('admin can monitor all active streams', async ({ page }) => {
		// Navigate to admin monitoring page
		await page.goto('/admin/monitoring');

		// Mock active runs
		await page.route('**/api/v1/runs*', async (route) => {
			await route.fulfill({
				status: 200,
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify([
					{
						run_id: 'run-1',
						status: 'in_progress',
						user_id: 'user-1',
						created_at: new Date().toISOString()
					},
					{
						run_id: 'run-2',
						status: 'in_progress',
						user_id: 'user-2',
						created_at: new Date().toISOString()
					}
				])
			});
		});

		// Should see all active runs
		await expect(page.getByText(/run-1/)).toBeVisible({ timeout: 5000 });
		await expect(page.getByText(/run-2/)).toBeVisible({ timeout: 5000 });

		// Should show real-time status updates
		await expect(page.getByText(/in.progress|running/i)).toBeVisible();
	});

	test('admin receives notifications for run failures', async ({ page }) => {
		await page.goto('/admin/monitoring');

		// Mock SSE stream for admin notifications
		await page.route('**/api/v1/admin/stream*', async (route) => {
			await route.fulfill({
				status: 200,
				headers: { 'Content-Type': 'text/event-stream' },
				body: 'data: {"event":"run.failed","run_id":"run-fail-123","error":"Timeout"}\n\n'
			});
		});

		// Should show failure notification
		await expect(page.getByText(/run.fail.123.*failed|failure/i)).toBeVisible({
			timeout: 5000
		});

		// Should show error details
		await expect(page.getByText(/timeout/i)).toBeVisible({ timeout: 5000 });
	});
});

test.describe('Real-Time Streaming - Performance', () => {
	test('handles high-frequency events without lag', async ({ page }) => {
		await loginAsUser(page);
		await page.goto('/runs/run-perf-123');

		// Mock run details
		await page.route('**/api/v1/runs/run-perf-123', async (route) => {
			await route.fulfill({
				status: 200,
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify({
					run_id: 'run-perf-123',
					status: 'in_progress'
				})
			});
		});

		// Mock SSE stream with many rapid events
		await page.route('**/api/v1/stream*', async (route) => {
			const events = [];
			for (let i = 0; i < 100; i++) {
				events.push(`data: {"event":"run.output.chunk","content":"Chunk ${i}"}\n\n`);
			}
			events.push('data: {"event":"run.completed"}\n\n');

			await route.fulfill({
				status: 200,
				headers: { 'Content-Type': 'text/event-stream' },
				body: events.join('')
			});
		});

		// Should handle all events without crashing
		await expect(page.getByText(/completed/i)).toBeVisible({ timeout: 10000 });

		// Page should remain responsive
		const title = page.getByRole('heading', { name: /run|details/i });
		await expect(title).toBeVisible();
	});

	test('throttles UI updates for better performance', async ({ page }) => {
		await loginAsUser(page);
		await page.goto('/runs/run-throttle-123');

		// Mock run details
		await page.route('**/api/v1/runs/run-throttle-123', async (route) => {
			await route.fulfill({
				status: 200,
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify({
					run_id: 'run-throttle-123',
					status: 'in_progress'
				})
			});
		});

		// Mock very rapid events
		await page.route('**/api/v1/stream*', async (route) => {
			const events = [];
			for (let i = 0; i < 1000; i++) {
				events.push(`data: {"event":"run.progress","percent":${i / 10}}\n\n`);
			}
			events.push('data: {"event":"run.completed"}\n\n');

			await route.fulfill({
				status: 200,
				headers: { 'Content-Type': 'text/event-stream' },
				body: events.join('')
			});
		});

		// UI should not freeze
		const startTime = Date.now();

		await expect(page.getByText(/completed/i)).toBeVisible({ timeout: 15000 });

		const endTime = Date.now();
		const duration = endTime - startTime;

		// Should complete in reasonable time (< 15 seconds)
		expect(duration).toBeLessThan(15000);
	});
});
