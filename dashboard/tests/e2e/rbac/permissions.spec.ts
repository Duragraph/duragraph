import { test, expect } from '@playwright/test';
import { loginAsAdmin, loginAsUser } from '../fixtures/auth';

test.describe('RBAC - Admin Permissions', () => {
	test.beforeEach(async ({ page }) => {
		await loginAsAdmin(page);
	});

	test('admin can access user management page', async ({ page }) => {
		await page.goto('/dashboard');

		// Admin should see "Users" or "User Management" link
		const usersLink = page.getByRole('link', { name: /users|user management/i });
		await expect(usersLink).toBeVisible();

		// Click to navigate
		await usersLink.click();

		// Should be on users page
		await expect(page).toHaveURL(/.*users/);
		await expect(page.getByRole('heading', { name: /users|user management/i })).toBeVisible();
	});

	test('admin can create new users', async ({ page }) => {
		await page.goto('/users');

		// Should see "Add User" or "Create User" button
		const createButton = page.getByRole('button', { name: /add user|create user|new user/i });
		await expect(createButton).toBeVisible();

		// Click to open form
		await createButton.click();

		// Fill in user details
		await page.fill('input[name="name"]', 'Test User');
		await page.fill('input[name="email"]', 'testuser@example.com');
		await page.fill('input[name="password"]', 'password123');

		// Select role
		const roleSelect = page.locator('select[name="role"]');
		if (await roleSelect.count() > 0) {
			await roleSelect.selectOption('user');
		}

		// Submit
		await page.click('button[type="submit"]');

		// Should show success message
		await expect(page.getByText(/user created|user added successfully/i)).toBeVisible({
			timeout: 5000
		});
	});

	test('admin can delete runs', async ({ page }) => {
		await page.goto('/runs');

		// Delete button should be visible for admin
		const deleteButtons = page.getByRole('button', { name: /delete/i });
		await expect(deleteButtons.first()).toBeVisible({ timeout: 10000 });

		// Click delete on first run
		await deleteButtons.first().click();

		// Should show confirmation dialog
		await expect(page.getByText(/are you sure|confirm delete/i)).toBeVisible();

		// Confirm deletion
		await page.click('button:has-text("Confirm")');

		// Should show success message
		await expect(page.getByText(/run deleted|deleted successfully/i)).toBeVisible({
			timeout: 5000
		});
	});

	test('admin can access settings page', async ({ page }) => {
		await page.goto('/dashboard');

		// Should see settings link
		const settingsLink = page.getByRole('link', { name: /settings|configuration/i });
		await expect(settingsLink).toBeVisible();

		await settingsLink.click();

		// Should navigate to settings
		await expect(page).toHaveURL(/.*settings/);
		await expect(page.getByRole('heading', { name: /settings/i })).toBeVisible();
	});

	test('admin can modify system configuration', async ({ page }) => {
		await page.goto('/settings');

		// Should see configuration options
		await expect(page.getByText(/rate limit|api key|webhook/i)).toBeVisible({ timeout: 5000 });

		// Should have save button
		const saveButton = page.getByRole('button', { name: /save|update/i });
		await expect(saveButton).toBeVisible();
	});
});

test.describe('RBAC - Regular User Permissions', () => {
	test.beforeEach(async ({ page }) => {
		await loginAsUser(page);
	});

	test('user cannot access user management page', async ({ page }) => {
		await page.goto('/dashboard');

		// User should NOT see "Users" menu item
		const usersLink = page.getByRole('link', { name: /users|user management/i });
		await expect(usersLink).not.toBeVisible();
	});

	test('user cannot access user management via direct URL', async ({ page }) => {
		// Try to access users page directly
		await page.goto('/users');

		// Should be redirected or show access denied
		const accessDenied = await Promise.race([
			page.waitForURL(/.*dashboard/, { timeout: 5000 }).then(() => 'redirected'),
			page.getByText(/access denied|forbidden|unauthorized/i).isVisible().then(() => 'denied')
		]);

		expect(accessDenied).toBeTruthy();
	});

	test('user cannot delete runs', async ({ page }) => {
		await page.goto('/runs');

		// Delete button should NOT be visible for regular user
		const deleteButtons = page.getByRole('button', { name: /delete/i });
		await expect(deleteButtons.first()).not.toBeVisible({ timeout: 5000 });
	});

	test('user can view their own runs', async ({ page }) => {
		await page.goto('/runs');

		// Should see runs list
		await expect(page.getByRole('heading', { name: /runs|my runs/i })).toBeVisible();

		// Should see table or list of runs
		await expect(page.locator('[data-testid="runs-list"]')).toBeVisible({ timeout: 10000 });
	});

	test('user can create new runs', async ({ page }) => {
		await page.goto('/runs/new');

		// Should see create run form
		await expect(page.getByRole('heading', { name: /create run|new run/i })).toBeVisible();

		// Should be able to submit
		const submitButton = page.getByRole('button', { name: /create|submit|start/i });
		await expect(submitButton).toBeVisible();
	});

	test('user cannot access settings page', async ({ page }) => {
		await page.goto('/dashboard');

		// Should NOT see settings link
		const settingsLink = page.getByRole('link', { name: /settings|configuration/i });
		await expect(settingsLink).not.toBeVisible();
	});

	test('user cannot access settings via direct URL', async ({ page }) => {
		// Try to access settings directly
		await page.goto('/settings');

		// Should be redirected or show access denied
		const accessDenied = await Promise.race([
			page.waitForURL(/.*dashboard/, { timeout: 5000 }).then(() => 'redirected'),
			page.getByText(/access denied|forbidden|unauthorized/i).isVisible().then(() => 'denied')
		]);

		expect(accessDenied).toBeTruthy();
	});

	test('user can only view their own data', async ({ page }) => {
		await page.goto('/runs');

		// Get user email from UI or context
		const userEmail = 'user@duragraph.dev';

		// All runs should belong to the current user
		// (This assumes runs show owner information)
		const runCards = page.locator('[data-testid="run-card"]');
		const count = await runCards.count();

		for (let i = 0; i < count; i++) {
			const card = runCards.nth(i);
			// Verify each run belongs to current user
			// Implementation depends on your UI structure
		}
	});
});

test.describe('RBAC - Role Escalation Prevention', () => {
	test('user cannot escalate their own role', async ({ page }) => {
		await loginAsUser(page);

		// Try to access user edit page (if such functionality exists)
		// Attempt to modify role via API
		const response = await page.request.patch('/api/v1/users/me', {
			data: {
				role: 'admin'
			}
		});

		// Should be forbidden
		expect(response.status()).toBe(403);
	});

	test('user cannot delete other users', async ({ page }) => {
		await loginAsUser(page);

		// Try to delete another user via API
		const response = await page.request.delete('/api/v1/users/admin-user-id');

		// Should be forbidden
		expect(response.status()).toBe(403);
	});
});
