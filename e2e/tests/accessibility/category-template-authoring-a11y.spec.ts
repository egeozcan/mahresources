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

test.describe('Template generation disclosure accessibility', () => {
  test('the open disclosure dialog has no critical a11y violations', async ({ page, checkA11y }) => {
    await page.route('**/v1/category/generateTemplate', async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          groundingHash: 'stub-hash',
          destination: { host: 'api.deepseek.com', profiles: [{ kind: 'pair', model: 'deepseek-v4-flash', thinking: false, maxTokens: 4000, maxCalls: 1 }] },
          continuationRule: { version: 1, text: 'x' },
          sections: [
            { call: 0, role: 'system', key: 'system', label: 'System prompt', bytes: 12, text: 'System text.', constant: true },
            { call: 0, role: 'user', key: 'samples', label: 'Sample values', bytes: 20, text: '{"Name":"Sample"}', constant: false },
          ],
        }),
      });
    });
    await page.goto('/category/new');
    await page.locator('.cm-editor').first().waitFor({ state: 'visible', timeout: 15000 });
    await page.getByTestId('generate-prompt-CustomHeader').fill('a header');
    await page.getByTestId('generate-button-CustomHeader').click();
    const dialog = page.getByRole('dialog', { name: 'Review what will be sent' });
    await expect(dialog).toBeVisible();
    await expect(dialog.getByRole('button', { name: 'Cancel' })).toBeFocused();
    await checkA11y();
  });
});
