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

    const status = panel.getByRole('status');
    await expect(status).toBeVisible();
    await expect(status).toHaveAttribute('aria-live', 'polite');
    await expect(status).not.toBeEmpty();
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

  test('decorative icons in the trigger and close control are hidden from assistive technology', async ({ page }) => {
    const { trigger, panel } = await openPanel(page);
    await expect(trigger.locator('svg')).toHaveAttribute('aria-hidden', 'true');
    await expect(panel.getByRole('button', { name: 'Close Jobs panel' }).locator('svg'))
      .toHaveAttribute('aria-hidden', 'true');
  });
});
