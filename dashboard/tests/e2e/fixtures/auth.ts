import { Page } from '@playwright/test';

/**
 * Login helper for E2E tests
 */
export async function loginAs(
	page: Page,
	email: string,
	password: string = 'password123'
): Promise<void> {
	await page.goto('/login');

	await page.fill('input[name="email"]', email);
	await page.fill('input[name="password"]', password);
	await page.click('button[type="submit"]');

	// Wait for navigation to dashboard
	await page.waitForURL('/dashboard', { timeout: 10000 });
}

/**
 * Login as admin user
 */
export async function loginAsAdmin(page: Page): Promise<void> {
	await loginAs(page, 'admin@duragraph.dev', 'admin123');
}

/**
 * Login as regular user
 */
export async function loginAsUser(page: Page): Promise<void> {
	await loginAs(page, 'user@duragraph.dev', 'user123');
}

/**
 * Logout current user
 */
export async function logout(page: Page): Promise<void> {
	// Click user menu
	await page.click('[data-testid="user-menu"]');

	// Click logout button
	await page.click('button:has-text("Logout")');

	// Wait for redirect to login
	await page.waitForURL('/login', { timeout: 5000 });
}

/**
 * Check if user is authenticated
 */
export async function isAuthenticated(page: Page): Promise<boolean> {
	const cookies = await page.context().cookies();
	const authCookie = cookies.find((c) => c.name === 'auth_token' || c.name === 'session');
	return !!authCookie;
}

/**
 * Get auth token from cookies
 */
export async function getAuthToken(page: Page): Promise<string | undefined> {
	const cookies = await page.context().cookies();
	const authCookie = cookies.find((c) => c.name === 'auth_token' || c.name === 'session');
	return authCookie?.value;
}
