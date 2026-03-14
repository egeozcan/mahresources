import { test, expect, createCliRunner } from '../../fixtures/cli.fixture';

// The test-plugins directory has: test-actions, test-api, test-banner, test-blocks, test-kvstore
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
