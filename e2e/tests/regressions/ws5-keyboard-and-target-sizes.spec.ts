/**
 * UI bug hunt 2026-07-29, WS5: findings 6, 13, 36/105, 39, 48, 49, 76/139, 99.
 *
 * The markup half of WS5 — attributes and heading levels the server writes — is
 * asserted in Go (server/api_tests/ws5_keyboard_names_headings_test.go), because CI
 * runs `go test` and does not run this suite. What is here is only what Go cannot
 * reach: DOM focus, computed styles and measured boxes.
 *
 * Two rules this file follows, both paid for in earlier batches:
 *
 *  - For a target size, assert getBoundingClientRect(), never the CSS declaration.
 *    A rule can be present and overridden, or present and applied to the wrong box.
 *
 *  - For focusability, assert that a key actually moves focus, never that a
 *    `tabindex` attribute exists. Finding 6 is the case in point: the roving
 *    tabindex and aria-selected were both correct and moving on every ArrowDown,
 *    while document.activeElement never left the first option — an attribute-level
 *    assertion would have passed against the bug for as long as it existed.
 */
import path from 'path';
import type { Page } from '@playwright/test';
import { test, expect } from '../../fixtures/base.fixture';

/** Every resource this file creates needs a real file; one fixture serves all of them. */
const FIXTURE = path.join(__dirname, '../../test-assets/sample-image.png');

/** WCAG 2.2 SC 2.5.8 Target Size (Minimum). */
const MIN_TARGET = 24;

/** The focused element, described the way a reader would experience it. */
async function activeElement(page: Page) {
  return page.evaluate(() => {
    const el = document.activeElement as HTMLElement | null;
    if (!el) return { tag: 'NONE', label: '', text: '' };
    return {
      tag: el.tagName,
      label: el.getAttribute('aria-label') || '',
      text: (el.textContent || '').replace(/\s+/g, ' ').trim().slice(0, 30),
    };
  });
}

/**
 * The SETTLED focused element, not the first one that matches.
 *
 * expect.poll returns on its first matching sample, so it cannot assert where
 * focus ends up — only that it passed through somewhere. Anything with a
 * transition, a teardown or a deferred callback needs stability first.
 */
async function settledActiveElement(page: Page) {
  let previous = JSON.stringify(await activeElement(page));
  let stable = 0;
  for (let i = 0; i < 20; i++) {
    await page.waitForTimeout(120);
    const now = JSON.stringify(await activeElement(page));
    stable = now === previous ? stable + 1 : 0;
    previous = now;
    if (stable >= 5) break;
  }
  return JSON.parse(previous) as { tag: string; label: string; text: string };
}

/**
 * Open the add-block picker and wait for it to be usable.
 *
 * Both waits are load-bearing. The listbox is revealed by x-show with an
 * x-transition, so clicking an option the instant it becomes "visible" hits an
 * element Playwright considers unstable and times out. And the initial focus is
 * moved in an Alpine $nextTick — measured landing ~50ms after the click — so
 * reading document.activeElement synchronously reads the trigger, not the option.
 */
async function openBlockPicker(page: Page) {
  await page.locator('[data-testid=add-block-trigger]').click();
  const listbox = page.locator('#add-block-listbox');
  await expect(listbox).toBeVisible();
  await expect(listbox.locator('[role=option]').first()).toBeVisible();
  return listbox;
}

/** Add a block of the given type through the picker. */
async function addBlock(page: Page, label: RegExp) {
  const listbox = await openBlockPicker(page);
  await listbox.locator('[role=option]').filter({ hasText: label }).first().click();
  await expect(listbox).toBeHidden();
}

/**
 * The index of the option that holds DOM focus, once it stops moving.
 *
 * Polling for stability rather than for a match: expect.poll returns on its
 * first hit, so it would happily accept the pre-$nextTick state where nothing in
 * the listbox is focused at all.
 */
