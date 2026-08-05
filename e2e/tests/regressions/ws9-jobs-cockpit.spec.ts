/**
 * UI bug hunt 2026-07-29, WS9: findings 2, 40, 41 and 113, plus review remediation
 * findings 3, 4 and 5.
 *
 * Finding 2 is the last high-severity row in the campaign. Its server half is in
 * server/api_tests/ws9_jobs_cockpit_test.go and download_queue/cancel_paused_test.go;
 * its predicates are unit-tested in src/components/downloadCockpit.test.ts. What is
 * here is the part that needs a real browser and a real job: which controls a paused
 * row offers, what the progress bar announces, which element wins a hit test over an
 * open modal, and where focus lands.
 *
 * Every row is addressed by `[data-job-id]` — review remediation finding 5. The jobs
 * the panel shows come from a process-wide queue that the whole worker shares, and
 * every test download stalls against the same URL (`/v1/jobs/events`), so
 * `getJobTitle` renders every one of them as "events". Locating a row by that text
 * found *a* stalled download, not this test's own, and one negative assertion keyed
 * on a Name the panel never renders at all: it could not fail.
 */
import { test, expect } from '../../fixtures/base.fixture';
import type { Locator, Page } from '@playwright/test';

/** Submits a background download that will never finish on its own. */
async function submitStalledDownload(page: Page, label: string): Promise<string> {
  // /v1/jobs/events is an SSE endpoint: it answers 200 and then never closes, which
  // makes it a download that stays in flight for as long as the test needs. The CLI
  // doctest for `job cancel` uses the same trick.
  const res = await page.request.post(`/v1/download/submit`, {
    data: { URL: new URL('/v1/jobs/events', page.url() || 'http://127.0.0.1').toString(), Name: label },
  });
  expect(res.status(), await res.text()).toBe(202);
  const body = await res.json();
  return body.jobs[0].id;
}

async function jobStatus(page: Page, id: string): Promise<string> {
  const res = await page.request.get(`/v1/jobs/get?id=${id}`);
  if (!res.ok()) return `http-${res.status()}`;
  return (await res.json()).status;
}

/** The panel row for one specific job. */
function rowFor(page: Page, id: string): Locator {
  return page.locator(`[data-testid="cockpit-panel"] [data-job-id="${id}"]`);
}

/**
 * Wait for `document.activeElement` to stop moving, and report what it is.
 *
 * Two identical consecutive samples rather than one matching sample: the focus trap
 * arms on a timeout and the panel's teardown moves focus, so the first non-body value
 * seen is not necessarily where the reader is left.
 */
async function settledFocus(page: Page): Promise<string> {
  const describe = () =>
    page.evaluate(() => {
      const el = document.activeElement as HTMLElement | null;
      if (!el || el === document.body) return 'body';
      return `${el.tagName.toLowerCase()}#${el.id}.${el.className}|${(el.textContent || '').trim().slice(0, 40)}`;
    });
  let previous = await describe();
  for (let i = 0; i < 40; i++) {
    await page.waitForTimeout(50);
    const current = await describe();
    if (current === previous && current !== 'body') return current;
    previous = current;
  }
  return previous;
}

/** The position of each job's row in the rendered list, or -1 when it is absent. */
async function rowPositions(page: Page, ids: string[]): Promise<number[]> {
  return page.evaluate((wanted) => {
    const panel = document.querySelector('[data-testid="cockpit-panel"]');
    if (!panel) return wanted.map(() => -1);
    const rendered = [...panel.querySelectorAll('[data-job-id]')].map(
      (el) => el.getAttribute('data-job-id') ?? '',
    );
    return wanted.map((id) => rendered.indexOf(id));
  }, ids);
}

