/**
 * UI bug hunt 2026-07-29, WS11: findings 22, 23/46, 24, 125/158, 134 and 160.
 *
 * The server-visible half lives in server/api_tests/ws11_query_surfaces_test.go,
 * because CI runs `go test` and does not run this suite. What is here is what only
 * a browser knows: what CodeMirror's completion list actually renders and filters
 * on, whether a scroll container scrolls, and whether Tab reaches it.
 *
 * Two notes on method:
 *
 *  - The scroll-region test loops Tab rather than calling locator.focus().
 *    Chromium honours element.focus() on a <div> with no tabindex at all, so a
 *    focus()-based test passes against the unfixed page while 60 consecutive Tab
 *    presses never land on the element once.
 *  - Findings 22 and 160 turned out to be one bug, not two. The completion list
 *    rendered the server's `label` (a description) instead of its `value`, and
 *    CodeMirror filters options by their label — so typing `tags.c` matched
 *    nothing against "relation count" and the popup refused to open at all.
 */
import { test, expect } from '../../fixtures/base.fixture';

const DESKTOP = { width: 1280, height: 900 };

/**
 * apiClient.createGroup requires a categoryId, so make one. Each caller gets its
 * own category and its own group: a spec that reads "some group" off a list shared
 * by the whole worker is a spec whose fixture is whatever another file created last.
 */
async function makeGroup(
  apiClient: { createCategory: (n: string, d?: string) => Promise<{ ID: number }>; createGroup: (d: { name: string; categoryId: number }) => Promise<{ ID: number }> },
  name: string,
) {
  const category = await apiClient.createCategory(`${name}-cat`, 'ws11 fixture');
  return apiClient.createGroup({ name, categoryId: category.ID });
}

/** Type into the MRQL editor's CodeMirror instance. */
async function typeInEditor(page: import('@playwright/test').Page, text: string) {
  await page.locator('.cm-content').first().click();
  await page.keyboard.type(text);
}

/** The visible completion rows, in order, as the user sees them. */
async function completionRows(page: import('@playwright/test').Page) {
  return page.evaluate(() => {
    const tip = document.querySelector('.cm-tooltip-autocomplete');
    if (!tip) return null;
    return [...tip.querySelectorAll('li')].map((li) => ({
      label: li.querySelector('.cm-completionLabel')?.textContent ?? '',
      detail: li.querySelector('.cm-completionDetail')?.textContent ?? '',
    }));
  });
}

// ------------------------------------------------------------- findings 22, 160

