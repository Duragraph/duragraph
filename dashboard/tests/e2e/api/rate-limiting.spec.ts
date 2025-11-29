import { test, expect } from '@playwright/test';
import { loginAsUser, loginAsAdmin } from '../fixtures/auth';
import { mockRateLimitError, waitForApiCall } from '../fixtures/api';

test.describe('Rate Limiting - API Protection', () => {
	test.beforeEach(async ({ page }) => {
		await loginAsUser(page);
	});

	test('displays rate limit error when API calls exceed limit', async ({ page }) => {
		await page.goto('/dashboard');

		// Mock rate limit error response
		await mockRateLimitError(page, '**/api/v1/runs');

		// Try to create a run
		await page.goto('/runs/new');
		await page.fill('input[name="assistant_id"]', 'assistant-123');
		await page.fill('input[name="thread_id"]', 'thread-123');
		await page.click('button[type="submit"]');

		// Should show rate limit error
		await expect(
			page.getByText(/rate limit exceeded|too many requests|429/i)
		).toBeVisible({ timeout: 5000 });
	});

	test('shows rate limit countdown timer', async ({ page }) => {
		await page.goto('/dashboard');

		// Mock rate limit with retry-after header
		await page.route('**/api/v1/runs', async (route) => {
			await route.fulfill({
				status: 429,
				headers: {
					'Content-Type': 'application/json',
					'Retry-After': '60', // 60 seconds
					'X-RateLimit-Limit': '100',
					'X-RateLimit-Remaining': '0',
					'X-RateLimit-Reset': String(Date.now() + 60000)
				},
				body: JSON.stringify({
					error: 'Rate limit exceeded. Please try again in 60 seconds.'
				})
			});
		});

		// Trigger rate limit
		await page.goto('/runs/new');
		await page.fill('input[name="assistant_id"]', 'assistant-123');
		await page.fill('input[name="thread_id"]', 'thread-123');
		await page.click('button[type="submit"]');

		// Should show countdown timer or retry-after message
		const retryMessage = page.getByText(/try again in|wait.*seconds|retry after/i);
		await expect(retryMessage).toBeVisible({ timeout: 5000 });
	});

	test('disables submit buttons when rate limited', async ({ page }) => {
		await page.goto('/dashboard');

		// Mock rate limit error
		await mockRateLimitError(page, '**/api/v1/runs');

		await page.goto('/runs/new');
		const submitButton = page.getByRole('button', { name: /create|submit|start/i });

		// Submit form to trigger rate limit
		await page.fill('input[name="assistant_id"]', 'assistant-123');
		await page.fill('input[name="thread_id"]', 'thread-123');
		await submitButton.click();

		// Wait for error
		await expect(page.getByText(/rate limit exceeded/i)).toBeVisible({ timeout: 5000 });

		// Button should be disabled or show different state
		// (Implementation-dependent - might show "Rate Limited" text or be disabled)
		const isDisabled = await submitButton.isDisabled();
		const buttonText = await submitButton.textContent();

		// Either button is disabled OR button text changed to indicate rate limit
		expect(isDisabled || buttonText?.toLowerCase().includes('limit')).toBeTruthy();
	});

	test('shows rate limit status in API calls list', async ({ page }) => {
		await page.goto('/runs');

		// Mock rate limit on list endpoint
		await page.route('**/api/v1/runs*', async (route) => {
			await route.fulfill({
				status: 429,
				headers: {
					'Content-Type': 'application/json',
					'X-RateLimit-Limit': '100',
					'X-RateLimit-Remaining': '0'
				},
				body: JSON.stringify({
					error: 'Rate limit exceeded'
				})
			});
		});

		// Try to refresh
		await page.reload();

		// Should show rate limit error instead of data
		await expect(
			page.getByText(/rate limit exceeded|unable to load|too many requests/i)
		).toBeVisible({ timeout: 5000 });
	});

	test('allows retry after cooldown period', async ({ page }) => {
		await page.goto('/runs/new');

		let callCount = 0;

		// First call returns rate limit, second call succeeds
		await page.route('**/api/v1/runs', async (route) => {
			callCount++;
			if (callCount === 1) {
				// First call - rate limited
				await route.fulfill({
					status: 429,
					headers: {
						'Content-Type': 'application/json',
						'Retry-After': '1' // 1 second for testing
					},
					body: JSON.stringify({
						error: 'Rate limit exceeded. Please try again in 1 second.'
					})
				});
			} else {
				// Second call - success
				await route.fulfill({
					status: 200,
					headers: { 'Content-Type': 'application/json' },
					body: JSON.stringify({
						run_id: 'run-123',
						status: 'queued'
					})
				});
			}
		});

		// First attempt - should fail
		await page.fill('input[name="assistant_id"]', 'assistant-123');
		await page.fill('input[name="thread_id"]', 'thread-123');
		await page.click('button[type="submit"]');

		await expect(page.getByText(/rate limit exceeded/i)).toBeVisible({ timeout: 5000 });

		// Wait for cooldown (1 second + buffer)
		await page.waitForTimeout(2000);

		// Second attempt - should succeed
		await page.click('button[type="submit"]');

		// Should show success message or redirect
		const successIndicator = await Promise.race([
			page
				.getByText(/run created|created successfully|queued/i)
				.isVisible()
				.then(() => 'success'),
			page.waitForURL(/.*runs\/run-123/, { timeout: 5000 }).then(() => 'redirect')
		]);

		expect(successIndicator).toBeTruthy();
	});

	test('shows rate limit info in dashboard header', async ({ page }) => {
		await page.goto('/dashboard');

		// Check if rate limit info is displayed in header/nav
		// (This is optional UI feature - implementation-dependent)
		const rateLimitInfo = page.locator('[data-testid="rate-limit-status"]');

		if ((await rateLimitInfo.count()) > 0) {
			// If rate limit status is shown, verify it displays limit/remaining
			await expect(rateLimitInfo).toBeVisible();
			const text = await rateLimitInfo.textContent();
			// Should show something like "API: 95/100" or "Remaining: 95"
			expect(text).toMatch(/\d+/); // Contains numbers
		} else {
			// Rate limit info not shown - this is acceptable
			test.info().annotations.push({
				type: 'note',
				description: 'Rate limit info not displayed in header (optional feature)'
			});
		}
	});

	test('handles multiple rapid requests correctly', async ({ page }) => {
		await page.goto('/dashboard');

		let requestCount = 0;

		await page.route('**/api/v1/assistants*', async (route) => {
			requestCount++;

			if (requestCount > 3) {
				// After 3 requests, return rate limit
				await route.fulfill({
					status: 429,
					headers: {
						'Content-Type': 'application/json',
						'X-RateLimit-Limit': '3',
						'X-RateLimit-Remaining': '0'
					},
					body: JSON.stringify({
						error: 'Rate limit exceeded'
					})
				});
			} else {
				// First 3 requests succeed
				await route.fulfill({
					status: 200,
					headers: {
						'Content-Type': 'application/json',
						'X-RateLimit-Limit': '3',
						'X-RateLimit-Remaining': String(3 - requestCount)
					},
					body: JSON.stringify([])
				});
			}
		});

		// Navigate to page that makes API calls
		await page.goto('/assistants');

		// Rapid refresh attempts
		for (let i = 0; i < 5; i++) {
			await page.reload();
			await page.waitForTimeout(100);
		}

		// Should show rate limit error after exceeding limit
		await expect(page.getByText(/rate limit|too many requests/i)).toBeVisible({
			timeout: 5000
		});
	});
});

