import { test, expect } from '../fixtures/base.fixture';
import type { Page } from '@playwright/test';
import path from 'path';

/**
 * Tier 2 — chip-input niceties in the lightbox quick-tag panel (standalone autocompleter):
 *  - comma commits a tag chip
 *  - backspace on the empty input removes the last pill and fires saveTagRemoval
 *  - a "Create X" row creates + applies an unknown tag
 *  - a newly committed chip shows a pending state while /v1/resources/addTags is in flight
 *  - on a failed add the optimistic chip is rolled back
 */
test.describe('Lightbox chip-input', () => {
  const runId = Date.now();
  let categoryId: number;
  let ownerGroupId: number;
  const resourceIds: number[] = [];
  const tagIds: number[] = [];

  const LIGHTBOX =
    '[role="dialog"][aria-modal="true"]:not([aria-labelledby="paste-upload-title"]):not([aria-labelledby="entity-picker-title"])';
  const PILL = '[data-quick-tag-panel] span.bg-amber-700.rounded-full';

  test.beforeAll(async ({ apiClient }) => {
    const cat = await apiClient.createCategory(`ChipLightbox Cat ${runId}`);
    categoryId = cat.ID;
    const group = await apiClient.createGroup({ name: `ChipLightbox Owner ${runId}`, categoryId });
    ownerGroupId = group.ID;

    const files = [
      path.join(__dirname, '../test-assets/sample-image-13.png'),
      path.join(__dirname, '../test-assets/sample-image-2.png'),
      path.join(__dirname, '../test-assets/sample-image-3.png'),
      path.join(__dirname, '../test-assets/sample-image-4.png'),
      path.join(__dirname, '../test-assets/sample-image-5.png'),
      path.join(__dirname, '../test-assets/sample-image-6.png'),
    ];
    for (let i = 0; i < files.length; i++) {
      const r = await apiClient.createResource({
        filePath: files[i],
        name: `ChipLightbox Img ${i + 1} - ${runId}`,
        ownerId: ownerGroupId,
      });
      resourceIds.push(r.ID);
    }
  });

  test.afterAll(async ({ apiClient }) => {
    for (const id of resourceIds) { try { await apiClient.deleteResource(id); } catch { /* */ } }
    for (const id of tagIds) { try { await apiClient.deleteTag(id); } catch { /* */ } }
    if (ownerGroupId) await apiClient.deleteGroup(ownerGroupId);
    if (categoryId) await apiClient.deleteCategory(categoryId);
  });

  // Open the lightbox on a specific resource (by id, since the list is newest-first), open the
  // quick-tag panel, and return the tag input.
  async function openPanel(page: Page, resourceId: number) {
    await page.goto(`/resources?OwnerId=${ownerGroupId}`);
    await page.waitForLoadState('load');
    await page.locator(`[data-lightbox-item][data-resource-id="${resourceId}"]`).first().click();
    const lightbox = page.locator(LIGHTBOX);
    await expect(lightbox).toBeVisible();
    await page.keyboard.press('t');
    const input = lightbox.locator('[data-tag-editor-input]');
    await expect(input).toBeVisible({ timeout: 10000 });
    await expect
      .poll(() => page.evaluate(() => (window as any).Alpine.store('lightbox').resourceDetails !== null))
      .toBe(true);
    return { lightbox, input };
  }

  test('comma commits a tag chip in the lightbox', async ({ page, apiClient }) => {
    const tag = await apiClient.createTag(`LbComma${runId}`);
    tagIds.push(tag.ID);

    const { lightbox, input } = await openPanel(page, resourceIds[0]);

    const addReq = page.waitForRequest((r) => r.url().includes('/v1/resources/addTags') && r.method() === 'POST');

    await input.click();
    await input.fill(tag.Name);
    await expect(lightbox.locator('[role="option"]').filter({ hasText: tag.Name })).toBeVisible({ timeout: 5000 });
    await input.press(',');

    await addReq;
    await expect(lightbox.locator(PILL).filter({ hasText: tag.Name })).toBeVisible({ timeout: 5000 });
    await expect(input).toHaveValue('');
  });

  test('backspace on the empty input removes the last pill and fires removeTags', async ({ page, apiClient }) => {
    const tag = await apiClient.createTag(`LbBackspace${runId}`);
    tagIds.push(tag.ID);
    // Seed: apply the tag to the second resource so a pill exists on open.
    await apiClient.addTagsToResources([resourceIds[1]], [tag.ID]);

    const { lightbox, input } = await openPanel(page, resourceIds[1]);
    await expect(lightbox.locator(PILL).filter({ hasText: tag.Name })).toBeVisible({ timeout: 5000 });

    const removeReq = page.waitForRequest((r) => r.url().includes('/v1/resources/removeTags') && r.method() === 'POST');

    await input.click();
    await expect(input).toHaveValue('');
    await input.press('Backspace');

    await removeReq;
    await expect(lightbox.locator(PILL).filter({ hasText: tag.Name })).toHaveCount(0, { timeout: 5000 });
  });

  test('a "Create X" row creates and applies an unknown tag', async ({ page }) => {
    const newName = `LbCreate${runId}`;

    const { lightbox, input } = await openPanel(page, resourceIds[2]);

    await input.click();
    await input.fill(newName);

    const createRow = lightbox.locator('[role="option"]').filter({ hasText: `Create "${newName}"` });
    await expect(createRow).toBeVisible({ timeout: 5000 });
    await createRow.click();

    await expect(lightbox.locator(PILL).filter({ hasText: newName })).toBeVisible({ timeout: 5000 });
  });

  test('a newly committed chip shows a pending state while addTags is in flight', async ({ page, apiClient }) => {
    const tag = await apiClient.createTag(`LbPending${runId}`);
    tagIds.push(tag.ID);

    const { lightbox, input } = await openPanel(page, resourceIds[3]);

    // Delay the add so the pending state is observable.
    await page.route('**/v1/resources/addTags', async (route) => {
      await new Promise((r) => setTimeout(r, 1200));
      await route.continue();
    });

    await input.click();
    await input.fill(tag.Name);
    await expect(lightbox.locator('[role="option"]').filter({ hasText: tag.Name })).toBeVisible({ timeout: 5000 });
    await input.press(',');

    const pill = lightbox.locator(PILL).filter({ hasText: tag.Name });
    await expect(pill).toBeVisible({ timeout: 5000 });
    // Pending while in flight, then cleared on success.
    await expect(pill).toHaveAttribute('data-tag-pending', 'true');
    await expect(pill).toHaveAttribute('data-tag-pending', 'false', { timeout: 5000 });
  });

  test('comma-commits two brand-new tags back-to-back without dropping the second', async ({ page }) => {
    const nameA = `LbRaceA${runId}`;
    const nameB = `LbRaceB${runId}`;

    const { lightbox, input } = await openPanel(page, resourceIds[5]);

    // Delay tag creation so the second comma-commit's create call starts while the
    // first's POST /v1/tag is still in flight -- the exact race the bug depends on (a
    // single component-wide `loading` flag silently drops any create that starts mid-flight).
    await page.route('**/v1/tag', async (route) => {
      if (route.request().method() === 'POST') {
        await new Promise((r) => setTimeout(r, 600));
      }
      await route.continue();
    });

    await input.click();
    await input.fill(nameA);
    await input.press(',');
    await input.fill(nameB);
    await input.press(',');

    await expect(lightbox.locator(PILL).filter({ hasText: nameA })).toBeVisible({ timeout: 5000 });
    await expect(lightbox.locator(PILL).filter({ hasText: nameB })).toBeVisible({ timeout: 5000 });
  });

  test('a failed add rolls the optimistic chip back', async ({ page, apiClient }) => {
    const tag = await apiClient.createTag(`LbFail${runId}`);
    tagIds.push(tag.ID);

    const { lightbox, input } = await openPanel(page, resourceIds[4]);

    await page.route('**/v1/resources/addTags', (route) =>
      route.fulfill({ status: 500, contentType: 'application/json', body: '{"error":"boom"}' })
    );

    await input.click();
    await input.fill(tag.Name);
    await expect(lightbox.locator('[role="option"]').filter({ hasText: tag.Name })).toBeVisible({ timeout: 5000 });
    await input.press(',');

    const pill = lightbox.locator(PILL).filter({ hasText: tag.Name });
    // The optimistic chip is rolled back once the add fails.
    await expect(pill).toHaveCount(0, { timeout: 6000 });
  });
});
