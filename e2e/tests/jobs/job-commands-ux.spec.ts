import http from 'node:http';
import type { AddressInfo } from 'node:net';
import { test, expect } from '../../fixtures/base.fixture';
import { gotoMockedJobPage } from '../../helpers/job-page';
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
    await gotoMockedJobPage(page, 'confirm-cancel-detail');
    await page.getByRole('group', { name: 'Job actions' }).getByRole('button', { name: 'Cancel', exact: true }).click();

    const confirmation = page.getByRole('alertdialog');
    await expect(confirmation.getByRole('button')).toHaveText(['Go back', 'Cancel']);
    await page.keyboard.press('Escape');
  });
});

// A mutable set of Jobs the mocked API answers from, with each command's
// answer decided by the test.
function jobStore(initial: MockJob[]) {
  const jobs = new Map(initial.map(job => [job.id, job]));
  return {
    jobs,
    rows: () => [...jobs.values()].filter(job => !job.dismissed),
    set(id: string, patch: MockJob) { jobs.set(id, { ...jobs.get(id), ...patch }); },
  };
}

async function serveStore(page: Page, store: ReturnType<typeof jobStore>) {
  await page.route(/\/v1\/jobs(?:\?.*)?$/, route => {
    const params = new URL(route.request().url()).searchParams;
    const states = params.getAll('state');
    const listed = params.get('dismissed') === 'false' ? store.rows() : [...store.jobs.values()];
    return route.fulfill({ json: { jobs: listed.filter(job => states.includes(job.state)), nextCursor: null } });
  });
  await page.route(/\/v1\/jobs\/[^/?]+$/, route => {
    const id = decodeURIComponent(new URL(route.request().url()).pathname.split('/').pop() || '');
    const job = store.jobs.get(id);
    return job ? route.fulfill({ json: job }) : route.fulfill({ status: 404, json: { error: 'job not found' } });
  });
  await page.route(/\/v1\/jobs\/[^/?]+\/events/, route => route.fulfill({ json: { events: [] } }));
}

function withCommands(job: MockJob, keys: Array<[string, string, MockJob?]>) {
  return {
    ...job,
    commands: keys.map(([key, label, extra]) => ({ key, label, endpoint: `/v1/jobs/${job.id}/commands/${key}`, jobVersion: job.version, ...(extra || {}) })),
  };
}

test.describe('Job commands keep their controls current', () => {
  test('Forget in the drawer takes Retry and Forget away, and a refused command says the Kind\'s reason', async ({ page }) => {
    const store = jobStore([withCommands(failedJob('stale-drawer', 'Stale drawer job'), [
      ['retry', 'Retry'], ['dismiss', 'Dismiss', { bulk: true }], ['forget', 'Forget replay input', { destructive: true }],
    ])]);
    await serveStore(page, store);
    await page.route('**/v1/jobs/stale-drawer/commands/forget', route => {
      store.set('stale-drawer', withCommands(store.jobs.get('stale-drawer')!, [['dismiss', 'Dismiss', { bulk: true }]]));
      return route.fulfill({ json: { jobId: 'stale-drawer', key: 'forget', status: 'succeeded', code: 'applied', message: 'replay input forgotten' } });
    });

    await page.goto('/dashboard');
    const drawer = await openDrawer(page);
    const row = drawer.locator('article[data-job-id="stale-drawer"]');
    await row.locator('summary').click();
    // Forget cannot be undone: it asks, and its button reads as destructive.
    const forget = row.getByRole('button', { name: 'Forget replay input' });
    await expect(forget).toHaveClass(/text-red-700/);
    await forget.click();
    await page.getByRole('alertdialog').getByRole('button', { name: 'Forget replay input' }).click();

    await expect(drawer.locator('[data-job-panel-notice]')).toHaveText('Stale drawer job: replay input forgotten.');
    await expect(row.getByRole('button', { name: 'Retry', exact: true })).toHaveCount(0);
    await expect(row.getByRole('button', { name: 'Forget replay input' })).toHaveCount(0);
  });

  test('a refusal on the detail page says the Kind\'s own reason and replaces the controls', async ({ page }) => {
    const store = jobStore([withCommands(failedJob('refused-detail', 'Refused detail job'), [['retry', 'Retry'], ['dismiss', 'Dismiss', { bulk: true }]])]);
    await serveStore(page, store);
    await page.route('**/v1/jobs/refused-detail/commands/retry', route => {
      store.set('refused-detail', withCommands(store.jobs.get('refused-detail')!, [['dismiss', 'Dismiss', { bulk: true }]]));
      return route.fulfill({ status: 409, json: {
        error: 'The group this download files into no longer exists.',
        result: { jobId: 'refused-detail', key: 'retry', status: 'failed', code: 'refused', message: 'The group this download files into no longer exists.' },
      } });
    });

    await gotoMockedJobPage(page, 'refused-detail');
    const commands = page.getByRole('group', { name: 'Job actions' });
    await commands.getByRole('button', { name: 'Retry', exact: true }).click();

    await expect(page.locator('[data-job-notice]')).toHaveText('Retry refused for Refused detail job: The group this download files into no longer exists.');
    await expect(commands.getByRole('button', { name: 'Retry', exact: true })).toHaveCount(0);
  });

  test('a double press of Resume on the detail page sends one command', async ({ page }) => {
    const blocked = { ...failedJob('double-resume', 'Double resume job'), state: 'blocked', failure: undefined };
    const store = jobStore([withCommands(blocked, [['resume', 'Resume'], ['cancel', 'Cancel', { destructive: true, confirmation: 'Stop this download?' }]])]);
    await serveStore(page, store);
    let posts = 0;
    let release = () => {};
    await page.route('**/v1/jobs/double-resume/commands/resume', async route => {
      posts += 1;
      await new Promise<void>(resolve => { release = resolve; });
      store.set('double-resume', withCommands({ ...store.jobs.get('double-resume')!, state: 'queued', version: 3 }, [['cancel', 'Cancel', { destructive: true, confirmation: 'Stop this download?' }]]));
      return route.fulfill({ json: { status: 'succeeded', code: 'applied', message: 'queued to start again', job: store.jobs.get('double-resume') } });
    });

    await gotoMockedJobPage(page, 'double-resume');
    const resume = page.getByRole('group', { name: 'Job actions' }).getByRole('button', { name: 'Resume', exact: true });
    await resume.dblclick();
    await expect.poll(() => posts).toBe(1);
    await expect(resume).toHaveAttribute('aria-disabled', 'true');
    release();
    await expect(resume).toHaveCount(0);
    expect(posts).toBe(1);
  });
});

