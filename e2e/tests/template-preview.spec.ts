/**
 * Step 4 (category template authoring): live template preview. Editing a
 * Custom* slot on the category form renders it against a real group inside a
 * sandboxed iframe.
 */
import { test, expect } from '../fixtures/base.fixture';

test.describe('Template live preview', () => {
  test('editing Custom Header renders against a seeded group', async ({ page, apiClient }) => {
    const category = await apiClient.createCategory('Preview Cat');
    const groupName = `Preview Target ${Date.now()}`;
    await apiClient.createGroup({ name: groupName, categoryId: category.ID });

    await page.goto('/category/new');
    await page.waitForLoadState('load');

    // The preview pane defaults to the most recent group; give it a moment.
    const entityInput = page.locator('#tp-entity-group');
    await expect(entityInput).toBeVisible({ timeout: 10000 });
    await expect(entityInput).toHaveValue(groupName, { timeout: 10000 });

    // Type a property shortcode into the Custom Header editor.
    const header = page.locator('.cm-content[aria-label="Custom Header"]');
    await expect(header).toBeVisible({ timeout: 10000 });
    await header.click();
    await page.keyboard.type('[property path="Name"]');

    // The sandboxed iframe should render the group's name (debounced refresh).
    const frame = page.frameLocator('iframe[title="Template slot preview"]');
    await expect(frame.locator('body')).toContainText(groupName, { timeout: 10000 });

    // The app bundle must hydrate inside the sandboxed (opaque-origin) frame:
    // module scripts are CORS-fetched, so this only works while /public/ is
    // served with Access-Control-Allow-Origin. Guards against regressing the
    // "web components and Alpine widgets hydrate in preview" behaviour.
    await expect
      .poll(
        async () => {
          const srcdocFrame = page.frames().find((f) => f.url() === 'about:srcdoc');
          if (!srcdocFrame) return false;
          return srcdocFrame
            .evaluate(() => typeof (window as { Alpine?: unknown }).Alpine !== 'undefined')
            .catch(() => false);
        },
        { timeout: 10000 },
      )
      .toBe(true);
  });
});
