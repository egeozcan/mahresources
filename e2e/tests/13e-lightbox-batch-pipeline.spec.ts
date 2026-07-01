import { test, expect } from '../fixtures/base.fixture';
import type { Page } from '@playwright/test';
import { seedQuickTags } from '../helpers/quick-tags';
import path from 'path';

/**
 * Tier 1: Batch tagging pipeline in the lightbox.
 *
 *  - Item 4 (carry-forward): press R to re-apply the previous image's tags.
 *  - Item 5 (flow mode): auto-advance to the next image after a quick-slot add.
 *  - Item 6 (global undo): press U / Ctrl+Z to invert the last batch tag change,
 *    even after navigating away from the affected image.
 *
 * All three are driven through the real key bindings and verified against the
 * server (getResource) plus the lightbox live region.
 */
test.describe('Lightbox batch tagging pipeline', () => {
  let categoryId: number;
  let ownerGroupId: number;
  const createdResourceIds: number[] = [];
  const testRunId = Date.now();

  const LIGHTBOX =
    '[role="dialog"][aria-modal="true"]:not([aria-labelledby="paste-upload-title"]):not([aria-labelledby="entity-picker-title"])';

  test.beforeAll(async ({ apiClient }) => {
    const category = await apiClient.createCategory(
      `Lightbox Batch Category ${testRunId}`,
      'Category for lightbox batch pipeline tests'
    );
    categoryId = category.ID;

    const ownerGroup = await apiClient.createGroup({
      name: `Lightbox Batch Owner ${testRunId}`,
      description: 'Owner for lightbox batch pipeline resources',
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
        name: `Lightbox Batch Image ${i + 1} - ${testRunId}`,
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
    flowMode?: boolean;
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
      ...(seed.flowMode !== undefined ? { flowMode: seed.flowMode } : {}),
    };
  }

  // Seed quick-tag localStorage, reload so the store re-reads it, open the lightbox
  // on the first image with the panel auto-open, and blur the input so the global
  // letter/digit shortcuts are live.
  async function seedAndOpen(page: Page, seed: Seed) {
    await page.goto(`/resources?OwnerId=${ownerGroupId}`);
    await page.waitForLoadState('load');
    await seedQuickTags(page, buildSeed(seed));
    await page.goto(`/resources?OwnerId=${ownerGroupId}`);
    await page.waitForLoadState('load');

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

    // Blur so document.activeElement is the body — canPanelShortcut() then returns true.
    await page.evaluate(() => (document.activeElement as HTMLElement)?.blur());

    const ids: number[] = await page.evaluate(() =>
      (window as any).Alpine.store('lightbox').items.map((i: any) => i.id)
    );
    return { lightbox, panel, ids };
  }

  async function resourceHasTag(apiClient: any, id: number, tagName: string): Promise<boolean> {
    const r = await apiClient.getResource(id);
    return (r.Tags || []).some((t: any) => t.Name === tagName);
  }

  // The page has many [role="status"] regions (each autocompleter creates one). Read the
  // lightbox store's own live region directly to avoid strict-mode ambiguity.
  async function liveText(page: Page): Promise<string> {
    return page.evaluate(() => (window as any).Alpine.store('lightbox').liveRegion?.textContent || '');
  }

  // Wait until navigation has fully settled: the new image's details are loaded AND every
  // background prefetch (_preloadDetailsUpcoming) has drained. A cross-resource undo write
  // issued while those reads are in flight contends with them on SQLite (the E2E server runs
  // with -max-db-connections=2), which intermittently 500s the write.
  async function waitDetailsIdle(page: Page, expectedIndex: number, expectedId: number) {
    await expect
      .poll(() =>
        page.evaluate(
          ({ idx, id }) => {
            const s = (window as any).Alpine.store('lightbox');
            return (
              s.currentIndex === idx &&
              s.detailsLoading === false &&
              s.resourceDetails?.ID === id &&
              (s._detailsInFlight?.size ?? 0) === 0
            );
          },
          { idx: expectedIndex, id: expectedId }
        )
      )
      .toBe(true);
  }

  // The undo/repeat/digit shortcuts route through canPanelShortcut(), which reads
  // document.activeElement and bails when focus is inside the panel or a text field. They are
  // bound as @keydown.*.window, so the window listener fires regardless of which element has
  // focus — only canPanelShortcut's activeElement check matters. To make that deterministic we
  // focus the dialog root (a non-input element inside the focus trap) AND dispatch the keydown
  // in the SAME evaluate, so the focus-trap cannot drift focus in the gap between two CDP calls
  // (the source of intermittent "key did nothing" flakes under parallel load).
  async function blurAndPress(page: Page, key: string) {
    await page.evaluate((rawKey) => {
      const dlg = document.querySelector(
        '[role="dialog"][aria-modal="true"]:not([aria-labelledby="paste-upload-title"]):not([aria-labelledby="entity-picker-title"])'
      );
      (dlg as HTMLElement)?.focus();
      let ctrlKey = false;
      let metaKey = false;
      let key = rawKey;
      if (rawKey.startsWith('Control+')) {
        ctrlKey = true;
        key = rawKey.slice('Control+'.length);
      } else if (rawKey.startsWith('Meta+')) {
        metaKey = true;
        key = rawKey.slice('Meta+'.length);
      }
      window.dispatchEvent(
        new KeyboardEvent('keydown', { key, ctrlKey, metaKey, bubbles: true, cancelable: true })
      );
    }, key);
  }

  test('Item 4: R re-applies the previous image tags to the current one', async ({ page, apiClient }) => {
    const tagA = await apiClient.createTag(`CarryA-${testRunId}`);
    const tagB = await apiClient.createTag(`CarryB-${testRunId}`);

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

    // Apply tagA (key 1) and tagB (key 2) to image1.
    await blurAndPress(page, '1');
    await expect.poll(() => resourceHasTag(apiClient, image1, tagA.Name)).toBe(true);
    await blurAndPress(page, '2');
    await expect.poll(() => resourceHasTag(apiClient, image1, tagB.Name)).toBe(true);

    // Navigate to image2 (snapshots image1's tag set) and repeat with R.
    await blurAndPress(page, 'ArrowRight');
    await expect
      .poll(() => page.evaluate(() => (window as any).Alpine.store('lightbox').currentIndex))
      .toBe(1);
    await blurAndPress(page, 'r');

    await expect.poll(() => resourceHasTag(apiClient, image2, tagA.Name)).toBe(true);
    await expect.poll(() => resourceHasTag(apiClient, image2, tagB.Name)).toBe(true);

    // Live region names the repeat with a count.
    await expect.poll(async () => /repeated 2 tag/i.test(await liveText(page))).toBe(true);
  });

  test('Item 5: flow mode auto-advances after a quick-slot add', async ({ page, apiClient }) => {
    const tagA = await apiClient.createTag(`FlowA-${testRunId}`);

    const { lightbox } = await seedAndOpen(page, {
      flowMode: true,
      quickSlots: [[[{ id: tagA.ID, name: tagA.Name }]]],
    });

    const counter = lightbox.locator('div.bg-black\\/50:has-text("/")').first();
    await expect(counter).toContainText('1 /');

    const ids: number[] = await page.evaluate(() =>
      (window as any).Alpine.store('lightbox').items.map((i: any) => i.id)
    );
    const image1 = ids[0];

    await blurAndPress(page, '1');

    // Auto-advanced to the next image.
    await expect(counter).toContainText('2 /');
    await expect.poll(() => resourceHasTag(apiClient, image1, tagA.Name)).toBe(true);

    // The single combined announcement names the tag AND the new position.
    await expect
      .poll(async () => {
        const t = await liveText(page);
        return new RegExp(tagA.Name).test(t) && /2 of/i.test(t);
      })
      .toBe(true);
  });

  test('Item 5: the flow toggle turns on and a subsequent add advances', async ({ page, apiClient }) => {
    const tagA = await apiClient.createTag(`FlowToggle-${testRunId}`);

    const { lightbox, panel } = await seedAndOpen(page, {
      flowMode: false,
      quickSlots: [[[{ id: tagA.ID, name: tagA.Name }]]],
    });

    // Flow is a toggle button (aria-pressed), not role="switch", to avoid colliding with the
    // many role="switch" controls elsewhere (the lightbox partial is on every page).
    const flowToggle = panel.getByRole('button', { name: 'Auto-advance after tagging (flow mode)' });
    await expect(flowToggle).toHaveAttribute('aria-pressed', 'false');
    await flowToggle.click();
    await expect(flowToggle).toHaveAttribute('aria-pressed', 'true');

    const counter = lightbox.locator('div.bg-black\\/50:has-text("/")').first();
    await expect(counter).toContainText('1 /');
    await blurAndPress(page, '1');
    await expect(counter).toContainText('2 /');
  });

  test('Item 6: U undoes the last tag change on the originating image after navigation', async ({
    page,
    apiClient,
  }) => {
    const tagA = await apiClient.createTag(`UndoA-${testRunId}`);

    const { ids } = await seedAndOpen(page, {
      quickSlots: [[[{ id: tagA.ID, name: tagA.Name }]]],
    });
    const image1 = ids[0];
    const image2 = ids[1];

    await blurAndPress(page, '1');
    await expect.poll(() => resourceHasTag(apiClient, image1, tagA.Name)).toBe(true);

    // Move to image2 and let its detail fetch + background prefetches settle, so the undo
    // write to image1 does not race the navigation's in-flight reads, then undo.
    await blurAndPress(page, 'ArrowRight');
    await waitDetailsIdle(page, 1, image2);
    await blurAndPress(page, 'u');

    await expect.poll(() => resourceHasTag(apiClient, image1, tagA.Name)).toBe(false);
    await expect.poll(async () => /removed/i.test(await liveText(page))).toBe(true);
  });

  test('Item 6: Ctrl+Z undoes without switching the active tab', async ({ page, apiClient }) => {
    const tagA = await apiClient.createTag(`UndoCtrlZ-${testRunId}`);

    const { ids } = await seedAndOpen(page, {
      quickSlots: [[[{ id: tagA.ID, name: tagA.Name }]]],
    });
    const image1 = ids[0];
    const image2 = ids[1];

    await blurAndPress(page, '1');
    await expect.poll(() => resourceHasTag(apiClient, image1, tagA.Name)).toBe(true);

    await blurAndPress(page, 'ArrowRight');
    await waitDetailsIdle(page, 1, image2);
    // Switch to QUICK 2 (key x) so a stray switchTab(0) from the z collision would be visible.
    await blurAndPress(page, 'x');
    await expect
      .poll(() => page.evaluate(() => (window as any).Alpine.store('lightbox').activeTab))
      .toBe(1);

    await blurAndPress(page, 'Control+z');

    await expect.poll(() => resourceHasTag(apiClient, image1, tagA.Name)).toBe(false);
    // Ctrl+Z must NOT have switched to QUICK 1.
    await expect
      .poll(() => page.evaluate(() => (window as any).Alpine.store('lightbox').activeTab))
      .toBe(1);
  });

  test('Item 6: Ctrl+Z undoes a tag applied by clicking a quick-slot button, with focus left on that button', async ({
    page,
    apiClient,
  }) => {
    const tagA = await apiClient.createTag(`UndoClickFocus-${testRunId}`);

    const { panel, ids } = await seedAndOpen(page, {
      quickSlots: [[[{ id: tagA.ID, name: tagA.Name }]]],
    });
    const image1 = ids[0];

    // Apply the tag with a REAL mouse click on the slot button (not the blurAndPress
    // helper), so focus naturally lands on that button afterward -- the exact state most
    // users are in immediately after tagging with the mouse.
    const slotButton = panel.getByRole('button', { name: `Add ${tagA.Name}` });
    await slotButton.click();
    await expect.poll(() => resourceHasTag(apiClient, image1, tagA.Name)).toBe(true);
    await expect(panel.getByRole('button', { name: `Remove ${tagA.Name}` })).toBeFocused();

    // Ctrl+Z must still undo even though focus never left the quick-tag panel.
    await page.keyboard.press('Control+z');

    await expect.poll(() => resourceHasTag(apiClient, image1, tagA.Name)).toBe(false);
    await expect.poll(async () => /removed/i.test(await liveText(page))).toBe(true);
  });
});