test.describe('findings 2 and 41 — a paused download keeps its readout and can be abandoned', () => {
  test('the paused row offers Cancel, keeps its progress bar, and cancelling works', async ({ page }) => {
    const errors: string[] = [];
    page.on('pageerror', e => errors.push(e.message));

    await page.goto('/dashboard');
    const id = await submitStalledDownload(page, `ws9 paused ${Date.now()}`);

    await expect.poll(() => jobStatus(page, id), { timeout: 10_000 }).toBe('downloading');
    const paused = await page.request.post(`/v1/jobs/pause?id=${id}`);
    expect(paused.status()).toBe(200);
    await expect.poll(() => jobStatus(page, id), { timeout: 10_000 }).toBe('paused');

    // A second, *running* download submitted after the paused one. The panel sorts
    // newest first, so this decoy is row 1 — which is what the old locator
    // (`.filter({ hasText: 'events' }).first()`) would have picked, and a running row
    // offers no Resume. It is here to keep the assertions below honest about which
    // row they are reading.
    const decoy = await submitStalledDownload(page, `ws9 paused decoy ${Date.now()}`);
    await expect.poll(() => jobStatus(page, decoy), { timeout: 10_000 }).toBe('downloading');

    await page.locator('[data-testid="cockpit-trigger"]').click();
    const panel = page.locator('[data-testid="cockpit-panel"]');
    await expect(panel).toBeVisible();

    const row = rowFor(page, id);
    await expect(row).toHaveCount(1, { timeout: 10_000 });

    // Finding 41: the row used to collapse to "⏸ … Paused … Resume" — no bytes, no
    // percentage, no bar. Measured on the server, a paused job still reports its
    // progress, so this is purely what the panel renders.
    await expect(row.locator('[data-testid="cockpit-progressbar"]')).toBeVisible();
    // Finding 2's UI half.
    await expect(row.getByRole('button', { name: 'Cancel' })).toBeVisible();
    await expect(row.getByRole('button', { name: 'Resume' })).toBeVisible();
    // The decoy is the control for the two assertions above: a running row must not
    // offer Resume, so they are reading the paused row and not just any row.
    await expect(rowFor(page, decoy).getByRole('button', { name: 'Resume' })).toHaveCount(0);

    await row.getByRole('button', { name: 'Cancel' }).click();

    // Asserted through the API: the panel updating is necessary and not sufficient.
    await expect.poll(() => jobStatus(page, id), { timeout: 10_000 }).toBe('cancelled');
    await expect(row.getByRole('button', { name: 'Retry' })).toBeVisible();
    // The decoy is untouched — cancelling one row must not reach another.
    expect(await jobStatus(page, decoy)).toBe('downloading');
    expect(errors).toEqual([]);

    await page.request.post(`/v1/jobs/cancel?id=${decoy}`);
  });

  test('a running download is unchanged: Pause and Cancel, and no Resume', async ({ page }) => {
    // The control for the test above. If the fix had turned every row into a paused
    // row, the assertions there would still pass.
    await page.goto('/dashboard');
    const id = await submitStalledDownload(page, `ws9 running ${Date.now()}`);
    await expect.poll(() => jobStatus(page, id), { timeout: 10_000 }).toBe('downloading');

    await page.locator('[data-testid="cockpit-trigger"]').click();
    const row = rowFor(page, id);
    await expect(row).toHaveCount(1, { timeout: 10_000 });
    await expect(row.getByRole('button', { name: 'Pause' })).toBeVisible();
    await expect(row.getByRole('button', { name: 'Cancel' })).toBeVisible();
    await expect(row.getByRole('button', { name: 'Resume' })).toHaveCount(0);

    await page.request.post(`/v1/jobs/cancel?id=${id}`);
  });
});

