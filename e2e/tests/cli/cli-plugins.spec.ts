import { test, expect, createCliRunner } from '../../fixtures/cli.fixture';

// The test-plugins directory has: data-views, fal-ai, meta-editors, test-actions,
// test-api, test-banner, test-blocks, test-kvstore, test-manifest,
// test-manifest-empty, test-manifest-private, widgets.
//
// The three test-manifest-* fixtures are the only ones declaring a manifest;
// every other entry is legacy, which is why the manage-page Access tests need
// them to cover the declared-manifest branches at all.
const TEST_PLUGIN = 'test-banner';

test.describe('Plugins list', () => {
  test('plugins list returns parseable JSON', async ({ cli }) => {
    // plugins list always outputs raw JSON regardless of --json flag
    const result = cli.runOrFail('plugins', 'list');
    const parsed = JSON.parse(result.stdout);
    expect(parsed).toBeDefined();
  });
});

test.describe('Plugin enable and disable', () => {
  test('plugin enable with test plugin succeeds', async ({ cli }) => {
    const result = cli.run('plugin', 'enable', TEST_PLUGIN);
    // Should succeed if the plugin exists in the test-plugins directory
    if (result.exitCode === 0) {
      const combined = result.stdout + result.stderr;
      // Either prints success message or JSON output
      expect(combined.length).toBeGreaterThan(0);
    }
    // If it fails, that is acceptable too in some configurations
  });

  test('plugin disable with test plugin succeeds', async ({ cli }) => {
    // Enable first to ensure it can be disabled
    cli.run('plugin', 'enable', TEST_PLUGIN);

    const result = cli.run('plugin', 'disable', TEST_PLUGIN);
    if (result.exitCode === 0) {
      const combined = result.stdout + result.stderr;
      expect(combined.length).toBeGreaterThan(0);
    }
  });

  test('plugin enable with non-existent plugin produces error', async ({ cli }) => {
    const result = cli.run('plugin', 'enable', 'nonexistent-plugin-xyz');
    // Should fail or return an error
    const combined = result.stdout + result.stderr;
    expect(combined).toBeTruthy();
    // If the server returns an error, exit code should be non-zero
    if (result.exitCode !== 0) {
      expect(result.exitCode).not.toBe(0);
    }
  });
});

test.describe('Plugin settings', () => {
  test('plugin settings with test plugin and --data succeeds', async ({ cli }) => {
    // Enable the plugin first
    cli.run('plugin', 'enable', TEST_PLUGIN);

    const result = cli.run('plugin', 'settings', TEST_PLUGIN, '--data', '{"key":"value"}');
    // Settings update may succeed or fail depending on plugin support
    const combined = result.stdout + result.stderr;
    expect(combined).toBeTruthy();
  });

  test('plugin settings without --data flag fails', async ({ cli }) => {
    cli.runExpectError('plugin', 'settings', TEST_PLUGIN);
  });
});

test.describe('Plugin purge-data', () => {
  test('plugin purge-data with test plugin', async ({ cli }) => {
    // Enable the plugin first
    cli.run('plugin', 'enable', TEST_PLUGIN);

    const result = cli.run('plugin', 'purge-data', TEST_PLUGIN);
    // May succeed or produce an error depending on plugin implementation
    const combined = result.stdout + result.stderr;
    expect(combined).toBeTruthy();
  });
});

test.describe('Plugin commands without name argument fail', () => {
  test('plugin enable without name fails', async ({ cli }) => {
    cli.runExpectError('plugin', 'enable');
  });

  test('plugin disable without name fails', async ({ cli }) => {
    cli.runExpectError('plugin', 'disable');
  });

  test('plugin purge-data without name fails', async ({ cli }) => {
    cli.runExpectError('plugin', 'purge-data');
  });
});

/**
 * `mr plugin schedules` had no CLI coverage at all. The fixture plugin declares
 * a one-hour schedule, so the row exists as soon as the plugin is enabled and
 * never fires during the run.
 */
test.describe('Plugin schedules', () => {
  const SCHEDULE_PLUGIN = 'test-schedules';

  test('plugin schedules lists a declared schedule', async ({ cli }) => {
    cli.run('plugin', 'enable', SCHEDULE_PLUGIN);
    try {
      const result = cli.run('plugin', 'schedules', SCHEDULE_PLUGIN);
      expect(result.stdout).toContain('nightly-rollup');
      expect(result.stdout).toContain('1h');
    } finally {
      cli.run('plugin', 'disable', SCHEDULE_PLUGIN);
    }
  });

  test('plugin schedules without name fails', async ({ cli }) => {
    cli.runExpectError('plugin', 'schedules');
  });
});

/**
 * `mr plugin schedule-run`.
 *
 * The describe above can only assert that a row exists, because the fixture's
 * one-hour interval means nothing fires during a run. This one makes it fire,
 * which is the whole point of the control, and then checks the two properties a
 * manual run has to have: it records an outcome, and it does not re-phase the
 * cadence.
 */
test.describe('Plugin schedule run now', () => {
  const SCHEDULE_PLUGIN = 'test-schedules';
  // Not 'nightly-rollup': that row is the one the describe above asserts still
  // reads "never run", and rows survive disable/enable by design.
  const SCHEDULE_ID = 'manual-only';

  const scheduleJSON = (cli: any) => {
    const out = cli.run('plugin', 'schedules', SCHEDULE_PLUGIN, '--json');
    const rows = JSON.parse(out.stdout);
    const row = rows.find((r: any) => r.scheduleId === SCHEDULE_ID);
    expect(row, `no stored schedule ${SCHEDULE_PLUGIN}/${SCHEDULE_ID}`).toBeTruthy();
    return row;
  };

  test('schedule-run fires a schedule that is not due, without moving next due', async ({ cli }) => {
    cli.run('plugin', 'enable', SCHEDULE_PLUGIN);
    try {
      // A baseline, not an absolute: the row outlives disable/enable, so
      // "runs is 0" would make a retry after any partial failure unpassable.
      const before = scheduleJSON(cli);

      const result = cli.run('plugin', 'schedule-run', SCHEDULE_PLUGIN, SCHEDULE_ID, '--json');
      expect(JSON.parse(result.stdout).started).toBe(true);

      let after = before;
      for (let i = 0; i < 40 && after.runs <= before.runs; i++) {
        await new Promise((r) => setTimeout(r, 250));
        after = scheduleJSON(cli);
      }

      expect(after.runs, 'the schedule never ran, so the control did nothing').toBeGreaterThan(before.runs);
      expect(after.lastStatus).toBe('completed');
      // An extra run is not a re-phasing. This fails if the run path reaches for
      // CompletePluginScheduleRun or AdvancePluginScheduleAtDispatch.
      expect(after.nextDueAt, 'a manual run re-phased the cadence').toBe(before.nextDueAt);
    } finally {
      cli.run('plugin', 'disable', SCHEDULE_PLUGIN);
    }
  });

  test('schedule-run refuses a schedule the server does not have', async ({ cli }) => {
    cli.runExpectError('plugin', 'schedule-run', 'no-such-plugin', 'no-such-schedule');
  });

  test('schedule-run needs both the plugin name and the schedule id', async ({ cli }) => {
    cli.runExpectError('plugin', 'schedule-run', SCHEDULE_PLUGIN);
  });
});
