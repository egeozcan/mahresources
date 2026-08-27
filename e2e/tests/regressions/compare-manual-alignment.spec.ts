import { test, expect } from '../../fixtures/base.fixture';
import type { Locator, Page } from '@playwright/test';
import fs from 'fs';
import os from 'os';
import path from 'path';

/**
 * Package 2 of the compare-page extension board: manual alignment.
 *
 * Package 1's three scale policies are whole-pair decisions computed from
 * intrinsic dimensions. None of them can act on a pair that is the right size
 * and simply not in register -- a scan placed differently on the glass, a
 * re-photograph a few percent off. This is the correction the reader drives.
 *
 * Everything here asserts *rendered geometry*, never the inline style strings
 * that produce it, for the reason the package 1 spec gives at length: the
 * `.compare-overlay-box:not([style*="aspect-ratio"])` branch in `index.css` can
 * override those styles wholesale, so a style assertion would pass on a page
 * that draws the two images completely wrong.
 */
test.describe.serial('compare page manual alignment', () => {
  let runId: number;
  let categoryId: number;
  let ownerGroupId: number;
  let fixtureDir: string;
  let pairResourceId: number;
  let heicResourceId: number;

  /**
   * A committed fixture with a unique ASCII marker appended.
   *
   * Every upload in a worker shares one ephemeral server, so identical bytes
   * resolve to another spec's resource through the global content-hash dedup.
   * `tag` distinguishes the two versions of one file from each other, which is
   * what lets this spec use a **congruent** pair: two 400x300 images occupy
   * exactly the same rectangle at rest, so the displacement between them is the
   * measurement, with no centring baseline to subtract first.
   */
  function marked(name: string, tag: string): { path: string; buffer: Buffer } {
    const source = fs.readFileSync(path.join(__dirname, '../../test-assets', name));
    const buffer = Buffer.concat([source, Buffer.from(`\ncompare-align-${runId}-${tag}-${name}\n`, 'ascii')]);
    const target = path.join(fixtureDir, `${runId}-${tag}-${name}`);
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

  /** Onion skin is the only mode that paints both images at once. */
  function onionImages(page: Page): { box: Locator; lead: Locator; trail: Locator } {
    const box = page.locator('.compare-overlay-box')
      .filter({ has: page.locator('.compare-overlay-img--over') });
    return {
      box,
      lead: box.locator('.compare-overlay-img').first(),
      trail: box.locator('.compare-overlay-img--over'),
    };
  }

  /** The alignment is driven by `load` events, so a measurement before both
   *  images have decoded reads the pre-load placeholder and means nothing. */
  async function showOnionSkin(page: Page) {
    await page.getByRole('radio', { name: 'Onion skin' }).click();
    await expect(onionImages(page).trail).toBeVisible();
    await page.waitForFunction(() =>
      [...document.querySelectorAll('img.compare-overlay-img')]
        .every((i) => (i as HTMLImageElement).complete && (i as HTMLImageElement).naturalWidth > 0));
  }

  const alignButton = (page: Page) => page.getByRole('button', { name: /^Nudge and zoom / });
  const readout = (page: Page) => page.locator('.compare-offset-readout');

  test.beforeAll(async ({ apiClient, request, baseURL }) => {
    runId = Date.now() + Math.floor(Math.random() * 100000);
    fixtureDir = fs.mkdtempSync(path.join(os.tmpdir(), 'compare-align-'));

    const category = await apiClient.createCategory(
      `Alignment Category ${runId}`, 'Category for compare alignment regressions');
    categoryId = category.ID;
    const group = await apiClient.createGroup({ name: `Alignment Owner ${runId}`, categoryId });
    ownerGroupId = group.ID;

    // Two 400x300 PNGs: the case the package exists for. Both versions are the
    // same size, so every automatic scale policy agrees they are already
    // correctly scaled and only the reader can say they are out of register.
    const v1 = marked('compare-scale-400x300.png', 'v1');
    const pair = await apiClient.createResource({
      filePath: v1.path,
      contentType: 'image/png',
      exactBytes: true,
      name: `Alignment Pair ${runId}`,
      ownerId: ownerGroupId,
    });
    pairResourceId = pair.ID;
    const v2 = marked('compare-scale-400x300.png', 'v2');
    await uploadVersion(request, baseURL!, pairResourceId,
      'alignment-v2.png', 'image/png', v2.buffer, 'Rescanned, off register');

    // Neither Go nor Chromium can decode HEIC, so no coordinate space exists on
    // either side and the alignment has to refuse rather than be drawn as
    // though it worked.
    const heicV1 = marked('compare-heic-undecodable.heic', 'v1');
    const heicResource = await apiClient.createResource({
      filePath: heicV1.path,
      contentType: 'image/heic',
      exactBytes: true,
      name: `Alignment HEIC ${runId}`,
      ownerId: ownerGroupId,
    });
    heicResourceId = heicResource.ID;
    const heicV2 = marked('compare-heic-undecodable.heic', 'v2');
    await uploadVersion(request, baseURL!, heicResourceId,
      'alignment-v2.heic', 'image/heic', heicV2.buffer, 'Second HEIC');
  });

  test.afterAll(async ({ apiClient }) => {
    for (const id of [pairResourceId, heicResourceId]) {
      if (id) {
        try { await apiClient.deleteResource(id); } catch { /* already gone */ }
      }
    }
    if (ownerGroupId) {
      try { await apiClient.deleteGroup(ownerGroupId); } catch { /* already gone */ }
    }
    if (fixtureDir) fs.rmSync(fixtureDir, { recursive: true, force: true });
  });

  test('the arrow keys move nothing until the reader arms the mode', async ({ page }) => {
    // Focus is on the Align button itself, which is the path that matters: it
    // holds its own keydown handler, so "disarmed" has to be a decision that
    // handler makes rather than an accident of where focus happens to be.
    await page.goto(`/resource/compare?r1=${pairResourceId}&v1=1&v2=2`);
    await showOnionSkin(page);
    const { lead, trail } = onionImages(page);

    await alignButton(page).focus();
    await expect(alignButton(page)).toHaveAttribute('aria-pressed', 'false');
    const before = (await trail.boundingBox())!;
    for (let i = 0; i < 4; i++) await page.keyboard.press('Shift+ArrowRight');

    const after = (await trail.boundingBox())!;
    expect(after.x).toBeCloseTo(before.x, 0);
    expect(after.y).toBeCloseTo((await lead.boundingBox())!.y, 0);
    await expect(readout(page)).toHaveText(/0, 0, 100%/);
  });

  test('an armed nudge moves the trailing version and leaves the leading one where it is', async ({ page }) => {
    await page.goto(`/resource/compare?r1=${pairResourceId}&v1=1&v2=2`);
    await showOnionSkin(page);
    const { lead, trail } = onionImages(page);

    const leadBefore = (await lead.boundingBox())!;
    const trailBefore = (await trail.boundingBox())!;
    // Congruent pair: the two rectangles coincide exactly before any nudge.
    expect(trailBefore.x).toBeCloseTo(leadBefore.x, 0);
    expect(trailBefore.width).toBeCloseTo(leadBefore.width, 0);

    await alignButton(page).click();
    await expect(alignButton(page)).toHaveAttribute('aria-pressed', 'true');
    // Four large steps: 40 box pixels of a 400-box-pixel element, so 10% of the
    // rendered width whatever the window is. The offset is in box pixels
    // precisely so this ratio does not depend on the viewport.
    for (let i = 0; i < 4; i++) await page.keyboard.press('Shift+ArrowRight');

    const leadAfter = (await lead.boundingBox())!;
    const trailAfter = (await trail.boundingBox())!;
    expect(leadAfter.x).toBeCloseTo(leadBefore.x, 0);
    expect(trailAfter.x - trailBefore.x).toBeCloseTo(leadBefore.width * 0.1, 0);
    await expect(readout(page)).toHaveText(/\+40, 0, 100%/);
  });

  test('a flip inverts the correction instead of dropping it', async ({ page }) => {
    // A flip is how a reader checks that the correction took, so discarding it
    // there is exactly the wrong moment. The two images exchange sides, so the
    // displacement between the elements reverses sign and keeps its size.
    await page.goto(`/resource/compare?r1=${pairResourceId}&v1=1&v2=2`);
    await showOnionSkin(page);
    const { lead, trail } = onionImages(page);

    await alignButton(page).click();
    for (let i = 0; i < 4; i++) await page.keyboard.press('Shift+ArrowRight');
    const displaced = (await trail.boundingBox())!.x - (await lead.boundingBox())!.x;
    expect(displaced).toBeGreaterThan(1);

    await page.getByRole('button', { name: 'Flip which image is shown first' }).click();
    const flipped = (await trail.boundingBox())!.x - (await lead.boundingBox())!.x;
    expect(flipped).toBeCloseTo(-displaced, 0);
    await expect(readout(page)).toHaveText(/-40, 0, 100%/);
  });

  test('reset puts both versions back and says so', async ({ page }) => {
    await page.goto(`/resource/compare?r1=${pairResourceId}&v1=1&v2=2`);
    await showOnionSkin(page);
    const { lead, trail } = onionImages(page);

    await alignButton(page).click();
    for (let i = 0; i < 4; i++) await page.keyboard.press('Shift+ArrowRight');
    await page.keyboard.press('Shift+Equal');
    await expect(readout(page)).toHaveText(/\+40, 0, 110%/);

    const reset = page.getByRole('button', { name: 'Reset the alignment' });
    // Absent rather than "false": Alpine drops a falsy aria-disabled, and ARIA
    // reads an absent one as "not disabled". Same assertion the package 1 spec
    // makes of the anchor toggle.
    await expect(reset).not.toHaveAttribute('aria-disabled', 'true');
    await reset.click();

    await expect(readout(page)).toHaveText(/0, 0, 100%/);
    await expect(reset).toHaveAttribute('aria-disabled', 'true');
    expect((await trail.boundingBox())!.x).toBeCloseTo((await lead.boundingBox())!.x, 0);
    // The arming survives a reset: the reason you reset is that you are about
    // to try again.
    await expect(alignButton(page)).toHaveAttribute('aria-pressed', 'true');
  });

  test('a pair with no dimensions refuses to be aligned', async ({ page }) => {
    // Under the no-dimensions fallback the two images are not in a shared
    // coordinate space at all, and HEIC paints nothing to align in any case.
    await page.goto(`/resource/compare?r1=${heicResourceId}&v1=1&v2=2`);
    await page.getByRole('radio', { name: 'Onion skin' }).click();
    await expect(alignButton(page)).toBeVisible();
    await expect(alignButton(page)).toHaveAttribute('aria-disabled', 'true');

    // Forced, because aria-disabled suppresses neither focus nor pointer events
    // -- a real reader can press this and the component has to refuse it.
    // Playwright's own actionability check reads the attribute and would not
    // click, which is the one thing the browser will not do for us.
    await alignButton(page).click({ force: true });
    await expect(alignButton(page)).toHaveAttribute('aria-pressed', 'false');
    await page.keyboard.press('Shift+ArrowRight');
    await expect(readout(page)).toHaveText(/0, 0, 100%/);
  });

  test('an armed drag on the slider handle still moves the reveal, not the image', async ({ page }) => {
    // The handle lives inside the box whose `mousedown` starts an alignment
    // drag, and its own `mousedown` bubbles there. Without the guard an armed
    // drag on the handle moves the reveal position and the trailing image at
    // once, which is the one gesture that has to keep meaning what it did.
    await page.goto(`/resource/compare?r1=${pairResourceId}&v1=1&v2=2`);
    await showOnionSkin(page);
    await alignButton(page).click();
    await page.getByRole('radio', { name: 'Slider' }).click();

    const handle = page.getByRole('slider', { name: 'Reveal position' });
    await expect(handle).toBeVisible();
    const sliderBox = page.locator('.compare-overlay-box').filter({ has: handle });
    const trail = sliderBox.locator('.compare-overlay-img').first();
    const before = (await trail.boundingBox())!;
    await handle.scrollIntoViewIfNeeded();
    const grip = (await handle.boundingBox())!;
    // Near the handle's top, not its centre. The handle is `inset-y-0`, so it
    // is as tall as the overlay box -- on a portrait pair that centre is below
    // the fold, and `page.mouse` does not scroll the way `click()` does, so the
    // press lands on nothing and neither drag starts.
    const x = grip.x + grip.width / 2;
    const y = grip.y + Math.min(grip.height / 2, 60);

    await page.mouse.move(x, y);
    await page.mouse.down();
    await page.mouse.move(x + 80, y, { steps: 8 });
    await page.mouse.up();

    const after = (await trail.boundingBox())!;
    expect(after.x).toBeCloseTo(before.x, 0);
    await expect(readout(page)).toHaveText(/0, 0, 100%/);
    expect(Number(await handle.getAttribute('aria-valuenow'))).toBeGreaterThan(55);
  });
});
