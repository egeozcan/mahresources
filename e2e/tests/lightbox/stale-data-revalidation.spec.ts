import { test, expect } from '../../fixtures/base.fixture';
import type { Page, Route } from '@playwright/test';
import path from 'path';

/**
 * The lightbox must never show stale data — neither another resource's, nor an outdated copy
 * of this one's.
 *
 *  A. Display fence: during a cache-miss navigation window `resourceDetails` still holds the
 *     PREVIOUS image (carry-forward and the add/remove decision poll depend on that), so every
 *     display site reads through displayDetails(), which is null until the details describe
 *     what is on screen. The panel shows its loading state instead of the previous image's
 *     name, size and tags.
 *
 *  B. Revalidation: navigation is stale-while-revalidate. A cached copy paints instantly and is
 *     then checked against the server, so a change made anywhere else — another tab, a bulk
 *     action, a background job — corrects itself within one round-trip instead of persisting.
 *
 *  C. Cross-session: the detail cache is dropped when the viewer closes, so a later session
 *     never paints a snapshot from the previous one.
 *
 *  D. Metadata includes the bitmap: when a details fetch reports a Hash the item does not have,
 *     the item is re-pointed at the new version so the panel cannot describe a file the viewer
 *     is not showing.
 *
 * Navigation is driven through the store rather than next()/key events: under parallel load
 * next() can early-return on a short items list, which is the root of earlier flakes here.
 */