test('finding 113 — the progress bar is named after its job and is indeterminate when the size is unknown', async ({ page }) => {
  await page.goto('/dashboard');
  const id = await submitStalledDownload(page, `ws9 naming ${Date.now()}`);
  await expect.poll(() => jobStatus(page, id), { timeout: 10_000 }).toBe('downloading');

  await page.locator('[data-testid="cockpit-trigger"]').click();
  const bar = rowFor(page, id).locator('[data-testid="cockpit-progressbar"]');
  await expect(bar).toBeVisible({ timeout: 10_000 });

  const announced = await bar.evaluate((el) => ({
    label: el.getAttribute('aria-label'),
    valuenow: el.getAttribute('aria-valuenow'),
    valuetext: el.getAttribute('aria-valuetext'),
  }));

  // Measured before the fix: {"label":"Download progress: ","now":"0","txt":null} —
  // an unnamed bar reporting 0% for the whole transfer.
  expect(announced.label).not.toBe('Download progress: ');
  expect(announced.label).toContain('events');
  // SSE has no Content-Length, so this download's total is genuinely unknown. An
  // indeterminate progressbar must not claim a value.
  expect(announced.valuenow).toBeNull();
  expect(announced.valuetext, 'an indeterminate bar has to describe itself somehow').toBeTruthy();

  await page.request.post(`/v1/jobs/cancel?id=${id}`);
});

