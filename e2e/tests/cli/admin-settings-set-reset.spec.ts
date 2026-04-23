import { test, expect } from '../../fixtures/cli.fixture';

test.describe('mr admin settings set/reset', () => {
  test('round-trip on max_upload_size', async ({ cli }) => {
    // Ensure clean start — reset in case a prior test left an override
    cli.run('admin', 'settings', 'reset', 'max_upload_size');

    // Set 1 MiB override
    const set = cli.run(
      'admin', 'settings', 'set', 'max_upload_size', '1048576',
      '--reason', 'cli-e2e',
    );
    expect(set.exitCode).toBe(0);

    // Verify via list --json
    const afterSet = cli.runOrFail('admin', 'settings', 'list', '--json');
    const views = JSON.parse(afterSet.stdout);
    const mus = views.find((v: any) => v.key === 'max_upload_size');
    expect(mus).toBeTruthy();
    expect(mus.overridden).toBe(true);
    expect(mus.current).toBe(1048576);

    // Reset
    const reset = cli.run(
      'admin', 'settings', 'reset', 'max_upload_size',
      '--reason', 'cli-e2e-revert',
    );
    expect(reset.exitCode).toBe(0);

    // Verify revert
    const afterReset = cli.runOrFail('admin', 'settings', 'list', '--json');
    const musAfter = JSON.parse(afterReset.stdout).find((v: any) => v.key === 'max_upload_size');
    expect(musAfter.overridden).toBe(false);
  });

  test('set with K/M/G suffix parses correctly', async ({ cli }) => {
    cli.run('admin', 'settings', 'reset', 'max_upload_size');

    const set = cli.run('admin', 'settings', 'set', 'max_upload_size', '2G');
    expect(set.exitCode).toBe(0);

    const list = cli.runOrFail('admin', 'settings', 'list', '--json');
    const mus = JSON.parse(list.stdout).find((v: any) => v.key === 'max_upload_size');
    expect(mus.current).toBe(2 * 1024 * 1024 * 1024);

    cli.run('admin', 'settings', 'reset', 'max_upload_size');
  });
});
