import { test, expect } from '@playwright/test';
import { loginAs, loginAsAdmin, logout, isAuthenticated } from '../fixtures/auth';

test.describe('Authentication - Login', () => {
	test.beforeEach(async ({ page }) => {
		// Ensure we start from a logged-out state
		await page.goto('/');
	});

	test('successful login redirects to dashboard', async ({ page }) => {
		// Navigate to login page
		await page.goto('/login');

		// Fill in credentials
		await page.fill('input[name="email"]', 'admin@duragraph.dev');
		await page.fill('input[name="password"]', 'admin123');

		// Submit form
		await page.click('button[type="submit"]');

		// Should redirect to dashboard
		await expect(page).toHaveURL('/dashboard', { timeout: 10000 });

		// Should see welcome message or dashboard heading
		await expect(
			page.getByRole('heading', { name: /dashboard|welcome/i })
		).toBeVisible();

		// Should have auth token in cookies
		const cookies = await page.context().cookies();
		const authCookie = cookies.find((c) => c.name === 'auth_token' || c.name === 'session');
		expect(authCookie).toBeDefined();
	});

	test('invalid credentials show error message', async ({ page }) => {
		await page.goto('/login');

		// Fill in wrong credentials
		await page.fill('input[name="email"]', 'wrong@example.com');
		await page.fill('input[name="password"]', 'wrongpassword');
		await page.click('button[type="submit"]');

		// Should stay on login page
		await expect(page).toHaveURL(/.*login/);

		// Should show error message
		await expect(page.getByText(/invalid credentials|incorrect email or password/i)).toBeVisible({
			timeout: 5000
		});

		// Should NOT have auth token
		const authenticated = await isAuthenticated(page);
		expect(authenticated).toBe(false);
	});

	test('empty email shows validation error', async ({ page }) => {
		await page.goto('/login');

		// Submit without email
		await page.fill('input[name="password"]', 'password123');
		await page.click('button[type="submit"]');

		// Should show validation error
		await expect(
			page.getByText(/email is required|please enter your email/i)
		).toBeVisible();
	});

	test('empty password shows validation error', async ({ page }) => {
		await page.goto('/login');

		// Submit without password
		await page.fill('input[name="email"]', 'user@example.com');
		await page.click('button[type="submit"]');

		// Should show validation error
		await expect(
			page.getByText(/password is required|please enter your password/i)
		).toBeVisible();
	});

	test('logout clears session and redirects to login', async ({ page }) => {
		// Login first
		await loginAsAdmin(page);

		// Verify we're logged in
		await expect(page).toHaveURL('/dashboard');

		// Logout
		await logout(page);

		// Should redirect to login
		await expect(page).toHaveURL('/login');

		// Auth token should be cleared
		const authenticated = await isAuthenticated(page);
		expect(authenticated).toBe(false);

		// Trying to access dashboard should redirect to login
		await page.goto('/dashboard');
		await expect(page).toHaveURL(/.*login/);
	});

	test('remember me checkbox persists session', async ({ page }) => {
		await page.goto('/login');

		await page.fill('input[name="email"]', 'user@duragraph.dev');
		await page.fill('input[name="password"]', 'user123');

		// Check "Remember me" if it exists
		const rememberMeCheckbox = page.locator('input[name="remember"]');
		if (await rememberMeCheckbox.count() > 0) {
			await rememberMeCheckbox.check();
		}

		await page.click('button[type="submit"]');

		await expect(page).toHaveURL('/dashboard');

		// Close and reopen page (simulates new session)
		await page.close();
		const context = page.context();
		const newPage = await context.newPage();

		// Should still be authenticated
		await newPage.goto('/dashboard');
		await expect(newPage).toHaveURL('/dashboard', { timeout: 5000 });
	});

	test('unauthorized user cannot access protected routes', async ({ page }) => {
		// Try to access dashboard without login
		await page.goto('/dashboard');

		// Should redirect to login
		await expect(page).toHaveURL(/.*login/);

		// Should show message asking to log in
		await expect(
			page.getByText(/please log in|sign in to continue/i)
		).toBeVisible({ timeout: 5000 });
	});

	test('session expiry redirects to login', async ({ page }) => {
		// Login
		await loginAsAdmin(page);

		// Manually expire the session (delete auth cookie)
		const context = page.context();
		await context.clearCookies();

		// Try to access protected route
		await page.goto('/runs');

		// Should redirect to login
		await expect(page).toHaveURL(/.*login/, { timeout: 10000 });

		// Should show session expired message
		await expect(
			page.getByText(/session expired|please log in again/i)
		).toBeVisible({ timeout: 5000 });
	});
});
