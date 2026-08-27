import { test, expect } from '../../fixtures/base.fixture';
import type { Locator, Page } from '@playwright/test';
import fs from 'fs';
import os from 'os';
import path from 'path';

/**
 * Package 1 of the compare-page extension board: registration and scale.
 *
 * The three overlay modes share one box of `max(w1,w2) x max(h1,h2)` and size
 * each image inside it as a percentage of its own intrinsic dimensions. That is
 * true relative scale, and until this package it was the only policy on offer
 * and it depended entirely on dimensions the database might not hold.
 *
 * Everything here asserts *rendered geometry*, never the inline style strings
 * that produce it. `index.css`'s `.compare-overlay-box:not([style*="aspect-ratio"])`
 * branch can override those styles wholesale, so a style assertion would pass on
 * a page that draws the two images completely wrong -- which is the exact failure
 * these tests exist to catch.
 */
test.describe.serial('compare page registration and scale', () => {
  let runId: number;
  let categoryId: number;
  let ownerGroupId: number;
  let fixtureDir: string;
  let avifResourceId: number;
  let exifResourceId: number;

  /**
   * A committed fixture with a unique ASCII marker appended.
   *
   * Every upload in a worker shares one ephemeral server, so identical bytes
   * resolve to another spec's resource through the global content-hash dedup.
   * `helpers/unique-upload.ts` does this automatically for png/jpg/gif and
   * deliberately not for anything else, so the AVIF pair has to be marked here.
   * Verified rather than assumed: a marked AVIF still sniffs as `image/avif` in
   * Go's mimetype detector and still decodes to its true size in Chromium, the
   * same trailing-byte tolerance png and jpeg are relied on for.
   */
  function marked(name: string): { path: string; buffer: Buffer } {
    const source = fs.readFileSync(path.join(__dirname, '../../test-assets', name));
    const buffer = Buffer.concat([source, Buffer.from(`\ncompare-scale-${runId}-${name}\n`, 'ascii')]);
    const target = path.join(fixtureDir, `${runId}-${name}`);
    fs.writeFileSync(target, buffer);
    return { path: target, buffer };
  }

  async function uploadVersion(
    request: any, baseURL: string, resourceId: number,
    name: string, mimeType: string, buffer: Buffer, comment: string,
  ) {
    const response = await request.post(`${baseURL}/v1/resource/versions?resourceId=${resourceId}`, {
      multipart: { file: { name, mimeType, buffer }, comment },
    });
    expect(response.ok()).toBeTruthy();
  }

  /**
   * Onion skin is the only mode that paints both images at once, so it is the
   * one that can be measured. Its container is identifiable without a test-only
   * attribute: it is the only overlay box holding an `--over` image.
   */
  function onionImages(page: Page): { box: Locator; lead: Locator; trail: Locator } {
    const box = page.locator('.compare-overlay-box')
      .filter({ has: page.locator('.compare-overlay-img--over') });
    return {
      box,
      lead: box.locator('.compare-overlay-img').first(),
      trail: box.locator('.compare-overlay-img--over'),
    };
  }

  /**
   * The registration this package builds is driven by `load` events, so a
   * measurement taken before both images have decoded reads the pre-load
   * placeholder and is meaningless.
   */
  async function showOnionSkin(page: Page) {
    await page.getByRole('radio', { name: 'Onion skin' }).click();
    await expect(onionImages(page).trail).toBeVisible();
    await page.waitForFunction(() =>
      [...document.querySelectorAll('img.compare-overlay-img')]
        .every((i) => (i as HTMLImageElement).complete && (i as HTMLImageElement).naturalWidth > 0));
  }

  test.beforeAll(async ({ apiClient, request, baseURL }) => {
    runId = Date.now() + Math.floor(Math.random() * 100000);
    fixtureDir = fs.mkdtempSync(path.join(os.tmpdir(), 'compare-scale-'));

    const category = await apiClient.createCategory(
      `Registration Category ${runId}`, 'Category for compare registration regressions');
    categoryId = category.ID;
    const group = await apiClient.createGroup({ name: `Registration Owner ${runId}`, categoryId });
    ownerGroupId = group.ID;

    // AVIF is an accepted content type with no Go decoder anywhere in the tree,
    // so both versions store 0x0 and the page has no dimensions to register
    // with. Chromium renders them and reports their true sizes.
    // TestGetDimensionsFromContentDecodesEveryStoredRasterFormat pins that.
    const avifV1 = marked('compare-avif-400x300.avif');
    const avifResource = await apiClient.createResource({
      filePath: avifV1.path,
      contentType: 'image/avif',
      exactBytes: true,
      name: `Registration AVIF ${runId}`,
      ownerId: ownerGroupId,
    });
    avifResourceId = avifResource.ID;
    const avifV2 = marked('compare-avif-800x600.avif');
    await uploadVersion(request, baseURL!, avifResourceId,
      'registration-v2.avif', 'image/avif', avifV2.buffer, 'Rescanned at 2x');

    // Version 1 carries EXIF Orientation=6 over an 800x600 JPEG, so Go's
    // image.DecodeConfig -- which ignores EXIF, as nothing in this tree reads it
    // -- stores 800x600 while every browser paints and reports it as 600x800.
    // Version 2 is a plain 600x800. The stored pair and the painted pair
    // therefore disagree about the shape of the shared box.
    const exifV1 = marked('compare-exif-rot90-stored800x600.jpg');
    const exifResource = await apiClient.createResource({
      filePath: exifV1.path,
      contentType: 'image/jpeg',
      exactBytes: true,
      name: `Registration EXIF ${runId}`,
      ownerId: ownerGroupId,
    });
    exifResourceId = exifResource.ID;
    const exifV2 = marked('compare-exif-partner-600x800.jpg');
    await uploadVersion(request, baseURL!, exifResourceId,
      'registration-v2.jpg', 'image/jpeg', exifV2.buffer, 'Re-exported upright');
  });

  test.afterAll(async ({ apiClient }) => {
    for (const id of [avifResourceId, exifResourceId]) {
      if (id) {
        try { await apiClient.deleteResource(id); } catch { /* already gone */ }
      }
    }
    if (ownerGroupId) {
      try { await apiClient.deleteGroup(ownerGroupId); } catch { /* already gone */ }
    }
    if (fixtureDir) fs.rmSync(fixtureDir, { recursive: true, force: true });
  });

  test('a pair the database holds no dimensions for is registered from the loaded images', async ({ page }) => {
    await page.goto(`/resource/compare?r1=${avifResourceId}&v1=1&v2=2`);
    await showOnionSkin(page);

    const { lead, trail } = onionImages(page);
    const leadBox = (await lead.boundingBox())!;
    const trailBox = (await trail.boundingBox())!;

    // 400x300 against 800x600: the older version is drawn at exactly half the
    // width and half the height of the newer one. Without dimensions both fall
    // to the CSS branch that draws each at the container's full width, so this
    // ratio reads 1 and the two images are stacked at one origin, registered
    // against nothing.
    expect(trailBox.width / leadBox.width).toBeCloseTo(2, 1);
    expect(trailBox.height / leadBox.height).toBeCloseTo(2, 1);
  });

  test('a version whose stored dimensions disagree with the browser is registered by the browser', async ({ page }) => {
    await page.goto(`/resource/compare?r1=${exifResourceId}&v1=1&v2=2`);
    await showOnionSkin(page);

    const { lead, trail } = onionImages(page);
    const leadBox = (await lead.boundingBox())!;
    const trailBox = (await trail.boundingBox())!;

    // Both versions paint as 600x800, so they occupy exactly the same rectangle.
    // Trusting the stored values instead would build a 800x800 box and draw the
    // rotated version at 100%x75% against the other's 75%x100% -- two different
    // rectangles for two images the reader sees as identically shaped.
    expect(trailBox.width).toBeCloseTo(leadBox.width, 0);
    expect(trailBox.height).toBeCloseTo(leadBox.height, 0);
    expect(leadBox.width / leadBox.height).toBeCloseTo(600 / 800, 1);
  });

  // Registration made the box's own style reactive for the first time, and
  // Alpine's string `x-bind:style` replaces the whole style attribute -- the
  // same attribute `x-show` writes `display: none` onto. The first version of
  // this package therefore painted every mode at once the moment the images
  // finished loading. The two tests below are that failure, from both sides.
  test('only the selected mode is painted, after the images have loaded', async ({ page }) => {
    await page.goto(`/resource/compare?r1=${avifResourceId}&v1=1&v2=2`);
    await showOnionSkin(page);
    await expect(page.locator('[x-data^="imageCompare"] .compare-overlay-box:visible')).toHaveCount(1);

    await page.getByRole('radio', { name: 'Slider' }).click();
    await expect(page.locator('[x-data^="imageCompare"] .compare-overlay-box:visible')).toHaveCount(1);
  });

  test('toggle mode shows one image at a time, after the images have loaded', async ({ page }) => {
    await page.goto(`/resource/compare?r1=${avifResourceId}&v1=1&v2=2`);
    await showOnionSkin(page);
    await page.getByRole('radio', { name: 'Toggle' }).click();

    const shown = page.locator('[x-data^="imageCompare"] img.compare-overlay-img:visible');
    await expect(shown).toHaveCount(1);
    await page.locator('button.compare-overlay-box').click();
    await expect(shown).toHaveCount(1);
  });
});
