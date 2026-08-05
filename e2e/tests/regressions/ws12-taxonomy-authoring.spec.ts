/**
 * UI bug hunt 2026-07-29, WS12: findings 17/93 (client half), 28, 95, 96 and 154.
 *
 * The server-visible half lives in server/api_tests/ws12_taxonomy_authoring_test.go
 * — CI runs `go test` and does not run this suite. Here is what only a browser
 * knows: what the CodeMirror gutter shows, how many options a <select> really has
 * after its Alpine component has loaded them, what the console holds, and whether
 * the in-app confirm dialog fires.
 *
 * Finding 96 is a REJECTION and is pinned here: "Format JSON" does report the parse
 * error. The report's live-region sweep looked for [aria-live] elements and the
 * message lives in a role="alert" paragraph that is not inside one, so the probe
 * measured the wrong property. The "no lint marker" half of its evidence is real
 * and belongs to 17/93, which is why both are in this file.
 */
import { test, expect } from '../../fixtures/base.fixture';
import { acceptConfirm, dismissConfirm, expectNoConfirm } from '../../helpers/confirm-dialog';

const DESKTOP = { width: 1280, height: 900 };
const INVALID_SCHEMA = '{ not valid json ][';

/** Replace a CodeMirror-backed field's content by name. */
async function setEditor(page: import('@playwright/test').Page, field: string, value: string) {
  await page.evaluate(
    ({ field, value }) => {
      const input = document.querySelector(`input[name="${field}"]`) as HTMLInputElement;
      const view = (input.closest('[x-data]')!.querySelector('[x-ref="editorContainer"]') as never as {
        _cmView: { state: { doc: { length: number } }; dispatch: (t: unknown) => void };
      })._cmView;
      view.dispatch({ changes: { from: 0, to: view.state.doc.length, insert: value } });
    },
    { field, value },
  );
}

async function editorReady(page: import('@playwright/test').Page, field: string) {
  await page.waitForFunction(
    (f) => {
      const input = document.querySelector(`input[name="${f}"]`);
      const container = input?.closest('[x-data]')?.querySelector('[x-ref="editorContainer"]');
      return !!(container as never as { _cmView?: unknown })?._cmView;
    },
    field,
    { timeout: 20_000 },
  );
}

// ------------------------------------------------------- findings 17/93 (client)

test.describe('findings 17/93 — an unparseable Meta JSON Schema is marked in the editor', () => {
  test('the schema editor shows a lint diagnostic, and a valid schema shows none', async ({ page, apiClient }) => {
    const cat = await apiClient.createCategory(`ws12-lint-${Date.now()}`, 'schema lint');

    await page.setViewportSize(DESKTOP);
    await page.goto(`/category/edit?id=${cat.ID}`);
    await editorReady(page, 'MetaSchema');

    // Positive control first: a valid schema lints clean, so "a marker appeared"
    // below cannot be satisfied by an editor that always complains.
    await setEditor(page, 'MetaSchema', '{"type":"object","properties":{"camera":{"type":"string"}}}');
    await page.waitForTimeout(1200);
    expect(
      await page.locator('input[name="MetaSchema"]').evaluate((el) =>
        (el.closest('[x-data]') as HTMLElement).querySelectorAll('.cm-lint-marker-error').length,
      ),
    ).toBe(0);

    // Measured on the unfixed page: {"value":"{ not valid json ][","lintMarkers":0}
    await setEditor(page, 'MetaSchema', INVALID_SCHEMA);
    await expect
      .poll(
        () =>
          page.locator('input[name="MetaSchema"]').evaluate((el) =>
            (el.closest('[x-data]') as HTMLElement).querySelectorAll('.cm-lint-marker-error').length,
          ),
        { timeout: 10_000 },
      )
      .toBeGreaterThan(0);
  });

  test('saving an unparseable schema is refused and the reason is on screen', async ({ page, apiClient }) => {
    const cat = await apiClient.createCategory(`ws12-reject-${Date.now()}`, 'schema reject');

    await page.setViewportSize(DESKTOP);
    await page.goto(`/category/edit?id=${cat.ID}`);
    await editorReady(page, 'MetaSchema');
    await setEditor(page, 'MetaSchema', INVALID_SCHEMA);
    await page.waitForTimeout(1200);

    // The pre-save lint confirm (extended to cover the JSON editor) fires first.
    // Submitting now only *opens* that dialog — the save does not start until it
    // is accepted — so every assertion about the outcome comes after the accept.
    await page.locator('form').first().evaluate((f: HTMLFormElement) => f.requestSubmit());
    const askedBeforeSaving = await acceptConfirm(page);
    expect(askedBeforeSaving).toMatch(/save anyway/i);

    await expect(page.locator('[data-testid="form-error-banner"]')).toBeVisible({ timeout: 15_000 });
    await expect(page.locator('[data-testid="form-error-banner"]')).toContainText(/json/i);

    // And nothing was persisted: the report's symptom was the garbage reloading.
    const stored = await page.request.get(`/v1/category?id=${cat.ID}`);
    const body = await stored.json();
    expect(body.MetaSchema ?? '').not.toContain('not valid json');
  });
});

