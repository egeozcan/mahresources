/**
 * Bug: Tags and Categories list/timeline pages are missing the Sort UI
 * that all other entity list pages have.
 *
 * Tags: TagListContextProvider doesn't include sortValues in context,
 * and the templates don't include the sort partial.
 *
 * Categories: CategoryListContextProvider provides sortValues, but
 * the templates don't include the sort partial.
 */
import { test, expect } from '../fixtures/base.fixture';

test.describe('Sort UI present on tags and categories pages', () => {
  test('tags list page has Sort section in sidebar', async ({ page }) => {
    await page.goto('/tags');

    const sidebar = page.locator('aside, [role="complementary"]');
    await expect(sidebar.getByText('Sort', { exact: true })).toBeVisible();
    await expect(sidebar.locator('[aria-label="Sort options"]')).toBeVisible();
  });

  test('tags timeline page has Sort section in sidebar', async ({ page }) => {
    await page.goto('/tags/timeline');

    const sidebar = page.locator('aside, [role="complementary"]');
    await expect(sidebar.getByText('Sort', { exact: true })).toBeVisible();
    await expect(sidebar.locator('[aria-label="Sort options"]')).toBeVisible();
  });

  test('categories list page has Sort section in sidebar', async ({ page }) => {
    await page.goto('/categories');

    const sidebar = page.locator('aside, [role="complementary"]');
    await expect(sidebar.getByText('Sort', { exact: true })).toBeVisible();
    await expect(sidebar.locator('[aria-label="Sort options"]')).toBeVisible();
  });

  test('categories timeline page has Sort section in sidebar', async ({ page }) => {
    await page.goto('/categories/timeline');

    const sidebar = page.locator('aside, [role="complementary"]');
    await expect(sidebar.getByText('Sort', { exact: true })).toBeVisible();
    await expect(sidebar.locator('[aria-label="Sort options"]')).toBeVisible();
  });
});