test.describe('findings 22 and 160 — the MRQL completion list shows the token it inserts', () => {
  test('every row is labelled with the field name, with the description as detail', async ({ page }) => {
    await page.setViewportSize(DESKTOP);
    await page.goto('/mrql');
    await page.waitForSelector('.cm-content', { state: 'visible' });
    await page.waitForTimeout(500);

    await typeInEditor(page, 'type = resource AND ');
    await page.keyboard.press('Control+Space');

    await expect.poll(async () => (await completionRows(page))?.length ?? 0, { timeout: 10_000 })
      .toBeGreaterThan(10);

    const rows = (await completionRows(page))!;
    const labels = rows.map((r) => r.label);

    // Positive control: the list really is the MRQL field list.
    expect(labels).toContain('name');
    expect(labels).toContain('created');

    // The report's exact symptom: four indistinguishable rows all reading
    // "relation count", plus descriptions standing in for field names.
    const relationCountLabels = labels.filter((l) => l === 'relation count');
    expect(relationCountLabels, 'a description is being used as a completion label').toHaveLength(0);
    for (const description of [
      'any ancestor group',
      'any descendant group',
      'entity type filter',
      'full-text search',
      'perceptual similarity',
    ]) {
      expect(labels, `"${description}" is a description, not a token`).not.toContain(description);
    }

    // The tokens that were hidden behind "relation count" are now distinguishable.
    expect(labels).toContain('tags.count');
    expect(labels).toContain('groups.count');
    expect(labels).toContain('notes.count');

    // And the description survives as secondary detail rather than being dropped.
    const tagsCount = rows.find((r) => r.label === 'tags.count');
    expect(tagsCount?.detail).toBe('relation count');
  });

  test('filtering matches the field name, not the description', async ({ page }) => {
    await page.setViewportSize(DESKTOP);
    await page.goto('/mrql');
    await page.waitForSelector('.cm-content', { state: 'visible' });
    await page.waitForTimeout(500);

    await typeInEditor(page, 'type = resource AND ');
    await page.keyboard.press('Control+Space');
    await expect.poll(async () => (await completionRows(page))?.length ?? 0, { timeout: 10_000 })
      .toBeGreaterThan(10);

    // "anc" used to match the descriptions "any ancestor group" / "any descendant
    // group" and, bizarrely, all four "relation count" rows.
    await page.keyboard.type('anc');
    await page.waitForTimeout(800);

    const rows = await completionRows(page);
    if (rows && rows.length > 0) {
      for (const row of rows) {
        expect(row.label, 'a row matched on its description, not its name').not.toBe('relation count');
      }
    }
  });

  test('finding 160 — the popup opens for a member completion after a dot', async ({ page }) => {
    await page.setViewportSize(DESKTOP);
    await page.goto('/mrql');
    await page.waitForSelector('.cm-content', { state: 'visible' });
    await page.waitForTimeout(500);

    // Measured on the unfixed page: no tooltip, automatically or on Ctrl+Space,
    // while POST /v1/mrql/complete returned 29 suggestions including tags.count.
    await typeInEditor(page, 'type = resource AND tags.c');
    await page.keyboard.press('Control+Space');

    await expect.poll(async () => {
      const rows = await completionRows(page);
      return rows ? rows.map((r) => r.label) : [];
    }, { timeout: 10_000 }).toContain('tags.count');
  });
});

// --------------------------------------------------------------- findings 23/46

test.describe('findings 23/46 — the plan and the results always describe the same query', () => {
  test('running a different query clears a stale Explain panel', async ({ page, apiClient }) => {
    await makeGroup(apiClient, `ws11-explain-${Date.now()}`);
    await page.setViewportSize(DESKTOP);
    await page.goto('/mrql');
    await page.waitForSelector('.cm-content', { state: 'visible' });
    await page.waitForTimeout(500);

    // Explain one query...
    await typeInEditor(page, 'type = note LIMIT 3');
    await page.keyboard.press('ControlOrMeta+Shift+Enter');
    const plan = page.locator('[aria-label="Query plan"]');
    await expect(plan).toBeVisible({ timeout: 15_000 });
    await expect(plan).toContainText('Explain (note)');

    // ...then run another.
    await page.locator('.cm-content').first().click();
    await page.keyboard.press('ControlOrMeta+A');
    await page.keyboard.type('type = group LIMIT 3');
    await page.keyboard.press('ControlOrMeta+Enter');

    const results = page.locator('[aria-label="Query results"]');
    await expect(results).toContainText('Entity: group', { timeout: 15_000 });

    // The measured symptom: the plan still read "Explain (note) / SELECT * FROM
    // `notes` WHERE 1 = 1 LIMIT 3" beside "Results (3 items) / Entity: group".
    await expect(plan).toHaveCount(0);
  });

  test('explaining a different query clears the previous result rows', async ({ page, apiClient }) => {
    await makeGroup(apiClient, `ws11-explain2-${Date.now()}`);
    await page.setViewportSize(DESKTOP);
    await page.goto('/mrql');
    await page.waitForSelector('.cm-content', { state: 'visible' });
    await page.waitForTimeout(500);

    await typeInEditor(page, 'type = group LIMIT 3');
    await page.keyboard.press('ControlOrMeta+Enter');
    const results = page.locator('[aria-label="Query results"]');
    await expect(results).toContainText('Entity: group', { timeout: 15_000 });

    await page.locator('.cm-content').first().click();
    await page.keyboard.press('ControlOrMeta+A');
    await page.keyboard.type('type = note LIMIT 3');
    await page.keyboard.press('ControlOrMeta+Shift+Enter');

    const plan = page.locator('[aria-label="Query plan"]');
    await expect(plan).toContainText('Explain (note)', { timeout: 15_000 });
    await expect(results).not.toContainText('Entity: group');
  });

  test('explaining and then running the SAME query keeps both panels', async ({ page, apiClient }) => {
    // The control that stops the fix from being "clear the other panel always",
    // which would break the workflow the Explain button exists for.
    await makeGroup(apiClient, `ws11-explain3-${Date.now()}`);
    await page.setViewportSize(DESKTOP);
    await page.goto('/mrql');
    await page.waitForSelector('.cm-content', { state: 'visible' });
    await page.waitForTimeout(500);

    await typeInEditor(page, 'type = group LIMIT 3');
    await page.keyboard.press('ControlOrMeta+Shift+Enter');
    const plan = page.locator('[aria-label="Query plan"]');
    await expect(plan).toContainText('Explain (group)', { timeout: 15_000 });

    await page.keyboard.press('ControlOrMeta+Enter');
    const results = page.locator('[aria-label="Query results"]');
    await expect(results).toContainText('Entity: group', { timeout: 15_000 });
    await expect(plan).toContainText('Explain (group)');
  });
});

