import { test, expect } from '../../fixtures/base.fixture';

/**
 * The schedules table on /plugins/manage.
 *
 * mah.schedule has Go coverage for claiming, dispatch and the TTL arithmetic,
 * but nothing referenced the two testids this template emits, so a markup
 * regression here would have shipped silently.
 *
 * The fixture plugin declares a one-hour interval and never fires during a run.
 * That is the point: the row is what is under test, and a schedule that fired
 * would add a job to unrelated specs.
 */
test.describe('Plugin schedules', () => {
  test.beforeEach(async ({ apiClient }) => {
    try {
      await apiClient.disablePlugin('test-schedules');
    } catch {
      // not enabled yet
    }
  });

  test.afterEach(async ({ apiClient }) => {
    try {
      await apiClient.disablePlugin('test-schedules');
    } catch {
      // already disabled
    }
  });

  test('a declared schedule is rendered on the manage page', async ({ page, apiClient }) => {
    await apiClient.enablePlugin('test-schedules');

    await page.goto('/plugins/manage');
    await page.waitForLoadState('load');

    const section = page.getByTestId('plugin-schedules-test-schedules');
    await expect(section).toBeVisible();
    await expect(section).toContainText('Schedules');

    const row = page.getByTestId('plugin-schedule-test-schedules-nightly-rollup');
    await expect(row).toBeVisible();
    await expect(row).toContainText('nightly-rollup');
    await expect(row).toContainText('1h');
    // Never run, and never going to be within a test run.
    await expect(row).toContainText('never run');
  });

  test('the row survives a disable, and says the schedule is no longer declared', async ({ page, apiClient }) => {
    await apiClient.enablePlugin('test-schedules');
    await page.goto('/plugins/manage');
    await expect(page.getByTestId('plugin-schedule-test-schedules-nightly-rollup')).toBeVisible();

    await apiClient.disablePlugin('test-schedules');
    await page.goto('/plugins/manage');
    await page.waitForLoadState('load');

    // Rows are deliberately never deleted on disable, so re-enabling resumes
    // with the row's history and its operator binding. What changes is that the
    // row is no longer backed by a live registration, and the table says so
    // rather than showing a next-due time that will never arrive.
    const row = page.getByTestId('plugin-schedule-test-schedules-nightly-rollup');
    await expect(row).toBeVisible();
    await expect(row).toContainText('Not declared');
  });
});