test.describe('Job command answers in the drawer', () => {
  test('Retry leaves the page the drawer sits on, and offers its new job as a link', async ({ page }) => {
    const store = jobStore([withCommands(failedJob('retry-stays', 'Retry stays job'), [['retry', 'Retry'], ['dismiss', 'Dismiss', { bulk: true }]])]);
    await serveStore(page, store);
    await page.route('**/v1/jobs/retry-stays/commands/retry', route => {
      store.set('retry-stays', withCommands(store.jobs.get('retry-stays')!, [['dismiss', 'Dismiss', { bulk: true }]]));
      return route.fulfill({ json: { jobId: 'retry-stays', key: 'retry', status: 'succeeded', code: 'applied', message: 'a new job was created', successorId: 'retry-stays-2' } });
    });

    await page.goto('/note/new');
    await page.locator('input#Name').fill('Unsaved note title');
    const drawer = await openDrawer(page);
    await drawer.locator('article[data-job-id="retry-stays"]').getByRole('button', { name: 'Retry', exact: true }).click();

    const notice = drawer.locator('[data-job-panel-notice]');
    await expect(notice).toContainText('Retry started a new job for Retry stays job.');
    await expect(notice.getByRole('link', { name: 'Open the new job' })).toHaveAttribute('href', '/job?id=retry-stays-2');
    await page.waitForTimeout(300);
    expect(new URL(page.url()).pathname).toBe('/note/new');
    await expect(page.locator('input#Name')).toHaveValue('Unsaved note title');
  });

  test('a Cancel request names its job, and the box does not outlive the drawer', async ({ page }) => {
    const store = jobStore([runningJob('cancel-box', 'Cancel box download')]);
    await serveStore(page, store);
    await page.route('**/v1/jobs/cancel-box/commands/cancel', route => {
      store.set('cancel-box', { version: 3, controlIntent: 'cancel', phase: 'cancelling' });
      return route.fulfill({ json: { jobId: 'cancel-box', key: 'cancel', status: 'succeeded', code: 'requested', message: 'cancelling', job: store.jobs.get('cancel-box') } });
    });

    await page.goto('/dashboard');
    const drawer = await openDrawer(page);
    await drawer.locator('article[data-job-id="cancel-box"]').getByRole('button', { name: 'Cancel', exact: true }).click();
    await page.getByRole('alertdialog').getByRole('button', { name: 'Cancel', exact: true }).click();
    const notice = drawer.locator('[data-job-panel-notice]');
    await expect(notice).toHaveText('Cancel requested for Cancel box download.');

    // The executor acts: the row moves on, and the box leaves with it.
    store.set('cancel-box', { state: 'cancelled', version: 4, controlIntent: undefined, phase: undefined, commands: [] });
    await page.evaluate(snapshot => {
      const root = document.querySelector('[data-testid="job-panel-root"]');
      (window as any).Alpine.$data(root).applyStreamSnapshot(snapshot);
    }, store.jobs.get('cancel-box'));
    await expect(notice).toBeHidden();
  });

  test('a notice is gone when the drawer opens again', async ({ page }) => {
    const store = jobStore([withCommands(failedJob('forget-close', 'Forget close job'), [['forget', 'Forget replay input', { destructive: true }]])]);
    await serveStore(page, store);
    await page.route('**/v1/jobs/forget-close/commands/forget', route => route.fulfill({ json: { status: 'succeeded', code: 'applied', message: 'replay input forgotten' } }));

    await page.goto('/dashboard');
    let drawer = await openDrawer(page);
    const row = drawer.locator('article[data-job-id="forget-close"]');
    await row.locator('summary').click();
    await row.getByRole('button', { name: 'Forget replay input' }).click();
    await page.getByRole('alertdialog').getByRole('button', { name: 'Forget replay input' }).click();
    await expect(drawer.locator('[data-job-panel-notice]')).toBeVisible();

    await page.keyboard.press('Escape');
    await expect(drawer).toHaveCount(0);
    drawer = await openDrawer(page);
    await expect(drawer.locator('[data-job-panel-notice]')).toBeHidden();
  });

  test('a Dismiss can be undone from its box, and the row comes back', async ({ page }) => {
    const store = jobStore([withCommands(failedJob('undo-dismiss', 'Undo dismiss job'), [
      ['dismiss', 'Dismiss', { bulk: true }], ['undismiss', 'Undismiss', { bulk: true }],
    ])]);
    await serveStore(page, store);
    for (const key of ['dismiss', 'undismiss']) {
      await page.route(`**/v1/jobs/undo-dismiss/commands/${key}`, route => {
        store.set('undo-dismiss', { dismissed: key === 'dismiss' });
        return route.fulfill({ json: { status: 'succeeded', code: 'applied', message: `${key}ed`, job: store.jobs.get('undo-dismiss') } });
      });
    }

    await page.goto('/dashboard');
    const drawer = await openDrawer(page);
    const row = drawer.locator('article[data-job-id="undo-dismiss"]');
    await row.getByRole('button', { name: 'Dismiss', exact: true }).click();
    await expect(row).toHaveCount(0);
    const undo = drawer.locator('[data-job-panel-notice]').getByRole('button', { name: 'Undo' });
    await undo.focus();
    await page.keyboard.press('Enter');

    await expect(row).toHaveCount(1);
    await expect(drawer.locator('[data-job-panel-notice]')).toBeHidden();
    // Focus lands on the returned row rather than falling out of the drawer.
    await expect(row.getByRole('button', { name: 'Dismiss', exact: true })).toBeFocused();
  });
});