test.describe('finding 40 — newest first, and finished jobs can be dismissed', () => {
  test('a job just submitted sorts above one submitted before it', async ({ page }) => {
    await page.goto('/dashboard');
    const older = await submitStalledDownload(page, `ws9 order one ${Date.now()}`);
    await expect.poll(() => jobStatus(page, older), { timeout: 10_000 }).toBe('downloading');
    await page.request.post(`/v1/jobs/cancel?id=${older}`);

    const newer = await submitStalledDownload(page, `ws9 order two ${Date.now()}`);
    await expect.poll(() => jobStatus(page, newer), { timeout: 10_000 }).toBe('downloading');

    await page.locator('[data-testid="cockpit-trigger"]').click();
    await expect(rowFor(page, newer)).toHaveCount(1, { timeout: 10_000 });
    await expect(rowFor(page, older)).toHaveCount(1);

    // Both jobs' actual positions, compared with each other, so the rest of the
    // worker's queue cannot decide the outcome. The old assertion asked only whether
    // *some* row existed, which is true under either ordering.
    const [newerAt, olderAt] = await rowPositions(page, [newer, older]);
    expect(newerAt, 'the job submitted second is not in the panel').toBeGreaterThanOrEqual(0);
    expect(olderAt, 'the job submitted first is not in the panel').toBeGreaterThanOrEqual(0);
    expect(newerAt, `newest-first: row ${newerAt} must come before row ${olderAt}`).toBeLessThan(olderAt);

    // And it is above the fold: the panel opens scrolled to the top, so the newest
    // row being first only helps if it is on screen. Before the fix the newest row
    // was last, measured ~1340px below the fold with 20 rows above it.
    const onScreen = await rowFor(page, newer).evaluate((el) => {
      const list = el.closest('.overflow-y-auto') as HTMLElement;
      const row = el.getBoundingClientRect();
      const box = list.getBoundingClientRect();
      return { visible: row.top >= box.top - 1 && row.bottom <= box.bottom + 1, scrollTop: list.scrollTop };
    });
    expect(onScreen.visible, `the newest row is not within the list's visible box (scrollTop ${onScreen.scrollTop})`).toBe(true);

    await page.request.post(`/v1/jobs/cancel?id=${newer}`);
  });

  test('Clear completed removes finished jobs and they stay gone', async ({ page }) => {
    await page.goto('/dashboard');
    const id = await submitStalledDownload(page, `ws9 clear ${Date.now()}`);
    await expect.poll(() => jobStatus(page, id), { timeout: 10_000 }).toBe('downloading');
    await page.request.post(`/v1/jobs/cancel?id=${id}`);
    await expect.poll(() => jobStatus(page, id), { timeout: 10_000 }).toBe('cancelled');

    // A second job left running, as the positive control for the clear: an unfinished
    // row must survive it. Without this, a "Clear completed" that emptied the panel
    // entirely would pass.
    const kept = await submitStalledDownload(page, `ws9 clear kept ${Date.now()}`);
    await expect.poll(() => jobStatus(page, kept), { timeout: 10_000 }).toBe('downloading');

    await page.locator('[data-testid="cockpit-trigger"]').click();
    // The precondition, asserted rather than assumed: the row is on screen before the
    // button is pressed, so its absence afterwards means something.
    await expect(rowFor(page, id)).toHaveCount(1, { timeout: 10_000 });

    const clear = page.locator('[data-testid="cockpit-clear-completed"]');
    await expect(clear).toBeVisible({ timeout: 10_000 });
    await clear.click();

    // Gone from the queue, which is what makes it durable: a client-only hide is
    // undone by the next SSE init event.
    await expect.poll(() => jobStatus(page, id), { timeout: 10_000 }).toBe('http-404');
    await expect(rowFor(page, id)).toHaveCount(0);
    await expect(rowFor(page, kept)).toHaveCount(1);

    await page.reload();
    await page.locator('[data-testid="cockpit-trigger"]').click();
    await expect(page.locator('[data-testid="cockpit-panel"]')).toBeVisible();
    await expect(rowFor(page, kept)).toHaveCount(1, { timeout: 10_000 });
    await expect(rowFor(page, id)).toHaveCount(0);

    await page.request.post(`/v1/jobs/cancel?id=${kept}`);
  });

  test('a job that finishes while the clear is in flight does not come back', async ({ page }) => {
    // Review remediation finding 2. The panel used to dismiss the ids *it* thought
    // were finished, snapshotted before the request; the server decides at handling
    // time. A job that crossed into a terminal state inside that window was cleared
    // server-side, was missing from the dismissed set, and the `removed` handler —
    // whose job is to retain finished jobs for display — put it straight back.
    //
    // The window is widened deliberately by holding the response: the clear is
    // routed through a handler that waits until the second job has been cancelled.
    await page.goto('/dashboard');
    const finished = await submitStalledDownload(page, `ws9 window one ${Date.now()}`);
    const racer = await submitStalledDownload(page, `ws9 window two ${Date.now()}`);
    await expect.poll(() => jobStatus(page, finished), { timeout: 10_000 }).toBe('downloading');
    await expect.poll(() => jobStatus(page, racer), { timeout: 10_000 }).toBe('downloading');
    await page.request.post(`/v1/jobs/cancel?id=${finished}`);
    await expect.poll(() => jobStatus(page, finished), { timeout: 10_000 }).toBe('cancelled');

    await page.locator('[data-testid="cockpit-trigger"]').click();
    await expect(rowFor(page, racer)).toHaveCount(1, { timeout: 10_000 });

    // The racer reaches its terminal state *after* the browser has decided what to
    // dismiss and before the server handles the clear.
    await page.route('**/v1/jobs/clearCompleted', async (route) => {
      await page.request.post(`/v1/jobs/cancel?id=${racer}`);
      // The cancel is only the request. A downloading job's worker stamps the
      // terminal state when it unwinds, and the clear only takes finished jobs — so
      // hold the request until the server would really clear this one, or the window
      // being tested is not open yet.
      const deadline = Date.now() + 15_000;
      while (Date.now() < deadline && (await jobStatus(page, racer)) !== 'cancelled') {
        await new Promise((resolve) => setTimeout(resolve, 50));
      }
      await route.continue();
    });

    await page.locator('[data-testid="cockpit-clear-completed"]').click();

    // Why this can fail against the old client, rather than passing for the wrong
    // reason: the racer's terminal `updated` is emitted by its worker before the route
    // handler above observes `cancelled`, which is before the clear is handled and its
    // `removed` is emitted. Both events reach the browser on one ordered SSE stream, so
    // by the time `removed` arrives the client's copy of the racer is terminal — and
    // the old `removed` handler retains a terminal job it has not been told to dismiss.
    await expect.poll(() => jobStatus(page, racer), { timeout: 10_000 }).toBe('http-404');
    // The row must not be retained for display: the server has no such job, so the
    // panel would be showing a job that does not exist until the next reconnect.
    await expect(rowFor(page, racer)).toHaveCount(0);
    await expect(rowFor(page, finished)).toHaveCount(0);
    await page.unroute('**/v1/jobs/clearCompleted');
  });

  test('Clear completed is not offered when nothing is finished', async ({ page }) => {
    // The control: the button must be conditional, or the assertion above would pass
    // against a button that is always there and does nothing.
    await page.goto('/dashboard');
    await page.locator('[data-testid="cockpit-trigger"]').click();
    const panel = page.locator('[data-testid="cockpit-panel"]');
    await expect(panel).toBeVisible();

    const state = await page.evaluate(() => {
      const root = document.querySelector('.download-cockpit') as HTMLElement & { _x_dataStack?: any[] };
      const data = root?._x_dataStack?.[0];
      if (!data) return null;
      data.jobs = [];
      data.retainedCompletedJobs = [];
      return true;
    });
    expect(state, 'could not reach the cockpit component state').toBe(true);

    await expect(page.locator('[data-testid="cockpit-clear-completed"]')).toHaveCount(0);
  });
});