// ---------------------------------------------------------------- finding 96

test.describe('finding 96 — REJECTED: Format JSON does report why it cannot format', () => {
  test('the parse error is painted in a visible alert next to the editor', async ({ page, apiClient }) => {
    // Measured: clicking Format JSON on `{ not valid json ][` renders
    // "Expected property name or '}' in JSON at position 2 (line 1 column 3)" in a
    // role="alert" that computes display:block and 32px tall. The report's sweep
    // looked at [aria-live] regions and reported "no error, no announcement".
    const cat = await apiClient.createCategory(`ws12-format-${Date.now()}`, 'format json');

    await page.setViewportSize(DESKTOP);
    await page.goto(`/category/edit?id=${cat.ID}`);
    await editorReady(page, 'MetaSchema');
    await setEditor(page, 'MetaSchema', INVALID_SCHEMA);
    await page.waitForTimeout(600);

    const scope = page.locator('input[name="MetaSchema"]').locator('xpath=ancestor::*[@x-data][1]');
    await scope.locator('button[aria-label="Format JSON content"]').click();

    const alert = scope.locator('[role="alert"]').filter({ hasText: /json/i });
    await expect(alert.first()).toBeVisible();
    const painted = await alert.first().evaluate((el) => {
      const r = el.getBoundingClientRect();
      return { h: Math.round(r.height), display: getComputedStyle(el).display };
    });
    expect(painted.h).toBeGreaterThan(8);
    expect(painted.display).not.toBe('none');

    // Positive control: valid JSON formats silently, so the assertion above is not
    // satisfied by an alert that is always present.
    await setEditor(page, 'MetaSchema', '{"type":"object"}');
    await page.waitForTimeout(600);
    await scope.locator('button[aria-label="Format JSON content"]').click();
    await page.waitForTimeout(600);
    await expect(scope.locator('[role="alert"]').filter({ hasText: /position/i })).toHaveCount(0);
  });
});

// ---------------------------------------------------------------- finding 28

test.describe('finding 28 — the copy-from picker offers every source, not the first 50', () => {
  test('a taxonomy larger than one page is fully offered', async ({ page, apiClient, request }) => {
    // This has to actually exceed one page, or it is vacuous: the first draft
    // created three categories on a near-empty worker server, so they landed on
    // page 1 and the test passed against the unfixed single-request code.
    //
    // The worker's server is shared, so count what is there first and top it up
    // past a page boundary. Names sort last so the new ones are on the final page —
    // which is exactly where the report's missing 'Person' / 'Vendor' were.
    const PAGE_SIZE = 50;
    let existing = 0;
    for (let p = 1; p <= 20; p++) {
      const resp = await request.get(`/v1/categories?page=${p}`);
      const rows = await resp.json();
      existing += rows.length;
      if (rows.length < PAGE_SIZE) break;
    }

    const stamp = Date.now();
    const created: string[] = [];
    // Enough to push past the next page boundary, plus a few to sit on it.
    const wanted = PAGE_SIZE - (existing % PAGE_SIZE) + 3;
    for (let i = 0; i < wanted; i++) {
      const name = `zzz-ws12-copy-${stamp}-${String(i).padStart(3, '0')}`;
      await apiClient.createCategory(name, 'copy source');
      created.push(name);
    }
    expect(existing + created.length).toBeGreaterThan(PAGE_SIZE);

    // Count the paged requests the component makes, which proves the fix directly
    // and does not depend on how much data happens to be there.
    const pagesRequested = new Set<string>();
    page.on('request', (req) => {
      const u = new URL(req.url());
      if (u.pathname === '/v1/categories') pagesRequested.add(u.searchParams.get('page') ?? 'none');
    });

    await page.setViewportSize(DESKTOP);
    await page.goto('/category/new');
    const picker = page.locator('#tb-copy');
    await expect(picker).toBeVisible();

    await expect
      .poll(async () => picker.locator('option').count(), { timeout: 30_000 })
      .toBeGreaterThan(PAGE_SIZE);
    // Let the last page land.
    await page.waitForTimeout(2000);

    expect(
      pagesRequested.size,
      `the picker issued only ${[...pagesRequested].join(',')} — it is not paging`,
    ).toBeGreaterThan(1);

    const options = await picker.locator('option').allTextContents();
    // The last-created name sorts after everything and can only be on a later page.
    const last = created[created.length - 1];
    expect(options, `${last} is not offered as a copy source`).toContain(last);
  });
});