test.describe('Jobs drawer keeps focus when its box clears', () => {
  test('focus on Undo moves to Reload page when the drawer stops', async ({ page }) => {
    const store = jobStore([withCommands(failedJob('undo-reset', 'Undo reset job'), [
      ['dismiss', 'Dismiss', { bulk: true }], ['undismiss', 'Undismiss', { bulk: true }],
    ])]);
    await serveStore(page, store);
    await page.route('**/v1/jobs/undo-reset/commands/dismiss', route => {
      store.set('undo-reset', { dismissed: true });
      return route.fulfill({ json: { status: 'succeeded', code: 'applied', message: 'dismissed', job: store.jobs.get('undo-reset') } });
    });

    await page.goto('/dashboard');
    const drawer = await openDrawer(page);
    await drawer.locator('article[data-job-id="undo-reset"]').getByRole('button', { name: 'Dismiss', exact: true }).click();
    // The Dismiss places focus once its refresh settles: with no row left, on All jobs.
    await expect(drawer.getByRole('link', { name: 'All jobs', exact: true })).toBeFocused();
    const undo = drawer.locator('[data-job-panel-notice]').getByRole('button', { name: 'Undo' });
    await undo.focus();

    // The database behind the drawer was restored: it stops and clears the box.
    await page.evaluate(() => {
      const root = document.querySelector('[data-testid="job-panel-root"]');
      (window as any).Alpine.$data(root).stopForStreamReset();
    });

    await expect(undo).toHaveCount(0);
    await expect(drawer.getByRole('button', { name: 'Reload page' })).toBeFocused();
  });
});

