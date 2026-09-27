import http from 'node:http';
import type { AddressInfo } from 'node:net';
import { test, expect } from '../../fixtures/base.fixture';
import type { Page, Route } from '@playwright/test';

// The drawer and the detail page read their Jobs from the canonical API. These
// tests answer those reads themselves, so each one controls exactly which rows
// exist and what a command answers.

type MockJob = Record<string, any>;

function failedJob(id: string, title: string, extra: MockJob = {}): MockJob {
  return {
    id, title, kind: 'remote-download', state: 'failed', version: 2,
    acceptedAt: extra.acceptedAt || '2026-09-26T10:00:00Z',
    failure: { message: 'connection refused' },
    commands: [
      { key: 'retry', label: 'Retry', endpoint: `/v1/jobs/${id}/commands/retry`, jobVersion: 2 },
      { key: 'dismiss', label: 'Dismiss', endpoint: `/v1/jobs/${id}/commands/dismiss`, jobVersion: 2, bulk: true },
    ],
    outputs: [], lineage: { ancestors: [], successors: [], parents: [], children: [] },
    ...extra,
  };
}

function runningJob(id: string, title: string, extra: MockJob = {}): MockJob {
  return {
    id, title, kind: 'remote-download', state: 'running', version: 2, phase: 'downloading',
    acceptedAt: extra.acceptedAt || '2026-09-26T10:00:00Z',
    progress: { completed: 10, total: 100, unit: 'bytes' },
    commands: [
      { key: 'cancel', label: 'Cancel', endpoint: `/v1/jobs/${id}/commands/cancel`, jobVersion: 2, confirmation: 'Stop this download? A file already saved stays in the library.' },
    ],
    outputs: [], lineage: { ancestors: [], successors: [], parents: [], children: [] },
    ...extra,
  };
}

// Answers the drawer's three group reads from `rows()`, by state, and each
// Job's detail read from the same rows.
async function serveJobs(page: Page, rows: () => MockJob[]) {
  await page.route(/\/v1\/jobs(?:\?.*)?$/, route => {
    const states = new URL(route.request().url()).searchParams.getAll('state');
    return route.fulfill({ json: { jobs: rows().filter(job => states.includes(job.state)), nextCursor: null } });
  });
  await page.route(/\/v1\/jobs\/[^/?]+$/, (route: Route) => {
    const id = decodeURIComponent(new URL(route.request().url()).pathname.split('/').pop() || '');
    const job = rows().find(row => row.id === id);
    return job ? route.fulfill({ json: job }) : route.fulfill({ status: 404, json: { error: 'job not found' } });
  });
}

async function openDrawer(page: Page) {
  await page.keyboard.press('Control+Shift+D');
  const drawer = page.getByRole('dialog', { name: 'Jobs' });
  await expect(drawer).toBeVisible();
  // Focus moves in a tick after the drawer mounts; keys pressed before that
  // still go to the page.
  await expect.poll(() => page.evaluate(() => !!document.activeElement?.closest('#job-center-panel'))).toBe(true);
  return drawer;
}

test.describe('Jobs drawer focus return', () => {
  test('closing a drawer opened by the shortcut returns focus to the field the reader was typing in', async ({ page }) => {
    await page.goto('/note/new');
    const title = page.locator('input#Name');
    await title.click();
    await page.keyboard.type('ab');

    await openDrawer(page);
    await page.keyboard.press('Escape');
    await expect(page.getByRole('dialog', { name: 'Jobs' })).toHaveCount(0);

    await expect(title).toBeFocused();
    await page.keyboard.type('cd');
    await expect(title).toHaveValue('abcd');
  });

  test('toggling the drawer shut with the shortcut returns focus to the field too', async ({ page }) => {
    await page.goto('/note/new');
    const title = page.locator('input#Name');
    await title.click();

    await openDrawer(page);
    await page.keyboard.press('Control+Shift+D');
    await expect(page.getByRole('dialog', { name: 'Jobs' })).toHaveCount(0);
    await expect(title).toBeFocused();
  });
});

test.describe('Jobs drawer focus visibility', () => {
  test('Shift+Tab never leaves the focused control behind a sticky group heading', async ({ page }) => {
    const rows = Array.from({ length: 24 }, (_, index) => failedJob(`sticky-${index}`, `Sticky heading job ${index}`, {
      acceptedAt: `2026-09-26T10:${String(59 - index).padStart(2, '0')}:00Z`,
    }));
    await serveJobs(page, () => rows);
    await page.setViewportSize({ width: 1280, height: 700 });
    await page.goto('/dashboard');
    const drawer = await openDrawer(page);
    await expect(drawer.locator('article[data-job-id]')).toHaveCount(rows.length);

    for (let step = 0; step < 50; step++) await page.keyboard.press('Tab');
    const obscured: string[] = [];
    for (let step = 0; step < 40; step++) {
      await page.keyboard.press('Shift+Tab');
      const overlap = await page.evaluate(() => {
        const active = document.activeElement as HTMLElement | null;
        if (!active || !active.closest('#job-center-panel article')) return null;
        const box = active.getBoundingClientRect();
        let worst = 0;
        for (const heading of document.querySelectorAll('#job-center-panel h3')) {
          const cover = heading.getBoundingClientRect();
          const covered = Math.max(0, Math.min(box.bottom, cover.bottom) - Math.max(box.top, cover.top));
          if (box.height > 0) worst = Math.max(worst, covered / box.height);
        }
        return { worst, label: active.textContent?.trim() || active.tagName };
      });
      if (overlap && overlap.worst > 0) obscured.push(`${overlap.label} ${Math.round(overlap.worst * 100)}%`);
    }
    expect(obscured).toEqual([]);
  });
});

