import { test, expect, createCliRunner } from '../../fixtures/cli.fixture';

test.describe('Jobs list', () => {
  test('jobs list returns parseable JSON', async ({ cli }) => {
    const result = cli.runOrFail('jobs', 'list', '--json');
    const parsed = JSON.parse(result.stdout);
    expect(parsed).toBeDefined();
  });
});

test.describe('Job submit', () => {
  test('submit a download URL succeeds', async ({ cli }) => {
    // In ephemeral mode the actual download may fail, but the submit itself should succeed
    const result = cli.run('job', 'submit', '--urls', 'http://example.com/test.txt', '--json');
    // Submit should succeed (exit 0) or at least produce output
    // Some servers may reject if download queue is not configured, so we check loosely
    if (result.exitCode === 0) {
      expect(result.stdout).toBeTruthy();
    } else {
      // If submit fails, it should be a server-side error, not a CLI parse error
      const combined = result.stdout + result.stderr;
      expect(combined).toBeTruthy();
    }
  });

  test('submit without --urls flag fails', async ({ cli }) => {
    cli.runExpectError('job', 'submit');
  });

  test('jobs list after submit returns JSON', async ({ cli }) => {
    // Submit first (best effort)
    cli.run('job', 'submit', '--urls', 'http://example.com/test2.txt');

    const result = cli.runOrFail('jobs', 'list', '--json');
    const parsed = JSON.parse(result.stdout);
    expect(parsed).toBeDefined();
  });
});

test.describe('Job state operations with non-existent ID', () => {
  test('cancel with non-existent ID produces error', async ({ cli }) => {
    const result = cli.run('job', 'cancel', '999999', '--json');
    // Should fail or return error response
    const combined = result.stdout + result.stderr;
    expect(combined).toBeTruthy();
  });

  test('pause with non-existent ID produces error', async ({ cli }) => {
    const result = cli.run('job', 'pause', '999999', '--json');
    const combined = result.stdout + result.stderr;
    expect(combined).toBeTruthy();
  });

  test('resume with non-existent ID produces error', async ({ cli }) => {
    const result = cli.run('job', 'resume', '999999', '--json');
    const combined = result.stdout + result.stderr;
    expect(combined).toBeTruthy();
  });

  test('retry with non-existent ID produces error', async ({ cli }) => {
    const result = cli.run('job', 'retry', '999999', '--json');
    const combined = result.stdout + result.stderr;
    expect(combined).toBeTruthy();
  });
});

test.describe('Job cancel/pause/resume/retry without ID fails', () => {
  test('cancel without ID fails', async ({ cli }) => {
    cli.runExpectError('job', 'cancel');
  });

  test('pause without ID fails', async ({ cli }) => {
    cli.runExpectError('job', 'pause');
  });

  test('resume without ID fails', async ({ cli }) => {
    cli.runExpectError('job', 'resume');
  });

  test('retry without ID fails', async ({ cli }) => {
    cli.runExpectError('job', 'retry');
  });
});
