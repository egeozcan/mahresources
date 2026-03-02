import { test, expect } from '../../fixtures/base.fixture';

test.describe('Plugin Pages', () => {
  test('should show Plugins dropdown in navigation', async ({ page }) => {
    await page.goto('/notes');
    await page.waitForLoadState('load');
    const pluginsButton = page.locator('button', { hasText: 'Plugins' });
    await expect(pluginsButton).toBeVisible();
  });

  test('should show plugin menu items in dropdown', async ({ page }) => {
    await page.goto('/notes');
    await page.waitForLoadState('load');
    const desktopNav = page.locator('.navbar-links');
    const pluginsButton = desktopNav.locator('button', { hasText: 'Plugins' });
    await pluginsButton.click();
    await expect(desktopNav.locator('a[href="/plugins/test-banner/test-page"]')).toBeVisible();
    await expect(desktopNav.locator('a[href="/plugins/test-banner/echo-query"]')).toBeVisible();
  });

  test('should navigate to plugin page and display content', async ({ page }) => {
    await page.goto('/plugins/test-banner/test-page');
    await page.waitForLoadState('load');
    const content = page.getByTestId('plugin-page-content');
    await expect(content).toBeVisible();
    await expect(content).toContainText('Test Plugin Page');
    await expect(content).toContainText('Method: GET');
  });

  test('should pass query parameters to plugin page', async ({ page }) => {
    await page.goto('/plugins/test-banner/echo-query?msg=hello+world');
    await page.waitForLoadState('load');
    const echo = page.getByTestId('plugin-echo');
    await expect(echo).toBeVisible();
    await expect(echo).toContainText('hello world');
  });

  test('should show error for nonexistent plugin page', async ({ page }) => {
    await page.goto('/plugins/test-banner/nonexistent');
    await page.waitForLoadState('load');
    await expect(page.locator('text=Page not found')).toBeVisible();
  });

  test('should show error for nonexistent plugin', async ({ page }) => {
    await page.goto('/plugins/no-such-plugin/anything');
    await page.waitForLoadState('load');
    await expect(page.locator('text=Page not found')).toBeVisible();
  });

  test('plugin page should have standard navigation', async ({ page }) => {
    await page.goto('/plugins/test-banner/test-page');
    await page.waitForLoadState('load');
    // Should have the standard nav bar (scoped to desktop nav to avoid mobile duplicates)
    const desktopNav = page.locator('.navbar-links');
    await expect(desktopNav.locator('a[href="/notes"]')).toBeVisible();
    await expect(desktopNav.locator('a[href="/resources"]')).toBeVisible();
  });
});
