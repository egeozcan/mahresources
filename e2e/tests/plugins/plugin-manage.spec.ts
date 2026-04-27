import { test, expect } from '../../fixtures/base.fixture';

test.describe('Plugin Management', () => {
  test.beforeEach(async ({ apiClient }) => {
    // Ensure plugin is disabled at test start
    try {
      await apiClient.disablePlugin('test-banner');
    } catch {
      // Ignore if already disabled
    }
  });

  test('management page shows discovered plugins', async ({ page }) => {
    await page.goto('/plugins/manage');
    await page.waitForLoadState('load');
    const card = page.getByTestId('plugin-card-test-banner');
    await expect(card).toBeVisible();
    await expect(card).toContainText('test-banner');
    await expect(card).toContainText('v1.0');
  });

  test('management page shows settings form', async ({ page }) => {
    await page.goto('/plugins/manage');
    await page.waitForLoadState('load');
    const form = page.getByTestId('plugin-settings-test-banner');
    await expect(form).toBeVisible();
    await expect(form.getByTestId('setting-banner_text')).toBeVisible();
    await expect(form.getByTestId('setting-api_key')).toBeVisible();
    await expect(form.getByTestId('setting-show_banner')).toBeVisible();
    await expect(form.getByTestId('setting-mode')).toBeVisible();
    await expect(form.getByTestId('setting-count')).toBeVisible();
  });

  test('can enable a plugin after configuring required settings', async ({ page, apiClient }) => {
    await apiClient.savePluginSettings('test-banner', {
      api_key: 'my-test-key',
      banner_text: 'Test Banner',
      show_banner: true,
      mode: 'simple',
      count: 5,
    });

    await page.goto('/plugins/manage');
    await page.waitForLoadState('load');

    const enableButton = page.getByTestId('plugin-toggle-test-banner');
    await enableButton.click();
    await page.waitForLoadState('load');

    const disableButton = page.getByTestId('plugin-toggle-test-banner');
    await expect(disableButton).toContainText('Disable');
  });

  test('disabled plugin does not inject banner', async ({ page, apiClient }) => {
    // Disable right before navigating to minimize the race window with parallel
    // plugin test files that may re-enable the plugin between beforeEach and here.
    await apiClient.disablePlugin('test-banner');
    await page.goto('/notes');
    await page.waitForLoadState('load');
    await expect(page.getByTestId('plugin-banner')).not.toBeVisible();
  });

  test('enabled plugin injects banner', async ({ page, apiClient }) => {
    await apiClient.savePluginSettings('test-banner', {
      api_key: 'key',
      banner_text: 'My Custom Banner',
      show_banner: true,
      mode: 'simple',
      count: 5,
    });
    await apiClient.enablePlugin('test-banner');

    await page.goto('/notes');
    await page.waitForLoadState('load');
    const banner = page.getByTestId('plugin-banner');
    await expect(banner).toBeVisible();
    await expect(banner).toContainText('My Custom Banner');
  });

  test('disabling plugin removes banner', async ({ page, apiClient }) => {
    await apiClient.savePluginSettings('test-banner', {
      api_key: 'key',
      banner_text: 'Banner',
      show_banner: true,
      mode: 'simple',
      count: 5,
    });
    await apiClient.enablePlugin('test-banner');

    await page.goto('/notes');
    await page.waitForLoadState('load');
    await expect(page.getByTestId('plugin-banner')).toBeVisible();

    await apiClient.disablePlugin('test-banner');

    await page.reload();
    await page.waitForLoadState('load');
    await expect(page.getByTestId('plugin-banner')).not.toBeVisible();
  });

  test('plugin can read settings at runtime', async ({ page, apiClient }) => {
    await apiClient.savePluginSettings('test-banner', {
      api_key: 'secret-api-key',
      banner_text: 'Banner',
      show_banner: true,
      mode: 'advanced',
      count: 42,
    });
    await apiClient.enablePlugin('test-banner');

    await page.goto('/plugins/test-banner/show-settings');
    await page.waitForLoadState('load');

    const display = page.getByTestId('plugin-settings-display');
    await expect(display).toBeVisible();
    await expect(page.getByTestId('setting-api-key')).toContainText('secret-api-key');
    await expect(page.getByTestId('setting-mode')).toContainText('advanced');
    await expect(page.getByTestId('setting-count')).toContainText('42');
  });

  test('settings persist after page reload', async ({ page, apiClient }) => {
    await apiClient.savePluginSettings('test-banner', {
      api_key: 'persistent-key',
      banner_text: 'Persistent Banner',
      show_banner: true,
      mode: 'simple',
      count: 10,
    });

    await page.goto('/plugins/manage');
    await page.waitForLoadState('load');

    const bannerForm = page.getByTestId('plugin-settings-test-banner');
    const apiKeyInput = bannerForm.getByTestId('setting-api_key');
    await expect(apiKeyInput).toHaveValue('persistent-key');
  });

  test('Plugins dropdown always visible with manage link', async ({ page }) => {
    await page.goto('/notes');
    await page.waitForLoadState('load');
    const desktopNav = page.locator('.navbar-links');
    const pluginsButton = desktopNav.locator('button', { hasText: 'Plugins' });
    await expect(pluginsButton).toBeVisible();
    await pluginsButton.click();
    await expect(desktopNav.locator('a[href="/plugins/manage"]')).toBeVisible();
  });
});