test.describe('Lightbox never shows stale data', () => {
  let categoryId: number;
  let ownerGroupId: number;
  const createdResourceIds: number[] = [];
  const testRunId = Date.now();

  const LIGHTBOX =
    '[role="dialog"][aria-modal="true"]:not([aria-labelledby="paste-upload-title"]):not([aria-labelledby="entity-picker-title"])';

  test.beforeAll(async ({ apiClient }) => {
    const category = await apiClient.createCategory(
      `Lightbox Revalidate Category ${testRunId}`,
      'Category for lightbox revalidation tests'
    );
    categoryId = category.ID;

    const ownerGroup = await apiClient.createGroup({
      name: `Lightbox Revalidate Owner ${testRunId}`,
      description: 'Owner for lightbox revalidation resources',
      categoryId: categoryId,
    });
    ownerGroupId = ownerGroup.ID;

    const testImageFiles = [
      path.join(__dirname, '../../test-assets/sample-image-13.png'),
      path.join(__dirname, '../../test-assets/sample-image-2.png'),
      path.join(__dirname, '../../test-assets/sample-image-3.png'),
    ];
    for (let i = 0; i < testImageFiles.length; i++) {
      const resource = await apiClient.createResource({
        filePath: testImageFiles[i],
        name: `Lightbox Revalidate Image ${i + 1} - ${testRunId}`,
        ownerId: ownerGroupId,
      });
      createdResourceIds.push(resource.ID);
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

  test.beforeEach(async ({ page }) => {
    await page.request.delete('/v1/account/settings/quickTags');
  });

  // Open the gallery scoped to this test's owner group, open the lightbox on the first item and
  // open the quick-tag panel (which is what loads details).
  async function openWithPanel(page: Page) {
    await page.goto(`/resources?OwnerId=${ownerGroupId}&sort=ID&order=desc`);
    await page.waitForLoadState('load');
    await expect(page.locator('[data-lightbox-item]')).toHaveCount(createdResourceIds.length, {
      timeout: 15000,
    });
    await page.locator('[data-lightbox-item]').first().click();
    const lightbox = page.locator(LIGHTBOX);
    await expect(lightbox).toBeVisible();

    await page.evaluate(() => {
      const s = (window as any).Alpine.store('lightbox');
      s.openQuickTagPanel();
      s.openEditPanel();
    });
    await expect
      .poll(() => page.evaluate(() => (window as any).Alpine.store('lightbox').displayDetails() !== null))
      .toBe(true);

    const ids: number[] = await page.evaluate(() =>
      (window as any).Alpine.store('lightbox').items.map((i: any) => i.id)
    );
    expect(ids.length).toBeGreaterThanOrEqual(2);
    return { lightbox, ids };
  }

  // Point the viewer at `id` and run the resource-change hook, exactly as arrow-key navigation
  // does, without the pagination races of next().
  async function navigateTo(page: Page, id: number, { awaitChange = true } = {}) {
    await page.evaluate(
      async ({ target, wait }) => {
        const s = (window as any).Alpine.store('lightbox');
        s.currentIndex = s.items.findIndex((i: any) => i.id === target);
        const changed = s.onResourceChange();
        if (wait) await changed;
      },
      { target: id, wait: awaitChange }
    );
  }

  test('A: the panel shows no details at all while the incoming image is still loading', async ({
    page,
  }) => {
    const { ids } = await openWithPanel(page);
    const image1 = ids[0];
    const image2 = ids[1];

    // Force a genuine cache miss on image2 with a slow response, so the window stays open.
    await expect
      .poll(() => page.evaluate(() => (window as any).Alpine.store('lightbox')._detailsInFlight.size))
      .toBe(0);
    await page.evaluate((rid) => {
      const s = (window as any).Alpine.store('lightbox');
      s.detailsCache.delete(rid);
      s._detailsInFlight.delete(rid);
    }, image2);
    await page.route('**/resource.json**', async (route: Route) => {
      const matches = new URL(route.request().url()).searchParams.get('id') === String(image2);
      if (matches) await new Promise((r) => setTimeout(r, 1500));
      await route.continue();
    });

    await navigateTo(page, image2, { awaitChange: false });

    // Inside the window: the raw state still holds image1 (carry-forward needs it), but nothing
    // the user can see does.
    const inWindow = await page.evaluate(() => {
      const s = (window as any).Alpine.store('lightbox');
      return { rawId: s.resourceDetails?.ID ?? null, display: s.displayDetails() };
    });
    expect(inWindow.rawId).toBe(image1);
    expect(inWindow.display).toBeNull();

    // The info panel must not be printing the previous image's name/size while it loads.
    const infoPanel = page.locator('[data-edit-panel]');
    await expect(infoPanel).toHaveAttribute('aria-busy', 'true');
    await expect(infoPanel.locator('#lightbox-edit-name')).toHaveCount(0);
    await expect(page.locator('[data-quick-tag-panel]')).toContainText('Loading tags...');

    // And once it lands, the panel describes the image on screen.
    await expect
      .poll(
        () => page.evaluate(() => (window as any).Alpine.store('lightbox').displayDetails()?.ID ?? null),
        { timeout: 10000 }
      )
      .toBe(image2);

    await page.unroute('**/resource.json**');
  });

  test('B: a tag added elsewhere shows up on the next visit to a cached image', async ({
    page,
    apiClient,
  }) => {
    const tag = await apiClient.createTag(`RevalidateTag-${testRunId}`);
    const { ids } = await openWithPanel(page);
    const image1 = ids[0];
    const image2 = ids[1];

    // Visit image2 so its details are cached, then come back.
    await navigateTo(page, image2);
    await navigateTo(page, image1);
    expect(
      await page.evaluate((rid) => (window as any).Alpine.store('lightbox').detailsCache.has(rid), image2)
    ).toBe(true);

    // Someone else tags image2 while the viewer sits on image1.
    await apiClient.addTagsToResources([image2], [tag.ID]);

    // Returning to it must not paint the pre-change cache copy and leave it there.
    await navigateTo(page, image2);
    await expect
      .poll(
        () =>
          page.evaluate(() =>
            ((window as any).Alpine.store('lightbox').displayDetails()?.Tags || []).map(
              (t: any) => t.Name
            )
          ),
        { timeout: 10000 }
      )
      .toContain(tag.Name);
  });

  test('C: a change made while the viewer is closed is not painted from the previous session', async ({
    page,
    apiClient,
  }) => {
    const tag = await apiClient.createTag(`ReopenTag-${testRunId}`);
    const { ids } = await openWithPanel(page);
    const image1 = ids[0];

    // Close the viewer with image1's details cached, then change it out of band.
    await page.evaluate(() => (window as any).Alpine.store('lightbox').close());
    expect(
      await page.evaluate(() => (window as any).Alpine.store('lightbox').detailsCache.size)
    ).toBe(0);
    await apiClient.addTagsToResources([image1], [tag.ID]);

    // Re-open on the same image: the very first painted details already carry the new tag.
    await page.locator('[data-lightbox-item]').first().click();
    await expect(page.locator(LIGHTBOX)).toBeVisible();
    await page.evaluate(() => (window as any).Alpine.store('lightbox').openQuickTagPanel());

    await expect
      .poll(
        () =>
          page.evaluate(() =>
            ((window as any).Alpine.store('lightbox').displayDetails()?.Tags || []).map(
              (t: any) => t.Name
            )
          ),
        { timeout: 10000 }
      )
      .toContain(tag.Name);
  });

  test('D: a new version created elsewhere re-points the viewer at the new file', async ({
    page,
  }) => {
    const { ids } = await openWithPanel(page);
    const image1 = ids[0];
    const image2 = ids[1];

    const hashBefore: string = await page.evaluate(
      (rid) => (window as any).Alpine.store('lightbox').items.find((i: any) => i.id === rid).hash,
      image1
    );

    // A rotate from anywhere else (the resource page, another tab, the API) writes a new
    // version: new Hash, new bytes behind the same id.
    const response = await page.request.post('/v1/resources/rotate', {
      form: { id: String(image1), degrees: '90' },
      headers: { Accept: 'application/json' },
    });
    expect(response.ok()).toBe(true);

    // Navigating away and back revalidates image1's details, which now disagree with the item.
    await navigateTo(page, image2);
    await navigateTo(page, image1);

    await expect
      .poll(
        () =>
          page.evaluate(
            (rid) => (window as any).Alpine.store('lightbox').items.find((i: any) => i.id === rid).hash,
            image1
          ),
        { timeout: 10000 }
      )
      .not.toBe(hashBefore);

    const item = await page.evaluate(
      (rid) => (window as any).Alpine.store('lightbox').items.find((i: any) => i.id === rid),
      image1
    );
    expect(item.viewUrl).toBe(`/v1/resource/view?id=${image1}&v=${item.hash}`);
    // And the media element on screen is showing that new version, not the cached old bytes.
    await expect
      .poll(() =>
        page.evaluate(() => {
          const img = document.querySelector('[role="dialog"] img') as HTMLImageElement | null;
          return img?.getAttribute('src') || '';
        })
      )
      .toContain(`v=${item.hash}`);
  });
});