// ---------------------------------------------------------------- finding 95

test.describe('finding 95 — the sandboxed preview iframe stops flooding the console', () => {
  test('no CORS failures on /v1/account/settings, on load or on refresh', async ({ page, apiClient }) => {
    const cat = await apiClient.createCategory(`ws12-preview-${Date.now()}`, 'preview', {
      CustomHeader: '<div class="p-2">[property name]</div>',
    });
    await apiClient.createGroup({ name: `ws12-preview-member-${Date.now()}`, categoryId: cat.ID });

    const consoleErrors: string[] = [];
    page.on('console', (m) => {
      if (m.type() === 'error') consoleErrors.push(m.text());
    });

    await page.setViewportSize(DESKTOP);
    await page.goto(`/category/edit?id=${cat.ID}`);
    // The preview renders on init; wait for the frame to have a document.
    await page.waitForTimeout(4000);

    const settingsErrors = () =>
      consoleErrors.filter((e) => e.includes('/v1/account/settings')).length;

    // Measured on the unfixed page: 6 after load, 12 after one Refresh (the report
    // saw it climb 6 -> 18 -> 30).
    expect(settingsErrors(), `console errors: ${consoleErrors.join(' | ')}`).toBe(0);

    await page.getByRole('button', { name: 'Refresh' }).first().click();
    await page.waitForTimeout(4000);
    expect(settingsErrors(), `after refresh: ${consoleErrors.join(' | ')}`).toBe(0);

    // Positive control: the preview really did render, so "no errors" is not
    // satisfied by a frame that never ran the bundle.
    const frameHasContent = await page.evaluate(() => {
      const f = document.querySelector('iframe[title="Template slot preview"]') as HTMLIFrameElement;
      return (f?.getAttribute('srcdoc') || '').includes('/public/dist/main.js');
    });
    expect(frameHasContent).toBe(true);
  });
});

// ---------------------------------------------------------------- finding 154

