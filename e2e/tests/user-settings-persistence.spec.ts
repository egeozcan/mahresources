import { test, expect } from '../fixtures/base.fixture';

// Regression coverage for the localStorage → server-backed user-settings migration.
// The quick-tag panel's server persistence is exercised by the 13* lightbox specs (they
// seed the server and reload); this file covers the two paths those don't: the
// showDescriptions toggle (storeConfig) and the one-time legacy-localStorage import.

test.describe('User settings persistence (server-backed)', () => {
  test.beforeEach(async ({ page }) => {
    // Clean slate for the shared root-user settings this suite touches.
    await page.request.delete('/v1/account/settings/uiSettings');
    await page.request.delete('/v1/account/settings/quickTags');
  });

  test('showDescriptions toggle persists across reload via the server', async ({ page }) => {
    await page.goto('/resources');
    await page.waitForLoadState('load');

    // Open the settings dropdown and turn Show Descriptions off (default is on).
    await page.locator('button[aria-label="Settings"]').click();
    const checkbox = page.locator('input[name="showDescriptions"]');
    await expect(checkbox).toBeVisible();
    await checkbox.uncheck();

    // The debounced PUT should land the change on the server.
    await expect
      .poll(async () => {
        const res = await page.request.get('/v1/account/settings');
        const body = await res.json();
        return body?.uiSettings?.showDescriptions;
      })
      .toBe(false);

    // Reload: the checkbox must restore to unchecked from the server, not the default.
    await page.goto('/resources');
    await page.waitForLoadState('load');
    await page.locator('button[aria-label="Settings"]').click();
    await expect(page.locator('input[name="showDescriptions"]')).not.toBeChecked();
  });

  test('legacy localStorage quick tags migrate to the server on first load', async ({ page }) => {
    // Land on an app page (same-origin), seed the OLD localStorage key, ensure the
    // server has nothing yet.
    await page.goto('/resources');
    await page.waitForLoadState('load');
    await page.request.delete('/v1/account/settings/quickTags');
    await page.evaluate(() => {
      localStorage.setItem(
        'mahresources_quickTags',
        JSON.stringify({
          version: 3,
          quickSlots: [Array(9).fill(null), Array(9).fill(null), Array(9).fill(null), Array(9).fill(null)],
          recentTags: Array(9).fill(null),
          drawerOpen: false,
          activeTab: 2,
        }),
      );
    });

    // Reload: userSettings.load() imports the legacy blob to the server and clears it.
    await page.goto('/resources');
    await page.waitForLoadState('load');

    // The server now owns the migrated value...
    await expect
      .poll(async () => {
        const res = await page.request.get('/v1/account/settings');
        const body = await res.json();
        return body?.quickTags?.activeTab;
      })
      .toBe(2);

    // ...and the legacy localStorage key was removed.
    await expect
      .poll(() => page.evaluate(() => localStorage.getItem('mahresources_quickTags')))
      .toBeNull();
  });
});
