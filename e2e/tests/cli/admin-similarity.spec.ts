import { test, expect } from '../../fixtures/cli.fixture';

test.describe('mr admin similarity', () => {
  // A single recompute invocation: the job runs asynchronously and holds a
  // process-wide guard, so overlapping recompute calls would 409. One call is
  // enough to exercise the command end-to-end against the shared test server.
  test('recompute --json starts a background job with a jobId', async ({ cli }) => {
    const result = cli.runOrFail('admin', 'similarity', 'recompute', '--json');
    const parsed = JSON.parse(result.stdout);
    expect(typeof parsed.jobId).toBe('string');
    expect(parsed.jobId.length).toBeGreaterThan(0);
  });

  test('retry-failed reports how many rows were reset', async ({ cli }) => {
    const result = cli.runOrFail('admin', 'similarity', 'retry-failed');
    expect(result.stdout).toMatch(/Reset \d+ failed hash\(es\) for retry\./);
  });

  test('retry-failed --json emits a reset count', async ({ cli }) => {
    const result = cli.runOrFail('admin', 'similarity', 'retry-failed', '--json');
    const parsed = JSON.parse(result.stdout);
    expect(typeof parsed.reset).toBe('number');
  });
});
