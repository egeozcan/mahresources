/**
 * Step 2 (category template authoring): shortcode linting in the Custom*
 * template editors. Typing broken shortcode markup should surface CodeMirror
 * diagnostics (lint gutter markers), and a valid template should not.
 */
import { test, expect } from '../fixtures/base.fixture';

test.describe('Shortcode lint in category template editors', () => {
  test('typing a broken [conditional] surfaces a lint diagnostic', async ({ page }) => {
    await page.goto('/category/new');
    await page.waitForLoadState('load');

    // The Custom Header editor is an HTML CodeMirror with aria-label "Custom Header".
    const header = page.locator('.cm-content[aria-label="Custom Header"]');
    await expect(header).toBeVisible({ timeout: 10000 });
    await header.click();

    // A block-required shortcode with no operator and no closing tag: multiple errors.
    await page.keyboard.type('[conditional]hello');

    // The lint gutter marker appears once the debounced server lint returns.
    const errorMarker = page
      .locator('div', { has: page.locator('input[name="CustomHeader"]') })
      .locator('.cm-lint-marker-error')
      .first();
    await expect(errorMarker).toBeVisible({ timeout: 10000 });
  });

  test('a valid shortcode template produces no error diagnostics', async ({ page }) => {
    await page.goto('/category/new');
    await page.waitForLoadState('load');

    const header = page.locator('.cm-content[aria-label="Custom Header"]');
    await expect(header).toBeVisible({ timeout: 10000 });
    await header.click();
    await page.keyboard.type('[meta path="status"]');

    // Give the debounced linter time to run, then assert no error marker.
    await page.waitForTimeout(1200);
    const scope = page.locator('div', { has: page.locator('input[name="CustomHeader"]') });
    await expect(scope.locator('.cm-lint-marker-error')).toHaveCount(0);
  });
});
