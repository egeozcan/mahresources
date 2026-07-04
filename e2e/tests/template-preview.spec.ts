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

  test('Alpine expressions see the same entity scope as the real pages', async ({
    page,
    apiClient,
  }) => {
    const category = await apiClient.createCategory('Alpine Scope Cat');
    const groupName = `Alpine Scope Target ${Date.now()}`;
    await apiClient.createGroup({ name: groupName, categoryId: category.ID });

    await page.goto('/category/new');
    await page.waitForLoadState('load');

    const entityInput = page.locator('#tp-entity-group');
    await expect(entityInput).toBeVisible({ timeout: 10000 });
    await expect(entityInput).toHaveValue(groupName, { timeout: 10000 });

    const header = page.locator('.cm-content[aria-label="Custom Header"]');
    await expect(header).toBeVisible({ timeout: 10000 });
    await header.click();
    // Only Alpine can materialize this text: the server returns the markup
    // verbatim, so the assertion proves the frame recreates the display pages'
    // x-data="{ entity: ... }" scope.
    await page.keyboard.type('<div id="scope-probe" x-text="entity.Name"></div>');

    const frame = page.frameLocator('iframe[title="Template slot preview"]');
    await expect(frame.locator('#scope-probe')).toHaveText(groupName, { timeout: 10000 });
  });

  test('editing a category only offers entities from that category', async ({
    page,
    apiClient,
  }) => {
    const stamp = Date.now();
    const catA = await apiClient.createCategory(`Scoped Cat A ${stamp}`);
    const catB = await apiClient.createCategory(`Scoped Cat B ${stamp}`);
    const inA = `In Category A ${stamp}`;
    const inB = `In Category B ${stamp}`;
    await apiClient.createGroup({ name: inA, categoryId: catA.ID });
    // Created last, so it is the most recent group overall — an unfiltered
    // default would pick this one.
    await apiClient.createGroup({ name: inB, categoryId: catB.ID });

    await page.goto(`/category/edit?id=${catA.ID}`);
    await page.waitForLoadState('load');

    // The default preview entity must come from category A, not the newer
    // group in category B.
    const entityInput = page.locator('#tp-entity-group');
    await expect(entityInput).toBeVisible({ timeout: 10000 });
    await expect(entityInput).toHaveValue(inA, { timeout: 10000 });

    // Searching by the other category's group name yields no suggestions.
    await entityInput.fill(inB);
    await page.waitForTimeout(600); // debounce + request
    await expect(page.locator('#tp-suggestions-group li')).toHaveCount(0);
  });
});