test.describe('Job detail page', () => {
  test('a dismissed Job says so and offers Undismiss instead of Dismiss', async ({ page }) => {
    const store = jobStore([withCommands({ ...failedJob('detail-dismissed', 'Detail dismissed job'), dismissed: true }, [
      ['retry', 'Retry'], ['dismiss', 'Dismiss', { bulk: true }], ['undismiss', 'Undismiss', { bulk: true }],
    ])]);
    await serveStore(page, store);
    await page.route('**/v1/jobs/detail-dismissed/commands/undismiss', route => {
      store.set('detail-dismissed', { dismissed: false });
      return route.fulfill({ json: { status: 'succeeded', code: 'applied', message: 'undismissed', job: store.jobs.get('detail-dismissed') } });
    });

    await gotoMockedJobPage(page, 'detail-dismissed');
    const commands = page.getByRole('group', { name: 'Job actions' });
    await expect(page.getByText('Dismissed by you', { exact: true })).toBeVisible();
    await expect(commands.getByRole('button', { name: 'Dismiss', exact: true })).toHaveCount(0);
    await commands.getByRole('button', { name: 'Undismiss', exact: true }).click();

    await expect(page.getByText('Dismissed by you', { exact: true })).toBeHidden();
    await expect(commands.getByRole('button', { name: 'Dismiss', exact: true })).toBeVisible();
  });

  test('a cancelled Job shows no phase it no longer has', async ({ page }) => {
    const store = jobStore([runningJob('detail-phase', 'Detail phase download')]);
    await serveStore(page, store);
    await page.route('**/v1/jobs/detail-phase/commands/cancel', route => {
      store.set('detail-phase', { version: 3, phase: 'cancelling', controlIntent: 'cancel' });
      return route.fulfill({ json: { status: 'succeeded', code: 'requested', message: 'cancelling', job: store.jobs.get('detail-phase') } });
    });

    await gotoMockedJobPage(page, 'detail-phase');
    await page.getByRole('group', { name: 'Job actions' }).getByRole('button', { name: 'Cancel', exact: true }).click();
    await page.getByRole('alertdialog').getByRole('button', { name: 'Cancel', exact: true }).click();
    await expect(page.locator('[data-job-notice]')).toHaveText('Cancel requested for Detail phase download.');

    const { phase: _phase, controlIntent: _intent, ...cancelled } = { ...store.jobs.get('detail-phase')!, state: 'cancelled', version: 4, commands: [] };
    await page.evaluate(snapshot => {
      const root = document.querySelector('[data-testid="job-detail"]');
      (window as any).Alpine.$data(root).applyStreamSnapshot(snapshot);
    }, cancelled);

    const header = page.locator('[data-testid="job-detail"] header');
    await expect(header).toContainText('Cancelled');
    await expect(header).not.toContainText('cancelling');
    await expect(page.locator('[data-job-notice]')).toBeHidden();
  });
});

async function refreshDrawer(page: Page) {
  await page.evaluate(() => {
    const root = document.querySelector('[data-testid="job-panel-root"]');
    return (window as any).Alpine.$data(root).refresh();
  });
}