// ------------------------------------------------- review remediation finding 3

test('review finding 3 — an open header dropdown must not paint or click through the jobs modal', async ({ page }) => {
  // Moving the trigger into the header (findings 83/102) put the panel inside the
  // header's stacking context: .header is position:sticky with z-index 40, so its
  // descendants are ordered against each other, not against the page. The settings
  // and account dropdowns are *later* siblings at z-50, so at z-50 the panel painted
  // below them and they stayed hit-testable over an aria-modal dialog.
  //
  // Item 7 answered that structurally — the panel is teleported into `.overlays`
  // (z-index 41, root stacking context), so it outranks the whole header layer at 40
  // rather than out-numbering two of its siblings. This test is unchanged in what it
  // measures; only the locator moved, because `.download-cockpit` is now just the
  // trigger's wrapper and no longer contains the panel.
  //
  // Asserted with elementFromPoint over a point inside the dropdown, not with the
  // computed z-index: reading the z-index back would pass against the bug.
  await page.goto('/dashboard');

  const settings = page.locator('.settings button').first();
  await settings.click();
  const dropdown = page.locator('.settings [x-show="active"]');
  await expect(dropdown).toBeVisible();

  // The precondition: with no modal open, the dropdown owns its own pixels. If this
  // ever stopped being true the assertion below would pass for the wrong reason.
  const before = await dropdown.evaluate((el) => {
    const r = el.getBoundingClientRect();
    const hit = document.elementFromPoint(r.left + r.width / 2, r.top + Math.min(12, r.height / 2));
    return hit?.closest('.settings') !== null;
  });
  expect(before, 'the dropdown does not win a hit test over itself, so this test measures nothing').toBe(true);

  // Opened by the keyboard shortcut, which is how the report reproduced it.
  await page.keyboard.press('ControlOrMeta+Shift+KeyD');
  const panel = page.locator('[data-testid="cockpit-panel"]');
  await expect(panel).toBeVisible();

  const covered = await dropdown.evaluate((el) => {
    const r = el.getBoundingClientRect();
    const x = r.left + r.width / 2;
    const y = r.top + Math.min(12, r.height / 2);
    const hit = document.elementFromPoint(x, y) as HTMLElement | null;
    return {
      inDropdown: hit?.closest('.settings') !== null,
      inCockpit: hit?.closest('[data-testid="cockpit-overlay"]') !== null,
      tag: hit?.tagName.toLowerCase() ?? 'null',
      point: [Math.round(x), Math.round(y)],
    };
  });

  expect(
    covered.inDropdown,
    `the settings dropdown still wins the hit test at ${covered.point} while an aria-modal dialog is open (hit ${covered.tag})`,
  ).toBe(false);
  expect(covered.inCockpit, `the point at ${covered.point} belongs to the modal (hit ${covered.tag})`).toBe(true);
});

