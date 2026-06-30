import { test, expect } from '../fixtures/base.fixture';
import type { Page } from '@playwright/test';
import path from 'path';

/**
 * Tier 0 / Item 1: lightbox tag-state must survive navigation.
 *
 * Before this fix, onResourceChange() nulled `resourceDetails` and evicted the
 * incoming resource's cache entry on every next/prev, so quick-slot match colors
 * flashed neutral and every image cost a /resource.json round-trip. The fix keeps
 * the prior details visible (under aria-busy) and prefetches upcoming items' tag
 * details the way bitmaps are already prefetched.
 */
test.describe('Lightbox tag-detail prefetch + no-blank', () => {
  let categoryId: number;
  let ownerGroupId: number;
  const createdResourceIds: number[] = [];
  const testRunId = Date.now();

  const LIGHTBOX =
    '[role="dialog"][aria-modal="true"]:not([aria-labelledby="paste-upload-title"]):not([aria-labelledby="entity-picker-title"])';

  test.beforeAll(async ({ apiClient }) => {
    const category = await apiClient.createCategory(
      `Lightbox Prefetch Category ${testRunId}`,
      'Category for lightbox prefetch tests'
    );
    categoryId = category.ID;

    const ownerGroup = await apiClient.createGroup({
      name: `Lightbox Prefetch Owner ${testRunId}`,
      description: 'Owner for lightbox prefetch resources',
      categoryId: categoryId,
    });
    ownerGroupId = ownerGroup.ID;

    const testImageFiles = [
      path.join(__dirname, '../test-assets/sample-image-13.png'),
      path.join(__dirname, '../test-assets/sample-image-2.png'),
      path.join(__dirname, '../test-assets/sample-image-3.png'),
    ];
    for (let i = 0; i < testImageFiles.length; i++) {
      const resource = await apiClient.createResource({
        filePath: testImageFiles[i],
        name: `Lightbox Prefetch Image ${i + 1} - ${testRunId}`,
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

  // Scope the list to this test's owner group so only our 3 images load.
  async function openLightboxWithPanel(page: Page) {
    await page.goto(`/resources?OwnerId=${ownerGroupId}`);
    await page.waitForLoadState('load');
    await page.locator('[data-lightbox-item]').first().click();
    const lightbox = page.locator(LIGHTBOX);
    await expect(lightbox).toBeVisible();
    // Open the quick-tag panel and load the current resource's details.
    await page.evaluate(async () => {
      const s = (window as any).Alpine.store('lightbox');
      s.openQuickTagPanel();
      await s.fetchResourceDetails();
    });
    await expect
      .poll(() => page.evaluate(() => (window as any).Alpine.store('lightbox').resourceDetails !== null))
      .toBe(true);
    return lightbox;
  }

  test('A: resourceDetails is not blanked on navigation', async ({ page }) => {
    await openLightboxWithPanel(page);

    // Fire next() and synchronously observe resourceDetails right after the
    // (synchronous) prefix of onResourceChange runs. The pre-fix code sets it to
    // null here, which is what made the quick-slot colors flash neutral.
    const observed = await page.evaluate(() => {
      const s = (window as any).Alpine.store('lightbox');
      const beforeNull = s.resourceDetails === null;
      s.next();
      const afterNull = s.resourceDetails === null;
      return { beforeNull, afterNull };
    });

    expect(observed.beforeNull).toBe(false);
    expect(observed.afterNull).toBe(false);
  });

  test('B: upcoming details are prefetched and a prefetched neighbour is a cache hit', async ({ page }) => {
    const counts: Record<string, number> = {};
    await page.route('**/resource.json**', async (route) => {
      const id = new URL(route.request().url()).searchParams.get('id') || '?';
      counts[id] = (counts[id] || 0) + 1;
      await route.continue();
    });

    await openLightboxWithPanel(page);

    const ids: string[] = await page.evaluate(() =>
      (window as any).Alpine.store('lightbox').items.map((i: any) => String(i.id))
    );
    expect(ids.length).toBeGreaterThanOrEqual(3);
    const id2 = ids[2];

    // Move to index 1; this should prefetch index 2's tag details in the background.
    await page.evaluate(() => (window as any).Alpine.store('lightbox').next());
    await expect
      .poll(
        () => page.evaluate((i) => (window as any).Alpine.store('lightbox').detailsCache.has(Number(i)), id2),
        { timeout: 4000 }
      )
      .toBe(true);

    const prefetchedCount = counts[id2] || 0;
    expect(prefetchedCount).toBeGreaterThanOrEqual(1);

    // Navigate into the prefetched neighbour: it must be served from cache with
    // no additional /resource.json request.
    await page.evaluate(() => (window as any).Alpine.store('lightbox').next());
    await page.waitForTimeout(300);
    expect(counts[id2]).toBe(prefetchedCount);
  });

  test('C: panel exposes aria-busy during detail revalidation', async ({ page }) => {
    await openLightboxWithPanel(page);

    // Slow every details fetch so the loading state is observable.
    await page.route('**/resource.json**', async (route) => {
      await new Promise((r) => setTimeout(r, 1000));
      await route.continue();
    });

    const panel = page.locator('[data-quick-tag-panel]');
    // Navigate to an uncached neighbour to trigger a foreground fetch.
    await page.evaluate(() => (window as any).Alpine.store('lightbox').next());
    await expect(panel).toHaveAttribute('aria-busy', 'true');
    await expect(panel).toHaveAttribute('aria-busy', 'false');
  });

  test('D: navigating between images does not spuriously announce in the chip-input live region', async ({
    page,
    apiClient,
  }) => {
    const tag = await apiClient.createTag(`PrefetchLiveRegion-${testRunId}`);
    await apiClient.addTagsToResources([createdResourceIds[1]], [tag.ID]);

    await openLightboxWithPanel(page);

    // Read the chip-input's OWN role="status" live region (one per autocompleter
    // instance, created via createLiveRegion(this.$el)) -- not the lightbox's top-level
    // status line -- since the bug is specifically the chip-input's generic
    // $watch('selectedResults', ...) misfiring when the lightbox swaps resourceDetails.
    const chipInputLiveText = () =>
      page.evaluate(() => {
        const input = document.querySelector('[data-tag-editor-input]');
        const root = input?.closest('[x-effect]');
        return root?.querySelector('[role="status"]')?.textContent || '';
      });

    // Let the mount-time announce debounce (50ms) settle before taking a baseline.
    await page.waitForTimeout(150);

    // The gallery lists newest-first, so the opened item is not necessarily
    // createdResourceIds[0] -- capture the actual starting id instead of assuming it.
    const startId: number = await page.evaluate(
      () => (window as any).Alpine.store('lightbox').resourceDetails?.ID
    );

    // Navigate to the tagged second image -- a real tag-state change in DISPLAY, but not
    // a user-initiated add on the current image.
    await page.evaluate(() => (window as any).Alpine.store('lightbox').next());
    await expect
      .poll(() =>
        page.evaluate(
          (id) => (window as any).Alpine.store('lightbox').resourceDetails?.ID === id,
          createdResourceIds[1]
        )
      )
      .toBe(true);
    await page.waitForTimeout(150);

    const textAfterForwardNav = await chipInputLiveText();
    expect(textAfterForwardNav).not.toMatch(/^Added /i);
    expect(textAfterForwardNav).not.toMatch(/^Removed item/i);

    // And the reverse direction (back to the starting image) must not announce
    // "Removed item" either.
    await page.evaluate(() => (window as any).Alpine.store('lightbox').prev());
    await expect
      .poll(() =>
        page.evaluate(
          (id) => (window as any).Alpine.store('lightbox').resourceDetails?.ID === id,
          startId
        )
      )
      .toBe(true);
    await page.waitForTimeout(150);

    const textAfterBackNav = await chipInputLiveText();
    expect(textAfterBackNav).not.toMatch(/^Added /i);
    expect(textAfterBackNav).not.toMatch(/^Removed item/i);
  });
});