// ------------------------------------------------------------- findings 125/158

test.describe('findings 125/158 — the results header counts and warns honestly', () => {
  test('a single result reads "1 item" and carries no truncation warning', async ({ page, apiClient }) => {
    const marker = `ws11-single-${Date.now()}`;
    await makeGroup(apiClient, marker);

    await page.setViewportSize(DESKTOP);
    await page.goto(`/mrql?q=${encodeURIComponent(`type = group AND name ~ "${marker}"`)}`);

    const results = page.locator('[aria-label="Query results"]');
    await expect(results).toContainText('(1 item)', { timeout: 20_000 });
    await expect(results).not.toContainText('(1 items)');

    // Measured on the unfixed page: a full-width amber banner reading "Default
    // limit applied (500 rows) — add LIMIT / OFFSET to the query to paginate."
    // over a single row that had not been truncated.
    await expect(page.locator('[data-testid="mrql-default-limit-banner"]')).toHaveCount(0);
  });

  test('the truncation warning still appears when the limit really did cut rows off', async ({ page, apiClient }) => {
    // Positive control. Without it, "no banner" is satisfied by a banner that
    // never renders at all.
    const marker = `ws11-many-${Date.now()}`;
    for (let i = 0; i < 3; i++) {
      await makeGroup(apiClient, `${marker}-${i}`);
    }

    await page.setViewportSize(DESKTOP);
    // An explicit LIMIT would suppress the banner by design, so drive the default
    // limit down instead — the same runtime setting finding 159 turned out to be about.
    await page.request.put('/v1/admin/settings/mrql_default_limit', { data: { value: '2' } });
    try {
      await page.goto(`/mrql?q=${encodeURIComponent(`type = group AND name ~ "${marker}"`)}`);
      const results = page.locator('[aria-label="Query results"]');
      await expect(results).toContainText('(2 items)', { timeout: 20_000 });
      await expect(page.locator('[data-testid="mrql-default-limit-banner"]')).toBeVisible();
    } finally {
      await page.request.delete('/v1/admin/settings/mrql_default_limit');
    }
  });
});

// ----------------------------------------------------------------- finding 24

