/**
 * Plan 2026-10-24-template-generation-quality, step 1d: a MetaSchema property
 * forced to x-display "geo" whose stored value lacks a usable coordinate pair
 * used to throw inside both the display panel and the [meta] shortcode. It now
 * falls through to the normal rendering of the value.
 */
import { test, expect } from '../../fixtures/base.fixture';

test.describe('Forced geo renderer with a partial stored value', () => {
  let categoryId: number;
  let groupId: number;

  test.beforeAll(async ({ apiClient }) => {
    const cat = await apiClient.createCategory(
      `Partial Geo ${Date.now()}`,
      'Forced geo x-display with a partial value',
      {
        MetaSchema: JSON.stringify({
          type: 'object',
          properties: {
            location: {
              type: 'object',
              title: 'Location',
              'x-display': 'geo',
              properties: {
                latitude: { type: 'number' },
                longitude: { type: 'number' },
              },
            },
          },
        }),
        CustomHeader: '<div class="partial-geo-header">[meta path="location"]</div>',
      },
    );
    categoryId = cat.ID;
    const group = await apiClient.createGroup({
      name: `Partial Geo Group ${Date.now()}`,
      categoryId: cat.ID,
      meta: JSON.stringify({ location: { latitude: 41.5 } }),
    });
    groupId = group.ID;
  });

  test.afterAll(async ({ apiClient }) => {
    if (groupId) await apiClient.deleteGroup(groupId);
    if (categoryId) await apiClient.deleteCategory(categoryId);
  });

  test('detail page and [meta] render without a console error', async ({ page }) => {
    const errors: string[] = [];
    page.on('pageerror', error => errors.push(error.message));
    page.on('console', msg => {
      if (msg.type() === 'error') errors.push(msg.text());
    });

    await page.goto(`/group?id=${groupId}`);
    await page.waitForLoadState('load');

    const display = page.locator('schema-editor[mode="display"]');
    await expect(display).toBeVisible({ timeout: 5000 });
    // The panel falls back to the key/value grid for the object.
    await expect(display).toContainText('Latitude');
    await expect(display).toContainText('41.5');
    await expect(display.locator('a[href*="openstreetmap.org"]')).toHaveCount(0);

    const shortcode = page.locator('.partial-geo-header meta-shortcode');
    await expect(shortcode).toContainText('{"latitude":41.5}');

    expect(errors).toEqual([]);
  });
});
