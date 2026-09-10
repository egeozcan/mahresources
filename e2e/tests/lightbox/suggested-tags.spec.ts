import { test, expect } from '../../fixtures/base.fixture';
import type { Page } from '@playwright/test';
import path from 'path';

/**
 * Tier 3 — context-aware Suggested Tags row in the lightbox quick-tag panel.
 *
 * Seeds deterministically via the owner-group popular-tag source (perceptual
 * similarity needs the non-deterministic background hash worker, so that path
 * is covered by the Go tests instead): a group whose sibling resources carry
 * shared tags, opened on a tag-less resource from the same group.
 *
 *  - the Suggested row renders the owner group's common tags
 *  - clicking a chip applies the tag (verified via API) and drops the chip
 *  - Shift+1 applies the first suggestion and announces it
 *  - navigating to another resource refetches the row
 */
test.describe('Lightbox suggested tags', () => {
  const runId = Date.now();
  let categoryId: number;
  let ownerGroupId: number;
  const targetIds: number[] = [];
  const tagIds: number[] = [];
  const tagNames = {
    alpha: `SugAlpha${runId}`,
    beta: `SugBeta${runId}`,
    gamma: `SugGamma${runId}`,
  };

  const LIGHTBOX =
    '[role="dialog"][x-show="$store.lightbox.isOpen"]';
  const CHIP = '[data-quick-tag-panel] [data-suggested-tag]';

  test.beforeAll(async ({ apiClient }) => {
    const cat = await apiClient.createCategory(`SugLightbox Cat ${runId}`);
    categoryId = cat.ID;
    const group = await apiClient.createGroup({ name: `SugLightbox Owner ${runId}`, categoryId });
    ownerGroupId = group.ID;

    const alpha = await apiClient.createTag(tagNames.alpha);
    const beta = await apiClient.createTag(tagNames.beta);
    const gamma = await apiClient.createTag(tagNames.gamma);
    tagIds.push(alpha.ID, beta.ID, gamma.ID);

    // Distinct image files per resource: the app dedupes by content hash within a
    // parent group, so reusing a file would 409. We need 8 unique images.
    const asset = (n: number) => path.join(__dirname, `../../test-assets/sample-image-${n}.png`);
    let fileIdx = 2; // sample-image-2.png onward (sample-image.png has no numeric suffix)

    // Three siblings carry all three tags; an extra resource carries only alpha
    // so alpha is the most-used (ranks first). These drive the group source.
    for (let i = 0; i < 3; i++) {
      const r = await apiClient.createResource({
        filePath: asset(fileIdx++),
        name: `SugSibling ${i + 1} - ${runId}`,
        ownerId: ownerGroupId,
      });
      await apiClient.addTagsToResources([r.ID], [alpha.ID, beta.ID, gamma.ID]);
    }
    const extra = await apiClient.createResource({
      filePath: asset(fileIdx++),
      name: `SugExtraAlpha - ${runId}`,
      ownerId: ownerGroupId,
    });
    await apiClient.addTagsToResources([extra.ID], [alpha.ID]);

    // Tag-less targets, one per test that mutates state.
    for (let i = 0; i < 7; i++) {
      const r = await apiClient.createResource({
        filePath: asset(fileIdx++),
        name: `SugTarget ${i + 1} - ${runId}`,
        ownerId: ownerGroupId,
      });
      targetIds.push(r.ID);
    }
  });

  test.afterAll(async ({ apiClient }) => {
    for (const id of targetIds) { try { await apiClient.deleteResource(id); } catch { /* */ } }
    if (ownerGroupId) {
      // Sibling/extra resources are owned by the group; cleaned up with it.
      try { await apiClient.deleteGroup(ownerGroupId); } catch { /* */ }
    }
    for (const id of tagIds) { try { await apiClient.deleteTag(id); } catch { /* */ } }
    if (categoryId) await apiClient.deleteCategory(categoryId);
  });

  test.beforeEach(async ({ page }) => {
    // Quick-tag state is server-side now (shared across tests). Reset for isolation.
    await page.request.delete('/v1/account/settings/quickTags');
  });

  // Open the lightbox on a specific resource (list is newest-first, so open by id),
  // open the quick-tag panel, and wait for the suggested row to populate.
  async function openPanel(page: Page, resourceId: number) {
    await page.goto(`/resources?OwnerId=${ownerGroupId}`);
    await page.waitForLoadState('load');
    await page.locator(`[data-lightbox-item][data-resource-id="${resourceId}"]`).first().click();
    const lightbox = page.locator(LIGHTBOX);
    await expect(lightbox).toBeVisible();
    await page.keyboard.press('t');
    await expect(lightbox.locator('[data-tag-editor-input]')).toBeVisible({ timeout: 10000 });
    await expect
      .poll(() => page.evaluate(() => (window as any).Alpine.store('lightbox').resourceDetails !== null))
      .toBe(true);
    return lightbox;
  }

  test('renders the owner group common tags as suggestions', async ({ page }) => {
    const lightbox = await openPanel(page, targetIds[0]);

    const chips = lightbox.locator(CHIP);
    await expect(chips.filter({ hasText: tagNames.alpha })).toBeVisible({ timeout: 10000 });
    await expect(chips.filter({ hasText: tagNames.beta })).toBeVisible();
    await expect(chips.filter({ hasText: tagNames.gamma })).toBeVisible();
    // alpha is the most-used → ranks first.
    await expect(chips.first()).toContainText(tagNames.alpha);
  });

  test('clicking a suggested chip applies the tag and drops the chip', async ({ page, apiClient }) => {
    const lightbox = await openPanel(page, targetIds[1]);
    const alphaChip = lightbox.locator(CHIP).filter({ hasText: tagNames.alpha });
    await expect(alphaChip).toBeVisible({ timeout: 10000 });

    const addReq = page.waitForRequest(
      (r) => r.url().includes('/v1/resources/addTags') && r.method() === 'POST',
    );
    await alphaChip.click();
    await addReq;

    // The chip is optimistically removed from the suggested row.
    await expect(alphaChip).toHaveCount(0, { timeout: 5000 });

    // An association applied outside the tag editor still reaches its chip list: the
    // editor reconciles same-resource changes rather than only resetting on navigation.
    await expect(
      lightbox.locator('[data-quick-tag-panel] .flex.flex-wrap.gap-2 span.tag-pop')
        .filter({ hasText: tagNames.alpha }),
    ).toBeVisible({ timeout: 5000 });

    // Verified server-side: the resource now carries alpha.
    await expect
      .poll(async () => {
        const res = await apiClient.getResource(targetIds[1]) as any;
        return (res.Tags || []).some((t: any) => t.Name === tagNames.alpha);
      })
      .toBe(true);
  });

  test('Shift+1 applies the first suggestion and announces it', async ({ page, apiClient }) => {
    const lightbox = await openPanel(page, targetIds[2]);
    const firstChip = lightbox.locator(CHIP).first();
    await expect(firstChip).toBeVisible({ timeout: 10000 });
    const firstName = (await firstChip.textContent())?.trim().replace(/⇧\d+$/, '').trim() || '';

    // Move focus to the dialog root so the window-level shortcut fires (a focused
    // panel control would make canPanelShortcut() bail).
    await lightbox.focus();

    const addReq = page.waitForRequest(
      (r) => r.url().includes('/v1/resources/addTags') && r.method() === 'POST',
    );
    await page.keyboard.press('Shift+Digit1');
    await addReq;

    await expect
      .poll(async () => {
        const res = await apiClient.getResource(targetIds[2]) as any;
        return (res.Tags || []).some((t: any) => t.Name === firstName);
      })
      .toBe(true);

    // The live region announced the applied tag.
    await expect
      .poll(() =>
        page.evaluate(() =>
          /added/i.test((window as any).Alpine.store('lightbox').liveRegion?.textContent || ''),
        ),
      )
      .toBe(true);
  });

  test('applying a suggestion via the main search box also drops it from the suggested row', async ({
    page,
    apiClient,
  }) => {
    const lightbox = await openPanel(page, targetIds[4]);
    const alphaChip = lightbox.locator(CHIP).filter({ hasText: tagNames.alpha });
    await expect(alphaChip).toBeVisible({ timeout: 10000 });

    // Apply "alpha" via the main "Search or add tags" autocompleter, NOT the suggested-row
    // chip itself -- this exercises saveTagAddition(), a separate code path from
    // applySuggestedTag() that historically didn't prune the suggested row.
    const input = lightbox.locator('[data-tag-editor-input]');
    await input.click();
    await input.fill(tagNames.alpha);
    await expect(lightbox.locator('[role="option"]').filter({ hasText: tagNames.alpha })).toBeVisible({
      timeout: 5000,
    });
    await input.press(',');

    await expect
      .poll(async () => {
        const res = (await apiClient.getResource(targetIds[4])) as any;
        return (res.Tags || []).some((t: any) => t.Name === tagNames.alpha);
      })
      .toBe(true);

    // The now-applied tag must disappear from the Suggested row too.
    await expect(alphaChip).toHaveCount(0, { timeout: 5000 });
  });

  test('navigating to another resource refetches the suggested row', async ({ page }) => {
    const lightbox = await openPanel(page, targetIds[3]);
    await expect(lightbox.locator(CHIP).first()).toBeVisible({ timeout: 10000 });

    const startId = await page.evaluate(() =>
      (window as any).Alpine.store('lightbox').getCurrentItem()?.id,
    );

    const refetch = page.waitForRequest((r) => {
      if (!r.url().includes('/v1/resource/suggestedTags')) return false;
      const id = new URL(r.url()).searchParams.get('id');
      return id !== null && Number(id) !== startId;
    });

    await page.keyboard.press('ArrowRight');
    const req = await refetch;
    const newId = Number(new URL(req.url()).searchParams.get('id'));
    expect(newId).not.toBe(startId);
  });
  test('quick-slot additions immediately remove suggested chips and refresh after every edit', async ({ page, apiClient }) => {
    // Persist real quick-slot settings before loading the page, matching the
    // keyboard path used during sustained tagging rather than calling store methods.
    const response = await page.request.put('/v1/account/settings/quickTags', {
      data: { value: {
        version: 3,
        quickSlots: [
          [
            [{ id: tagIds[0], name: tagNames.alpha }],
            [{ id: tagIds[1], name: tagNames.beta }],
            ...Array(7).fill(null),
          ],
          ...Array.from({ length: 3 }, () => Array(9).fill(null)),
        ],
        recentTags: [],
      } },
    });
    expect(response.ok()).toBe(true);
    const lightbox = await openPanel(page, targetIds[6]);
    const chips = lightbox.locator(CHIP);
    await expect(chips.filter({ hasText: tagNames.alpha })).toBeVisible();
    await expect(chips.filter({ hasText: tagNames.beta })).toBeVisible();
    await lightbox.focus();
    for (const [key, name, id] of [['1', tagNames.alpha, tagIds[0]], ['2', tagNames.beta, tagIds[1]]] as const) {
      const refreshed = page.waitForResponse(r => r.url().includes(`/v1/resource/suggestedTags?id=${targetIds[6]}`));
      await page.keyboard.press(key);
      await expect.poll(async () => ((await apiClient.getResource(targetIds[6])) as any).Tags.some((tag: any) => tag.ID === id)).toBe(true);
      await expect(chips.filter({ hasText: name })).toHaveCount(0);
      expect((await refreshed).ok()).toBe(true);
    }
  });

  test('adding a seed tag reveals a related suggestion without navigation', async ({ page, apiClient }) => {
    const seed = await apiClient.createTag(`SugContextSeed${runId}`);
    const related = await apiClient.createTag(`SugRelated${runId}`);
    const filler = [];
    for (let i = 0; i < 8; i++) filler.push(await apiClient.createTag(`SugFiller${i}-${runId}`));
    tagIds.push(seed.ID, related.ID, ...filler.map(tag => tag.ID));
    const created: number[] = [];
    try {
      // Text peers avoid nondeterministic background image similarity. Eight more
      // popular tags keep the related tag out of the initial suggestion row.
      for (let i = 0; i < 11; i++) {
        const response = await page.request.post('/v1/resource', {
          multipart: {
            Name: `Suggested context peer ${i}`,
            OwnerId: String(ownerGroupId),
            resource: { name: `context-${i}.txt`, mimeType: 'text/plain', buffer: Buffer.from(`suggested context ${runId} ${i}`) },
          },
        });
        expect(response.ok()).toBe(true);
        const [resource] = await response.json();
        created.push(resource.ID);
        await apiClient.addTagsToResources([resource.ID], i < 6 ? filler.map(tag => tag.ID) : [seed.ID, related.ID]);
      }
      const lightbox = await openPanel(page, targetIds[5]);
      const chips = lightbox.locator(CHIP);
      await expect(chips).toHaveCount(8);
      await expect(chips.filter({ hasText: related.Name })).toHaveCount(0);
      const input = lightbox.locator('[data-tag-editor-input]');
      await input.fill(seed.Name);
      await lightbox.getByRole('option', { name: seed.Name, exact: true }).click();
      await expect(chips.filter({ hasText: related.Name })).toBeVisible();
      expect(await page.evaluate(() => (window as any).Alpine.store('lightbox').getCurrentItem()?.id)).toBe(targetIds[5]);
      const response = await page.request.get(`/v1/resource/suggestedTags?id=${targetIds[5]}`);
      const body = await response.json();
      expect(body.suggestions.find((tag: any) => tag.ID === related.ID).sources).toContain('cooccurrence');
    } finally {
      for (const id of created) await apiClient.deleteResource(id);
    }
  });

});
