import { test, expect, type Page } from '../fixtures/base.fixture';
import path from 'path';
import fs from 'fs';
import os from 'os';
import { uniqueAssetFile, uniqueMarker } from '../helpers/unique-upload';

/**
 * The client-side bulk upload widget on /resource/new.
 *
 * Above a threshold (more than 10 files, or more than 1 GiB total) the form
 * stops posting one multipart body and instead sends one request per file
 * through a bounded pool, reporting real byte progress. Below the threshold the
 * native post is untouched, which is why every other create-form spec keeps
 * working unchanged.
 */

const ASSETS = path.join(__dirname, '../test-assets');

/**
 * Eleven distinct files — one more than the default file threshold of 10.
 *
 * The sample-image fixtures are numbered from 2, and every one is copied to a
 * temp file with unique trailing bytes, so sharing indices with another spec
 * cannot collide on the server's global content hash.
 */
function uniqueFilesFrom(first: number, count: number): string[] {
  return Array.from({ length: count }, (_, i) =>
    uniqueAssetFile(path.join(ASSETS, `sample-image-${first + i}.png`))
  );
}

function elevenUniqueFiles(): string[] {
  return uniqueFilesFrom(2, 11);
}

/** Resolves once nothing is in flight any more, whatever the outcome. */
async function waitForBatchToSettle(page: Page) {
  await expect(page.getByTestId('bulk-upload-cancel')).toBeHidden({ timeout: 60000 });
}

async function gotoNewResource(page: Page) {
  await page.goto('/resource/new');
  await page.waitForLoadState('load');
}

/** Collects uncaught page errors so a spec can assert the widget threw none. */
function trackPageErrors(page: Page): string[] {
  const errors: string[] = [];
  page.on('pageerror', (e) => errors.push(e.message));
  return errors;
}

