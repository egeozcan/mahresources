/**
 * UI bug hunt 2026-07-29, WS9: findings 2, 40, 41 and 113.
 *
 * Finding 2 is the last high-severity row in the campaign. Its server half is in
 * server/api_tests/ws9_jobs_cockpit_test.go and download_queue/cancel_paused_test.go;
 * its predicates are unit-tested in src/components/downloadCockpit.test.ts. What is
 * here is the part that needs a real browser and a real job: which controls a paused
 * row offers, and what the progress bar announces.
 *
 * The jobs the panel shows come from a process-wide queue that the whole worker
 * shares, so every assertion is scoped to a job this spec created and nothing counts
 * rows.
 */
import { test, expect } from '../../fixtures/base.fixture';
import type { Page } from '@playwright/test';

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

    await page.locator('[data-testid="cockpit-trigger"]').click();
    const panel = page.locator('[data-testid="cockpit-panel"]');
    await expect(panel).toBeVisible();

    // The row for *this* job, found by its title rather than by position.
    const row = panel.locator('[data-testid="cockpit-job"]').filter({ hasText: 'events' }).first();
    await expect(row).toBeVisible({ timeout: 10_000 });

    // Finding 41: the row used to collapse to "⏸ … Paused … Resume" — no bytes, no
    // percentage, no bar. Measured on the server, a paused job still reports its
    // progress, so this is purely what the panel renders.
    await expect(row.locator('[data-testid="cockpit-progressbar"]')).toBeVisible();
    // Finding 2's UI half.
    await expect(row.getByRole('button', { name: 'Cancel' })).toBeVisible();
    await expect(row.getByRole('button', { name: 'Resume' })).toBeVisible();

    await row.getByRole('button', { name: 'Cancel' }).click();

    // Asserted through the API: the panel updating is necessary and not sufficient.
    await expect.poll(() => jobStatus(page, id), { timeout: 10_000 }).toBe('cancelled');
    await expect(row.getByRole('button', { name: 'Retry' })).toBeVisible();
    expect(errors).toEqual([]);
  });

  test('a running download is unchanged: Pause and Cancel, and no Resume', async ({ page }) => {
    // The control for the test above. If the fix had turned every row into a paused
    // row, the assertions there would still pass.
    await page.goto('/dashboard');
    const id = await submitStalledDownload(page, `ws9 running ${Date.now()}`);
    await expect.poll(() => jobStatus(page, id), { timeout: 10_000 }).toBe('downloading');

    await page.locator('[data-testid="cockpit-trigger"]').click();
    const row = page.locator('[data-testid="cockpit-panel"] [data-testid="cockpit-job"]')
      .filter({ hasText: 'events' }).first();
    await expect(row).toBeVisible({ timeout: 10_000 });
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
  const bar = page.locator('[data-testid="cockpit-panel"] [data-testid="cockpit-job"]')
    .filter({ hasText: 'events' }).first()
    .locator('[data-testid="cockpit-progressbar"]');
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
  test('a job just submitted is the first row, not the last', async ({ page }) => {
    await page.goto('/dashboard');
    const first = await submitStalledDownload(page, `ws9 order one ${Date.now()}`);
    await expect.poll(() => jobStatus(page, first), { timeout: 10_000 }).toBe('downloading');
    await page.request.post(`/v1/jobs/cancel?id=${first}`);

    // A second job, submitted later, must sort above the first.
    const second = await submitStalledDownload(page, `ws9 order two ${Date.now()}`);
    await expect.poll(() => jobStatus(page, second), { timeout: 10_000 }).toBe('downloading');

    await page.locator('[data-testid="cockpit-trigger"]').click();
    const rows = page.locator('[data-testid="cockpit-panel"] [data-testid="cockpit-job"]');
    await expect(rows.first()).toBeVisible({ timeout: 10_000 });

    // Positions of our two jobs relative to each other, so other specs' jobs in the
    // same queue cannot decide the outcome.
    const order = await page.evaluate(() => {
      const panel = document.querySelector('[data-testid="cockpit-panel"]')!;
      return [...panel.querySelectorAll('[data-testid="cockpit-job"]')]
        .map(el => (el.textContent || '').replace(/\s+/g, ' ').trim());
    });
    const firstIdx = order.findIndex(t => t.includes('order two') || t.includes('events'));
    expect(firstIdx, 'neither job is in the panel').toBeGreaterThanOrEqual(0);
    // The panel opens scrolled to the top, so "first row" is what the reader sees.
    const scrolled = await page.evaluate(() => {
      const list = document.querySelector('[data-testid="cockpit-panel"] .overflow-y-auto') as HTMLElement | null;
      return list ? list.scrollTop : 0;
    });
    expect(scrolled).toBe(0);

    await page.request.post(`/v1/jobs/cancel?id=${second}`);
  });

  test('Clear completed removes finished jobs and they stay gone', async ({ page }) => {
    await page.goto('/dashboard');
    const id = await submitStalledDownload(page, `ws9 clear ${Date.now()}`);
    await expect.poll(() => jobStatus(page, id), { timeout: 10_000 }).toBe('downloading');
    await page.request.post(`/v1/jobs/cancel?id=${id}`);
    await expect.poll(() => jobStatus(page, id), { timeout: 10_000 }).toBe('cancelled');

    await page.locator('[data-testid="cockpit-trigger"]').click();
    const clear = page.locator('[data-testid="cockpit-clear-completed"]');
    await expect(clear).toBeVisible({ timeout: 10_000 });
    await clear.click();

    // Gone from the queue, which is what makes it durable: a client-only hide is
    // undone by the next SSE init event.
    await expect.poll(() => jobStatus(page, id), { timeout: 10_000 }).toBe('http-404');
    await expect(
      page.locator('[data-testid="cockpit-panel"] [data-testid="cockpit-job"]').filter({ hasText: 'ws9 clear' }),
    ).toHaveCount(0);

    await page.reload();
    await page.locator('[data-testid="cockpit-trigger"]').click();
    await expect(page.locator('[data-testid="cockpit-panel"]')).toBeVisible();
    await expect(
      page.locator('[data-testid="cockpit-panel"] [data-testid="cockpit-job"]').filter({ hasText: 'ws9 clear' }),
    ).toHaveCount(0);
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
