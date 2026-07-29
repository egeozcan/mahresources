import { test, expect } from '../fixtures/base.fixture';

test.describe('Dynamic Page Titles (WCAG 2.4.2)', () => {
  test('notes list page should have descriptive title', async ({ page }) => {
    await page.goto('/notes');
    await expect(page).toHaveTitle(/Notes/);
  });

  test('resources list page should have descriptive title', async ({ page }) => {
    await page.goto('/resources');
    await expect(page).toHaveTitle(/Resources/);
  });

  test('groups list page should have descriptive title', async ({ page }) => {
    await page.goto('/groups');
    await expect(page).toHaveTitle(/Groups/);
  });

  test('tags list page should have descriptive title', async ({ page }) => {
    await page.goto('/tags');
    await expect(page).toHaveTitle(/Tags/);
  });

  test('all pages should still contain app name in title', async ({ page }) => {
    await page.goto('/notes');
    await expect(page).toHaveTitle(/mahresources/);
  });
});
