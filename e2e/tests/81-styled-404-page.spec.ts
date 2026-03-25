import { test, expect } from '../fixtures/base.fixture';

test.describe('Styled 404 page', () => {
  test('should show styled 404 page with navigation for unknown URLs', async ({ page }) => {
    const response = await page.goto('/this-page-does-not-exist');
    expect(response?.status()).toBe(404);

    // Should have the app's navigation (not bare plain text)
    await expect(page.locator('nav')).toBeVisible();
  });
});