async function settledPickerFocus(page: Page) {
  const read = () =>
    page.evaluate(() => {
      const lb = document.querySelector('#add-block-listbox');
      if (!lb) return { focusIdx: -2, stateIdx: -2, count: 0 };
      const opts = [...lb.querySelectorAll('[role=option]')];
      const w = window as unknown as { Alpine?: { $data: (el: Element) => Record<string, unknown> } };
      const root = document.querySelector('.block-editor');
      const state = w.Alpine && root ? w.Alpine.$data(root) : null;
      return {
        focusIdx: opts.indexOf(document.activeElement as Element),
        stateIdx: state ? (state.activePickerIndex as number) : -99,
        count: opts.length,
      };
    });
  let previous = JSON.stringify(await read());
  let stable = 0;
  for (let i = 0; i < 20; i++) {
    await page.waitForTimeout(80);
    const now = JSON.stringify(await read());
    stable = now === previous ? stable + 1 : 0;
    previous = now;
    if (stable >= 4) break;
  }
  return JSON.parse(previous) as { focusIdx: number; stateIdx: number; count: number };
}

// ─────────────────────────────────────────────── finding 6

test.describe('finding 6 — the block-type listbox', () => {
  test('arrow keys move DOM focus, not only the roving tabindex', async ({ page, apiClient }) => {
    const note = await apiClient.createNote({ name: `ws5-listbox-${Date.now()}` });
    const errors: string[] = [];
    page.on('pageerror', (e) => errors.push(String(e)));

    await page.goto(`/note?id=${note.ID}`);
    await page.getByRole('button', { name: /edit blocks/i }).click();
    await openBlockPicker(page);

    /**
     * Read the option index that has DOM focus *and* the one the component thinks
     * is active. The bug was precisely a divergence between the two: the state and
     * the ARIA walked 0→1→2→7→0 while focus stayed on 0, because focusPickerItem's
     * `this.$el` was the <li> that handled the key rather than the component root,
     * so its querySelector for the listbox returned null and the .focus() call was
     * skipped silently.
     */
    const indices = () => settledPickerFocus(page);

    const opened = await indices();
    expect(opened.count, 'the picker should offer every block type').toBeGreaterThan(2);
    expect(opened.focusIdx, 'opening the picker focuses the first option').toBe(0);

    // Positive control: the state moves. Without it, a component that stopped
    // reacting to keys entirely would satisfy "focus tracks state" trivially.
    await page.keyboard.press('ArrowDown');
    const down1 = await indices();
    expect(down1.stateIdx, 'ArrowDown advances the active option').toBe(1);
    expect(down1.focusIdx, 'and DOM focus follows it').toBe(1);

    await page.keyboard.press('ArrowDown');
    expect(await indices()).toMatchObject({ focusIdx: 2, stateIdx: 2 });

    await page.keyboard.press('End');
    const end = await indices();
    expect(end.stateIdx, 'End jumps to the last option').toBe(end.count - 1);
    expect(end.focusIdx).toBe(end.count - 1);

    await page.keyboard.press('Home');
    expect(await indices()).toMatchObject({ focusIdx: 0, stateIdx: 0 });

    await page.keyboard.press('ArrowUp');
    expect(await indices(), 'ArrowUp at the first option stays put rather than wrapping into nothing')
      .toMatchObject({ focusIdx: 0, stateIdx: 0 });

    expect(errors, 'no page errors while driving the listbox').toEqual([]);
  });

  test('Tab dismisses the picker and leaves, rather than trapping focus', async ({ page, apiClient }) => {
    const note = await apiClient.createNote({ name: `ws5-listbox-tab-${Date.now()}` });
    await page.goto(`/note?id=${note.ID}`);
    await page.getByRole('button', { name: /edit blocks/i }).click();
    await openBlockPicker(page);
    // Focus has to be *inside* the listbox before Tab means anything: the handler
    // that dismisses the picker is on the option, not on the trigger.
    const before = await settledPickerFocus(page);
    expect(before.focusIdx, 'focus must start on an option').toBe(0);

    await page.keyboard.press('Tab');

    // The popup must close (it is absolutely positioned over the page), focus must
    // not still be inside it, and — the discriminating part — Tab must actually
    // move ON rather than bouncing back to the trigger.
    //
    // The handler used to be @keydown.tab.prevent, which closed the picker and let
    // the close-watcher pull focus back to "+ Add Block": the popup was gone and
    // focus was on a real control, so a test that only checked those two things
    // passed against it while a keyboard user still needed two Tab presses to
    // leave. WCAG 2.1.2 is satisfied either way; this pins the behaviour.
    await expect(page.locator('#add-block-listbox')).toBeHidden();
    const settled = await settledActiveElement(page);
    const insideListbox = await page.evaluate(
      () => !!document.activeElement?.closest('#add-block-listbox'),
    );
    expect(insideListbox, `Tab left focus inside the dismissed listbox (on ${settled.tag})`).toBe(false);
    expect(settled.tag, 'focus is on a real control, not <body>').not.toBe('BODY');

    const onTrigger = await page.evaluate(
      () => !!(document.activeElement as HTMLElement | null)?.closest('[data-testid=add-block-trigger]'),
    );
    expect(
      onTrigger,
      'one Tab press should leave the picker, not return focus to the button that opened it',
    ).toBe(false);
  });
});