// ------------------------------------------------- review remediation finding 4

test('review finding 4 — closing the panel opened by its shortcut returns focus', async ({ page }) => {
  // toggle(event) captures event.currentTarget, and the Cmd/Ctrl+Shift+D handler
  // calls toggle() with no event — so with no prior click there was nothing to
  // return to, the restore was gated on having something, and the panel is
  // x-trap.noreturn. Focus fell to <body>.
  //
  // Tab rather than locator.focus(): focus() succeeds on things the browser would
  // never give focus to, so it proves nothing about the Tab order.
  await page.goto('/dashboard');
  await page.waitForSelector('[data-testid="cockpit-trigger"]');

  await page.keyboard.press('Tab');
  const opener = await page.evaluate(() => {
    const el = document.activeElement as HTMLElement | null;
    return { tag: el?.tagName.toLowerCase() ?? 'null', text: (el?.textContent || '').trim() };
  });
  expect(opener.tag, 'one Tab from a fresh page should land on a real control').not.toBe('body');

  await page.keyboard.press('ControlOrMeta+Shift+KeyD');
  const panel = page.locator('[data-testid="cockpit-panel"]');
  await expect(panel).toBeVisible();
  // The trap arms on a setTimeout and recomputes when the contents change, so wait
  // for focus to be inside before closing — otherwise the close races the open.
  await expect
    .poll(() => page.evaluate(() => document.activeElement?.closest('[data-testid="cockpit-panel"]') !== null))
    .toBe(true);

  await page.keyboard.press('Escape');
  await expect(panel).toHaveCount(0);

  // Poll for focus to *settle*, then assert once. `expect.poll` returns on its first
  // matching sample, so a poll for "not body" can be satisfied by a transient landing
  // and say nothing about where focus ended up.
  await settledFocus(page);
  const landed = await page.evaluate(() => {
    const el = document.activeElement as HTMLElement | null;
    return { tag: el?.tagName.toLowerCase() ?? 'null', text: (el?.textContent || '').trim() };
  });
  expect(landed, 'focus must come back to whatever had it when the shortcut was pressed').toEqual(opener);
});

test('review finding 4 — with focus nowhere, the panel still hands it back to the trigger', async ({ page }) => {
  // The other half: pressing the shortcut on a freshly loaded page, where
  // document.activeElement is <body> and there is nothing to capture at all. The
  // trigger is the floor — <body> is not a focus target, it is the bug.
  await page.goto('/dashboard');
  await page.waitForSelector('[data-testid="cockpit-trigger"]');
  expect(await page.evaluate(() => document.activeElement === document.body)).toBe(true);

  await page.keyboard.press('ControlOrMeta+Shift+KeyD');
  await expect(page.locator('[data-testid="cockpit-panel"]')).toBeVisible();
  await expect
    .poll(() => page.evaluate(() => document.activeElement?.closest('[data-testid="cockpit-panel"]') !== null))
    .toBe(true);

  await page.keyboard.press('Escape');
  await expect(page.locator('[data-testid="cockpit-panel"]')).toHaveCount(0);

  await settledFocus(page);
  const onTrigger = await page.evaluate(
    () => document.activeElement?.closest('[data-testid="cockpit-trigger"]') !== null,
  );
  expect(onTrigger, 'focus was left on <body>, which is where the bug left it').toBe(true);
});

