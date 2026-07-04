/**
 * Step 3 (category template authoring): shortcode autocomplete + hover docs in
 * the Custom* template editors.
 */
import { test, expect } from '../fixtures/base.fixture';

test.describe('Shortcode autocomplete', () => {
  test('typing "[m" in Custom Header offers built-in shortcode names', async ({ page }) => {
    await page.goto('/category/new');
    await page.waitForLoadState('load');

    const header = page.locator('.cm-content[aria-label="Custom Header"]');
    await expect(header).toBeVisible({ timeout: 10000 });
    await header.click();
    await page.keyboard.type('[m');

    // The autocomplete popup should list meta and mrql.
    const popup = page.locator('.cm-tooltip-autocomplete');
    await expect(popup).toBeVisible({ timeout: 10000 });
    await expect(popup.locator('.cm-completionLabel', { hasText: /^meta$/ })).toBeVisible();
    await expect(popup.locator('.cm-completionLabel', { hasText: /^mrql$/ })).toBeVisible();
  });

  test('attribute completion offers required attrs after a shortcode name', async ({ page }) => {
    await page.goto('/category/new');
    await page.waitForLoadState('load');

    const header = page.locator('.cm-content[aria-label="Custom Header"]');
    await expect(header).toBeVisible({ timeout: 10000 });
    await header.click();
    // "[meta " should offer the path attribute.
    await page.keyboard.type('[meta p');

    const popup = page.locator('.cm-tooltip-autocomplete');
    await expect(popup).toBeVisible({ timeout: 10000 });
    await expect(popup.locator('.cm-completionLabel', { hasText: /^path$/ })).toBeVisible();
  });
});