// ─────────────────────────────────────────────── finding 13

test('finding 13 — the details table scroller is reachable and scrollable by keyboard', async ({ page, apiClient }) => {
  // A long name is what makes the table wider than its container in the first
  // place; without it there is nothing to scroll and the test proves nothing.
  await apiClient.createResource({
    filePath: FIXTURE,
    name: `ws5 a resource with a deliberately very long descriptive name so that the details table is wider than its scroll container ${Date.now()}`,
  });

  await page.goto('/resources/details');
  const wrap = page.locator('.detail-table-wrap');
  await expect(wrap).toBeVisible();

  // Precondition: there IS horizontal overflow. Asserted, not assumed.
  const overflow = await wrap.evaluate((el) => ({ sw: el.scrollWidth, cw: el.clientWidth }));
  expect(overflow.sw, `the table must overflow for this test to mean anything (${JSON.stringify(overflow)})`)
    .toBeGreaterThan(overflow.cw);

  /**
   * Tab has to REACH the scroller — that is what "keyboard operable" means, and it
   * is the only assertion here that discriminates.
   *
   * The obvious version of this test used `wrap.focus()` and passed against the
   * bug: Playwright's locator.focus() calls element.focus(), and Chromium honours
   * that on a div with no tabindex at all, so document.activeElement became the
   * wrapper and ArrowRight duly scrolled it — measured scrollLeft 0 -> 80 on the
   * unfixed page, while 60 consecutive Tab presses never landed on it once.
   * Programmatic focus is not keyboard operability.
   */
  let reached = false;
  for (let i = 0; i < 60 && !reached; i++) {
    await page.keyboard.press('Tab');
    reached = await page.evaluate(() =>
      String(document.activeElement?.className || '').includes('detail-table-wrap'),
    );
  }
  expect(reached, 'Tab never reached the horizontally scrollable table region').toBe(true);

  // And once focused, the arrow key scrolls it — the point of making it focusable.
  const before = await wrap.evaluate((el) => el.scrollLeft);
  await page.keyboard.press('ArrowRight');
  await page.keyboard.press('ArrowRight');
  await expect
    .poll(async () => wrap.evaluate((el) => el.scrollLeft), { timeout: 3000 })
    .toBeGreaterThan(before);
});

// ─────────────────────────────────────────────── findings 76/139, 48, 99

test('findings 76/139 — the details row checkbox meets the minimum target size', async ({ page, apiClient }) => {
  await apiClient.createResource({ filePath: FIXTURE, name: `ws5-target-size-${Date.now()}` });
  await page.goto('/resources/details');

  const box = await page.locator('tbody .detail-table-checkbox').first().boundingBox();
  expect(box, 'a row checkbox must be rendered').not.toBeNull();
  expect(box!.width, `row checkbox width ${box!.width}px`).toBeGreaterThanOrEqual(MIN_TARGET);
  expect(box!.height, `row checkbox height ${box!.height}px`).toBeGreaterThanOrEqual(MIN_TARGET);

  // …and it matches the grid view of the same list, which is the inconsistency
  // half of finding 139. The grid is the control: if it ever shrinks, this test
  // should fail rather than silently agree at a smaller size.
  await page.goto('/resources');
  const gridBox = await page.locator('.card-checkbox').first().boundingBox();
  expect(gridBox!.width).toBeGreaterThanOrEqual(MIN_TARGET);
  expect(box!.width, 'table and grid checkboxes should be the same size').toBe(gridBox!.width);
});

test('finding 76/139 — still a 24px target on a touch viewport', async ({ page, apiClient }) => {
  await apiClient.createResource({ filePath: FIXTURE, name: `ws5-target-size-mobile-${Date.now()}` });
  await page.setViewportSize({ width: 390, height: 844 });
  await page.goto('/resources/details');

  const box = await page.locator('tbody .detail-table-checkbox').first().boundingBox();
  expect(box!.width, `row checkbox width at 390px: ${box!.width}px`).toBeGreaterThanOrEqual(MIN_TARGET);
  expect(box!.height).toBeGreaterThanOrEqual(MIN_TARGET);
});

