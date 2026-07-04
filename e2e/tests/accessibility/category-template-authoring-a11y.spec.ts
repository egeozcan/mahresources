/**
 * Accessibility tests for the category / note type template authoring forms,
 * which grew a live-preview pane and shortcode-aware editors in Phase 1.
 */
import { test, expect } from '../../fixtures/a11y.fixture';

test.describe('Category template authoring accessibility', () => {
  test('category create form has no critical a11y violations', async ({ page, checkA11y }) => {
    await page.goto('/category/new');
    await page.waitForLoadState('load');
    await page.locator('.cm-editor').first().waitFor({ state: 'visible', timeout: 15000 });
    await checkA11y();
  });

  test('preview pane controls are labelled', async ({ page }) => {
    await page.goto('/category/new');
    await page.waitForLoadState('load');

    // Entity picker and slot selector both have associated labels.
    const entityInput = page.locator('#tp-entity-group');
    await expect(entityInput).toBeVisible({ timeout: 10000 });
    await expect(page.locator('label[for="tp-entity-group"]')).toBeVisible();
    await expect(page.locator('label[for="tp-slot-group"]')).toBeVisible();

    // The preview iframe carries a title for assistive technology.
    await expect(page.locator('iframe[title="Template slot preview"]')).toBeAttached();
  });

  test('note type create form has no critical a11y violations', async ({ page, checkA11y }) => {
    await page.goto('/noteType/new');
    await page.waitForLoadState('load');
    await page.locator('.cm-editor').first().waitFor({ state: 'visible', timeout: 15000 });
    await checkA11y();
  });
});