test.describe('finding 154 — applying a preset over authored content asks first', () => {
  test('a preset prompts and, when declined, changes nothing', async ({ page, apiClient }) => {
    const authored = '<div class="p-2 bg-blue-50 rounded">[property name] authored</div>';
    const cat = await apiClient.createCategory(`ws12-preset-${Date.now()}`, 'preset', {
      CustomHeader: authored,
    });

    await page.setViewportSize(DESKTOP);
    await page.goto(`/category/edit?id=${cat.ID}`);
    await editorReady(page, 'CustomHeader');
    // Give templateBundle time to load its presets.
    await expect.poll(async () => page.locator('#tb-preset option').count(), { timeout: 20_000 })
      .toBeGreaterThan(1);

    await page.locator('#tb-preset').selectOption({ index: 1 });
    // The click only *opens* the confirm now; applyPreset is awaiting the answer,
    // so nothing is written until the dialog is dismissed below.
    await page.locator('[data-testid="tb-apply-preset"]').click();

    // Measured on the unfixed page: CustomHeader went 107 -> 353 characters with
    // zero dialogs. dismissConfirm waits for the dialog, so it is also the
    // "a confirmation fired before replacing authored content" assertion — it
    // fails on a page that replaces silently.
    const asked = await dismissConfirm(page);
    expect(asked).toMatch(/replace/i);
    expect(asked).toMatch(/template field/i);

    // Give a wrongly-proceeding apply time to land before reading the field, so
    // "unchanged" is not just measured before the write.
    await page.waitForTimeout(800);
    const after = await page.locator('input[name="CustomHeader"]').inputValue();
    expect(after, 'the declined preset was applied anyway').toBe(authored);
  });

  test('an empty form applies a preset with no prompt at all', async ({ page }) => {
    // The control: prompting on a create form with nothing to lose would be noise.
    await page.setViewportSize(DESKTOP);
    await page.goto('/category/new');
    await editorReady(page, 'CustomHeader');
    await expect.poll(async () => page.locator('#tb-preset option').count(), { timeout: 20_000 })
      .toBeGreaterThan(1);

    await page.locator('#tb-preset').selectOption({ index: 1 });
    await page.locator('[data-testid="tb-apply-preset"]').click();
    // The confirm would open asynchronously, so give it the same window the apply
    // gets before asserting it never came — checking immediately would pass even
    // on a page that prompts one tick later.
    await page.waitForTimeout(800);
    await expectNoConfirm(page);

    expect(await page.locator('input[name="CustomHeader"]').inputValue()).not.toBe('');
  });
});

// --------------------------------------------- findings 17/93, the a11y half

test.describe('findings 17/93 — the JSON lint is reported to assistive technology, not only painted', () => {
  test('an unparseable schema sets aria-invalid and announces the reason; fixing it clears both', async ({ page, apiClient }) => {
    // Batch 11's lint is `linter()` + `lintGutter()`, which is a gutter marker and
    // an underlined range — pixels. A screen reader got nothing: no aria-invalid on
    // the editor, no description, no announcement. Driven through the real editor
    // because what carries the state is CodeMirror's own contentDOM, which only
    // exists once the component has mounted.
    const cat = await apiClient.createCategory(`ws12-a11y-lint-${Date.now()}`, 'schema lint a11y');

    await page.setViewportSize(DESKTOP);
    await page.goto(`/category/edit?id=${cat.ID}`);
    await editorReady(page, 'MetaSchema');

    const editorContent = page.locator('input[name="MetaSchema"]')
      .locator('xpath=ancestor::*[@x-data][1]')
      .locator('.cm-content');
    const region = page.getByTestId('json-lint-MetaSchema');

    // Positive control first: a valid schema reports nothing, so the assertions
    // below cannot be satisfied by an editor that is permanently invalid.
    await setEditor(page, 'MetaSchema', '{"type":"object","properties":{"camera":{"type":"string"}}}');
    await expect.poll(() => editorContent.getAttribute('aria-invalid'), { timeout: 5_000 }).toBeNull();
    await expect(region).toBeHidden();

    await setEditor(page, 'MetaSchema', INVALID_SCHEMA);

    await expect(editorContent).toHaveAttribute('aria-invalid', 'true', { timeout: 5_000 });
    await expect(region).toBeVisible();
    await expect(region).toContainText('Invalid JSON');

    // The description has to actually resolve: aria-describedby pointing at an id
    // that is not on the page is the same silence with extra markup.
    const described = await page.evaluate(() => {
      const content = document.querySelector('input[name="MetaSchema"]')
        ?.closest('[x-data]')?.querySelector('.cm-content');
      const id = content?.getAttribute('aria-describedby') || '';
      const target = id ? document.getElementById(id) : null;
      return { id, resolved: !!target, text: target?.textContent?.trim() || '' };
    });
    expect(described.id, 'the editor has no aria-describedby').not.toBe('');
    expect(described.resolved, `aria-describedby="${described.id}" points at no element`).toBe(true);
    expect(described.text).toContain('Invalid JSON');

    // And it is polite, not assertive: this fires on a debounce while typing.
    await expect(region).toHaveAttribute('aria-live', 'polite');

    // Fixing the schema takes both away again.
    await setEditor(page, 'MetaSchema', '{"type":"object"}');
    await expect.poll(() => editorContent.getAttribute('aria-invalid'), { timeout: 5_000 }).toBeNull();
    await expect(region).toBeHidden();
  });
});