test.describe('Jobs drawer keeps focus on the row the reader is on', () => {
  function manyRows() {
    // Enough finished rows that the Finished group starts below the fold.
    return Array.from({ length: 12 }, (_, index) => ({
      ...failedJob(`focus-filler-${index}`, `Filler ${index}`, { acceptedAt: `2026-09-26T09:${String(10 + index).padStart(2, '0')}:00Z` }),
      state: 'succeeded', failure: undefined,
    }));
  }

  for (const start of ['title', 'Cancel'] as const) {
    test(`a row that finishes while focus is on its ${start} keeps focus on that row, in its new group`, async ({ page }) => {
      const store = jobStore([runningJob('focus-follow', 'Focus follows download', { acceptedAt: '2026-09-26T11:00:00Z' }), ...manyRows()]);
      await serveStore(page, store);
      await page.setViewportSize({ width: 1280, height: 700 });
      await page.goto('/dashboard');
      const drawer = await openDrawer(page);
      const row = drawer.locator('article[data-job-id="focus-follow"]');
      const control = start === 'title'
        ? row.getByRole('link', { name: 'Focus follows download' })
        : row.getByRole('button', { name: 'Cancel', exact: true });
      await control.focus();

      store.set('focus-follow', { state: 'succeeded', version: 3, phase: undefined, progress: { completed: 100, total: 100, unit: 'bytes' }, commands: [
        { key: 'dismiss', label: 'Dismiss', endpoint: '/v1/jobs/focus-follow/commands/dismiss', jobVersion: 3, bulk: true },
      ] });
      await refreshDrawer(page);

      await expect(drawer.locator('[data-job-panel-group="finished"] article[data-job-id="focus-follow"]')).toHaveCount(1);
      const title = row.getByRole('link', { name: 'Focus follows download' });
      await expect(title).toBeFocused();
      await expect(title).toBeInViewport();
    });
  }

  test('a row that moves while another row\'s command is still running keeps focus too', async ({ page }) => {
    const store = jobStore([
      withCommands(failedJob('focus-a', 'Commanded row', { acceptedAt: '2026-09-26T10:02:00Z' }), [['forget', 'Forget replay input', { destructive: true }]]),
      runningJob('focus-b', 'Moving row', { acceptedAt: '2026-09-26T10:01:00Z' }),
    ]);
    await serveStore(page, store);
    let release = () => {};
    await page.route('**/v1/jobs/focus-a/commands/forget', async route => {
      await new Promise<void>(resolve => { release = resolve; });
      return route.fulfill({ json: { status: 'succeeded', code: 'applied', message: 'replay input forgotten' } });
    });

    await page.goto('/dashboard');
    const drawer = await openDrawer(page);
    const rowA = drawer.locator('article[data-job-id="focus-a"]');
    await rowA.locator('summary').click();
    await rowA.getByRole('button', { name: 'Forget replay input' }).click();
    await page.getByRole('alertdialog').getByRole('button', { name: 'Forget replay input' }).click();
    // The confirmation hands focus back to Forget once it has closed.
    await expect(rowA.getByRole('button', { name: 'Forget replay input' })).toBeFocused();

    // While A's command waits, the reader moves to B, and B finishes.
    const titleB = drawer.locator('article[data-job-id="focus-b"]').getByRole('link', { name: 'Moving row' });
    await titleB.focus();
    store.set('focus-b', { state: 'succeeded', version: 3, phase: undefined, progress: { completed: 100, total: 100, unit: 'bytes' }, commands: [] });
    await refreshDrawer(page);
    await expect(drawer.locator('[data-job-panel-group="finished"] article[data-job-id="focus-b"]')).toHaveCount(1);
    await expect(titleB).toBeFocused();
    release();
  });

  test('a Resume whose row moves on later keeps focus on that row', async ({ page }) => {
    const blocked = withCommands({ ...failedJob('focus-resume', 'Focus resume download'), state: 'blocked', failure: undefined }, [
      ['cancel', 'Cancel', { destructive: true, confirmation: 'Stop this download?' }], ['resume', 'Resume'],
    ]);
    const store = jobStore([blocked]);
    await serveStore(page, store);
    await page.route('**/v1/jobs/focus-resume/commands/resume', route => {
      store.set('focus-resume', withCommands({ ...store.jobs.get('focus-resume')!, state: 'queued', version: 3 }, [['cancel', 'Cancel', { destructive: true, confirmation: 'Stop this download?' }]]));
      return route.fulfill({ json: { status: 'succeeded', code: 'applied', message: 'queued to start again', job: store.jobs.get('focus-resume') } });
    });

    await page.goto('/dashboard');
    const drawer = await openDrawer(page);
    const row = drawer.locator('article[data-job-id="focus-resume"]');
    await row.getByRole('button', { name: 'Resume', exact: true }).focus();
    await page.keyboard.press('Enter');
    await expect(row.getByRole('button', { name: 'Resume', exact: true })).toHaveCount(0);
    await expect.poll(() => page.evaluate(() => document.activeElement?.closest('article')?.getAttribute('data-job-id'))).toBe('focus-resume');

    // Later the transfer fails: the row moves to Needs attention, and focus
    // stays on it.
    store.set('focus-resume', withCommands({ ...failedJob('focus-resume', 'Focus resume download'), version: 4 }, [['retry', 'Retry'], ['dismiss', 'Dismiss', { bulk: true }]]));
    await refreshDrawer(page);
    await expect(drawer.locator('[data-job-panel-group="attention"] article[data-job-id="focus-resume"]')).toHaveCount(1);
    await expect.poll(() => page.evaluate(() => document.activeElement?.closest('article')?.getAttribute('data-job-id'))).toBe('focus-resume');
    await expect(drawer.getByRole('button', { name: 'Close Jobs panel' })).not.toBeFocused();
  });
});

