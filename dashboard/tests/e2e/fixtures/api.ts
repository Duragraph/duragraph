import { Page } from '@playwright/test';

/**
 * API helper for E2E tests
 * Provides utilities for mocking API responses and waiting for API calls
 */

export interface MockApiOptions {
	status?: number;
	body?: any;
	delay?: number;
}

/**
 * Mock a successful API response
 */
export async function mockApiSuccess(
	page: Page,
	url: string,
	body: any,
	options: MockApiOptions = {}
): Promise<void> {
	await page.route(url, async (route) => {
		if (options.delay) {
			await new Promise((resolve) => setTimeout(resolve, options.delay));
		}

		await route.fulfill({
			status: options.status || 200,
			contentType: 'application/json',
			body: JSON.stringify(body)
		});
	});
}

/**
 * Mock an API error response
 */
export async function mockApiError(
	page: Page,
	url: string,
	status: number,
	message: string
): Promise<void> {
	await page.route(url, async (route) => {
		await route.fulfill({
			status,
			contentType: 'application/json',
			body: JSON.stringify({ error: message })
		});
	});
}

/**
 * Mock rate limit error (429)
 */
export async function mockRateLimitError(page: Page, url: string): Promise<void> {
	await mockApiError(page, url, 429, 'Rate limit exceeded. Try again in 60 seconds.');
}

/**
 * Mock unauthorized error (401)
 */
export async function mockUnauthorizedError(page: Page, url: string): Promise<void> {
	await mockApiError(page, url, 401, 'Unauthorized');
}

/**
 * Mock forbidden error (403)
 */
export async function mockForbiddenError(page: Page, url: string): Promise<void> {
	await mockApiError(page, url, 403, 'Forbidden: Insufficient permissions');
}

/**
 * Wait for API call to complete
 */
export async function waitForApiCall(page: Page, urlPattern: string | RegExp): Promise<void> {
	await page.waitForResponse((response) => {
		const url = response.url();
		if (typeof urlPattern === 'string') {
			return url.includes(urlPattern);
		}
		return urlPattern.test(url);
	});
}

/**
 * Intercept and capture API request
 */
export async function captureApiRequest(
	page: Page,
	urlPattern: string | RegExp
): Promise<any> {
	return new Promise((resolve) => {
		page.on('request', (request) => {
			const url = request.url();
			const matches =
				typeof urlPattern === 'string' ? url.includes(urlPattern) : urlPattern.test(url);

			if (matches) {
				resolve({
					url: request.url(),
					method: request.method(),
					headers: request.headers(),
					postData: request.postData()
				});
			}
		});
	});
}
