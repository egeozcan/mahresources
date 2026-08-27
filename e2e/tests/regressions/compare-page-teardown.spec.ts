import { test, expect } from '../../fixtures/base.fixture';
import fs from 'fs';
import os from 'os';
import path from 'path';
import zlib from 'zlib';

/**
 * Covers the compare-page defects found in the 2026-08-25 review that had no
 * existing coverage: the PDF panel downloading its files on load, the split diff
 * failing to align, the picker not naming what it holds, the empty layout
 * sidebar, and the dead end at `/resource/compare?r1=<id>`.
 */
test.describe.serial('compare page teardown fixes', () => {
  let runId: number;
  let categoryId: number;
  let ownerGroupId: number;
  let fixtureDir: string;
  let textResourceId: number;
  let pdfResourceId: number;
  let imageResourceId: number;
  let imageVersionedId: number;
  let dimensionlessId: number;
  let foldResourceId: number;

  const CONFIG_V1 = [
    '{',
    '  "service": "mahresources",',
    '  "database": {',
    '    "type": "SQLITE",',
    '    "maxConnections": 10',
    '  },',
    '  "features": {',
    '    "fts": true,',
    '    "plugins": false',
    '  }',
    '}',
    '',
  ].join('\n');

  const CONFIG_V2 = [
    '{',
    '  "service": "mahresources",',
    '  "database": {',
    '    "type": "POSTGRES",',
    '    "maxConnections": 25,',
    '    "readOnlyDsn": "postgres://replica"',
    '  },',
    '  "features": {',
    '    "fts": true,',
    '    "plugins": true',
    '  }',
    '}',
    '',
  ].join('\n');

  /**
   * A long log with two edits in it, so the unchanged runs between them are long
   * enough to collapse. Folding only appears above MIN_FOLD_LINES.
   */
  function longLog(marker: string, edits: Record<number, string>): string {
    const lines: string[] = [];
    for (let i = 1; i <= 200; i++) {
      lines.push(edits[i] ?? `2026-08-25 12:00:${String(i % 60).padStart(2, '0')} indexed batch ${i} of 200 (${marker})`);
    }
    return lines.join('\n') + '\n';
  }

  /** A one-page PDF, hand-assembled so the suite needs no binary fixture. */
  function tinyPdf(text: string): Buffer {
    const content = `BT /F1 24 Tf 72 700 Td (${text}) Tj ET`;
    const objects = [
      '<< /Type /Catalog /Pages 2 0 R >>',
      '<< /Type /Pages /Kids [3 0 R] /Count 1 >>',
      '<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Contents 4 0 R /Resources << /Font << /F1 5 0 R >> >> >>',
      `<< /Length ${content.length} >>\nstream\n${content}\nendstream`,
      '<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>',
    ];
    let out = '%PDF-1.4\n';
    const offsets: number[] = [];
    objects.forEach((body, i) => {
      offsets.push(out.length);
      out += `${i + 1} 0 obj\n${body}\nendobj\n`;
    });
    const xref = out.length;
    out += `xref\n0 ${objects.length + 1}\n0000000000 65535 f \n`;
    for (const offset of offsets) out += `${String(offset).padStart(10, '0')} 00000 n \n`;
    out += `trailer\n<< /Size ${objects.length + 1} /Root 1 0 R >>\nstartxref\n${xref}\n%%EOF\n`;
    return Buffer.from(out, 'latin1');
  }

  /**
   * A solid-colour PNG. Built rather than borrowed from test-assets so the two
   * images have known, deliberately different dimensions — the overlay modes are
   * about exactly that — and so their bytes cannot collide with another spec's.
   */
  function solidPng(width: number, height: number, rgb: [number, number, number]): Buffer {
    const chunk = (type: string, body: Buffer) => {
      const length = Buffer.alloc(4);
      length.writeUInt32BE(body.length);
      const typed = Buffer.concat([Buffer.from(type, 'ascii'), body]);
      const crc = Buffer.alloc(4);
      crc.writeUInt32BE(zlib.crc32 ? zlib.crc32(typed) : crc32(typed));
      return Buffer.concat([length, typed, crc]);
    };

    const ihdr = Buffer.alloc(13);
    ihdr.writeUInt32BE(width, 0);
    ihdr.writeUInt32BE(height, 4);
    ihdr[8] = 8;   // bit depth
    ihdr[9] = 2;   // colour type: truecolour
    const raw = Buffer.alloc(height * (1 + width * 3));
    for (let y = 0; y < height; y++) {
      const row = y * (1 + width * 3);
      raw[row] = 0; // no filter
      for (let x = 0; x < width; x++) {
        raw[row + 1 + x * 3] = rgb[0];
        raw[row + 2 + x * 3] = rgb[1];
        raw[row + 3 + x * 3] = rgb[2];
      }
    }
    return Buffer.concat([
      Buffer.from([0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a]),
      chunk('IHDR', ihdr),
      chunk('IDAT', zlib.deflateSync(raw)),
      chunk('IEND', Buffer.alloc(0)),
    ]);
  }

  /** CRC-32, for Node versions without zlib.crc32. */
  function crc32(buf: Buffer): number {
    let c = ~0;
    for (let i = 0; i < buf.length; i++) {
      c ^= buf[i];
      for (let k = 0; k < 8; k++) c = (c >>> 1) ^ (0xedb88320 & -(c & 1));
    }
    return (~c) >>> 0;
  }

  /**
   * The same PNG wrapped as an icon. The server sniffs the type and stores
   * `image/x-icon`, which reaches the image comparator, but no decoder in the
   * binary can read an icon's header — so the version rows carry no width or
   * height. That is the state the overlay modes have to survive, and it is not
   * reachable with any format Go can measure.
   */
  function pngIcon(width: number, height: number, rgb: [number, number, number]): Buffer {
    const png = solidPng(width, height, rgb);
    const dir = Buffer.alloc(6);
    dir.writeUInt16LE(1, 2); // type: icon
    dir.writeUInt16LE(1, 4); // one image
    const entry = Buffer.alloc(16);
    entry[0] = width >= 256 ? 0 : width;
    entry[1] = height >= 256 ? 0 : height;
    entry.writeUInt16LE(1, 4);  // colour planes
    entry.writeUInt16LE(32, 6); // bits per pixel
    entry.writeUInt32LE(png.length, 8);
    entry.writeUInt32LE(22, 12); // payload offset
    return Buffer.concat([dir, entry, png]);
  }

  /** Writes a fixture and hands back its path, since createResource takes a path. */
  function fixture(name: string, body: Buffer | string): string {
    const target = path.join(fixtureDir, name);
    fs.writeFileSync(target, body);
    return target;
  }

  async function uploadVersion(
    request: any,
    baseURL: string,
    resourceId: number,
    name: string,
    mimeType: string,
    buffer: Buffer,
    comment: string,
  ) {
    const response = await request.post(`${baseURL}/v1/resource/versions?resourceId=${resourceId}`, {
      multipart: { file: { name, mimeType, buffer }, comment },
    });
    expect(response.ok()).toBeTruthy();
  }

  test.beforeAll(async ({ apiClient, request, baseURL }) => {
    runId = Date.now() + Math.floor(Math.random() * 100000);
    fixtureDir = fs.mkdtempSync(path.join(os.tmpdir(), 'compare-teardown-'));

    const category = await apiClient.createCategory(
      `Teardown Category ${runId}`,
      'Category for compare teardown regressions',
    );
    categoryId = category.ID;

    const group = await apiClient.createGroup({ name: `Teardown Owner ${runId}`, categoryId });
    ownerGroupId = group.ID;

    const textResource = await apiClient.createResource({
      filePath: fixture('config.json', CONFIG_V1 + `\n// ${runId}\n`),
      contentType: 'application/json',
      exactBytes: true,
      name: `Teardown Config ${runId}`,
      ownerId: ownerGroupId,
    });
    textResourceId = textResource.ID;
    await uploadVersion(
      request, baseURL!, textResourceId, 'config-v2.json', 'application/json',
      Buffer.from(CONFIG_V2 + `\n// ${runId}\n`), 'Switch to Postgres',
    );

    const pdfResource = await apiClient.createResource({
      filePath: fixture('report.pdf', tinyPdf(`Teardown draft ${runId}`)),
      contentType: 'application/pdf',
      exactBytes: true,
      name: `Teardown Report ${runId}`,
      ownerId: ownerGroupId,
    });
    pdfResourceId = pdfResource.ID;
    await uploadVersion(
      request, baseURL!, pdfResourceId, 'report-v2.pdf', 'application/pdf',
      tinyPdf(`Teardown final ${runId}`), 'Final revisions',
    );

    const imageResource = await apiClient.createResource({
      filePath: path.join(__dirname, '../../test-assets/sample-image-27.png'),
      name: `Teardown Image ${runId}`,
      ownerId: ownerGroupId,
    });
    imageResourceId = imageResource.ID;

    // A second image resource with two versions, deliberately different shapes,
    // for the image comparison modes.
    const versioned = await apiClient.createResource({
      filePath: fixture('versioned-v1.png', solidPng(80, 60, [30, 120, 190])),
      exactBytes: true,
      name: `Teardown Versioned ${runId}`,
      ownerId: ownerGroupId,
    });
    imageVersionedId = versioned.ID;
    await uploadVersion(
      request, baseURL!, imageVersionedId, 'versioned-v2.png', 'image/png',
      solidPng(160, 50, [190, 60, 30]), 'Second image version',
    );

    // A long log, for the collapsed-context controls.
    const foldResource = await apiClient.createResource({
      filePath: fixture('indexer.log', longLog(`run ${runId}`, {})),
      contentType: 'text/plain',
      exactBytes: true,
      name: `Teardown Log ${runId}`,
      ownerId: ownerGroupId,
    });
    foldResourceId = foldResource.ID;
    await uploadVersion(
      request, baseURL!, foldResourceId, 'indexer-v2.log', 'text/plain',
      Buffer.from(longLog(`run ${runId}`, { 40: 'ERROR could not open shard 3', 150: 'WARN retrying shard 7' })),
      'Second log',
    );

    // A third image resource whose versions carry no dimensions at all.
    const dimensionless = await apiClient.createResource({
      filePath: fixture('iconless-v1.ico', pngIcon(80, 60, [40, 140, 90])),
      exactBytes: true,
      name: `Teardown Iconless ${runId}`,
      ownerId: ownerGroupId,
    });
    dimensionlessId = dimensionless.ID;
    await uploadVersion(
      request, baseURL!, dimensionlessId, 'iconless-v2.ico', 'image/x-icon',
      pngIcon(120, 40, [200, 80, 40]), 'Second icon version',
    );
  });

  test.afterAll(async ({ apiClient }) => {
    for (const id of [textResourceId, pdfResourceId, imageResourceId, imageVersionedId, dimensionlessId, foldResourceId]) {
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
    if (fixtureDir) {
      fs.rmSync(fixtureDir, { recursive: true, force: true });
    }
  });

  // Both frames were rendered up front and merely hidden, so the browser fetched
  // both PDFs on load — and the response is an attachment, so it downloaded them.
  test('opening a PDF comparison downloads nothing', async ({ page }) => {
    const downloads: string[] = [];
    page.on('download', (d) => downloads.push(d.suggestedFilename()));

    await page.goto(`/resource/compare?r1=${pdfResourceId}&v1=1&v2=2`);
    await page.waitForLoadState('load');
    await expect(page.getByRole('button', { name: /Load in viewer/ })).toBeVisible({ timeout: 10000 });
    await page.waitForTimeout(1500);

    expect(downloads).toEqual([]);
    expect(await page.locator('iframe').count()).toBe(0);
  });

  // The version-file route answers with X-Frame-Options: DENY and an attachment
  // disposition, so the framed viewer could never render. Inline is opt-in and
  // safelisted to PDFs.
  test('the PDF viewer loads on request and can be closed again', async ({ page, request, baseURL }) => {
    await page.goto(`/resource/compare?r1=${pdfResourceId}&v1=1&v2=2`);
    await page.waitForLoadState('load');

    await page.getByRole('button', { name: /Load in viewer/ }).click();

    const frames = page.locator('iframe');
    await expect(frames).toHaveCount(2);
    const sources = await frames.evaluateAll((els) => els.map((e) => (e as HTMLIFrameElement).src));
    for (const src of sources) {
      expect(src).toContain('disposition=inline');
    }

    // Whether a frame paints depends on the browser having a PDF viewer, which
    // the bundled Chromium does not. What this page controls is the response:
    // an attachment cannot render in a frame, and X-Frame-Options: DENY blocks
    // the frame outright. Both had to change for the viewer to work at all.
    for (const src of sources) {
      const response = await request.get(src.replace(baseURL!, baseURL!));
      expect(response.ok()).toBeTruthy();
      expect(response.headers()['content-disposition']).toContain('inline');
      expect(response.headers()['x-frame-options']).toBe('SAMEORIGIN');
    }

    // The button used to remove itself, stranding the reader in the viewer.
    await page.getByRole('button', { name: /Close viewer/ }).click();
    await expect(page.locator('iframe')).toHaveCount(0);
  });

  test('an inline disposition is refused for anything but a PDF', async ({ request, baseURL }) => {
    const versions = await (await request.get(`${baseURL}/v1/resource/versions?resourceId=${imageResourceId}`)).json();
    const response = await request.get(
      `${baseURL}/v1/resource/version/file?versionId=${versions[0].id}&disposition=inline`,
    );
    expect(response.ok()).toBeTruthy();
    // Arbitrary uploads served inline and same-origin-framable are stored XSS.
    expect(response.headers()['content-disposition']).toContain('attachment');
    expect(response.headers()['x-frame-options']).toBe('DENY');
  });

  // The padding rows were emitted but rendered at zero height, so the two
  // columns drifted apart exactly where a diff needs them together.
  test('the split diff lines its two columns up', async ({ page }) => {
    await page.goto(`/resource/compare?r1=${textResourceId}&v1=1&v2=2`);
    await page.waitForLoadState('load');
    await page.getByRole('radio', { name: 'Side by side' }).click();

    const columns = page.locator('.compare-diff--split');
    await expect(columns).toHaveCount(2);

    const rowTops = await page.evaluate(() => {
      const [left, right] = [...document.querySelectorAll('.compare-diff--split')];
      const tops = (root: Element) =>
        [...root.querySelectorAll('.compare-diff-line')].map((r) => Math.round(r.getBoundingClientRect().top));
      return { left: tops(left), right: tops(right) };
    });

    expect(rowTops.left.length).toBe(rowTops.right.length);
    expect(rowTops.left).toEqual(rowTops.right);
  });

  test('a changed line marks only the part that changed', async ({ page }) => {
    await page.goto(`/resource/compare?r1=${textResourceId}&v1=1&v2=2`);
    await page.waitForLoadState('load');
    await expect(page.locator('.compare-diff-line').first()).toBeVisible({ timeout: 10000 });

    const marked = page.locator('.compare-word--removed, .compare-word--added');
    await expect(marked.first()).toBeVisible();
    const texts = await marked.allTextContents();
    expect(texts.join(' ')).toMatch(/SQLITE|POSTGRES|10|25/);
  });

  test('the diff can be navigated and copied', async ({ page, context }) => {
    await context.grantPermissions(['clipboard-read', 'clipboard-write']);
    await page.goto(`/resource/compare?r1=${textResourceId}&v1=1&v2=2`);
    await page.waitForLoadState('load');
    await expect(page.locator('.compare-diff-line').first()).toBeVisible({ timeout: 10000 });

    await expect(page.locator('.compare-nav-count')).toContainText(/change/);
    await page.getByRole('button', { name: 'Next change' }).click();
    await expect(page.locator('.compare-diff-row--active').first()).toBeVisible();

    await page.getByRole('button', { name: /Copy the diff as a patch/ }).click();
    const clipboard = await page.evaluate(() => navigator.clipboard.readText());
    expect(clipboard).toContain('-    "type": "SQLITE",');
    expect(clipboard).toContain('+    "type": "POSTGRES",');
  });

  // The pickers were a copy of the shared autocompleter with the selected-item
  // chips and every combobox attribute left out, so the page never said what it
  // was comparing and a screen reader was told nothing when results appeared.
  test('the pickers name what they hold and behave as comboboxes', async ({ page }) => {
    await page.goto(`/resource/compare?r1=${textResourceId}&v1=1&v2=2`);
    await page.waitForLoadState('load');

    await expect(page.getByText(`Teardown Config ${runId}`).first()).toBeVisible({ timeout: 10000 });

    const leftInput = page.getByRole('combobox', { name: 'Left resource' });
    await expect(leftInput).toHaveAttribute('aria-expanded', 'false');
    await expect(leftInput).toHaveAttribute('aria-controls', /listbox/);
    await expect(page.locator('#compare-left-resource-listbox')).toHaveAttribute('role', 'listbox');
  });

  // The layout reserves a 400px sidebar for any page that does not opt out, and
  // this is the page that most wants the width.
  test('no empty sidebar column is reserved', async ({ page }) => {
    await page.goto(`/resource/compare?r1=${textResourceId}&v1=1&v2=2`);
    await page.waitForLoadState('load');

    const width = await page.evaluate(() => {
      const aside = document.querySelector('.sidebar');
      return aside ? aside.getBoundingClientRect().width : 0;
    });
    expect(width).toBe(0);
  });

  // Trimming the URL to a single resource used to render "Ready to Compare" while
  // both dropdowns displayed a version, and picking one wrote v1=0.
  test('a URL naming only a resource resolves to previous versus current', async ({ page }) => {
    await page.goto(`/resource/compare?r1=${textResourceId}`);
    await page.waitForLoadState('load');

    const url = new URL(page.url());
    expect(url.searchParams.get('v1')).toBe('1');
    expect(url.searchParams.get('v2')).toBe('2');
    await expect(page.locator('summary:has-text("Metadata")')).toBeVisible({ timeout: 10000 });
    await expect(page.getByText('Ready to Compare')).toHaveCount(0);
  });

  // Deciding the comparator from the left-hand version alone sent a JSON versus
  // PNG comparison to the text diff, which printed the image's bytes as lines.
  test('two different file types fall back to the binary panel', async ({ page }) => {
    await page.goto(`/resource/compare?r1=${textResourceId}&v1=1&r2=${imageResourceId}&v2=1`);
    await page.waitForLoadState('load');

    await expect(page.locator('summary:has-text("Metadata")')).toBeVisible({ timeout: 10000 });
    await expect(page.getByText('different file types')).toBeVisible();
    await expect(page.locator('.compare-diff-line')).toHaveCount(0);
  });

  // A dimension of 0 means "this file has no dimensions", not a measurement.
  test('a file with no dimensions has no dimensions card', async ({ page }) => {
    await page.goto(`/resource/compare?r1=${textResourceId}&v1=1&v2=2`);
    await page.waitForLoadState('load');
    await expect(page.locator('summary:has-text("Metadata")')).toBeVisible({ timeout: 10000 });

    await expect(page.locator('.compare-meta-card-label:has-text("Dimensions")')).toHaveCount(0);
  });

  // Flip exchanged the two URLs and labels in place, so the pink "older" panel
  // could hold the newer file while the server-rendered alt still described
  // whichever version had originally been there.
  test('flipping the images moves the colour, the label and the alt together', async ({ page }) => {
    await page.goto(`/resource/compare?r1=${imageVersionedId}&v1=1&v2=2`);
    await page.waitForLoadState('load');
    // By accessible name, not by the style class: the toolbar's small-button
    // style is shared with the anchor toggle, and the identity of this control
    // is what it is called, not how it is painted.
    await expect(page.getByRole('button', { name: 'Flip which image is shown first' }))
      .toBeVisible({ timeout: 10000 });

    const panes = () => page.evaluate(() =>
      [...document.querySelectorAll('[x-show="mode === \'side-by-side\'"] > div')].map((pane) => {
        const header = pane.firstElementChild as HTMLElement;
        const img = pane.querySelector('img');
        return {
          old: header.className.includes('--old'),
          label: (header.textContent || '').trim(),
          src: img?.getAttribute('src') || '',
          alt: img?.getAttribute('alt') || '',
        };
      }));

    const before = await panes();
    expect(before).toHaveLength(2);
    for (const pane of before) expect(pane.alt).toBe(pane.label);

    await page.getByRole('button', { name: 'Flip which image is shown first' }).click();
    const after = await panes();

    // The images changed places...
    expect(after[0].src).toBe(before[1].src);
    expect(after[1].src).toBe(before[0].src);
    // ...and the colour, caption and alt went with them.
    for (const pane of after) expect(pane.alt).toBe(pane.label);
    expect(after[0].old).toBe(before[1].old);
    expect(after[1].old).toBe(before[0].old);
  });

  // The image shortcut listened on the document and the radiogroup handler did
  // not stop propagation, so one ArrowRight changed the mode and then moved
  // whatever the new mode's control was.
  test('arrow keys on the mode control do not also move the slider', async ({ page }) => {
    await page.goto(`/resource/compare?r1=${imageVersionedId}&v1=1&v2=2`);
    await page.waitForLoadState('load');

    const root = page.locator('[x-data^="imageCompare"]');
    const state = () => root.evaluate((el) => {
      const data = (window as any).Alpine.$data(el);
      return { mode: data.mode, sliderPos: data.sliderPos, opacity: data.opacity };
    });

    await page.getByRole('radio', { name: 'Slider' }).click();
    const before = await state();
    expect(before.mode).toBe('slider');

    await page.locator('[role="radiogroup"] [role="radio"][aria-checked="true"]').first().focus();
    await page.keyboard.press('ArrowRight');

    const after = await state();
    expect(after.mode).not.toBe(before.mode);
    expect(after.sliderPos).toBe(before.sliderPos);
    expect(after.opacity).toBe(before.opacity);
  });

  // The drag handle was an unlabelled div, reachable only through that
  // undiscoverable document shortcut and announced as nothing.
  test('the reveal slider is focusable, announced and keyboard-driven', async ({ page }) => {
    await page.goto(`/resource/compare?r1=${imageVersionedId}&v1=1&v2=2`);
    await page.waitForLoadState('load');
    await page.getByRole('radio', { name: 'Slider' }).click();

    const handle = page.getByRole('slider', { name: 'Reveal position' });
    await expect(handle).toBeVisible();
    await handle.focus();
    expect(await page.evaluate(() => document.activeElement?.getAttribute('role'))).toBe('slider');

    const startedAt = Number(await handle.getAttribute('aria-valuenow'));
    await page.keyboard.press('ArrowRight');
    await expect.poll(async () => Number(await handle.getAttribute('aria-valuenow'))).toBeGreaterThan(startedAt);
    await expect(handle).toHaveAttribute('aria-valuetext', /% of /);

    await page.keyboard.press('Home');
    await expect(handle).toHaveAttribute('aria-valuenow', '1');
  });

  // The +/- prefix is aria-hidden and the colour is not read at all, so without
  // a label in the gutter a diff is announced as line numbers and text with no
  // indication of what changed.
  test('diff rows announce whether they were added or removed', async ({ page }) => {
    await page.goto(`/resource/compare?r1=${textResourceId}&v1=1&v2=2`);
    await page.waitForLoadState('load');
    await expect(page.locator('.compare-diff--unified')).toBeVisible();

    const labels = await page.evaluate(() =>
      [...document.querySelectorAll('.compare-diff--unified .compare-diff-line')]
        .map((row) => row.querySelector('.compare-diff-num')?.getAttribute('aria-label'))
        .filter(Boolean));

    expect(labels.some((l) => /^Added line \d+$/.test(l!))).toBe(true);
    expect(labels.some((l) => /^Removed line \d+$/.test(l!))).toBe(true);
    expect(labels.some((l) => /^Unchanged line \d+$/.test(l!))).toBe(true);
  });

  // Both split columns give their folds the same ids, so a search from the
  // component root answered the right pane's control with the left pane's line.
  test('a fold opened in the right pane keeps focus in the right pane', async ({ page }) => {
    await page.goto(`/resource/compare?r1=${foldResourceId}&v1=1&v2=2`);
    await page.waitForLoadState('load');
    await page.getByRole('radio', { name: 'Side by side' }).click();

    const rightPane = page.locator('.compare-diff--split').nth(1);
    const fold = rightPane.getByRole('button', { name: /Show \d+ unchanged lines/ }).first();
    await expect(fold).toBeVisible();
    await fold.focus();
    await page.keyboard.press('Enter');

    await expect.poll(() => page.evaluate(() => {
      const el = document.activeElement as HTMLElement | null;
      if (!el || !el.getAttribute('data-fold')) return 'none';
      const panes = [...document.querySelectorAll('.compare-diff--split')];
      return String(panes.indexOf(el.closest('.compare-diff--split')!));
    })).toBe('1');
  });

  // `$el` read from a method is whichever element's expression made the call, so
  // the toolbar button searched inside itself and found no diff: the counter
  // advanced and the page never moved.
  test('next change scrolls the diff to that change', async ({ page }) => {
    await page.goto(`/resource/compare?r1=${foldResourceId}&v1=1&v2=2`);
    await page.waitForLoadState('load');
    await expect(page.locator('.compare-diff-nav')).toBeVisible();

    const firstChangeTop = async () => page.evaluate(() => {
      const row = document.querySelector('[data-change]');
      return row ? Math.round(row.getBoundingClientRect().top) : null;
    });

    const before = await firstChangeTop();
    expect(before).not.toBeNull();
    expect(before!).toBeGreaterThan(400);

    await page.getByRole('button', { name: 'Next change' }).click();
    await expect.poll(() => page.evaluate(() => Math.round(window.scrollY))).toBeGreaterThan(0);

    const after = await firstChangeTop();
    expect(after!).toBeLessThan(before!);
  });

  // Activating a fold removes the control from the page. Focus fell back to the
  // document, which in a four-thousand-line diff loses the reader's place.
  test('opening a fold moves focus to the first line it revealed', async ({ page }) => {
    await page.goto(`/resource/compare?r1=${foldResourceId}&v1=1&v2=2`);
    await page.waitForLoadState('load');

    const fold = page.getByRole('button', { name: /Show \d+ unchanged lines/ }).first();
    await expect(fold).toBeVisible();
    await fold.focus();
    await page.keyboard.press('Enter');

    // Polled: the rows arrive over several frames, so the component waits for
    // them and the assertion has to as well.
    await expect.poll(() => page.evaluate(() => {
      const el = document.activeElement as HTMLElement | null;
      return `${el?.tagName}/${el?.getAttribute('role')}/${el?.getAttribute('data-fold') ? 'fold' : 'none'}`;
    })).toBe('DIV/row/fold');
  });

  // Both arrow pairs select in a radiogroup; which one a reader reaches for
  // depends on how they read the control.
  test('the mode radiogroup answers Down and Up as well as Right and Left', async ({ page }) => {
    await page.goto(`/resource/compare?r1=${imageVersionedId}&v1=1&v2=2`);
    await page.waitForLoadState('load');

    const root = page.locator('[x-data^="imageCompare"]');
    const mode = () => root.evaluate((el) => (window as any).Alpine.$data(el).mode);

    await page.locator('[role="radiogroup"] [role="radio"][aria-checked="true"]').first().focus();
    await page.keyboard.press('ArrowDown');
    expect(await mode()).toBe('slider');
    await page.keyboard.press('ArrowUp');
    expect(await mode()).toBe('side-by-side');
  });

  // Without stored dimensions the overlay box has no aspect ratio, and the
  // fallback that gives it a height by putting the images back in the flow put
  // *both* of them there — so onion skin stopped overlaying and became two
  // images stacked with the lower one faded.
  //
  // The database still holds no dimensions for this pair; since the registration
  // package the browser supplies them from the loaded images, so the pair is now
  // registered at its true relative scale and the two images are centred in one
  // box rather than both pinned to its top-left. That is the point of the
  // package, so this asserts what "overlaid rather than stacked" actually means
  // — the rectangles intersect, and share a centre — instead of the identical
  // origin that only held while nothing was registered at all.
  test('onion skin overlays its two images when neither has stored dimensions', async ({ page }) => {
    await page.goto(`/resource/compare?r1=${dimensionlessId}&v1=1&v2=2`);
    await page.waitForLoadState('load');
    await page.getByRole('radio', { name: 'Onion skin' }).click();

    // Located by visibility, not by any class the fix introduced: onion skin is
    // the only overlay box on screen in this mode, so the assertion below is
    // about the layout rather than about the markup having changed.
    const box = page.locator('[x-data^="imageCompare"] .compare-overlay-box:visible');
    await expect(box).toHaveCount(1);

    const images = box.locator('.compare-overlay-img');
    await expect(images).toHaveCount(2);

    const under = await images.nth(0).boundingBox();
    const over = await images.nth(1).boundingBox();
    expect(under).not.toBeNull();
    expect(over).not.toBeNull();

    // Stacked, the second image begins at or below the bottom of the first and
    // the two rectangles never meet. Overlaid, they intersect on both axes.
    const overlapX = Math.min(under!.x + under!.width, over!.x + over!.width) - Math.max(under!.x, over!.x);
    const overlapY = Math.min(under!.y + under!.height, over!.y + over!.height) - Math.max(under!.y, over!.y);
    expect(overlapX).toBeGreaterThan(0);
    expect(overlapY).toBeGreaterThan(0);
    expect(over!.y).toBeLessThan(under!.y + under!.height);

    // And registered, not merely overlapping: two differently-shaped images
    // centred in one box share that box's centre.
    expect(over!.x + over!.width / 2).toBeCloseTo(under!.x + under!.width / 2, 0);
    expect(over!.y + over!.height / 2).toBeCloseTo(under!.y + under!.height / 2, 0);
  });

  // The visible mode label is hidden below 768px, which would leave the button
  // with nothing but an aria-hidden icon.
  test('mode buttons keep an accessible name on a narrow viewport', async ({ page }) => {
    await page.setViewportSize({ width: 390, height: 844 });
    await page.goto(`/resource/compare?r1=${imageVersionedId}&v1=1&v2=2`);
    await page.waitForLoadState('load');

    for (const name of ['Side by side', 'Slider', 'Onion skin', 'Toggle']) {
      await expect(page.getByRole('radio', { name })).toBeVisible();
    }
  });

  // Comparing a version against itself rendered a full "Files are identical"
  // report with nothing saying the choice was meaningless.
  test('comparing a version with itself says so', async ({ page }) => {
    await page.goto(`/resource/compare?r1=${textResourceId}&v1=2&v2=2`);
    await page.waitForLoadState('load');

    await expect(page.locator('.compare-notice')).toContainText('nothing to compare');
  });
});