test('finding 99 — the saved-query delete button is visible without hover and big enough', async ({ page, request }) => {
  // The button only exists when a saved query does.
  const created = await request.post('/v1/mrql/saved', {
    data: { name: `ws5 saved query ${Date.now()}`, query: 'name ~ "*ws5*"' },
  });
  expect(created.ok(), `creating a saved query: ${created.status()}`).toBe(true);

  for (const viewport of [{ width: 1280, height: 900 }, { width: 390, height: 844 }]) {
    await page.setViewportSize(viewport);
    await page.goto('/mrql');

    const del = page.locator('button[aria-label^="Delete saved query"]').first();
    await expect(del, `delete control at ${viewport.width}px`).toBeVisible();

    // opacity, specifically: the button was painted at opacity:0 and revealed only
    // by :hover, so it was "visible" to a naive check while being invisible and —
    // on a touch device, where there is no hover — unreachable.
    const opacity = await del.evaluate((el) => getComputedStyle(el).opacity);
    expect(Number(opacity), `opacity at ${viewport.width}px`).toBeGreaterThan(0.5);

    const box = await del.boundingBox();
    expect(box!.height, `delete button height at ${viewport.width}px: ${box!.height}px`)
      .toBeGreaterThanOrEqual(MIN_TARGET);
    expect(box!.width).toBeGreaterThanOrEqual(MIN_TARGET);
  }
});

test('finding 48 — every block-editor remove control meets the minimum target size', async ({ page, apiClient }) => {
  const note = await apiClient.createNote({ name: `ws5-remove-targets-${Date.now()}` });
  await page.goto(`/note?id=${note.ID}`);
  await page.getByRole('button', { name: /edit blocks/i }).click();

  // Build the two blocks whose remove controls were the smallest: a to-do item
  // (9.6x24) and a table column (8.4x20). A fresh todos block has no items, so the
  // item row — and its "×" — only exists after "+ Add item".
  await addBlock(page, /todos/i);
  await addBlock(page, /table/i);
  await page.locator('button', { hasText: '+ Add item' }).first().click();
  await expect(page.locator('.block-editor input[x-model="item.label"]').first()).toBeVisible();

  const undersized = await page.locator('.block-editor .remove-target').evaluateAll(
    (els, min) =>
      els
        .map((el) => {
          const r = el.getBoundingClientRect();
          return {
            name: el.getAttribute('aria-label') || (el.textContent || '').trim(),
            w: Math.round(r.width),
            h: Math.round(r.height),
          };
        })
        .filter((b) => b.w > 0 && (b.w < min || b.h < min)),
    MIN_TARGET,
  );

  // Positive control: there ARE remove controls on the page. Without it, a
  // selector that matched nothing would satisfy "none are undersized" forever.
  const total = await page.locator('.block-editor .remove-target').count();
  expect(total, 'the block editor should render remove controls').toBeGreaterThan(0);
  expect(undersized, `undersized remove controls: ${JSON.stringify(undersized)}`).toEqual([]);
});

// ─────────────────────────────────────────────── finding 49

test('finding 49 — a calendar day is reachable by keyboard and opens the new-event dialog', async ({ page, apiClient }) => {
  const note = await apiClient.createNote({ name: `ws5-calendar-${Date.now()}` });
  await page.goto(`/note?id=${note.ID}`);
  await page.getByRole('button', { name: /edit blocks/i }).click();
  await addBlock(page, /calendar/i);
  // The month grid renders in view mode only — measured: with editMode true the
  // calendar component reports view "month" and loading false and still renders
  // zero day cells. The report's own steps say to click Done, and they are right.
  await page.getByRole('button', { name: /^Done$/ }).click();

  const dayButtons = page.locator('button.calendar-day-number');
  await expect(dayButtons.first()).toBeVisible({ timeout: 10000 });

  const count = await dayButtons.count();
  expect(count, 'a month grid renders every day as a focusable control').toBeGreaterThanOrEqual(28);

  // Focusable, named, and a real 24px target.
  const geometry = await dayButtons.evaluateAll((els, min) =>
    els
      .map((el) => {
        const r = el.getBoundingClientRect();
        return { name: el.getAttribute('aria-label') || '', w: Math.round(r.width), h: Math.round(r.height) };
      })
      .filter((b) => b.w > 0 && (b.w < min || b.h < min || !b.name)),
    MIN_TARGET,
  );
  expect(geometry, `day controls that are unnamed or undersized: ${JSON.stringify(geometry.slice(0, 4))}`).toEqual([]);

  // Behaviour, not attributes: Enter on a focused day must open the event dialog.
  const target = dayButtons.nth(10);
  await target.focus();
  expect(await page.evaluate(() => document.activeElement?.className)).toContain('calendar-day-number');
  await page.keyboard.press('Enter');
  await expect(
    page.locator('input[type=date], input[name=start], [role=dialog]').first(),
    'Enter on a day cell opens the new-event dialog',
  ).toBeVisible({ timeout: 5000 });
});

