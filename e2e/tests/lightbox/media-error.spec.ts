import { test, expect } from '../../fixtures/base.fixture';
import path from 'path';

/**
 * A resource deleted from another session stays in the viewer's list and 404s when the
 * user reaches it. The viewer answers that with an inline "Could not load ..." message.
 * It must never answer it with a spinner as well: the media element is reused, so
 * reopening the viewer on the same item re-requests nothing and no load or error event
 * ever arrives to clear the loading flag.
 */
test.describe('Lightbox media load failure', () => {
  let categoryId: number;
  let ownerGroupId: number;
  let deletedResourceId: number;
  const createdResourceIds: number[] = [];
  const testRunId = Date.now();

  const LIGHTBOX = '[role="dialog"][x-show="$store.lightbox.isOpen"]';

  test.beforeAll(async ({ apiClient }) => {
    const category = await apiClient.createCategory(
      `Media Error Category ${testRunId}`,
      'Category for lightbox media-error tests'
    );
    categoryId = category.ID;

    const ownerGroup = await apiClient.createGroup({
      name: `Media Error Owner ${testRunId}`,
      description: 'Owner for lightbox media-error test resources',
      categoryId,
    });
    ownerGroupId = ownerGroup.ID;

    for (const [name, file] of [
      ['First image', 'sample-image-13.png'],
      ['Doomed image', 'sample-image.svg'],
    ] as const) {
      const resource = await apiClient.createResource({
        filePath: path.join(__dirname, '../../test-assets', file),
        name: `${name} ${testRunId}`,
        description: name,
        ownerId: ownerGroupId,
      });
      createdResourceIds.push(resource.ID);
      if (name === 'Doomed image') deletedResourceId = resource.ID;
    }
  });

  test.afterAll(async ({ apiClient }) => {
    for (const resourceId of createdResourceIds) {
      try {
        await apiClient.deleteResource(resourceId);
      } catch {
        /* ignore */
      }
    }
    if (ownerGroupId) await apiClient.deleteGroup(ownerGroupId);
    if (categoryId) await apiClient.deleteCategory(categoryId);
  });

  function storeState(page) {
    return page.evaluate(() => {
      const s = (window as any).Alpine.store('lightbox');
      return { loading: s.loading, hasError: s.hasMediaError() };
    });
  }

  test('a failed media load shows the error without the loading spinner', async ({ page, apiClient }) => {
    await page.goto(`/resources?OwnerId=${ownerGroupId}`);
    await page.waitForLoadState('load');

    const lightbox = page.locator(LIGHTBOX);
    await page.locator('[data-lightbox-item]').first().click();
    await expect(lightbox).toBeVisible();
    await expect(lightbox.locator('svg.animate-spin')).toBeHidden({ timeout: 10_000 });

    // Delete one of the two resources behind the viewer's back, then navigate onto it.
    await apiClient.deleteResource(deletedResourceId);
    const doomedIndex = await page.evaluate((id) =>
      (window as any).Alpine.store('lightbox').items.findIndex((i) => i.id === id), deletedResourceId);
    expect(doomedIndex).toBeGreaterThanOrEqual(0);

    await page.evaluate((index) => (window as any).Alpine.store('lightbox').open(index), doomedIndex);
    await expect(lightbox.locator('[data-media-error]')).toBeVisible();
    await expect(lightbox.locator('svg.animate-spin')).toBeHidden();
    expect(await storeState(page)).toEqual({ loading: false, hasError: true });

    // Reopening the viewer on the same item is the case that used to strand the spinner:
    // the media element keeps the URL it already failed on, so nothing is re-requested.
    await page.evaluate(() => (window as any).Alpine.store('lightbox').close());
    await page.evaluate((index) => (window as any).Alpine.store('lightbox').open(index), doomedIndex);
    await expect(lightbox.locator('[data-media-error]')).toBeVisible();
    await expect(lightbox.locator('svg.animate-spin')).toBeHidden();
    expect(await storeState(page)).toEqual({ loading: false, hasError: true });
  });
});