test.describe('Job pages keep focus on their commands', () => {
  test('Pin on the detail page hands focus to Unpin, and a Job that ends hands it to what it offers next', async ({ page }) => {
    const store = jobStore([withCommands(runningJob('detail-focus', 'Detail focus download'), [
      ['cancel', 'Cancel', { destructive: true, confirmation: 'Stop this download?' }], ['pin', 'Pin', { bulk: true }], ['unpin', 'Unpin', { bulk: true }],
    ])]);
    await serveStore(page, store);
    await page.route('**/v1/jobs/detail-focus/commands/pin', route => {
      store.set('detail-focus', { pinned: true });
      return route.fulfill({ json: { status: 'succeeded', code: 'applied', message: 'pinned', job: store.jobs.get('detail-focus') } });
    });

    await gotoMockedJobPage(page, 'detail-focus');
    const commands = page.getByRole('group', { name: 'Job actions' });
    await commands.getByRole('button', { name: 'Pin', exact: true }).focus();
    await page.keyboard.press('Enter');
    await expect(commands.getByRole('button', { name: 'Unpin', exact: true })).toBeFocused();

    // The download ends on its own: Cancel is no longer offered.
    await commands.getByRole('button', { name: 'Cancel', exact: true }).focus();
    const finished = withCommands({ ...store.jobs.get('detail-focus')!, state: 'succeeded', version: 3, phase: undefined }, [
      ['dismiss', 'Dismiss', { bulk: true }], ['pin', 'Pin', { bulk: true }], ['unpin', 'Unpin', { bulk: true }],
    ]);
    await page.evaluate(snapshot => {
      const root = document.querySelector('[data-testid="job-detail"]');
      (window as any).Alpine.$data(root).applyStreamSnapshot(snapshot);
    }, finished);
    await expect(commands.getByRole('button', { name: 'Cancel', exact: true })).toHaveCount(0);
    await expect(commands.getByRole('button', { name: 'Dismiss', exact: true })).toBeFocused();
  });

  test('the /jobs bulk bar keeps focus after Pin, and hands it to Select All once Dismiss empties the selection', async ({ page, request }) => {
    const server = http.createServer((_request, response) => {
      response.writeHead(404, { 'Content-Type': 'text/plain' });
      response.end('gone');
    });
    await new Promise<void>(resolve => server.listen(0, '127.0.0.1', resolve));
    try {
      const base = `http://127.0.0.1:${(server.address() as AddressInfo).port}`;
      const stamp = `bulk-focus-${Date.now()}`;
      const ids: string[] = [];
      for (const index of [0, 1]) {
        const name = `${stamp}-${index}.bin`;
        const submitted = await request.post('/v1/download/submit', { data: { URL: `${base}/${name}`, Name: name, FileName: name } });
        expect(submitted.status()).toBe(202);
        ids.push((await submitted.json()).jobs[0].canonicalJobId as string);
      }
      for (const id of ids) await expect.poll(async () => (await (await request.get(`/v1/jobs/${id}`)).json()).state, { timeout: 20_000 }).toBe('failed');
      const jobId = ids[0];

      await page.goto(`/jobs?dismissed=false&search=${encodeURIComponent(stamp)}`);
      await expect(page.locator('[data-job-id]')).toHaveCount(2);
      await page.locator(`[data-job-id="${jobId}"]`).getByRole('checkbox').check();
      const bulk = page.getByRole('group', { name: 'Commands for the selected jobs' });
      await bulk.getByRole('button', { name: 'Pin', exact: true }).focus();
      await page.keyboard.press('Enter');
      await expect(bulk.getByRole('button', { name: 'Unpin', exact: true })).toBeFocused({ timeout: 10_000 });

      await bulk.getByRole('button', { name: 'Dismiss', exact: true }).focus();
      await page.keyboard.press('Enter');
      await expect(page.locator(`[data-job-id="${jobId}"]`)).toHaveCount(0, { timeout: 10_000 });
      await expect(page.locator('[data-bulk-select-all]').first()).toBeFocused();
    } finally {
      server.close();
    }
  });
});

test.describe('Jobs drawer opened for a job just started', () => {
  for (const listed of [true, false]) {
    test(`shows that job, not the failures above it${listed ? '' : ', even before a list has it'}`, async ({ page }) => {
      const failures = Array.from({ length: 30 }, (_, index) => failedJob(`started-above-${index}`, `Old failure ${index}`, {
        acceptedAt: `2026-09-26T09:${String(10 + index).padStart(2, '0')}:00Z`,
      }));
      const started = runningJob('started-now', 'Group Sweep', { acceptedAt: '2026-09-26T08:00:00Z' });
      const store = jobStore([...failures, started]);
      // Unlisted, the job was accepted after the drawer's lists were read; the
      // next list read has it.
      let inLists = listed;
      await page.route(/\/v1\/jobs(?:\?.*)?$/, route => {
        const states = new URL(route.request().url()).searchParams.getAll('state');
        const rows = store.rows().filter(job => inLists || job.id !== 'started-now');
        return route.fulfill({ json: { jobs: rows.filter(job => states.includes(job.state)), nextCursor: null } });
      });
      await page.route(/\/v1\/jobs\/[^/?]+$/, route => {
        const id = decodeURIComponent(new URL(route.request().url()).pathname.split('/').pop() || '');
        if (id === 'started-now') inLists = true;
        const job = store.jobs.get(id);
        return job ? route.fulfill({ json: job }) : route.fulfill({ status: 404, json: { error: 'job not found' } });
      });
      await page.setViewportSize({ width: 1280, height: 800 });
      await page.goto('/dashboard');
      // The page's own lists were read before the job existed.
      await expect.poll(() => page.evaluate(() => {
        const root = document.querySelector('[data-testid="job-panel-root"]');
        return (window as any).Alpine.$data(root).jobs.length;
      })).toBe(listed ? 31 : 30);
      await page.evaluate(() => window.dispatchEvent(new CustomEvent('jobs-panel-open', { detail: { jobIds: ['started-now'] } })));

      const drawer = page.getByRole('dialog', { name: 'Jobs' });
      const title = drawer.locator('article[data-job-id="started-now"]').getByRole('link', { name: 'Group Sweep' });
      await expect(title).toBeFocused();
      await expect(title).toBeInViewport();
    });
  }
});

