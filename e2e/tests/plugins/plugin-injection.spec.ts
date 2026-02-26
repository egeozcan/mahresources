import { test, expect } from '../../fixtures/base.fixture';

test.describe('Plugin Injection', () => {
  test('should display plugin banner on resources page', async ({ page }) => {
    await page.goto('/resources');
    await page.waitForLoadState('load');
    const banner = page.getByTestId('plugin-banner');
    await expect(banner).toBeVisible();
    await expect(banner).toContainText('Plugin Banner Active');
  });

  test('should display plugin banner on notes page', async ({ page }) => {
    await page.goto('/notes');
    await page.waitForLoadState('load');
    const banner = page.getByTestId('plugin-banner');
    await expect(banner).toBeVisible();
    await expect(banner).toContainText('Plugin Banner Active');
  });

  test('should display plugin banner on groups page', async ({ page }) => {
    await page.goto('/groups');
    await page.waitForLoadState('load');
    const banner = page.getByTestId('plugin-banner');
    await expect(banner).toBeVisible();
    await expect(banner).toContainText('Plugin Banner Active');
  });
});
