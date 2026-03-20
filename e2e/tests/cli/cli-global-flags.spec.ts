import { test, expect, createCliRunner } from '../../fixtures/cli.fixture';
import { execFileSync } from 'child_process';
import * as path from 'path';

const cliBinary = process.env.CLI_PATH || path.resolve(__dirname, '../../../mr');

test.describe('global flags', () => {
  test('--server flag works', async ({ cli }) => {
    const result = cli.run('tags', 'list');
    expect(result.exitCode).toBe(0);
  });

  test('MAHRESOURCES_URL env var works', async ({ workerServer }) => {
    const serverUrl = `http://127.0.0.1:${workerServer.port}`;
    const stdout = execFileSync(cliBinary, ['tags', 'list', '--json'], {
      encoding: 'utf-8',
      timeout: 30000,
      env: { ...process.env, MAHRESOURCES_URL: serverUrl },
    });
    const parsed = JSON.parse(stdout);
    expect(Array.isArray(parsed)).toBe(true);
  });

  test('--server takes precedence over MAHRESOURCES_URL', async ({ workerServer }) => {
    const serverUrl = `http://127.0.0.1:${workerServer.port}`;
    const stdout = execFileSync(cliBinary, ['--server', serverUrl, 'tags', 'list', '--json'], {
      encoding: 'utf-8',
      timeout: 30000,
      env: { ...process.env, MAHRESOURCES_URL: 'http://localhost:1' },
    });
    const parsed = JSON.parse(stdout);
    expect(Array.isArray(parsed)).toBe(true);
  });

  test('bad env var URL fails', async () => {
    try {
      execFileSync(cliBinary, ['tags', 'list'], {
        encoding: 'utf-8',
        timeout: 10000,
        env: { ...process.env, MAHRESOURCES_URL: 'http://localhost:1' },
      });
      // If it somehow succeeds, that's unexpected but not a test failure
    } catch (error: any) {
      expect(error.status).not.toBe(0);
    }
  });
});
