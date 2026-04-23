import { test, expect } from '../../fixtures/cli.fixture';

test.describe('mr admin settings bounds + error handling', () => {
  test('rejects below-min with nonzero exit and informative message', async ({ cli }) => {
    // max_upload_size min is 1024 bytes; value 1 is below the minimum
    const result = cli.run('admin', 'settings', 'set', 'max_upload_size', '1');
    expect(result.exitCode).not.toBe(0);
    // Error message should be informative — cobra prints errors to stderr
    const combined = (result.stderr + result.stdout).toLowerCase();
    expect(combined).toMatch(/out of bounds|invalid|400/);
  });

  test('rejects unknown key with nonzero exit', async ({ cli }) => {
    const result = cli.run('admin', 'settings', 'set', 'not_a_real_key', '1');
    expect(result.exitCode).not.toBe(0);
  });

  test('get on unknown key exits nonzero', async ({ cli }) => {
    const result = cli.run('admin', 'settings', 'get', 'not_a_real_key');
    expect(result.exitCode).not.toBe(0);
  });
});