/**
 * Round-3 audit: the panel must not open underneath a true modal.
 *
 * The panel used to live in `.header` — position:sticky with z-index 40, so a
 * stacking context — and its z-[60] only ordered it against its header siblings. The
 * app's modals live in `.overlays` at z-index 41 in the *root* stacking context,
 * above the whole header layer, so the panel opened behind one, moved focus inside
 * itself and x-trap held it there.
 *
 * Deferred-work item 7 teleports the panel into `.overlays` too, so it is now
 * ordered against those modals rather than stranded below them — but that is not
 * what makes this safe, and the guard stays. Two aria-modal dialogs open at once is
 * the defect whichever one paints on top: each arms its own trap. The panel declines
 * rather than winning.
 *
 * A browser test because it depends on which `<template x-if>` branch Alpine
 * instantiated and on where focus actually is; the decision itself is unit-tested in
 * downloadCockpit.test.ts and the markup contract in ws9_jobs_cockpit_test.go.
 */
test('round 3 — the jobs panel declines to open underneath an open modal', async ({ page }) => {
  await page.goto('/dashboard');
  await page.waitForSelector('[data-testid="cockpit-trigger"]');
  await page.waitForFunction(() => (window as any).Alpine !== undefined);

  // filter({ visible }) rather than a `:visible` suffix: Tailwind scans this
  // directory, and the suffixed form makes it emit an arbitrary-variant rule into
  // the committed stylesheet.
  //
  // The cockpit panel is excluded explicitly. Since item 7 it is teleported into
  // `.overlays` and matches `[aria-modal="true"]` like everything else there, so
  // without this the counts below would stop meaning "the *other* dialog".
  const modals = page
    .locator('.overlays [aria-modal="true"]:not([data-testid="cockpit-panel"])')
    .filter({ visible: true });
  const panel = page.locator('[data-testid="cockpit-panel"]');

  await page.evaluate(() => {
    (window as any).Alpine.store('entityPicker').open({
      entityType: 'note', existingIds: [], multiSelect: false, onConfirm: () => {},
    });
  });
  await expect(modals).toHaveCount(1);
  await expect
    .poll(() => page.evaluate(
      () => document.activeElement?.closest('.overlays [aria-modal="true"]') !== null))
    .toBe(true);

  await page.keyboard.press('ControlOrMeta+Shift+KeyD');
  // Long enough for the panel's x-if to have rendered had it been going to.
  await page.waitForTimeout(300);

  await expect(panel, 'the panel opened behind a modal that paints over it').toHaveCount(0);
  await expect(modals, 'a second aria-modal dialog is open at the same time').toHaveCount(1);
  expect(
    await page.evaluate(
      () => document.activeElement?.closest('.overlays [aria-modal="true"]') !== null),
    'focus left the dialog the reader is in',
  ).toBe(true);

  // The positive control: with the modal closed, the very same key opens the panel.
  // Without it, a guard that simply broke the shortcut would pass everything above.
  await page.keyboard.press('Escape');
  await expect(modals).toHaveCount(0);
  await page.keyboard.press('ControlOrMeta+Shift+KeyD');
  await expect(panel).toBeVisible();
});

/**
 * Deferred-work item 7: the panel is teleported into `.overlays`.
 *
 * Round 2 diagnosed the cause and round 3 removed only the consequence. `.header`
 * is `position: sticky; z-index: 40`, so it is a stacking context: the panel's
 * `z-[60]` ordered it against the settings and account dropdowns and against
 * nothing else. The number was meaningless outside the header — raise a dropdown
 * above 60 and the panel goes under an aria-modal dialog again, with no test
 * failing, because the app's real overlay ordering lives in `.overlays` (z-index
 * 41, root stacking context) and the panel was not part of it.
 *
 * `x-teleport` moves the panel into that layer, where its z-index means what every
 * other overlay's z-index means. Nothing about paint order changes today, which is
 * the point: the invariant stops depending on the header's internal numbering.
 *
 * The wrap direction is load-bearing and fails silently if reversed. `x-if` must be
 * the outer template: with `x-teleport` outside, x-if inserts its clone with
 * `el.after(clone)` — a *sibling* of the teleported node — so Alpine's
 * `_x_teleportBack` hop is never taken, `closestRoot` finds no `[x-data]`, and
 * `x-ref="panel"` never registers. `focusFirstIn(this.$refs.panel)` would then do
 * nothing, with no error. Hence the focus assertion below.
 */
