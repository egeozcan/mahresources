import { test, expect } from '../fixtures/base.fixture';
import AxeBuilder from '@axe-core/playwright';
import fs from 'fs';
import os from 'os';
import path from 'path';

/**
 * Tier 3 — "Tag Untagged Only" launcher.
 *
 * `Untagged=1` on the resource list filters to resources with zero tags; the
 * lightbox naturally pages over only those items because the query string
 * (preserved as `baseUrl`) rides through `fetchPage`.
 *
 * Each resource is uploaded from a freshly-written PNG (a known-good base
 * image plus a unique trailing marker) rather than a shared test-asset file:
 * the app dedupes resources by content SHA1 *globally*, and many other specs
 * already reuse the small numbered sample-image-N.png pool on the same
 * worker's shared ephemeral DB. Reusing one of those here would silently
 * attach an existing (possibly already-tagged) resource from another spec
 * instead of creating a fresh untagged one, making this suite's assertions
 * about tag state flaky and worker-assignment-dependent.
 */
test.describe('Lightbox tag-untagged launcher', () => {
  const runId = Date.now();
  let categoryId: number;
  let ownerGroupId: number;
  let tagId: number;
  const taggedIds: number[] = [];
  const untaggedIds: number[] = [];
  const tmpFiles: string[] = [];

  function uniqueImage(marker: string): string {
    const base = fs.readFileSync(path.join(__dirname, '../test-assets/sample-image.png'));
    const unique = Buffer.concat([base, Buffer.from(`untagged-test-${runId}-${marker}`)]);
    const tmpPath = path.join(os.tmpdir(), `untagged-${runId}-${marker}.png`);
    fs.writeFileSync(tmpPath, unique);
    tmpFiles.push(tmpPath);
    return tmpPath;
  }

  test.beforeAll(async ({ apiClient }) => {
    const cat = await apiClient.createCategory(`UntaggedLightbox Cat ${runId}`);
    categoryId = cat.ID;
    const group = await apiClient.createGroup({ name: `UntaggedLightbox Owner ${runId}`, categoryId });
    ownerGroupId = group.ID;

    const tag = await apiClient.createTag(`UntaggedTestTag${runId}`);
    tagId = tag.ID;

    for (let i = 0; i < 2; i++) {
      const r = await apiClient.createResource({
        filePath: uniqueImage(`tagged-${i}`),
        name: `UntaggedSpec Tagged ${i + 1} - ${runId}`,
        ownerId: ownerGroupId,
      });
      await apiClient.addTagsToResources([r.ID], [tagId]);
      taggedIds.push(r.ID);
    }
    for (let i = 0; i < 3; i++) {
      const r = await apiClient.createResource({
        filePath: uniqueImage(`untagged-${i}`),
        name: `UntaggedSpec Untagged ${i + 1} - ${runId}`,
        ownerId: ownerGroupId,
      });
      untaggedIds.push(r.ID);
    }
  });

  test.afterAll(async ({ apiClient }) => {
    for (const id of [...taggedIds, ...untaggedIds]) {
      try { await apiClient.deleteResource(id); } catch { /* */ }
    }
    if (ownerGroupId) { try { await apiClient.deleteGroup(ownerGroupId); } catch { /* */ } }
    if (tagId) { try { await apiClient.deleteTag(tagId); } catch { /* */ } }
    if (categoryId) { try { await apiClient.deleteCategory(categoryId); } catch { /* */ } }
    for (const f of tmpFiles) { try { fs.unlinkSync(f); } catch { /* */ } }
  });

  test('Untagged=1 renders only the untagged resources', async ({ page }) => {
    await page.goto(`/resources/details?ownerId=${ownerGroupId}&Untagged=1`);
    await page.waitForLoadState('load');

    for (const id of untaggedIds) {
      await expect(page.locator(`[data-lightbox-item][data-resource-id="${id}"]`)).toBeVisible();
    }
    for (const id of taggedIds) {
      await expect(page.locator(`[data-lightbox-item][data-resource-id="${id}"]`)).toHaveCount(0);
    }
  });

  test('without Untagged, both tagged and untagged resources render', async ({ page }) => {
    await page.goto(`/resources/details?ownerId=${ownerGroupId}`);
    await page.waitForLoadState('load');

    for (const id of [...taggedIds, ...untaggedIds]) {
      await expect(page.locator(`[data-lightbox-item][data-resource-id="${id}"]`)).toBeVisible();
    }
  });

  test('the Untagged checkbox renders checked and labelled after filtering', async ({ page }) => {
    await page.goto(`/resources/details?ownerId=${ownerGroupId}&Untagged=1`);
    await page.waitForLoadState('load');

    const checkbox = page.locator('input[name="Untagged"]');
    await expect(checkbox).toBeChecked();
    const label = page.locator('label', { has: checkbox });
    await expect(label).toContainText('Only Untagged');
  });

  test('the lightbox pages over untagged items only, never landing on a tagged one', async ({ page }) => {
    await page.goto(`/resources/details?ownerId=${ownerGroupId}&Untagged=1`);
    await page.waitForLoadState('load');

    // The list renders newest-first; read the actual DOM order rather than
    // assuming it matches creation order (see lessons.md).
    const domOrderIds = await page
      .locator('[data-lightbox-item]')
      .evaluateAll((els) => els.map((el) => Number(el.getAttribute('data-resource-id'))));
    expect([...domOrderIds].sort((a, b) => a - b)).toEqual([...untaggedIds].sort((a, b) => a - b));

    await page.locator(`[data-lightbox-item][data-resource-id="${domOrderIds[0]}"]`).first().click();
    const lightbox = page.locator(
      '[role="dialog"][aria-modal="true"]:not([aria-labelledby="paste-upload-title"]):not([aria-labelledby="entity-picker-title"])',
    );
    await expect(lightbox).toBeVisible();

    const seen = new Set<number>();
    for (let i = 0; i < domOrderIds.length; i++) {
      // Wait for the navigation/details fetch from the previous ArrowRight to
      // settle before reading — getCurrentItem() updates synchronously on
      // navigation, but resourceDetails (and therefore a stable read) lags
      // behind a beat while fetchResourceDetails() is in flight.
      await expect
        .poll(() => page.evaluate(() => (window as any).Alpine.store('lightbox').detailsLoading))
        .toBe(false);
      const currentId = await page.evaluate(() =>
        (window as any).Alpine.store('lightbox').getCurrentItem()?.id,
      );
      expect(taggedIds, `lightbox must never land on a tagged resource (got ${currentId})`).not.toContain(currentId);
      seen.add(currentId);
      await page.keyboard.press('ArrowRight');
    }
    // every untagged resource was reachable while paging through the filtered set.
    for (const id of untaggedIds) {
      expect(seen.has(id), `expected to page through untagged resource ${id}`).toBe(true);
    }
  });

  test('the group page "Tag untagged" link lands on the filtered, owner-scoped list', async ({ page }) => {
    await page.goto(`/group?id=${ownerGroupId}`);
    await page.waitForLoadState('load');

    const link = page.getByRole('link', { name: 'Tag untagged' });
    await expect(link).toBeVisible();
    await link.click();
    await page.waitForLoadState('load');

    expect(page.url()).toContain(`ownerId=${ownerGroupId}`);
    expect(page.url()).toContain('Untagged=1');
    for (const id of untaggedIds) {
      await expect(page.locator(`[data-lightbox-item][data-resource-id="${id}"]`)).toBeVisible();
    }
    for (const id of taggedIds) {
      await expect(page.locator(`[data-lightbox-item][data-resource-id="${id}"]`)).toHaveCount(0);
    }
  });

  test('axe finds zero Serious+ violations on the filtered list with the new checkbox', async ({ page }) => {
    await page.goto(`/resources/details?ownerId=${ownerGroupId}&Untagged=1`);
    await page.waitForLoadState('load');

    const scan = await new AxeBuilder({ page }).analyze();
    const seriousPlus = scan.violations.filter(
      (v) => v.impact === 'serious' || v.impact === 'critical',
    );
    if (seriousPlus.length > 0) {
      console.error('Axe violations:', JSON.stringify(seriousPlus, null, 2));
    }
    expect(seriousPlus).toEqual([]);
  });
});