test.describe('finding 24 — a wide SQL result scrolls and a keyboard user can scroll it', () => {
  test('the results table overflows a scrollable, Tab-reachable region', async ({ page, apiClient }) => {
    const query = await apiClient.createQuery({
      name: `ws11-wide-${Date.now()}`,
      text: 'SELECT * FROM categories ORDER BY name ASC',
    });

    await page.setViewportSize(DESKTOP);
    await page.goto(`/query?id=${query.ID}`);
    await page.getByRole('button', { name: 'Run' }).click();
    await page.waitForSelector('.query-results table', { timeout: 15_000 });

    const geometry = await page.evaluate(() => {
      const box = document.querySelector('.query-results') as HTMLElement;
      const table = box.querySelector('table') as HTMLElement;
      const headers = [...table.querySelectorAll('th')];
      return {
        overflowX: getComputedStyle(box).overflowX,
        boxClientWidth: box.clientWidth,
        boxScrollWidth: box.scrollWidth,
        tableWidth: Math.round(table.getBoundingClientRect().width),
        headerCount: headers.length,
        tallestHeader: Math.max(...headers.map((h) => Math.round(h.getBoundingClientRect().height))),
      };
    });

    // Positive control: this really is the 16-column case the report measured.
    expect(geometry.headerCount).toBeGreaterThanOrEqual(14);

    expect(geometry.overflowX, 'the box was overflow-x:visible, so nothing scrolled').toBe('auto');
    expect(
      geometry.boxScrollWidth,
      'the table is still being squeezed to the container instead of overflowing it',
    ).toBeGreaterThan(geometry.boxClientWidth);
    // Measured on the unfixed page: the first <th> was 269px tall, one character
    // per line. A header that fits on one or two lines is under ~60px.
    expect(geometry.tallestHeader, 'header cells are still wrapping to one character per line')
      .toBeLessThan(80);

    // Keyboard operability, driven by the key that reaches it rather than by
    // element.focus() — which Chromium honours on a tabindex-less <div>.
    await page.locator('h1').first().click();
    const reached = await page.evaluate(async () => {
      // Walk the document's tab order and report whether the region is in it.
      const box = document.querySelector('.query-results') as HTMLElement;
      return box.getAttribute('tabindex') === '0' && box.getAttribute('role') === 'region';
    });
    expect(reached, 'the scroller is not marked as a focusable region').toBe(true);

    await page.keyboard.press('Escape');
    let landed = false;
    for (let i = 0; i < 80 && !landed; i++) {
      await page.keyboard.press('Tab');
      landed = await page.evaluate(
        () => document.activeElement === document.querySelector('.query-results'),
      );
    }
    expect(landed, 'Tab never reached the scrollable results region').toBe(true);

    // And once focused, an arrow key scrolls it.
    const before = await page.evaluate(() => document.querySelector('.query-results')!.scrollLeft);
    await page.keyboard.press('ArrowRight');
    await page.waitForTimeout(200);
    const after = await page.evaluate(() => document.querySelector('.query-results')!.scrollLeft);
    expect(after, 'the focused region does not scroll with the arrow keys').toBeGreaterThan(before);
  });
});

// ---------------------------------------------------------------- finding 134

test.describe('finding 134 — REJECTED: the MRQL filter bar does report a syntax error', () => {
  test('a single-quoted expression paints a visible alert naming the parse error', async ({ page, apiClient }) => {
    // The report says the single-quoted query "yields zero group cards with no
    // visible error in the filter area", and caveats itself: only the first 400
    // characters of main were captured. Measured: the alert is at
    // [16,294,824,20], display:block, visibility:visible, opacity:1, with
    // aria-invalid="true" on the input. This test pins the rejection, so it passes
    // in both directions — which is the point of a rejection control.
    await makeGroup(apiClient, `ws11-Photography-${Date.now()}`);

    await page.goto("/groups?mrql=name+%7E+%27Photo%27");
    const alert = page.locator('.mrql-bar [role="alert"]');
    await expect(alert).toBeVisible();
    await expect(alert).toContainText('expected value');
    await expect(page.locator('.mrql-bar input[name="mrql"]')).toHaveAttribute('aria-invalid', 'true');

    const painted = await page.evaluate(() => {
      const el = document.querySelector('.mrql-bar [role="alert"]') as HTMLElement;
      const r = el.getBoundingClientRect();
      const cs = getComputedStyle(el);
      return { w: Math.round(r.width), h: Math.round(r.height), opacity: cs.opacity, display: cs.display };
    });
    expect(painted.w).toBeGreaterThan(50);
    expect(painted.h).toBeGreaterThan(8);
    expect(painted.opacity).toBe('1');

    // Positive control: a valid expression paints no alert, so the assertion above
    // cannot be satisfied by a bar that always complains.
    await page.goto('/groups?mrql=name+%7E+%22Photo%22');
    await page.waitForLoadState('domcontentloaded');
    await expect(page.locator('.mrql-bar [role="alert"]')).toBeHidden();
  });
});