test.describe('Jobs drawer opened for a run that started many jobs', () => {
  test('shows one of them even when the first is past its group\'s cap', async ({ page }) => {
    // A bulk run started 60 jobs; the drawer lists the newest 50, so the first
    // one started is not among them.
    const started = Array.from({ length: 60 }, (_, index) => runningJob(`bulk-run-${index}`, `Bulk run ${index}`, {
      acceptedAt: `2026-09-26T10:${String(index).padStart(2, '0')}:00Z`,
    }));
    const store = jobStore(started);
    await serveStore(page, store);
    await page.setViewportSize({ width: 1280, height: 800 });
    await page.goto('/dashboard');
    await expect.poll(() => page.evaluate(() => {
      const root = document.querySelector('[data-testid="job-panel-root"]');
      return (window as any).Alpine.$data(root).jobs.length;
    })).toBe(50);
    await page.evaluate(ids => window.dispatchEvent(new CustomEvent('jobs-panel-open', { detail: { jobIds: ids } })), started.map(job => job.id));

    const focusedRow = () => page.evaluate(() => document.activeElement?.closest('article[data-job-id]')?.getAttribute('data-job-id') || '');
    await expect.poll(focusedRow).toMatch(/^bulk-run-/);
    // The topmost of them: the newest, which the group lists first.
    expect(await focusedRow()).toBe('bulk-run-59');
  });

  test('offers a link to the job when a full group leaves it out', async ({ page }) => {
    const newer = Array.from({ length: 12 }, (_, index) => ({
      ...failedJob(`newer-finished-${index}`, `Newer finished ${index}`, { acceptedAt: `2026-09-26T11:${String(10 + index).padStart(2, '0')}:00Z` }),
      state: 'succeeded', failure: undefined,
    }));
    // Finished before the drawer read it, and older than every finished row it shows.
    const quick = { ...failedJob('quick-run', 'Quick run', { acceptedAt: '2026-09-26T09:00:00Z' }), state: 'succeeded', failure: undefined };
    const store = jobStore([...newer, quick]);
    await page.route(/\/v1\/jobs(?:\?.*)?$/, route => {
      const states = new URL(route.request().url()).searchParams.getAll('state');
      return route.fulfill({ json: { jobs: newer.filter(job => states.includes(job.state)), nextCursor: null } });
    });
    await page.route(/\/v1\/jobs\/[^/?]+$/, route => {
      const id = decodeURIComponent(new URL(route.request().url()).pathname.split('/').pop() || '');
      const job = store.jobs.get(id);
      return job ? route.fulfill({ json: job }) : route.fulfill({ status: 404, json: { error: 'job not found' } });
    });
    await page.goto('/dashboard');
    await expect.poll(() => page.evaluate(() => {
      const root = document.querySelector('[data-testid="job-panel-root"]');
      return (window as any).Alpine.$data(root).jobs.length;
    })).toBe(10);
    await page.evaluate(() => window.dispatchEvent(new CustomEvent('jobs-panel-open', { detail: { jobIds: ['quick-run'] } })));

    const notice = page.getByRole('dialog', { name: 'Jobs' }).locator('[data-job-panel-notice]');
    await expect(notice).toContainText('Quick run started.');
    await expect(notice.getByRole('link', { name: 'Open the job' })).toHaveAttribute('href', '/job?id=quick-run');
  });
});

test.describe('Needs attention', () => {
  test('a failure somebody retried leaves Needs attention; its retry is what the drawer lists', async ({ page, request }) => {
    const server = http.createServer((_request, response) => {
      response.writeHead(404, { 'Content-Type': 'text/plain' });
      response.end('gone');
    });
    await new Promise<void>(resolve => server.listen(0, '127.0.0.1', resolve));
    try {
      const base = `http://127.0.0.1:${(server.address() as AddressInfo).port}`;
      const name = `retried-attention-${Date.now()}.bin`;
      const submitted = await request.post('/v1/download/submit', { data: { URL: `${base}/${name}`, Name: name, FileName: name } });
      expect(submitted.status()).toBe(202);
      const sourceId = (await submitted.json()).jobs[0].canonicalJobId as string;
      const readJob = async (id: string) => (await request.get(`/v1/jobs/${id}`)).json();
      await expect.poll(async () => (await readJob(sourceId)).state, { timeout: 20_000 }).toBe('failed');

      await page.goto('/dashboard');
      const drawer = await openDrawer(page);
      const attention = drawer.locator('[data-job-panel-group="attention"]');
      await expect(attention.locator(`article[data-job-id="${sourceId}"]`)).toHaveCount(1);

      const source = await readJob(sourceId);
      const retry = source.commands.find((command: any) => command.key === 'retry');
      const answer = await request.post(`/v1/jobs/${sourceId}/commands/retry`, {
        data: { expectedVersion: retry.jobVersion, idempotencyKey: `retry-${sourceId}` },
        headers: { 'Idempotency-Key': `retry-${sourceId}` },
      });
      expect(answer.ok(), await answer.text()).toBe(true);
      const successorId = (await answer.json()).successorId as string;
      await expect.poll(async () => (await readJob(successorId)).state, { timeout: 20_000 }).toBe('failed');

      await refreshDrawer(page);
      await expect(attention.locator(`article[data-job-id="${successorId}"]`)).toHaveCount(1);
      await expect(drawer.locator(`article[data-job-id="${sourceId}"]`)).toHaveCount(0);
    } finally {
      server.close();
    }
  });
});

