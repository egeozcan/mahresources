import { test, expect } from '../fixtures/base.fixture';
import type { Page, Route } from '@playwright/test';
import { seedQuickTags } from '../helpers/quick-tags';
import path from 'path';

/**
 * Regression coverage for three lightbox quick-tag review findings:
 *
 *  #1 (stale details): after navigating to an image whose details are not yet cached,
 *     the panel keeps showing the PREVIOUS image's details until the fetch resolves
 *     (deliberately, to avoid a color flash). A tag action fired in that window must not
 *     (a) poison the new image's details cache with the previous image's data, nor
 *     (b) decide add-vs-remove against the previous image's tag set.
 *
 *  #3 (repeat announce): pressing R to repeat the previous image's tags must not announce
 *     a false "Repeated…" success when the underlying write failed.
 *
 * The load window is forced deterministically by delaying (or, for #1a, holding) the
 * `/resource.json` response for the target image via a page route, after evicting it from
 * the in-memory cache and letting background prefetches drain.
 */
test.describe('Lightbox stale-details & repeat-announce fixes', () => {
  let categoryId: number;
  let ownerGroupId: number;
  const createdResourceIds: number[] = [];
  const testRunId = Date.now();

  const LIGHTBOX =
    '[role="dialog"][aria-modal="true"]:not([aria-labelledby="paste-upload-title"]):not([aria-labelledby="entity-picker-title"])';

  test.beforeAll(async ({ apiClient }) => {
    const category = await apiClient.createCategory(
      `Lightbox Stale Category ${testRunId}`,
      'Category for lightbox stale-details tests'
    );
    categoryId = category.ID;

    const ownerGroup = await apiClient.createGroup({
      name: `Lightbox Stale Owner ${testRunId}`,
      description: 'Owner for lightbox stale-details resources',
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
        name: `Lightbox Stale Image ${i + 1} - ${testRunId}`,
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
    // Quick-tag state is server-side now (shared across tests). Reset for isolation.
    await page.request.delete('/v1/account/settings/quickTags');
  });

  type Seed = {
    quickSlots: (Array<{ id: number; name: string }> | null)[][];
    activeTab?: number;
  };

  function buildSeed(seed: Seed) {
    const pad = (row: (Array<{ id: number; name: string }> | null)[]) => {
      const r = row.slice(0, 9);
      while (r.length < 9) r.push(null);
      return r;
    };
    return {
      version: 3,
      quickSlots: [
        pad(seed.quickSlots[0] || []),
        pad(seed.quickSlots[1] || []),
        pad(seed.quickSlots[2] || []),
        pad(seed.quickSlots[3] || []),
      ],
      recentTags: Array(9).fill(null),
      drawerOpen: true,
      activeTab: seed.activeTab ?? 0,
    };
  }

  async function seedAndOpen(page: Page, seed: Seed) {
    await page.goto(`/resources?OwnerId=${ownerGroupId}`);
    await page.waitForLoadState('load');
    await seedQuickTags(page, buildSeed(seed));
    await page.goto(`/resources?OwnerId=${ownerGroupId}`);
    await page.waitForLoadState('load');

    // Under heavy parallel load the gallery can still be rendering when we open; wait for every
    // seeded item first so the lightbox captures the FULL items array. A short array is the root
    // of the earlier flakes — findIndex(image2) returns -1 and next()/navigation no-ops.
    await expect(page.locator('[data-lightbox-item]')).toHaveCount(createdResourceIds.length, {
      timeout: 15000,
    });

    await page.locator('[data-lightbox-item]').first().click();
    const lightbox = page.locator(LIGHTBOX);
    await expect(lightbox).toBeVisible();

    // Panel-open state is transient now (not restored across reload), so open it explicitly.
    await page.keyboard.press('t');
    const panel = lightbox.locator('[data-quick-tag-panel]');
    await expect(panel).toBeVisible();
    await expect(panel.locator('[data-tag-editor-input]')).toBeVisible({ timeout: 10000 });
    await expect
      .poll(() => page.evaluate(() => (window as any).Alpine.store('lightbox').resourceDetails !== null))
      .toBe(true);

    await page.evaluate(() => (document.activeElement as HTMLElement)?.blur());

    const ids: number[] = await page.evaluate(() =>
      (window as any).Alpine.store('lightbox').items.map((i: any) => i.id)
    );
    // Fail loud and early if the viewer somehow opened with a short list, rather than surfacing
    // later as a confusing findIndex === -1.
    expect(ids.length).toBeGreaterThanOrEqual(2);
    return { lightbox, panel, ids };
  }

  async function resourceHasTag(apiClient: any, id: number, tagName: string): Promise<boolean> {
    const r = await apiClient.getResource(id);
    return (r.Tags || []).some((t: any) => t.Name === tagName);
  }

  async function liveText(page: Page): Promise<string> {
    return page.evaluate(() => (window as any).Alpine.store('lightbox').liveRegion?.textContent || '');
  }

  async function blurAndPress(page: Page, key: string) {
    await page.evaluate((rawKey) => {
      const dlg = document.querySelector(
        '[role="dialog"][aria-modal="true"]:not([aria-labelledby="paste-upload-title"]):not([aria-labelledby="entity-picker-title"])'
      );
      (dlg as HTMLElement)?.focus();
      window.dispatchEvent(
        new KeyboardEvent('keydown', { key: rawKey, bubbles: true, cancelable: true })
      );
    }, key);
  }

  // Drain any in-flight detail prefetches (panel-open warms the upcoming window) and evict
  // `id` from the details cache, so the NEXT navigation to it is a genuine cache miss and the
  // load window can be forced open by delaying its /resource.json.
  async function evictAndQuiesce(page: Page, id: number) {
    await expect
      .poll(() => page.evaluate(() => (window as any).Alpine.store('lightbox')._detailsInFlight.size))
      .toBe(0);
    await page.evaluate((rid) => {
      const s = (window as any).Alpine.store('lightbox');
      s.detailsCache.delete(rid);
      s._detailsInFlight.delete(rid);
    }, id);
  }

  // Delay every /resource.json for `id` by `ms`, passing everything else straight through, so
  // the cache-miss load window for that image stays open long enough to act inside it.
  async function delayDetailsFor(page: Page, id: number, ms: number) {
    await page.route('**/resource.json**', async (route: Route) => {
      let matches = false;
      try {
        matches = new URL(route.request().url()).searchParams.get('id') === String(id);
      } catch {
        matches = false;
      }
      if (matches) {
        await new Promise((r) => setTimeout(r, ms));
      }
      await route.continue();
    });
  }

  test('#1a: a batch tag write during the load window never poisons the new image cache', async ({
    page,
    apiClient,
  }) => {
    const tagC = await apiClient.createTag(`StaleCacheC-${testRunId}`);

    const { ids } = await seedAndOpen(page, {
      quickSlots: [[[{ id: tagC.ID, name: tagC.Name }]]],
    });
    const image1 = ids[0];
    const image2 = ids[1];

    // Enter the cache-miss load-window state deterministically: point currentIndex at image2
    // while resourceDetails still describes image1 (exactly what onResourceChange leaves during
    // an in-flight details fetch) — without next()/key events, which race navigation under heavy
    // parallel load (next() early-returns on pageLoading / a short items list). Then drive a
    // batch add directly (the path applySuggestedTag uses — no details await) and inspect the
    // cache: the _currentDetails guard must keep image1's details out of image2's cache slot.
    const result = await page.evaluate(
      async ({ tagId, tagName, img2 }) => {
        const s = (window as any).Alpine.store('lightbox');
        const img2Index = s.items.findIndex((i: any) => i.id === img2);
        s.currentIndex = img2Index;
        const detailsIdBefore = s.resourceDetails?.ID ?? null;
        await s._batchToggleTags([{ ID: tagId, Name: tagName }], 'add');
        const cached = s.detailsCache.get(img2);
        return { img2Index, detailsIdBefore, cachedId: cached ? cached.ID : null };
      },
      { tagId: tagC.ID, tagName: tagC.Name, img2: image2 }
    );

    // Sanity: image2 is a distinct later item, and the window really was open — currentIndex
    // points at image2 while the panel still held image1's details.
    expect(result.img2Index).toBeGreaterThan(0);
    expect(result.detailsIdBefore).toBe(image1);
    // The cache slot for image2 must never hold another image's details.
    expect(result.cachedId === null || result.cachedId === image2).toBe(true);

    // The write itself targeted the current resource (image2), and left image1 untouched.
    await expect.poll(() => resourceHasTag(apiClient, image2, tagC.Name)).toBe(true);
    expect(await resourceHasTag(apiClient, image1, tagC.Name)).toBe(false);
  });

  test('#1b (decision path): a slot press during the load window is decided against the NEW image, not the stale one', async ({
    page,
    apiClient,
  }) => {
    const tagC = await apiClient.createTag(`StaleDecideC-${testRunId}`);

    const { ids } = await seedAndOpen(page, {
      quickSlots: [[[{ id: tagC.ID, name: tagC.Name }]]],
    });
    const image1 = ids[0];
    const image2 = ids[1];

    // Give image1 the tag (and update its live details), so during image2's load window the
    // panel still shows the slot as fully applied. A decision made against those stale details
    // would issue a no-op REMOVE against image2 and the intended tag would never land.
    await page.evaluate(async ({ tagId, tagName }) => {
      await (window as any).Alpine.store('lightbox')._batchToggleTags(
        [{ ID: tagId, Name: tagName }],
        'add'
      );
    }, { tagId: tagC.ID, tagName: tagC.Name });
    await expect.poll(() => resourceHasTag(apiClient, image1, tagC.Name)).toBe(true);

    // Force image2 into a cache-miss window whose details fetch is slow.
    await evictAndQuiesce(page, image2);
    await delayDetailsFor(page, image2, 800);

    // Enter the window and act inside it deterministically (no next()/key races): point at image2,
    // fire its (delayed) details fetch as navigation would, and toggle the slot while resourceDetails
    // still holds image1. The decision-path fix waits for image2's real details before deciding, so
    // the tag is ADDED to image2 rather than a no-op remove against the stale image1 tag set.
    await page.evaluate(async ({ img2 }) => {
      const s = (window as any).Alpine.store('lightbox');
      s.currentIndex = s.items.findIndex((i: any) => i.id === img2);
      s.fetchResourceDetails(); // fire the delayed image2 fetch as onResourceChange would; do NOT await
      await s.toggleTabTag(0);
    }, { img2: image2 });

    await expect.poll(() => resourceHasTag(apiClient, image2, tagC.Name)).toBe(true);
    // image1 was not disturbed by the cross-window action.
    expect(await resourceHasTag(apiClient, image1, tagC.Name)).toBe(true);

    await page.unroute('**/resource.json**');
  });

  test('#3: R does not announce a false "Repeated" success when the tag write fails', async ({
    page,
    apiClient,
  }) => {
    const tagA = await apiClient.createTag(`RepeatFailA-${testRunId}`);
    const tagB = await apiClient.createTag(`RepeatFailB-${testRunId}`);

    const { ids } = await seedAndOpen(page, {
      quickSlots: [
        [
          [{ id: tagA.ID, name: tagA.Name }],
          [{ id: tagB.ID, name: tagB.Name }],
        ],
      ],
    });
    const image1 = ids[0];
    const image2 = ids[1];

    // Tag image1 (updating its live details) so navigating snapshots a non-empty carry-forward
    // set. Driven via the store to stay deterministic under heavy parallel load.
    await page.evaluate(async ({ a, b }) => {
      await (window as any).Alpine.store('lightbox')._batchToggleTags(
        [{ ID: a.id, Name: a.name }, { ID: b.id, Name: b.name }],
        'add'
      );
    }, { a: { id: tagA.ID, name: tagA.Name }, b: { id: tagB.ID, name: tagB.Name } });
    await expect.poll(() => resourceHasTag(apiClient, image1, tagA.Name)).toBe(true);
    await expect.poll(() => resourceHasTag(apiClient, image1, tagB.Name)).toBe(true);

    // Navigate deterministically: point at image2 and run the resource-change hook, which
    // snapshots image1's tags for carry-forward (exactly as ArrowRight would) and loads image2's
    // details so repeat diffs against the right tag set.
    await page.evaluate(async ({ img2 }) => {
      const s = (window as any).Alpine.store('lightbox');
      s.currentIndex = s.items.findIndex((i: any) => i.id === img2);
      await s.onResourceChange();
    }, { img2: image2 });
    await expect
      .poll(() => page.evaluate(() => (window as any).Alpine.store('lightbox').resourceDetails?.ID))
      .toBe(image2);

    // Fail every addTags attempt (the client retries 5xx up to 3 times).
    let addTagsAttempts = 0;
    await page.route('**/v1/resources/addTags', async (route: Route) => {
      addTagsAttempts++;
      await route.fulfill({
        status: 500,
        contentType: 'application/json',
        body: JSON.stringify({ error: 'forced failure' }),
      });
    });

    await blurAndPress(page, 'r');

    // Wait until the retry sequence is exhausted, then let the live region debounce settle.
    await expect.poll(() => addTagsAttempts).toBeGreaterThanOrEqual(3);
    await page.waitForTimeout(300);

    const settled = await liveText(page);
    expect(settled).toMatch(/failed to add tags/i);
    expect(settled).not.toMatch(/repeated/i);

    // And nothing was actually applied to image2.
    expect(await resourceHasTag(apiClient, image2, tagA.Name)).toBe(false);

    await page.unroute('**/v1/resources/addTags');
  });
});
