import { test, expect } from '../../fixtures/a11y.fixture';

test.describe('Job Center panel accessibility', () => {
  async function openPanel(page: import('@playwright/test').Page) {
    await page.goto('/dashboard');
    const trigger = page.getByRole('button', { name: 'Open Jobs panel' });
    await expect(trigger).toBeVisible();
    await trigger.click();
    const panel = page.getByRole('dialog', { name: 'Jobs' });
    await expect(panel).toBeVisible();
    return { trigger, panel };
  }

  test('the panel has dialog semantics and moves focus inside on open', async ({ page }) => {
    const { panel } = await openPanel(page);
    await expect(panel).toHaveAttribute('aria-modal', 'true');
    await expect.poll(() => page.evaluate(() =>
      document.activeElement?.closest('#job-center-panel') !== null,
    )).toBe(true);
  });

  test('the close control and status announcement have accessible names', async ({ page }) => {
    const { panel } = await openPanel(page);
    const close = panel.getByRole('button', { name: 'Close Jobs panel', exact: true });
    await expect(close).toBeVisible();
    await expect(close).toHaveAttribute('aria-label', 'Close Jobs panel');

    // Two polite status regions: the connection line, which is visible, and the
    // announcer the drawer speaks through while it is open (it is aria-modal, so
    // the page's own live region may go unheard), which is visually hidden.
    const status = panel.getByRole('status').filter({ hasText: /Live updates connected|Reconnecting|Connecting to live updates/ });
    await expect(status).toBeVisible();
    await expect(status).toHaveAttribute('aria-live', 'polite');
    await expect(status).not.toBeEmpty();

    const announcer = panel.locator('[data-job-panel-announcer]');
    await expect(announcer).toHaveAttribute('role', 'status');
    await expect(announcer).toHaveAttribute('aria-live', 'polite');
    await expect(panel.getByRole('status')).toHaveCount(2);
  });

  test('detail command controls are exposed as a named group', async ({ page }) => {
    const job = {
      id: 'a11y-detail-command-group',
      kind: 'remote-download',
      state: 'failed',
      version: 2,
      title: 'Detail command group job',
      commands: [{ key: 'retry', label: 'Retry', jobVersion: 2 }],
      outputs: [],
      lineage: { ancestors: [], successors: [], parents: [], children: [] },
    };
    await page.route(`**/v1/jobs/${job.id}/events**`, route => route.fulfill({ json: { events: [] } }));
    await page.route(`**/v1/jobs/${job.id}`, route => route.fulfill({ json: job }));

    await page.goto(`/job?id=${job.id}`);

    const detail = page.getByTestId('job-detail');
    const controls = detail.getByRole('group', { name: 'Advertised job commands' });
    await expect(controls).toBeVisible();
    await expect(controls.getByRole('button', { name: 'Retry' })).toBeVisible();
  });

  test('axe finds no serious or critical violations in the open panel', async ({ page, checkComponentA11y }) => {
    const job = {
      id: 'a11y-advertised-command',
      kind: 'remote-download',
      state: 'failed',
      version: 2,
      title: 'Accessibility regression job',
      acceptedAt: new Date().toISOString(),
    };
    await page.route('**/v1/jobs/summary', route => route.fulfill({ json: { byState: { failed: 1 } } }));
    await page.route(/\/v1\/jobs(?:\?.*)?$/, route => {
      const states = new URL(route.request().url()).searchParams.getAll('state');
      return route.fulfill({
        json: { jobs: states.includes('failed') ? [job] : [], nextCursor: null },
      });
    });
    await page.route(`**/v1/jobs/${job.id}`, route => route.fulfill({
      json: {
        ...job,
        commands: [{
          key: 'retry',
          label: 'Retry',
          endpoint: `/v1/jobs/${job.id}/commands/retry`,
          jobVersion: job.version,
        }],
        outputs: [],
        lineage: { ancestors: [], successors: [], parents: [], children: [] },
      },
    }));

    await openPanel(page);
    const controls = page.getByRole('group', { name: 'Advertised controls' });
    await expect(controls).toBeVisible();
    await expect(controls.getByRole('button', { name: 'Retry' })).toBeVisible();
    await checkComponentA11y('#job-center-panel', {
      // The dialog uses local header/footer sections; these are not the page's
      // banner or contentinfo landmarks.
      disableRules: ['landmark-no-duplicate-banner', 'landmark-no-duplicate-contentinfo'],
    });
  });

  test('axe finds no serious or critical violations in a running job\'s stats, metrics and graphs', async ({ page, checkComponentA11y }) => {
    const job = {
      id: 'a11y-running-download',
      kind: 'remote-download',
      state: 'running',
      version: 3,
      title: 'Accessibility running download',
      acceptedAt: new Date().toISOString(),
      progress: {
        completed: 3 * 1024 * 1024, total: 8 * 1024 * 1024, unit: 'bytes', message: 'Downloading',
        rate: 512 * 1024, eta: new Date(Date.now() + 10_000).toISOString(), etaEstimated: true,
        metrics: [{ key: 'segments', label: 'Segments', value: 12, total: 40, unit: 'items', graph: true }],
        series: {
          intervalMs: 1000, unit: 'bytes',
          points: [
            { t: 1000, c: 0, v: { segments: 0 } },
            { t: 2000, c: 1048576, r: 1048576, v: { segments: 4 } },
            { t: 3000, c: 3145728, r: 2097152, v: { segments: 12 } },
          ],
        },
      },
    };
    await page.route(/\/v1\/jobs(?:\?.*)?$/, route => {
      const states = new URL(route.request().url()).searchParams.getAll('state');
      return route.fulfill({ json: { jobs: states.includes('running') ? [job] : [], nextCursor: null } });
    });
    await page.route(`**/v1/jobs/${job.id}`, route => route.fulfill({
      json: { ...job, commands: [], outputs: [], lineage: { ancestors: [], successors: [], parents: [], children: [] } },
    }));

    const { panel } = await openPanel(page);
    const row = panel.locator('article', { hasText: 'Accessibility running download' });
    await expect(row.getByRole('progressbar', { name: 'Accessibility running download progress' }))
      .toHaveAttribute('aria-valuetext', /^38%, 3\.0 MB of 8\.0 MB, 512 KB\/s, about \d+ s left$/);
    await expect(row.getByRole('img', { name: /^Speed over 1 s: latest 2\.0 MB\/s/ })).toBeVisible();
    await expect(row.getByRole('img', { name: /^Segments over 2 s: latest 12/ })).toBeVisible();
    await expect(row.getByRole('term').filter({ hasText: 'Segments' })).toBeVisible();
    await checkComponentA11y('#job-center-panel', {
      disableRules: ['landmark-no-duplicate-banner', 'landmark-no-duplicate-contentinfo'],
    });
  });

  test('decorative icons in the trigger and close control are hidden from assistive technology', async ({ page }) => {
    const { trigger, panel } = await openPanel(page);
    await expect(trigger.locator('svg')).toHaveAttribute('aria-hidden', 'true');
    await expect(panel.getByRole('button', { name: 'Close Jobs panel' }).locator('svg'))
      .toHaveAttribute('aria-hidden', 'true');
  });
});

test.describe('Job Center list accessibility', () => {
  test('axe finds no serious or critical violations on /jobs with a job and a selection', async ({ page, request, checkA11y }) => {
    const stamp = Date.now();
    const group = await request.post('/v1/group', { data: { Name: `job-list-a11y-${stamp}` } });
    const groupId = (await group.json()).ID;
    const name = `job-list-a11y-${stamp}.bin`;
    const submitted = await request.post('/v1/download/submit', {
      data: { URL: `http://127.0.0.1:9/${name}`, OwnerId: groupId, FileName: name },
    });
    const jobId = (await submitted.json()).jobs[0].canonicalJobId as string;
    await expect.poll(async () => (await (await request.get(`/v1/jobs/${jobId}`)).json()).state, { timeout: 20_000 })
      .toBe('failed');

    await page.goto(`/jobs?search=${encodeURIComponent(name)}`);
    const row = page.locator(`[data-job-id="${jobId}"]`);
    await expect(row).toBeVisible();
    await checkA11y();

    await row.getByRole('checkbox').check();
    await expect(page.getByRole('group', { name: 'Commands for the selected jobs' }).getByRole('button', { name: 'Dismiss' })).toBeVisible();
    await row.locator('summary').click();
    await checkA11y();
  });
});
