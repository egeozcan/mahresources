import { test, expect, loginAs } from '../../fixtures/auth.fixture';

test.describe('auth: login flow', () => {
  test('rejects bad credentials with an error', async ({ page, authSeed }) => {
    await page.goto('/login');
    await page.fill('input[name="username"]', authSeed.admin.username);
    await page.fill('input[name="password"]', 'definitely-wrong');
    await page.click('button[type="submit"]');
    await expect(page).toHaveURL(/\/login/);
    await expect(page.locator('[role="alert"]')).toContainText(/invalid/i);
  });

  test('admin logs in, sees its identity, and can log out', async ({ page, authSeed }) => {
    await loginAs(page, authSeed.admin);
    await expect(page).not.toHaveURL(/\/login/);
    // The account control in the header shows the signed-in username.
    await expect(page.locator('.account')).toContainText(authSeed.admin.username);

    // Signing out from the account menu posts the form, clears the session and
    // returns to the login page.
    await page.getByRole('button', { name: 'Account menu' }).click();
    await page.getByRole('button', { name: 'Sign out' }).click();
    await expect(page).toHaveURL(/\/login/);
    await page.goto('/dashboard');
    await expect(page).toHaveURL(/\/login/);
  });

  test('an unauthenticated visit to a protected page redirects to /login', async ({ page }) => {
    await page.goto('/groups');
    await expect(page).toHaveURL(/\/login/);
  });
});

test.describe('auth: signing out without JavaScript', () => {
  test.use({ javaScriptEnabled: false });

  test('visiting /logout asks before signing out', async ({ page, authSeed }) => {
    await loginAs(page, authSeed.admin);
    await expect(page).not.toHaveURL(/\/login/);

    // A link to /logout, from this site or any other, only shows the question.
    await page.goto('/logout');
    await expect(page.getByRole('heading', { name: 'Sign out' })).toBeVisible();
    await page.goto('/dashboard');
    await expect(page).not.toHaveURL(/\/login/);

    await page.goto('/logout');
    await page.getByRole('button', { name: 'Sign out' }).click();
    await expect(page).toHaveURL(/\/login/);
    await page.goto('/dashboard');
    await expect(page).toHaveURL(/\/login/);
  });
});
