/**
 * Accessibility tests for the category / note type template authoring forms,
 * which grew a live-preview pane and shortcode-aware editors in Phase 1.
 */
import { test, expect } from '../../fixtures/a11y.fixture';
import { openTemplateSectionsOnLoad } from '../../helpers/template-sections';

// These tests work inside the template sections, which load collapsed.
test.beforeEach(async ({ page }) => { await openTemplateSectionsOnLoad(page); });

test.describe('Category template authoring accessibility', () => {
  test('category create form has no critical a11y violations', async ({ page, checkA11y }) => {
    await page.goto('/category/new');
    await page.waitForLoadState('load');
    await page.locator('.cm-editor').first().waitFor({ state: 'visible', timeout: 15000 });
    await checkA11y();
  });

  test('preview controls are labelled', async ({ page }) => {
    await page.goto('/category/new');
    await page.waitForLoadState('load');

    const entityInput = page.locator('#tp-entity-group');
    await expect(entityInput).toBeVisible({ timeout: 10000 });
    await expect(page.locator('label[for="tp-entity-group"]')).toBeVisible();

    // Every slot preview's frame carries a title naming its slot.
    const frames = page.locator('[data-slot-preview] iframe');
    expect(await frames.count()).toBeGreaterThan(8);
    for (const title of await frames.evaluateAll((els) => els.map((el) => el.getAttribute('title')))) {
      expect(title).toMatch(/^Preview of \S/);
    }
  });

  test('the open slot history dialog has no critical a11y violations', async ({ page, apiClient, checkA11y }) => {
    const name = `A11y History ${Date.now()}`;
    const cat = await apiClient.createCategory(name, undefined, { CustomHeader: '<p>one</p>' });
    const resp = await page.request.post('/v1/category', { form: { ID: String(cat.ID), name, CustomHeader: '<p>two</p>' } });
    expect(resp.ok()).toBeTruthy();
    await apiClient.createGroup({ name: `A11y History Target ${Date.now()}`, categoryId: cat.ID });

    await page.goto(`/category/edit?id=${cat.ID}`);
    await page.locator('.cm-editor').first().waitFor({ state: 'visible', timeout: 15000 });
    await page.getByTestId('slot-history-CustomHeader').click();
    const dialog = page.getByRole('dialog', { name: 'History of Custom Header' });
    await expect(dialog).toBeVisible();
    await dialog.getByTestId('slot-history-rev-1').check();
    await expect(dialog.getByTestId('slot-history-unified-CustomHeader')).toBeVisible();
    await checkA11y();
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