test.describe('A confirmation whose command went away while it was open', () => {
  for (const answer of ['Go back', 'Cancel'] as const) {
    test(`hands focus to the detail page's next command after ${answer}`, async ({ page }) => {
      const store = jobStore([runningJob('confirm-gone', 'Confirm gone download')]);
      await serveStore(page, store);
      await page.route('**/v1/jobs/confirm-gone/commands/cancel', route => route.fulfill({ status: 409, json: {
        error: 'the job does not offer that command', result: { code: 'not-advertised', message: 'the job does not offer that command' },
      } }));

      await gotoMockedJobPage(page, 'confirm-gone');
      const commands = page.getByRole('group', { name: 'Job actions' });
      await commands.getByRole('button', { name: 'Cancel', exact: true }).click();
      const confirmation = page.getByRole('alertdialog');
      await expect(confirmation).toBeVisible();

      // The download finishes while the dialog is open: Cancel is no longer offered.
      store.set('confirm-gone', withCommands({ ...store.jobs.get('confirm-gone')!, state: 'succeeded', version: 3, phase: undefined }, [['dismiss', 'Dismiss', { bulk: true }]]));
      await page.evaluate(snapshot => {
        const root = document.querySelector('[data-testid="job-detail"]');
        (window as any).Alpine.$data(root).applyStreamSnapshot(snapshot);
      }, store.jobs.get('confirm-gone'));
      await expect(commands.getByRole('button', { name: 'Cancel', exact: true })).toHaveCount(0);

      await confirmation.getByRole('button', { name: answer, exact: true }).click();
      await expect(commands.getByRole('button', { name: 'Dismiss', exact: true })).toBeFocused();
    });
  }

  test('hands focus to Select All when the /jobs selection it was asked about emptied', async ({ page, request }) => {
    const server = http.createServer((_request, response) => {
      response.writeHead(404, { 'Content-Type': 'text/plain' });
      response.end('gone');
    });
    await new Promise<void>(resolve => server.listen(0, '127.0.0.1', resolve));
    try {
      const base = `http://127.0.0.1:${(server.address() as AddressInfo).port}`;
      const stamp = `bulk-confirm-${Date.now()}`;
      const ids: string[] = [];
      for (const index of [0, 1]) {
        const name = `${stamp}-${index}.bin`;
        const submitted = await request.post('/v1/download/submit', { data: { URL: `${base}/${name}`, Name: name, FileName: name } });
        expect(submitted.status()).toBe(202);
        ids.push((await submitted.json()).jobs[0].canonicalJobId as string);
      }
      for (const id of ids) await expect.poll(async () => (await (await request.get(`/v1/jobs/${id}`)).json()).state, { timeout: 20_000 }).toBe('failed');
      const [jobId] = ids;
      const real = await (await request.get(`/v1/jobs/${jobId}`)).json();
      // The bar offers what the detail advertises; here a command that asks first.
      await page.route(`**/v1/jobs/${jobId}`, route => route.fulfill({ json: {
        ...real, commands: [{ key: 'cancel', label: 'Cancel', endpoint: `/v1/jobs/${jobId}/commands/cancel`, jobVersion: real.version, bulk: true, destructive: true, confirmation: 'Stop these?' }],
      } }));

      await page.goto(`/jobs?dismissed=false&search=${encodeURIComponent(stamp)}`);
      await expect(page.locator('[data-job-id]')).toHaveCount(2);
      await page.locator(`[data-job-id="${jobId}"]`).getByRole('checkbox').check();
      const bulk = page.getByRole('group', { name: 'Commands for the selected jobs' });
      await bulk.getByRole('button', { name: 'Cancel', exact: true }).click();
      const confirmation = page.getByRole('alertdialog');
      await expect(confirmation).toBeVisible();

      // Another tab dismisses the selected job while the dialog is open; the list
      // refreshes, the card leaves, and the bar hides with the empty selection.
      const dismissed = await request.post('/v1/jobs/commands/dismiss', { data: { jobIds: [jobId], idempotencyKey: `dismiss-${jobId}` }, headers: { 'Idempotency-Key': `dismiss-${jobId}` } });
      expect(dismissed.ok()).toBe(true);
      await page.evaluate(() => window.dispatchEvent(new CustomEvent('job-list-refresh')));
      await expect(page.locator(`[data-job-id="${jobId}"]`)).toHaveCount(0, { timeout: 10_000 });

      await confirmation.getByRole('button', { name: 'Go back', exact: true }).click();
      await expect(page.locator('[data-bulk-select-all]').first()).toBeFocused();
    } finally {
      server.close();
    }
  });
});
