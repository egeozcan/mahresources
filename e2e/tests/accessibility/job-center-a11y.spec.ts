import { test, expect } from '../../fixtures/a11y.fixture';
import { gotoMockedJobPage } from '../../helpers/job-page';

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

    await gotoMockedJobPage(page, job.id);

    const detail = page.getByTestId('job-detail');
    const controls = detail.getByRole('group', { name: 'Job actions' });
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
    const controls = page.getByRole('group', { name: 'Job actions' });
    await expect(controls).toBeVisible();
    await expect(controls.getByRole('button', { name: 'Retry' })).toBeVisible();
    await checkComponentA11y('#job-center-panel');
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
    await checkComponentA11y('#job-center-panel');
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

// Forced colors (Windows High Contrast) repaints colours with the user's system
// palette and drops box-shadow, which is all a focus ring is. A control that
// hid its outline behind a ring therefore shows no focus at all there. The
// check is on pixels: the control's surroundings must look different focused
// than not, whatever property draws the difference.
test.describe('Job Center focus in forced colors', () => {
  // One measurement: the same clip, focused and then not. A page that
  // re-renders the control (a live update landing mid-measurement) or moves it
  // invalidates the comparison, so that attempt is discarded and repeated
  // rather than read as a missing indicator.
  async function paintedFocus(page: import('@playwright/test').Page, control: import('@playwright/test').Locator) {
    const viewport = page.viewportSize()!;
    const unchanged = async (box: { x: number; y: number; width: number; height: number }) => {
      const now = await control.boundingBox();
      return !!now && ['x', 'y', 'width', 'height'].every(key => Math.abs((now as any)[key] - (box as any)[key]) < 0.5);
    };
    for (let attempt = 0; attempt < 5; attempt++) {
      await control.scrollIntoViewIfNeeded();
      // Finite ones only: a pulsing indicator never finishes.
      await page.evaluate(() => Promise.all(document.getAnimations()
        .filter(animation => animation.effect?.getComputedTiming().endTime !== Infinity)
        .map(animation => animation.finished.catch(() => null))));
      const box = await control.boundingBox();
      expect(box, 'the control must be rendered').not.toBeNull();
      const x = Math.max(0, Math.floor(box!.x) - 6);
      const y = Math.max(0, Math.floor(box!.y) - 6);
      const clip = {
        x, y,
        width: Math.min(viewport.width - x, Math.ceil(box!.width) + 12),
        height: Math.min(viewport.height - y, Math.ceil(box!.height) + 12),
      };
      const shot = () => page.screenshot({ clip, animations: 'disabled', caret: 'hide' });
      await control.focus();
      await expect(control).toBeFocused();
      const focused = await shot();
      const style = await control.evaluate(element => {
        const computed = getComputedStyle(element);
        return `outline=${computed.outlineStyle} ${computed.outlineWidth} shadow=${computed.boxShadow}`;
      });
      const stillFocused = await control.evaluate(element => element === document.activeElement);
      await control.evaluate(element => (element as HTMLElement).blur());
      const blurred = await shot();
      if (!stillFocused || !(await unchanged(box!))) continue;
      return { painted: !focused.equals(blurred), style };
    }
    throw new Error('the control kept moving or losing focus while it was measured');
  }

  async function expectPaintedFocus(page: import('@playwright/test').Page, control: import('@playwright/test').Locator, what: string) {
    const { painted, style } = await paintedFocus(page, control);
    // Soft, so one run names every control that has lost its indicator.
    expect.soft(painted, `${what} shows no focus in forced colors (${style})`).toBe(true);
  }

  test('the trigger, drawer, confirmation and bulk commands show focus', async ({ page, request }) => {
    const stamp = Date.now();
    const name = `forced-colors-${stamp}.bin`;
    const submitted = await request.post('/v1/download/submit', {
      data: { URL: `http://127.0.0.1:9/${name}`, Name: name, FileName: name },
    });
    expect(submitted.status(), await submitted.text()).toBe(202);
    const jobId = (await submitted.json()).jobs[0].canonicalJobId as string;
    await expect.poll(async () => (await (await request.get(`/v1/jobs/${jobId}`)).json()).state, { timeout: 20_000 })
      .toBe('failed');
    // Every event published, so the pages below load after the stream has
    // said everything about this job and nothing re-renders its row mid-check.
    await expect.poll(async () => {
      const events = (await (await request.get(`/v1/jobs/${jobId}/events`)).json()).events || [];
      return events.length > 0 && events.every((event: any) => Number(event.deliverySequence) > 0);
    }, { timeout: 20_000 }).toBe(true);

    await page.emulateMedia({ forcedColors: 'active' });
    await page.goto('/dashboard');
    const trigger = page.getByRole('button', { name: 'Open Jobs panel' });
    await expectPaintedFocus(page, trigger, 'the Jobs button');

    await trigger.focus();
    await page.keyboard.press('Enter');
    const drawer = page.getByRole('dialog', { name: 'Jobs' });
    await expect(drawer).toBeVisible();
    await drawer.evaluate(element => Promise.all(element.getAnimations().map(animation => animation.finished)));
    const row = drawer.locator(`article[data-job-id="${jobId}"]`);
    await expect(row.getByRole('button', { name: 'Dismiss' })).toBeVisible({ timeout: 10_000 });

    await expectPaintedFocus(page, drawer.getByRole('button', { name: 'Close Jobs panel' }), 'the drawer\'s Close button');
    await expectPaintedFocus(page, row.getByRole('link', { name }), 'a row\'s title link');
    await expectPaintedFocus(page, row.getByRole('button', { name: 'Dismiss' }), 'a row\'s Dismiss');
    await expectPaintedFocus(page, row.locator('summary'), 'a row\'s More');
    await expectPaintedFocus(page, drawer.getByRole('link', { name: 'All jobs' }), 'the All jobs link');

    // Forget cannot be undone, so it asks first.
    await row.locator('summary').click();
    await row.getByRole('button', { name: 'Forget replay input' }).focus();
    await page.keyboard.press('Enter');
    const confirm = page.getByRole('alertdialog');
    await expect(confirm).toBeVisible();
    const buttons = confirm.getByRole('button');
    await expect(buttons).toHaveCount(2);
    await expectPaintedFocus(page, buttons.nth(0), 'the confirmation\'s safe button');
    await expectPaintedFocus(page, buttons.nth(1), 'the confirmation\'s confirming button');
    await page.keyboard.press('Escape');
    await expect(confirm).toBeHidden();

    await page.goto(`/jobs?search=${encodeURIComponent(name)}`);
    const card = page.locator(`[data-job-id="${jobId}"]`);
    await card.getByRole('checkbox').check();
    const bulk = page.getByRole('group', { name: 'Commands for the selected jobs' });
    await expect(bulk.getByRole('button', { name: 'Dismiss' })).toBeVisible();
    await expectPaintedFocus(page, bulk.getByRole('button').first(), 'the first bulk command');
  });
});
