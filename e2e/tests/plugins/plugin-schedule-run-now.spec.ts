import { test, expect } from '../../fixtures/base.fixture';

/**
 * The "Run now" control on the plugin schedules table.
 *
 * This is the spec plugin-schedules.spec.ts could not write. That one asserts the
 * row renders and then says, in as many words, "never run, and never going to be
 * within a test run" — because the fixture declares a one-hour interval and
 * MinScheduleInterval (30s) makes "fires quickly" and "does not disturb anything"
 * mutually exclusive. A run-now control is what resolves that: the schedule fires
 * on demand, so the browser suite can finally assert that a schedule *ran* rather
 * than only that a row exists.
 *
 * The two assertions that matter are the outcome and the cadence. A manual run
 * must move `runs` and `lastStatus`, and must not move `nextDueAt` — an extra run
 * is not a re-phasing.
 */
test.describe('Plugin schedule run now', () => {
  const PLUGIN = 'test-schedules';
  const SCHEDULE = 'nightly-rollup';

  const runButton = (page: any) =>
    page.getByTestId(`plugin-schedule-run-${PLUGIN}-${SCHEDULE}`);
  const scheduleRow = (page: any) =>
    page.getByTestId(`plugin-schedule-${PLUGIN}-${SCHEDULE}`);

  test.beforeEach(async ({ apiClient }) => {
    try {
      await apiClient.disablePlugin(PLUGIN);
    } catch {
      // not enabled yet
    }
  });

  test.afterEach(async ({ apiClient }) => {
    try {
      await apiClient.disablePlugin(PLUGIN);
    } catch {
      // already disabled
    }
  });

  test('runs a schedule on demand, records the outcome, and leaves the cadence alone', async ({ page, apiClient }) => {
    await apiClient.enablePlugin(PLUGIN);
    await page.goto('/plugins/manage');
    await page.waitForLoadState('load');

    // The row starts where plugin-schedules.spec.ts leaves it.
    await expect(scheduleRow(page)).toContainText('never run');

    // Captured from the API rather than the rendered cell: the cell is formatted
    // to the minute, which is coarse enough to hide a re-base of a 1h schedule
    // only if the test happened to run in the same minute. The stored value is
    // what the assertion is actually about.
    const dueBefore = await scheduleNextDue(apiClient, PLUGIN, SCHEDULE);
    expect(dueBefore, 'the fixture schedule has no stored next-due time').toBeTruthy();

    const button = runButton(page);
    await expect(button).toBeVisible();
    // WCAG 2.5.3: the visible label must survive inside the accessible name, so
    // a speech-input user saying "click Run now" still matches.
    await expect(button).toHaveAccessibleName(/^Run now/);
    await button.click();

    // The control answers when the run has *started*. Completion is the jobs
    // panel's business and the row's, after a reload.
    await expect(
      page.getByTestId(`plugin-schedule-run-status-${PLUGIN}-${SCHEDULE}`),
    ).toHaveText(/Started/);

    // The handler is a single mah.kv.set, so this settles quickly; the retry is
    // for the dispatch hop, not for a slow plugin.
    await expect(async () => {
      const row = await scheduleState(apiClient, PLUGIN, SCHEDULE);
      expect(row.runs).toBeGreaterThanOrEqual(1);
      expect(row.lastStatus).toBe('completed');
    }).toPass({ timeout: 15_000 });

    // The cadence is untouched. This is the assertion that fails if the run path
    // reaches for CompletePluginScheduleRun or AdvancePluginScheduleAtDispatch,
    // both of which write next_due_at = now + every.
    const dueAfter = await scheduleNextDue(apiClient, PLUGIN, SCHEDULE);
    expect(
      dueAfter,
      'a manual run re-phased the schedule; an extra run is not a re-phasing',
    ).toBe(dueBefore);

    // And the page now shows the run it just made.
    await page.reload();
    await page.waitForLoadState('load');
    await expect(scheduleRow(page)).toContainText('completed');
    await expect(scheduleRow(page)).not.toContainText('never run');
  });

  test('the control is not offered for a schedule that is no longer declared', async ({ page, apiClient }) => {
    await apiClient.enablePlugin(PLUGIN);
    await page.goto('/plugins/manage');
    await expect(runButton(page)).toBeVisible();

    // Disabling leaves the row in place but drops the live registration, and a
    // run against it would be refused with 409. What is offered follows what is
    // allowed: the button goes rather than staying to fail.
    await apiClient.disablePlugin(PLUGIN);
    await page.goto('/plugins/manage');
    await page.waitForLoadState('load');

    await expect(scheduleRow(page)).toContainText('Not declared');
    await expect(runButton(page)).toHaveCount(0);
  });
});

async function scheduleState(apiClient: any, plugin: string, scheduleId: string) {
  const rows = await apiClient.getPluginSchedules(plugin);
  const row = rows.find((r: any) => r.scheduleId === scheduleId);
  if (!row) throw new Error(`no stored schedule ${plugin}/${scheduleId}`);
  return row;
}

async function scheduleNextDue(apiClient: any, plugin: string, scheduleId: string) {
  return (await scheduleState(apiClient, plugin, scheduleId)).nextDueAt;
}
