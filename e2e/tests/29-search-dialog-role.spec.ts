import { test, expect } from '../fixtures/base.fixture';

test.describe('Global search dialog accessibility', () => {
  test('search dialog should have role="dialog" and aria-modal', async ({ page }) => {
    await page.goto('/');

    // Open the global search with Ctrl+K
    await page.keyboard.press('Control+k');

    // Wait for the search input to appear
    const searchInput = page.locator('input[aria-label="Search"]');
    await expect(searchInput).toBeVisible({ timeout: 3000 });

    // The search dialog container should have role="dialog" with aria-label="Search"
    const dialog = page.getByRole('dialog', { name: 'Search' });
    await expect(dialog).toBeVisible({ timeout: 3000 });

    // It should also have aria-modal="true"
    await expect(dialog).toHaveAttribute('aria-modal', 'true');
  });

  test('search dialog should have aria-label', async ({ page }) => {
    await page.goto('/');

    await page.keyboard.press('Control+k');

    const searchInput = page.locator('input[aria-label="Search"]');
    await expect(searchInput).toBeVisible({ timeout: 3000 });

    // The search dialog should have an accessible name
    const dialog = page.getByRole('dialog', { name: 'Search' });
    await expect(dialog).toBeVisible({ timeout: 3000 });

    const ariaLabel = await dialog.getAttribute('aria-label');
    expect(ariaLabel).toBe('Search');
  });
});