// ─────────────────────────────────────────────── finding 36/105

test('findings 36/105 — the export group picker is arrow-key navigable', async ({ page, apiClient }) => {
  const stamp = Date.now();
  const category = await apiClient.createCategory(`ws5 picker category ${stamp}`);
  for (const suffix of ['alpha', 'beta']) {
    await apiClient.createGroup({ name: `ws5picker-${stamp}-${suffix}`, categoryId: category.ID });
  }

  await page.goto('/admin/export');
  const input = page.getByLabel('Search groups to add');
  await input.fill(`ws5picker-${stamp}`);

  const listbox = page.locator('#export-group-listbox');
  await expect(listbox).toBeVisible();
  await expect(listbox.locator('[role=option]')).toHaveCount(2);

  // aria-expanded must reflect reality, and aria-activedescendant must move with
  // the arrow key. Both are runtime state, hence a browser test.
  await expect(input).toHaveAttribute('aria-expanded', 'true');
  expect(await input.getAttribute('aria-activedescendant'), 'nothing is active before an arrow key').toBeNull();

  await page.keyboard.press('ArrowDown');
  await expect(input).toHaveAttribute('aria-activedescendant', 'export-group-option-0');
  await page.keyboard.press('ArrowDown');
  await expect(input).toHaveAttribute('aria-activedescendant', 'export-group-option-1');

  // Focus stays on the input — that is the combobox pattern, and the reason the
  // options carry tabindex="-1".
  expect(await page.evaluate(() => document.activeElement?.getAttribute('aria-label')))
    .toBe('Search groups to add');

  // Enter commits the marked option — whichever one that is. Reading the marked
  // option's own text rather than assuming a name for index 1: /v1/groups does not
  // promise alphabetical order, and hardcoding "beta" made this fail while every
  // ARIA assertion above passed.
  const markedName = (await listbox.locator('[role=option]').nth(1).innerText()).trim();
  expect(markedName, 'the marked option should carry a group name').toContain(`ws5picker-${stamp}`);

  await page.keyboard.press('Enter');
  await expect(page.getByTestId('export-group-chips')).toContainText(markedName);
  await expect(input).toHaveAttribute('aria-expanded', 'false');
});

// ─────────────────────────────────────────────── finding 39 (rejected)

/**
 * Finding 39 said /admin/settings controls paint no focus indicator, on the
 * evidence that they compute `outline-style: none`. They do — and a 2px amber
 * ring is painted by box-shadow instead, which the original probe never read.
 * The finding is rejected, and this test pins the ring so the rejection cannot
 * quietly become wrong: if someone removes the Tailwind focus:ring the indicator
 * really would disappear, and outline-style would still be none.
 */
test('finding 39 (rejected) — settings controls paint a focus ring via box-shadow', async ({ page }) => {
  await page.goto('/admin/settings');

  const field = page.locator('[data-testid^="setting-row-"] input').first();
  await field.focus();

  const painted = await field.evaluate((el) => {
    const cs = getComputedStyle(el);
    return { outlineStyle: cs.outlineStyle, boxShadow: cs.boxShadow };
  });

  // A ring exists, and it is not a fully transparent placeholder. Tailwind emits
  // `rgba(0, 0, 0, 0) 0px 0px 0px 0px` segments when no ring is configured, so
  // "boxShadow !== none" alone would pass against a missing ring.
  expect(painted.boxShadow, 'a focus indicator must be painted somehow').not.toBe('none');
  const opaqueRing = /(oklch|rgb)\([^)]*\)\s+0px\s+0px\s+0px\s+[1-9]/.test(painted.boxShadow);
  expect(
    opaqueRing,
    `no opaque focus ring found; outlineStyle=${painted.outlineStyle} boxShadow=${painted.boxShadow}`,
  ).toBe(true);
});