test('item 7 — the open panel lives in the .overlays layer, not in the header', async ({ page }) => {
  await page.goto('/dashboard');
  await page.waitForSelector('[data-testid="cockpit-trigger"]');

  // The trigger stays where findings 83/102 put it: header chrome.
  const triggerInHeader = await page
    .locator('[data-testid="cockpit-trigger"]')
    .evaluate((el) => el.closest('header') !== null);
  expect(triggerInHeader, 'the trigger must remain in the header').toBe(true);

  await page.locator('[data-testid="cockpit-trigger"]').click();
  const panel = page.locator('[data-testid="cockpit-panel"]');
  await expect(panel).toBeVisible();

  const placement = await panel.evaluate((el) => {
    const overlay = el.closest('[data-testid="cockpit-overlay"]');
    const overlays = document.querySelector('.overlays');
    return {
      inOverlays: el.closest('.overlays') !== null,
      inHeader: el.closest('header') !== null,
      // The teleported node must be a *direct* child of .overlays: `.overlays > *`
      // is what restores pointer-events on the pointer-events:none layer.
      overlayIsDirectChild: !!overlay && overlay.parentElement === overlays,
      overlayZ: overlay ? getComputedStyle(overlay).zIndex : null,
      // Every other overlay layer must still stack above it.
      //
      // "Layer" means a `position: fixed` element inside `.overlays`, not every
      // descendant with a z-index. Two of the six overlays put their z-index on a
      // descendant of a wrapper with no z-index (pluginActionModal's `.plugin-action-overlay`
      // at 60, confirmDialog's inner div at 50), so direct children alone would miss
      // them. But the lightbox also has a `sticky top-0 z-10` toolbar and an
      // crop overlay at z-index 40, both sealed inside the lightbox's own
      // stacking context — sweeping all descendants compares against numbers that
      // mean nothing here. `position: fixed` is what distinguishes a modal layer from
      // chrome inside one.
      siblingZ: overlays
        ? [...overlays.children]
            .filter((c) => c !== overlay)
            .flatMap((c) => [c, ...c.querySelectorAll('*')])
            .filter((c) => getComputedStyle(c).position === 'fixed')
            .map((c) => parseInt(getComputedStyle(c).zIndex, 10))
            .filter((z) => Number.isFinite(z))
        : [],
    };
  });

  expect(placement.inOverlays, 'the panel must be teleported into .overlays').toBe(true);
  expect(placement.inHeader, 'the panel must no longer paint inside the header stacking context').toBe(false);
  expect(placement.overlayIsDirectChild, 'the teleported overlay must be a direct child of .overlays').toBe(true);

  const z = parseInt(placement.overlayZ ?? '', 10);
  expect(Number.isFinite(z), `the teleported overlay needs a numeric z-index, got ${placement.overlayZ}`).toBe(true);
  for (const siblingZ of placement.siblingZ) {
    expect(
      z,
      `the jobs panel at z-index ${z} must stay below every other overlay (found ${siblingZ})`,
    ).toBeLessThan(siblingZ);
  }

  // x-ref still resolves through the teleport, which is what moves focus in. If the
  // templates were nested the wrong way round this is the only thing that notices.
  const focusInPanel = await page.evaluate(
    () => document.activeElement?.closest('[data-testid="cockpit-panel"]') !== null,
  );
  expect(focusInPanel, 'focus was not moved into the teleported panel — check the x-if/x-teleport nesting').toBe(true);

  // And the round-3 behaviour survives: Escape closes, focus returns to the trigger.
  await page.keyboard.press('Escape');
  await expect(panel).toHaveCount(0);
  await expect(page.locator('[data-testid="cockpit-trigger"]')).toBeFocused();
});
