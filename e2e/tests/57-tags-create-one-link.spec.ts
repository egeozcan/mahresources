/**
 * Tests that the "Create one" link in list empty states navigates to valid
 * creation pages, not 404s.
 *
 * Bug: The tags list template used href="/createTag" and categories used
 * href="/createCategory" — but the correct routes are "/tag/new" and
 * "/category/new". The old URLs returned 404.
 */
import { test, expect } from '../fixtures/base.fixture';

test.describe('Empty-state "Create one" links point to correct URLs', () => {
  test('tags list "Create one" link should use /tag/new', async ({
    page,
  }) => {
    // Navigate to the tags list — on a fresh ephemeral DB this is empty
    await page.goto('/tags');
    await page.waitForLoadState('load');

    // If there are tags (from parallel tests), the link won't exist — skip
    const createOneLink = page.getByRole('link', { name: 'Create one' });
    const isVisible = await createOneLink.isVisible().catch(() => false);
    if (!isVisible) {
      test.skip();
      return;
    }

    // Verify the href is correct (not the old broken /createTag)
    await expect(createOneLink).toHaveAttribute('href', '/tag/new');

    // Click and verify navigation works
    await createOneLink.click();
    await page.waitForLoadState('load');
    await expect(page).not.toHaveURL(/createTag/);
    await expect(page.locator('body')).not.toContainText('404 page not found');
    await expect(
      page.getByRole('textbox', { name: /name/i }),
    ).toBeVisible({ timeout: 3000 });
  });

  test('categories list "Create one" link should use /category/new', async ({
    page,
  }) => {
    await page.goto('/categories');
    await page.waitForLoadState('load');

    const createOneLink = page.getByRole('link', { name: 'Create one' });
    const isVisible = await createOneLink.isVisible().catch(() => false);
    if (!isVisible) {
      test.skip();
      return;
    }

    await expect(createOneLink).toHaveAttribute('href', '/category/new');

    await createOneLink.click();
    await page.waitForLoadState('load');
    await expect(page).not.toHaveURL(/createCategory/);
    await expect(page.locator('body')).not.toContainText('404 page not found');
    await expect(
      page.getByRole('textbox', { name: /name/i }),
    ).toBeVisible({ timeout: 3000 });
  });
});
