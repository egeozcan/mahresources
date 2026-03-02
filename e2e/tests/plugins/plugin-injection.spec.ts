import { test, expect } from '../../fixtures/base.fixture';

test.describe('Plugin Injection', () => {
  test.beforeEach(async ({ apiClient }) => {
    await apiClient.savePluginSettings('test-banner', {
      banner_text: 'Plugin Banner Active',
      api_key: 'test-key-123',
      show_banner: true,
      mode: 'simple',
      count: 5,
    });
    await apiClient.enablePlugin('test-banner');
  });

  test.afterEach(async ({ apiClient }) => {
    try {
      await apiClient.disablePlugin('test-banner');
    } catch {
      // Ignore if already disabled
    }
  });

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