test.describe('Job Center focus visibility', () => {
  test('Shift+Tab on /jobs never leaves the focused control under the sticky site header', async ({ page, request }) => {
    const server = http.createServer((_request, response) => {
      response.writeHead(404, { 'Content-Type': 'text/plain' });
      response.end('gone');
    });
    await new Promise<void>(resolve => server.listen(0, '127.0.0.1', resolve));
    try {
      const base = `http://127.0.0.1:${(server.address() as AddressInfo).port}`;
      const stamp = Date.now();
      for (let index = 0; index < 14; index++) {
        const name = `header-focus-${stamp}-${index}.bin`;
        const response = await request.post('/v1/download/submit', { data: { URL: `${base}/${name}`, Name: name, FileName: name } });
        expect(response.status()).toBe(202);
      }
      await page.setViewportSize({ width: 1280, height: 700 });
      await page.goto(`/jobs?dismissed=false&search=header-focus-${stamp}`);
      await expect(page.locator('[data-job-id]')).toHaveCount(14);

      // The browser scrolls a control Shift+Tab reaches only when it is out of
      // view. One that sits in the viewport's top edge, under the header, is
      // in view as far as the viewport is concerned, so each card's controls
      // are put there before Shift+Tab reaches them.
      const obscured: string[] = [];
      for (const index of [3, 7, 11]) {
        const card = page.locator('[data-job-id]').nth(index);
        for (const control of [card.locator('a[href]').first(), card.locator('input[type="checkbox"]').first()]) {
          const overlap = await control.evaluate(target => {
            const focusable = [...document.querySelectorAll<HTMLElement>('a[href], button, input, select, summary, textarea')]
              .filter(element => element.offsetParent !== null && element.tabIndex >= 0);
            const next = focusable[focusable.indexOf(target as HTMLElement) + 1];
            window.scrollTo(0, window.scrollY + target.getBoundingClientRect().top - 8);
            next.focus({ preventScroll: true });
            return null;
          });
          expect(overlap).toBeNull();
          await page.keyboard.press('Shift+Tab');
          await expect(control).toBeFocused();
          const covered = await control.evaluate(target => {
            const box = target.getBoundingClientRect();
            const cover = document.querySelector('header.header')!.getBoundingClientRect();
            return Math.max(0, Math.min(box.bottom, cover.bottom) - Math.max(box.top, cover.top)) / box.height;
          });
          if (covered > 0) obscured.push(`card ${index} ${await control.evaluate(target => target.tagName)} ${Math.round(covered * 100)}%`);
        }
      }
      expect(obscured).toEqual([]);
    } finally {
      server.close();
    }
  });
});

test.describe('Job command confirmations', () => {
  test('the Cancel confirmation names its two choices differently in the drawer', async ({ page }) => {
    const rows = [runningJob('confirm-cancel-drawer', 'Cancel names download')];
    await serveJobs(page, () => rows);
    await page.goto('/dashboard');
    const drawer = await openDrawer(page);
    await drawer.locator('article[data-job-id="confirm-cancel-drawer"]').getByRole('button', { name: 'Cancel', exact: true }).click();

    const confirmation = page.getByRole('alertdialog');
    await expect(confirmation.getByRole('button')).toHaveText(['Go back', 'Cancel']);
    await expect(confirmation.getByRole('button', { name: 'Go back', exact: true })).toBeFocused();
    await page.keyboard.press('Escape');
  });

  test('the Cancel confirmation names its two choices differently on the detail page', async ({ page }) => {
    const rows = [runningJob('confirm-cancel-detail', 'Cancel names detail')];
    await serveJobs(page, () => rows);
    await page.route('**/v1/jobs/confirm-cancel-detail/events*', route => route.fulfill({ json: { events: [] } }));
    await page.goto('/job?id=confirm-cancel-detail');
    await page.getByRole('group', { name: 'Advertised job commands' }).getByRole('button', { name: 'Cancel', exact: true }).click();

    const confirmation = page.getByRole('alertdialog');
    await expect(confirmation.getByRole('button')).toHaveText(['Go back', 'Cancel']);
    await page.keyboard.press('Escape');
  });
});