test.describe('Bulk upload widget', () => {
  // The settings row is process-wide and one ephemeral server is shared by every
  // spec this Playwright worker runs. An in-test `finally` is not enough: a test
  // that times out never reaches it, and a leaked max_upload_size or threshold
  // would then fail unrelated specs with a signature nobody traces back here.
  // Resetting an override that was never set is a no-op.
  test.afterEach(async ({ page }) => {
    await page.request.delete('/v1/admin/settings/max_upload_size').catch(() => {});
    await page.request.delete('/v1/admin/settings/upload_widget_file_threshold').catch(() => {});
  });

  test('a small selection still uses the native single-request post', async ({ page }) => {
    // The guard for every other create-form spec: two files is below both
    // thresholds, so the browser must post them in one body and the widget must
    // never appear.
    const errors = trackPageErrors(page);
    await gotoNewResource(page);

    const files = [
      uniqueAssetFile(path.join(ASSETS, 'sample-image-20.png')),
      uniqueAssetFile(path.join(ASSETS, 'sample-image-21.png')),
    ];
    await page.locator('input[type="file"]').setInputFiles(files);

    await expect(page.getByTestId('bulk-upload-hint')).toBeHidden();

    const uploadRequests: string[] = [];
    page.on('request', (r) => {
      if (r.method() === 'POST' && r.url().includes('/v1/resource')) uploadRequests.push(r.url());
    });

    await page.locator('button[type="submit"]:has-text("Save")').click();
    await page.waitForURL(/\/(resource\?id=|resources|group\?id=)/, { timeout: 20000 });

    expect(uploadRequests).toHaveLength(1);
    expect(page.getByTestId('bulk-upload-panel')).toBeHidden();
    expect(errors).toEqual([]);
  });

  test('eleven files upload one request each, with progress, and land on the owner group', async ({ page, apiClient }) => {
    const errors = trackPageErrors(page);
    const stamp = `${Date.now()}-${Math.floor(Math.random() * 100000)}`;

    const category = await apiClient.createCategory(`Bulk Upload Cat ${stamp}`);
    const owner = await apiClient.createGroup({
      name: `Bulk Upload Owner ${stamp}`,
      categoryId: category.ID,
    });
    const tag = await apiClient.createTag(`bulk-upload-${stamp}`);

    await gotoNewResource(page);

    const files = elevenUniqueFiles();
    await page.locator('input[type="file"]').setInputFiles(files);

    // The threshold is announced before Save, not discovered after it.
    await expect(page.getByTestId('bulk-upload-hint')).toBeVisible();
    await expect(page.getByTestId('bulk-upload-hint')).toContainText('11 files');

    // Every other form field must survive the FormData replay onto each request.
    await page.locator('input[name="Name"]').fill(`Bulk ${stamp}`);
    await page.locator('textarea[name="Description"]').fill('uploaded by the bulk widget');

    // Meta travels as a hidden input that the freeFields component writes, which
    // is the part of the "new FormData(form) reproduces native submission"
    // argument most likely to be silently wrong.
    await page.getByRole('button', { name: 'Add new field' }).first().click();
    await page.getByLabel('Field 1 name').fill('batch');
    await page.getByLabel('Field 1 value').fill(stamp);
    await selectAutocomplete(page, 'Owner', owner.Name, 'ownerId');
    await selectAutocomplete(page, 'Tags', tag.Name, 'tags');

    // One request per file, not one for the batch.
    const uploadRequests: string[] = [];
    page.on('request', (r) => {
      if (r.method() === 'POST' && r.url().includes('/v1/resource')) uploadRequests.push(r.url());
    });

    await page.locator('button[type="submit"]:has-text("Save")').click();

    await expect(page.getByTestId('bulk-upload-panel')).toBeVisible({ timeout: 10000 });

    // A full batch lands on the owner group, matching the server's own
    // multi-file redirect (without its /group?id=0 dead end for no owner).
    await page.waitForURL(new RegExp(`/group\\?id=${owner.ID}`), { timeout: 60000 });

    expect(uploadRequests).toHaveLength(11);

    // The payload actually arrived: name, description, owner and tag on each.
    const resources = await apiClient.getResources();
    const mine = resources.filter((r) => r.Name === `Bulk ${stamp}`);
    expect(mine).toHaveLength(11);

    const detail = (await apiClient.getResource(mine[0].ID)) as unknown as {
      Description: string;
      OwnerId: number;
      Tags: Array<{ ID: number }> | null;
      Meta: Record<string, unknown>;
    };
    expect(detail.Description).toBe('uploaded by the bulk widget');
    expect(detail.OwnerId).toBe(owner.ID);
    expect((detail.Tags ?? []).map((t) => t.ID)).toContain(tag.ID);
    expect(detail.Meta).toMatchObject({ batch: stamp });

    expect(errors).toEqual([]);
  });

  test('the progress bar is named, determinate and bounded', async ({ page }) => {
    // Held open deliberately: eleven small PNGs finish in milliseconds, so the
    // uploading state has to be stalled to be observed at all.
    // A latch, not a list of resolvers: only `concurrency` requests are in
    // flight at any moment, so releasing the ones seen so far would leave the
    // rest of the batch stalled against a handler nobody will ever release.
    let openLatch: () => void = () => {};
    const latch = new Promise<void>((resolve) => {
      openLatch = resolve;
    });
    await page.route('**/v1/resource', async (route) => {
      if (route.request().method() !== 'POST') return route.fallback();
      await latch;
      await route.fallback();
    });

    await gotoNewResource(page);
    await page.locator('input[type="file"]').setInputFiles(elevenUniqueFiles());
    await page.locator('button[type="submit"]:has-text("Save")').click();

    const bar = page.getByTestId('bulk-upload-progressbar');
    await expect(bar).toBeVisible({ timeout: 10000 });

    const announced = await bar.evaluate((el) => ({
      role: el.getAttribute('role'),
      label: el.getAttribute('aria-label'),
      valuenow: el.getAttribute('aria-valuenow'),
      valuemax: el.getAttribute('aria-valuemax'),
    }));

    expect(announced.role).toBe('progressbar');
    expect(announced.valuemax).toBe('100');
    // Never a bare prefix — the bug downloadCockpit.js:570 documents.
    expect(announced.label).not.toBe('Upload progress: ');
    expect(announced.label).toContain('of 11 files');
    // The total is known from the picker, so the bar must be determinate.
    // Asserted as a present numeric string first: Number(null) is 0, so a bar
    // that had lost aria-valuenow entirely would sail through a bare >= 0 check.
    //
    // It deliberately does not assert a value above zero. Every request is
    // latched open, so nothing has been acknowledged yet and 0 is the honest
    // reading — demanding more would be asserting a race. That progress
    // actually advances is unit-tested against the upload stream
    // ("reports byte progress from the upload stream"), where the events can be
    // driven rather than waited for.
    expect(announced.valuenow).not.toBeNull();
    expect(announced.valuenow).toMatch(/^\d+$/);
    expect(Number(announced.valuenow)).toBeLessThanOrEqual(100);

    // Save is unavailable while the batch is in flight.
    await expect(page.locator('button[type="submit"]:has-text("Save")')).toBeDisabled();

    // The in-flight list is capped by the concurrency setting (default 3).
    await expect(page.getByTestId('bulk-upload-inflight').locator('li')).toHaveCount(3);

    openLatch();
    await page.waitForURL(/\/(resources|group\?id=)/, { timeout: 60000 });
  });

  test('a duplicate fails on its own row, links to the collision, and is not retryable', async ({ page, apiClient }) => {
    const stamp = `${Date.now()}-${Math.floor(Math.random() * 100000)}`;
    const category = await apiClient.createCategory(`Dupe Cat ${stamp}`);
    const owner = await apiClient.createGroup({
      name: `Dupe Owner ${stamp}`,
      categoryId: category.ID,
    });

    // Uniquify ONCE and reuse that exact path, otherwise every upload gets
    // distinct bytes and there is no collision to observe. Both copies must
    // share an owner too: a different-owner collision is a success on this
    // endpoint, not a 409.
    const collider = uniqueAssetFile(path.join(ASSETS, 'sample-image-30.png'));
    const existing = await apiClient.createResource({
      filePath: collider,
      name: `Collider ${stamp}`,
      ownerId: owner.ID,
      exactBytes: true,
    });

    await gotoNewResource(page);
    await page.locator('input[type="file"]').setInputFiles([
      collider,
      ...uniqueFilesFrom(2, 10),
    ]);
    await selectAutocomplete(page, 'Owner', owner.Name, 'ownerId');

    await page.locator('button[type="submit"]:has-text("Save")').click();

    const failures = page.getByTestId('bulk-upload-failures');
    await expect(failures).toBeVisible({ timeout: 60000 });
    await waitForBatchToSettle(page);
    await expect(failures).toContainText(path.basename(collider));
    await expect(failures.locator(`a[href="/resource?id=${existing.ID}"]`)).toBeVisible();

    // A partial batch must not navigate away and lose the report.
    expect(page.url()).toContain('/resource/new');

    // Retry is offered only for failures a retry could change. A 409 will be
    // refused identically forever, so this batch offers no retry at all.
    await expect(page.getByTestId('bulk-upload-retry')).toBeHidden();

    // The ten good files were still saved.
    await expect(page.getByTestId('bulk-upload-summary')).toContainText('10 of 11 files');
  });

  test('an oversized file is refused before it is transferred', async ({ page }) => {
    // max_upload_size is a per-request bound, and the widget sends one file per
    // request, so it can reject a file up front instead of spending the whole
    // transfer to receive a raw MaxBytesError.
    const tmp = path.join(os.tmpdir(), `mahres-oversize-${uniqueMarker()}.bin`);
    fs.writeFileSync(tmp, Buffer.alloc(3 * 1024 * 1024));

    try {
      await page.request.put('/v1/admin/settings/max_upload_size', {
        data: { value: String(1024 * 1024), reason: 'e2e bulk upload' },
      });

      await gotoNewResource(page);
      await page.locator('input[type="file"]').setInputFiles([tmp, ...uniqueFilesFrom(20, 10)]);

      const uploadRequests: string[] = [];
      page.on('request', (r) => {
        if (r.method() === 'POST' && r.url().includes('/v1/resource')) uploadRequests.push(r.url());
      });

      await page.locator('button[type="submit"]:has-text("Save")').click();

      const failures = page.getByTestId('bulk-upload-failures');
      await expect(failures).toBeVisible({ timeout: 60000 });
      await expect(failures).toContainText('larger than');

      // The oversized file is refused client-side and therefore instantly, so
      // the failures panel is visible while the other ten are still uploading.
      // Counting requests before the batch settles is a race.
      await waitForBatchToSettle(page);

      // Ten requests, not eleven: the oversized one never left the browser.
      expect(uploadRequests).toHaveLength(10);
    } finally {
      await page.request.delete('/v1/admin/settings/max_upload_size');
      fs.rmSync(tmp, { force: true });
    }
  });

  test('a failed Meta schema validation stops the batch before anything uploads', async ({ page, apiClient }) => {
    // The ordering trap. `schema-form-mode` registers its own submit listener on
    // this form and preventDefault()s + stopPropagation()s when the schema fails.
    // It is created inside an x-if, so it connects AFTER Alpine wires the form —
    // and listeners on one element fire in registration order. A handler
    // attached to the form itself would therefore run first and upload eleven
    // files past a visible validation error.
    const stamp = `${Date.now()}-${Math.floor(Math.random() * 100000)}`;
    const category = await apiClient.createResourceCategory(
      `Required Meta ${stamp}`,
      'has a required field',
      {
        MetaSchema: JSON.stringify({
          type: 'object',
          properties: { ticket: { type: 'string', minLength: 3 } },
          required: ['ticket'],
        }),
      }
    );

    try {
      await gotoNewResource(page);
      await selectAutocomplete(page, 'Resource Category', category.Name, 'ResourceCategoryId');

      // The schema form must actually be on the page, or this test proves
      // nothing about ordering.
      await expect(page.locator('schema-form-mode')).toBeVisible({ timeout: 10000 });

      await page.locator('input[type="file"]').setInputFiles(elevenUniqueFiles());

      const uploadRequests: string[] = [];
      page.on('request', (r) => {
        if (r.method() === 'POST' && r.url().includes('/v1/resource')) uploadRequests.push(r.url());
      });

      await page.locator('button[type="submit"]:has-text("Save")').click();

      // Give a handler that ignored the validation time to start requests.
      await page.waitForTimeout(1500);

      expect(uploadRequests).toHaveLength(0);
      await expect(page.getByTestId('bulk-upload-panel')).toBeHidden();
      expect(page.url()).toContain('/resource/new');
    } finally {
      await apiClient.deleteResourceCategory(category.ID).catch(() => {});
    }
  });

  test('Save cannot resend a partial batch', async ({ page, apiClient }) => {
    // Save rebuilds the batch from the file input. Left enabled after a partial
    // run it would resend the files that already succeeded, turning each of them
    // into a duplicate failure — while the panel promised they were not re-sent.
    const stamp = `${Date.now()}-${Math.floor(Math.random() * 100000)}`;
    const category = await apiClient.createCategory(`Partial Cat ${stamp}`);
    const owner = await apiClient.createGroup({
      name: `Partial Owner ${stamp}`,
      categoryId: category.ID,
    });

    const collider = uniqueAssetFile(path.join(ASSETS, 'sample-image-35.png'));
    await apiClient.createResource({
      filePath: collider,
      name: `Partial Collider ${stamp}`,
      ownerId: owner.ID,
      exactBytes: true,
    });

    await gotoNewResource(page);
    await page.locator('input[type="file"]').setInputFiles([collider, ...uniqueFilesFrom(2, 10)]);
    await selectAutocomplete(page, 'Owner', owner.Name, 'ownerId');
    await page.locator('button[type="submit"]:has-text("Save")').click();

    await expect(page.getByTestId('bulk-upload-failures')).toBeVisible({ timeout: 60000 });
    await waitForBatchToSettle(page);

    await expect(page.locator('button[type="submit"]:has-text("Save")')).toBeDisabled();

    // Choosing files again is the way back to a fresh batch.
    await page.locator('input[type="file"]').setInputFiles(uniqueFilesFrom(20, 2));
    await expect(page.getByTestId('bulk-upload-panel')).toBeHidden();
    await expect(page.locator('button[type="submit"]:has-text("Save")')).toBeEnabled();
  });

  test('retrying moves focus onto the panel instead of dropping it to the body', async ({ page, apiClient }) => {
    // Retry hides the button that was just activated — run() sets phase to
    // 'uploading' synchronously — so focus would fall to <body> without a
    // handoff, exactly as Cancel would.
    const stamp = `${Date.now()}-${Math.floor(Math.random() * 100000)}`;
    const category = await apiClient.createCategory(`Retry Focus Cat ${stamp}`);
    const owner = await apiClient.createGroup({
      name: `Retry Focus Owner ${stamp}`,
      categoryId: category.ID,
    });

    // One injected 500 gives a retryable failure; a 409 would offer no Retry.
    // The retry itself is then held open, because the moment under test is while
    // it is in flight: a retry that completes navigates away, and the focus
    // question no longer exists.
    const files = uniqueFilesFrom(2, 11);
    const victim = path.basename(files[0]);
    let injected = false;
    let openLatch: () => void = () => {};
    const latch = new Promise<void>((resolve) => {
      openLatch = resolve;
    });

    await page.route('**/v1/resource', async (route) => {
      const body = route.request().postData() || '';
      if (route.request().method() !== 'POST' || !body.includes(victim)) {
        return route.fallback();
      }
      if (!injected) {
        injected = true;
        await route.fulfill({
          status: 500,
          contentType: 'application/json',
          body: JSON.stringify({ error: 'injected', details: [{ error: 'injected' }] }),
        });
        return;
      }
      await latch;
      await route.fallback();
    });

    await gotoNewResource(page);
    await page.locator('input[type="file"]').setInputFiles(files);
    await selectAutocomplete(page, 'Owner', owner.Name, 'ownerId');
    await page.locator('button[type="submit"]:has-text("Save")').click();

    const retry = page.getByTestId('bulk-upload-retry');
    await expect(retry).toBeVisible({ timeout: 60000 });
    await retry.click();

    await expect(retry).toBeHidden({ timeout: 30000 });
    // Polled rather than read once. Retry moves focus synchronously, before the
    // phase change hides the button, but Alpine's DOM update and the browser's
    // own focus handling land afterwards — polling means the assertion is about
    // where focus settles rather than about winning that ordering. Without the
    // handoff it settles on <body> and stays there, so this still fails against
    // the defect.
    await expect
      .poll(
        () =>
          page.evaluate(
            () => document.activeElement?.getAttribute('data-testid') ?? document.activeElement?.tagName
          ),
        { timeout: 5000, message: 'focus should have been parked on the panel summary' }
      )
      .toBe('bulk-upload-summary');

    openLatch();
  });

  test('cancelling moves focus onto the panel instead of dropping it to the body', async ({ page }) => {
    // Cancel is hidden by the phase change it causes, and a focused element that
    // disappears sends focus to <body> — a keyboard or screen-reader user would
    // have to navigate the whole page again.
    let openLatch: () => void = () => {};
    const latch = new Promise<void>((resolve) => {
      openLatch = resolve;
    });
    let stalling = true;
    await page.route('**/v1/resource', async (route) => {
      if (route.request().method() !== 'POST' || !stalling) return route.fallback();
      await latch;
      await route.fallback();
    });

    await gotoNewResource(page);
    await page.locator('input[type="file"]').setInputFiles(elevenUniqueFiles());
    await page.locator('button[type="submit"]:has-text("Save")').click();

    const cancel = page.getByTestId('bulk-upload-cancel');
    await expect(cancel).toBeVisible({ timeout: 10000 });
    await cancel.click();
    stalling = false;
    openLatch();

    await expect(cancel).toBeHidden({ timeout: 30000 });
    const focused = await page.evaluate(() => document.activeElement?.getAttribute('data-testid') ?? document.activeElement?.tagName);
    expect(focused).toBe('bulk-upload-summary');
  });

  test('the file-count threshold is driven by the runtime setting', async ({ page }) => {
    // Reset in a finally: the settings row is process-wide and one ephemeral
    // server is shared by every spec this worker runs.
    try {
      await page.request.put('/v1/admin/settings/upload_widget_file_threshold', {
        data: { value: '2', reason: 'e2e bulk upload' },
      });

      await gotoNewResource(page);
      await expect(page.locator('form[data-upload-file-threshold]')).toHaveAttribute(
        'data-upload-file-threshold',
        '2'
      );

      // And that the lowered value actually changes behaviour: three files is
      // below the shipped default of 10, so a widget that ignored the setting
      // would leave this hint hidden.
      await page.locator('input[type="file"]').setInputFiles(uniqueFilesFrom(2, 3));
      await expect(page.getByTestId('bulk-upload-hint')).toBeVisible();
      await expect(page.getByTestId('bulk-upload-hint')).toContainText('3 files');
    } finally {
      await page.request.delete('/v1/admin/settings/upload_widget_file_threshold');
    }

    await gotoNewResource(page);
    await expect(page.locator('form[data-upload-file-threshold]')).toHaveAttribute(
      'data-upload-file-threshold',
      '10'
    );
  });
});

/**
 * The create form's autocompleters, located by accessible name.
 *
 * Not by section label: Tags, Groups and Notes all sit inside one section whose
 * label is "Relations", so a `:has(span:has-text("Tags"))` locator matches
 * nothing. Each combobox is aria-labelledby its own title.
 */
async function selectAutocomplete(page: Page, label: string, value: string, fieldName: string) {
  const input = page.getByRole('combobox', { name: label }).first();
  await input.click();
  await input.fill(value);
  const option = page.locator(`div[role="option"]:has-text("${value}")`).first();
  await option.waitFor({ state: 'visible', timeout: 10000 });
  await option.click();
  // The selection materialises as a hidden input carrying the id. Waiting for it
  // means the FormData snapshot taken at submit cannot race the picker.
  await page.waitForSelector(`input[name="${fieldName}"][value]:not([disabled])`, {
    state: 'attached',
    timeout: 5000,
  });
}