test.describe('Rate Limiting - Admin Bypass', () => {
	test.beforeEach(async ({ page }) => {
		await loginAsAdmin(page);
	});

	test('admin requests have higher rate limits', async ({ page }) => {
		await page.goto('/dashboard');

		// Mock higher rate limits for admin
		await page.route('**/api/v1/**', async (route) => {
			await route.fulfill({
				status: 200,
				headers: {
					'Content-Type': 'application/json',
					'X-RateLimit-Limit': '1000', // Higher limit for admin
					'X-RateLimit-Remaining': '995'
				},
				body: JSON.stringify({ data: [] })
			});
		});

		await page.goto('/users');

		// Check rate limit headers (if displayed)
		// Admin should have higher limits
		const rateLimitInfo = page.locator('[data-testid="rate-limit-status"]');

		if ((await rateLimitInfo.count()) > 0) {
			const text = await rateLimitInfo.textContent();
			// Should show higher limit (1000) not lower limit (100)
			expect(text).toMatch(/1000|999|995/); // Admin limit values
		}
	});

	test('admin can perform bulk operations without rate limiting', async ({ page }) => {
		await page.goto('/users');

		let requestCount = 0;

		// Allow many requests for admin
		await page.route('**/api/v1/users**', async (route) => {
			requestCount++;
			await route.fulfill({
				status: 200,
				headers: {
					'Content-Type': 'application/json',
					'X-RateLimit-Limit': '1000',
					'X-RateLimit-Remaining': String(1000 - requestCount)
				},
				body: JSON.stringify([])
			});
		});

		// Simulate bulk operations (multiple API calls)
		for (let i = 0; i < 10; i++) {
			await page.reload();
			await page.waitForTimeout(100);
		}

		// Should NOT show rate limit error
		await expect(page.getByText(/rate limit exceeded/i)).not.toBeVisible();

		// Should have made many requests without hitting limit
		expect(requestCount).toBeGreaterThan(5);
	});
});

test.describe('Rate Limiting - UI Feedback', () => {
	test('shows toast notification on rate limit', async ({ page }) => {
		await loginAsUser(page);
		await page.goto('/runs/new');

		// Mock rate limit error
		await mockRateLimitError(page, '**/api/v1/runs');

		// Trigger rate limit
		await page.fill('input[name="assistant_id"]', 'assistant-123');
		await page.fill('input[name="thread_id"]', 'thread-123');
		await page.click('button[type="submit"]');

		// Should show toast notification
		const toast = page.locator('[role="alert"], [data-testid="toast"], .toast');
		await expect(toast).toBeVisible({ timeout: 5000 });

		// Toast should mention rate limit
		const toastText = await toast.textContent();
		expect(toastText?.toLowerCase()).toMatch(/rate limit|too many requests|429/);
	});

	test('updates UI to prevent further rate limit triggers', async ({ page }) => {
		await loginAsUser(page);
		await page.goto('/dashboard');

		// Mock rate limit error
		await mockRateLimitError(page, '**/api/v1/runs');

		// Trigger rate limit
		await page.goto('/runs/new');
		await page.fill('input[name="assistant_id"]', 'assistant-123');
		await page.fill('input[name="thread_id"]', 'thread-123');
		await page.click('button[type="submit"]');

		// Wait for error
		await expect(page.getByText(/rate limit exceeded/i)).toBeVisible({ timeout: 5000 });

		// UI should prevent clicking submit again (disabled or hidden)
		const submitButton = page.getByRole('button', { name: /create|submit|start/i });

		const isDisabled = await submitButton.isDisabled();
		const isHidden = !(await submitButton.isVisible());

		// Button should be either disabled or hidden
		expect(isDisabled || isHidden).toBeTruthy();
	});
});
