import { test, expect } from '../../fixtures/base.fixture';
import type { Locator, Page } from '@playwright/test';
import fs from 'fs';
import os from 'os';
import path from 'path';

/**
 * Package 3 of the compare-page extension board: difference and measurement.
 *
 * 3.1 — a difference blend as a fifth mode: black means identical, the fastest
 * way to see a retouch. 3.2 — a pixel-diff heatmap with "% pixels changed" in
 * the summary banner. 3.3 — a blink comparator with play/pause and a rate.
 *
 * Like the package 1 and 2 specs, assertions are about *rendered* state:
 * computed blend properties, bounding boxes, canvas pixels — not the style
 * strings that produce them, which the `index.css` fallback branch can
 * override wholesale.
 */
test.describe.serial('compare page: difference and measurement', () => {
  let runId: number;
  let categoryId: number;
  let ownerGroupId: number;
  let fixtureDir: string;
  let pairResourceId: number;
  let heicResourceId: number;

  /**
   * A committed fixture with a unique ASCII marker appended. Every upload in a
   * worker shares one ephemeral server, so identical bytes resolve to another
   * spec's resource through the global content-hash dedup. The marker sits
   * after the image data, so both versions of the pair render identically
   * while remaining distinct files with distinct hashes.
   */
  function marked(name: string, tag: string): { path: string; buffer: Buffer } {
    const source = fs.readFileSync(path.join(__dirname, '../../test-assets', name));
    const buffer = Buffer.concat([source, Buffer.from(`\ncompare-pkg3-${runId}-${tag}-${name}\n`, 'ascii')]);
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

  const diffBox = (page: Page) =>
    page.locator('.compare-overlay-box').filter({ has: page.locator('.compare-overlay-img--difference') });

  test.beforeAll(async ({ apiClient, request, baseURL }) => {
    runId = Date.now() + Math.floor(Math.random() * 100000);
    fixtureDir = fs.mkdtempSync(path.join(os.tmpdir(), 'compare-diff-'));

    const category = await apiClient.createCategory(
      `Pkg3 Category ${runId}`, 'Category for compare difference and measurement regressions');
    categoryId = category.ID;
    const group = await apiClient.createGroup({ name: `Pkg3 Owner ${runId}`, categoryId });
    ownerGroupId = group.ID;

    // Two visually identical 400x300 PNGs: the case the difference blend is
    // for. Every pixel of the overlay should read black.
    const v1 = marked('compare-scale-400x300.png', 'v1');
    const pair = await apiClient.createResource({
      filePath: v1.path,
      contentType: 'image/png',
      exactBytes: true,
      name: `Pkg3 Pair ${runId}`,
      ownerId: ownerGroupId,
    });
    pairResourceId = pair.ID;
    const v2 = marked('compare-scale-400x300.png', 'v2');
    await uploadVersion(request, baseURL!, pairResourceId,
      'pkg3-v2.png', 'image/png', v2.buffer, 'Re-exported, same pixels');

    // Neither Go nor Chromium can decode HEIC, so no coordinate space exists
    // on either side: 3.2 must refuse, 3.3 may still work.
    const heicV1 = marked('compare-heic-undecodable.heic', 'v1');
    const heicResource = await apiClient.createResource({
      filePath: heicV1.path,
      contentType: 'image/heic',
      exactBytes: true,
      name: `Pkg3 HEIC ${runId}`,
      ownerId: ownerGroupId,
    });
    heicResourceId = heicResource.ID;
    const heicV2 = marked('compare-heic-undecodable.heic', 'v2');
    await uploadVersion(request, baseURL!, heicResourceId,
      'pkg3-v2.heic', 'image/heic', heicV2.buffer, 'Second HEIC');
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
    if (categoryId) {
      try { await apiClient.deleteCategory(categoryId); } catch { /* already gone */ }
    }
    if (fixtureDir) fs.rmSync(fixtureDir, { recursive: true, force: true });
  });

  /** Waits until every compare image on the page has actually decoded. */
  async function waitDecoded(page: Page) {
    await page.waitForFunction(() =>
      [...document.querySelectorAll('img[data-compare-image]')]
        .every((i) => (i as HTMLImageElement).complete && (i as HTMLImageElement).naturalWidth > 0));
  }

  test.describe('3.1: difference blend', () => {
    test('the mode radiogroup offers Difference and reaches it with the arrow keys', async ({ page }) => {
      await page.goto(`/resource/compare?r1=${pairResourceId}&v1=1&v2=2`);
      const group = page.getByRole('radiogroup', { name: 'Comparison mode' });
      await expect(group).toBeVisible();

      // Four radios were the whole of the group; the fifth joins them.
      const radios = group.getByRole('radio');
      await expect(radios).toHaveCount(5);
      await expect(group.getByRole('radio', { name: /Difference/ })).toHaveAttribute('aria-checked', 'false');

      // Roving tabindex: exactly one tab stop in the group, and the keyboard
      // enumeration — which the template names mode by mode — reaches the new
      // one instead of wrapping over it.
      await group.getByRole('radio', { name: 'Side by side' }).focus();
      await page.keyboard.press('ArrowRight');
      await page.keyboard.press('ArrowRight');
      await page.keyboard.press('ArrowRight');
      await page.keyboard.press('ArrowRight');
      await expect(group.getByRole('radio', { name: /Difference/ })).toHaveAttribute('aria-checked', 'true');
      await expect(diffBox(page)).toBeVisible();
    });

    test('the difference box stacks the two versions with the blend applied', async ({ page }) => {
      await page.goto(`/resource/compare?r1=${pairResourceId}&v1=1&v2=2`);
      await page.getByRole('radio', { name: /Difference/ }).click();
      await waitDecoded(page);

      const box = diffBox(page);
      const lead = box.locator('.compare-overlay-img').first();
      const trail = box.locator('.compare-overlay-img--difference');

      // Same origin, same frame, one on top of the other: this is what makes
      // the blend a comparison rather than two pictures.
      const leadRect = (await lead.boundingBox())!;
      const trailRect = (await trail.boundingBox())!;
      expect(Math.abs(leadRect.x - trailRect.x)).toBeLessThan(1);
      expect(Math.abs(leadRect.y - trailRect.y)).toBeLessThan(1);

      // The blend itself, from the computed style — the one property whose
      // whole job is to say this, and which no fallback branch overrides.
      await expect(trail).toHaveCSS('mix-blend-mode', 'difference');
    });

    test('black means identical, with this pair as the proof', async ({ page }) => {
      // The precondition, measured in the browser: the two version files are
      // pixel-identical, so a difference composite of them is pure black.
      // The mode's promise ("black means identical") rests on that, and the
      // fixture marker sits after the image data so it changes the hash
      // without changing a pixel.
      await page.goto(`/resource/compare?r1=${pairResourceId}&v1=1&v2=2`);
      await waitDecoded(page);
      const maxChannel = await page.evaluate(async () => {
        const urls = [...document.querySelectorAll('img[data-compare-image]')]
          .map((i) => (i as HTMLImageElement).currentSrc)
          .filter((v, i, a) => v && a.indexOf(v) === i);
        const load = (src: string) => new Promise<HTMLImageElement>((resolve, reject) => {
          const img = new Image();
          img.onload = () => resolve(img);
          img.onerror = reject;
          img.src = src;
        });
        const [a, b] = await Promise.all(urls.map(load));
        const canvas = document.createElement('canvas');
        canvas.width = a.naturalWidth;
        canvas.height = a.naturalHeight;
        const ctx = canvas.getContext('2d')!;
        ctx.drawImage(a, 0, 0);
        ctx.globalCompositeOperation = 'difference';
        ctx.drawImage(b, 0, 0);
        const { data } = ctx.getImageData(0, 0, canvas.width, canvas.height);
        // RGB only: the alpha channel is 255 everywhere after the composite.
        let max = 0;
        for (let i = 0; i < data.length; i += 4) {
          for (let c = 0; c < 3; c++) if (data[i + c] > max) max = data[i + c];
        }
        return max;
      });
      expect(maxChannel).toBe(0);
    });

    test('scale, anchor and alignment compose with the difference mode', async ({ page }) => {
      // The blend is a mode like the others, not a separate page: the controls
      // packages 1 and 2 added must keep working under it, or the composite
      // shows a pair that was never scaled or registered.
      await page.goto(`/resource/compare?r1=${pairResourceId}&v1=1&v2=2`);
      await page.getByRole('radio', { name: /Difference/ }).click();
      await waitDecoded(page);

      const box = diffBox(page);
      const lead = box.locator('.compare-overlay-img').first();

      // Anchor: the corner toggle the toolbar already carries.
      await page.getByRole('button', { name: 'Anchor both versions to the top left corner' }).click();
      // Within the frame's 1px border, which the image sits inside — the same
      // tolerance the registration-scale spec's corner assertion uses.
      const boxRect = (await box.boundingBox())!;
      const leadRect = (await lead.boundingBox())!;
      expect(Math.abs(leadRect.x - boxRect.x)).toBeLessThanOrEqual(2);
      expect(Math.abs(leadRect.y - boxRect.y)).toBeLessThanOrEqual(2);

      // Align: the armed nudge from package 2, still owning the arrow keys.
      const align = page.getByRole('button', { name: /^Nudge and zoom / });
      await align.click();
      for (let i = 0; i < 2; i++) await page.keyboard.press('Shift+ArrowRight');
      await expect(page.locator('.compare-offset-readout')).toHaveText(/\+20, 0, 100%/);
    });

    test('the toolbar in difference mode passes an axe scan', async ({ page }) => {
      await page.goto(`/resource/compare?r1=${pairResourceId}&v1=1&v2=2`);
      await page.getByRole('radio', { name: /Difference/ }).click();
      const { AxeBuilder } = await import('@axe-core/playwright');
      const results = await new AxeBuilder({ page })
        .include('.compare-segmented-control')
        .analyze();
      expect(results.violations).toEqual([]);
    });
  });

  test.describe('3.3: blink comparator', () => {
    const toggleBox = (page: Page) => page.locator('button.compare-overlay-box');
    const blinkButton = (page: Page) => page.getByRole('button', { name: 'Blink between the two versions' });
    const rateInput = (page: Page) => page.getByRole('slider', { name: 'Blink rate in flashes per second' });

    /** Which version the toggle box is showing right now, sampled from its
     *  accessible name ("Showing X. Activate to show the other."). */
    async function showing(page: Page): Promise<string> {
      const label = (await toggleBox(page).getAttribute('aria-label'))!;
      return label.replace(/^Showing\s+(.*)\. Activate to show the other\.$/, '$1');
    }

    /** Sample the showing side for `ms`, returning the distinct states seen. */
    async function sampleSides(page: Page, ms: number): Promise<string[]> {
      return page.evaluate(async (duration) => {
        const box = document.querySelector('button.compare-overlay-box') as HTMLButtonElement | null;
        const seen: string[] = [];
        const read = () => {
          if (!box) return;
          const label = box.getAttribute('aria-label') || '';
          const side = label.replace(/^Showing\s+(.*)\. Activate to show the other\.$/, '$1');
          if (!seen.length || seen[seen.length - 1] !== side) seen.push(side);
        };
        read();
        const end = performance.now() + duration;
        while (performance.now() < end) {
          await new Promise((r) => setTimeout(r, 10));
          read();
        }
        return seen;
      }, ms);
    }

    test('starts stopped, on every visit', async ({ page }) => {
      await page.goto(`/resource/compare?r1=${pairResourceId}&v1=1&v2=2`);
      await page.locator('.compare-seg-btn:has-text("Toggle")').click();
      await expect(toggleBox(page)).toBeVisible();
      await expect(blinkButton(page)).toBeVisible();
      await expect(blinkButton(page)).toHaveAttribute('aria-pressed', 'false');
      const sides = await sampleSides(page, 500);
      expect(sides.length).toBe(1);
    });

    test('playing alternates the visible side; pause stops it', async ({ page }) => {
      await page.goto(`/resource/compare?r1=${pairResourceId}&v1=1&v2=2`);
      await page.locator('.compare-seg-btn:has-text("Toggle")').click();

      await blinkButton(page).click();
      await expect(blinkButton(page)).toHaveAttribute('aria-pressed', 'true');
      const sides = await sampleSides(page, 1200);
      // Default 4 Hz across a generous window: far more than one alternation,
      // and both sides named, whichever one started.
      expect(sides.length).toBeGreaterThanOrEqual(3);

      // The accessible name is fixed (aria-pressed carries the state), so
      // pausing is a second press on the same button. The affordance swaps
      // icons: the play triangle shows when stopped, the pause bars when
      // playing. Asserted on visibility, not textContent, which reads hidden
      // spans too.
      await blinkButton(page).click();
      await expect(blinkButton(page)).toHaveAttribute('aria-pressed', 'false');
      await expect(blinkButton(page).locator('svg').nth(0)).toBeVisible();
      await expect(blinkButton(page).locator('svg').nth(1)).toBeHidden();
      const rest = await sampleSides(page, 500);
      expect(rest.length).toBe(1);
    });

    test('the rate control changes how fast it flips', async ({ page }) => {
      await page.goto(`/resource/compare?r1=${pairResourceId}&v1=1&v2=2`);
      await page.locator('.compare-seg-btn:has-text("Toggle")').click();

      await rateInput(page).fill('8');
      await rateInput(page).dispatchEvent('change');
      await blinkButton(page).click();
      const fast = await sampleSides(page, 1000);
      // 8 Hz over a second, sampled at 10ms: every flip is caught.
      expect(fast.length).toBeGreaterThanOrEqual(9);
    });

    test('prefers-reduced-motion refuses with a stated reason', async ({ page }) => {
      await page.emulateMedia({ reducedMotion: 'reduce' });
      await page.goto(`/resource/compare?r1=${pairResourceId}&v1=1&v2=2`);
      await page.locator('.compare-seg-btn:has-text("Toggle")').click();

      await expect(blinkButton(page)).toBeVisible();
      await expect(blinkButton(page)).toHaveAttribute('aria-disabled', 'true');
      expect(await blinkButton(page).getAttribute('title')).toContain('reduced motion');

      // aria-disabled suppresses neither focus nor pointer events, so the
      // component itself has to refuse a forced press.
      await blinkButton(page).click({ force: true });
      await expect(blinkButton(page)).toHaveAttribute('aria-pressed', 'false');
      const sides = await sampleSides(page, 500);
      expect(sides.length).toBe(1);
    });

    test('leaving toggle mode pauses the blink', async ({ page }) => {
      await page.goto(`/resource/compare?r1=${pairResourceId}&v1=1&v2=2`);
      await page.locator('.compare-seg-btn:has-text("Toggle")').click();
      await blinkButton(page).click();
      await expect(blinkButton(page)).toHaveAttribute('aria-pressed', 'true');

      await page.locator('.compare-seg-btn:has-text("Onion")').click();
      await expect(blinkButton(page)).toBeHidden();
      // Back in toggle mode, still stopped: the pause was the reader's answer,
      // not a mode switch's side effect that a re-entry undoes.
      await page.locator('.compare-seg-btn:has-text("Toggle")').click();
      await expect(blinkButton(page)).toHaveAttribute('aria-pressed', 'false');
      const sides = await sampleSides(page, 500);
      expect(sides.length).toBe(1);
    });

    test('the toolbar with blink playing passes an axe scan', async ({ page }) => {
      await page.goto(`/resource/compare?r1=${pairResourceId}&v1=1&v2=2`);
      await page.locator('.compare-seg-btn:has-text("Toggle")').click();
      await blinkButton(page).click();
      const { AxeBuilder } = await import('@axe-core/playwright');
      const results = await new AxeBuilder({ page })
        .include('.compare-segmented-control')
        .analyze();
      expect(results.violations).toEqual([]);
    });
  });
});
