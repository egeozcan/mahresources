import { test, expect } from '../../fixtures/base.fixture';
import path from 'path';

/**
 * A background refresh must never take the picture away from an open viewer.
 *
 * `initFromDOM()` rebuilds `items` from the cards currently in the DOM. Four callers invoke
 * it without checking whether the viewer is open — most importantly the `download-completed`
 * listener in src/main.js, which fires off the download cockpit's SSE stream with no user
 * action at all. Once `items` is rebuilt, `currentIndex` can point past the end, and because
 * lightbox.tpl has no fallback branch every media element unmounts: the viewer stays open
 * over an empty black area, and only closing and reopening recovers (open() clamps).
 *
 * See docs/plans/2026-08-06-lightbox-bug-hunt.md.
 */

const LIGHTBOX =
  '[role="dialog"][aria-modal="true"]:not([aria-labelledby="paste-upload-title"]):not([aria-labelledby="entity-picker-title"])';

// One more than the server's constants.MaxResultsPerPage (50), so page 1 fills and a real
// second page exists to page into.
const RESOURCE_COUNT = 55;
const PAGE_SIZE = 50;

test.describe('Lightbox items refresh', () => {
  let categoryId: number;
  let ownerGroupId: number;
  const createdResourceIds: number[] = [];
  const testRunId = Date.now();

  test.beforeAll(async ({ apiClient }) => {
    const category = await apiClient.createCategory(
      `Lightbox Refresh Category ${testRunId}`,
      'Category for lightbox items-refresh tests'
    );
    categoryId = category.ID;

    const ownerGroup = await apiClient.createGroup({
      name: `Lightbox Refresh Owner ${testRunId}`,
      description: 'Owner for lightbox items-refresh resources',
      categoryId,
    });
    ownerGroupId = ownerGroup.ID;

    // createResource appends a unique marker per upload, so the same fixture file can be
    // reused without tripping the server's global content-hash dedup.
    const asset = path.join(__dirname, '../../test-assets/sample-image-2.png');
    for (let i = 0; i < RESOURCE_COUNT; i++) {
      const resource = await apiClient.createResource({
        filePath: asset,
        name: `Refresh Image ${String(i).padStart(3, '0')} - ${testRunId}`,
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
        // best effort
      }
    }
    if (ownerGroupId) await apiClient.deleteGroup(ownerGroupId);
    if (categoryId) await apiClient.deleteCategory(categoryId);
  });

  /** Open the viewer on the last card of page 1, then page across the boundary. */
  async function openPastPageBoundary(page: import('@playwright/test').Page) {
    await page.goto(`/resources?ownerId=${ownerGroupId}`);
    await page.waitForLoadState('load');

    await expect(page.locator('[data-lightbox-item]')).toHaveCount(PAGE_SIZE);

    await page.evaluate((last) => {
      (window as unknown as { Alpine: any }).Alpine.store('lightbox').open(last);
    }, PAGE_SIZE - 1);

    const lightbox = page.locator(LIGHTBOX);
    await expect(lightbox).toBeVisible();

    // Crossing the boundary makes loadNextPage() append page 2, so `items` is now longer
    // than the DOM the page was built from. That gap is the precondition for the bug.
    await page.locator('button[aria-label="Next"]').click();
    await page.waitForFunction(
      (size) => (window as unknown as { Alpine: any }).Alpine.store('lightbox').items.length > size,
      PAGE_SIZE,
      { timeout: 10000 }
    );

    return lightbox;
  }

  test('a DOM re-scan while open keeps the current image on screen', async ({ page }) => {
    const lightbox = await openPastPageBoundary(page);

    const image = lightbox.locator('img');
    await expect(image).toBeVisible();
    const srcBefore = await image.getAttribute('src');
    expect(srcBefore).toBeTruthy();

    // Exactly what main.js does after morphing the refreshed list back in.
    await page.evaluate(() => {
      (window as unknown as { Alpine: any }).Alpine.store('lightbox').initFromDOM();
    });

    await expect(image).toBeVisible();
    await expect(image).toHaveAttribute('src', srcBefore!);

    const state = await page.evaluate(() => {
      const s = (window as unknown as { Alpine: any }).Alpine.store('lightbox');
      return {
        index: s.currentIndex,
        length: s.items.length,
        id: s.getCurrentItem()?.id,
        loadedPages: [...s.loadedPages].sort((a: number, b: number) => a - b),
      };
    });
    expect(state.index).toBeLessThan(state.length);
    expect(state.id).toBeTruthy();
    // The page-2 items are still in the buffer, so the cursor must still say so. A rebuild
    // that reset `items` to page 1 while leaving page 2 marked loaded is what used to make
    // Next a no-op afterwards.
    expect(state.length).toBeGreaterThan(PAGE_SIZE);
    expect(state.loadedPages).toEqual([1, 2]);
  });

  test('a DOM re-scan while open does not renumber the counter past the total', async ({ page }) => {
    const lightbox = await openPastPageBoundary(page);

    await page.evaluate(() => {
      (window as unknown as { Alpine: any }).Alpine.store('lightbox').initFromDOM();
    });

    const counter = lightbox.locator('div.bg-black\\/50:has-text("/")').first();
    const text = (await counter.textContent()) ?? '';
    const [shown, total] = text.split('/').map((part) => parseInt(part.trim(), 10));
    expect(Number.isFinite(shown)).toBe(true);
    expect(shown).toBeLessThanOrEqual(total);
  });

  test('paging still works after a DOM re-scan', async ({ page }) => {
    const lightbox = await openPastPageBoundary(page);

    const idBefore = await page.evaluate(
      () => (window as unknown as { Alpine: any }).Alpine.store('lightbox').getCurrentItem()?.id
    );

    await page.evaluate(() => {
      (window as unknown as { Alpine: any }).Alpine.store('lightbox').initFromDOM();
    });

    // A stale `loadedPages` used to make the next few presses no-ops, because loadNextPage
    // short-circuits on a page it believes is already in `items`.
    await page.locator('button[aria-label="Next"]').click();

    await expect
      .poll(async () =>
        page.evaluate(
          () => (window as unknown as { Alpine: any }).Alpine.store('lightbox').getCurrentItem()?.id
        )
      )
      .not.toBe(idBefore);

    await expect(lightbox.locator('img')).toBeVisible();
  });

  test('a completed background download does not blank the viewer', async ({ page, apiClient }) => {
    const lightbox = await openPastPageBoundary(page);

    const image = lightbox.locator('img');
    await expect(image).toBeVisible();
    const srcBefore = await image.getAttribute('src');
    const idBefore = await page.evaluate(
      () => (window as unknown as { Alpine: any }).Alpine.store('lightbox').getCurrentItem()?.id
    );

    // Give the refresh something real to find. Lists are newest-first, so this lands at the
    // top of page 1 and pushes a media card off the end of it — which is what shrinks the
    // rebuilt list under the current index. Without it the refetched page is byte-identical
    // and the test would pass whether or not the fix is present.
    const added = await apiClient.createResource({
      filePath: path.join(__dirname, '../../test-assets/sample-image-3.png'),
      name: `Refresh Late Arrival ${testRunId}`,
      ownerId: ownerGroupId,
    });
    createdResourceIds.push(added.ID);

    // downloadCockpit.js dispatches exactly this off its SSE stream when a queued job
    // finishes. main.js re-fetches the page, morphs the list, and re-scans the DOM.
    await page.evaluate((resourceId) => {
      window.dispatchEvent(
        new CustomEvent('download-completed', { detail: { id: 1, resourceId, status: 'completed' } })
      );
    }, added.ID);

    // Poll for an observable effect of the handler rather than sleeping: the new resource
    // reaching the store proves the morph and the DOM re-scan both ran.
    await expect
      .poll(
        async () =>
          page.evaluate(
            (id) =>
              (window as unknown as { Alpine: any }).Alpine.store('lightbox').items.some(
                (item: { id: number }) => item.id === id
              ),
            added.ID
          ),
        { timeout: 15000 }
      )
      .toBe(true);

    // Same resource, same bitmap, still on screen.
    await expect(image).toBeVisible();
    await expect(image).toHaveAttribute('src', srcBefore!);
    expect(
      await page.evaluate(
        () => (window as unknown as { Alpine: any }).Alpine.store('lightbox').getCurrentItem()?.id
      )
    ).toBe(idBefore);
  });
});
