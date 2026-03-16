import { test as base, expect } from '@playwright/test';
import { CliRunner } from '../helpers/cli-runner';
import * as path from 'path';
import { ServerInfo, startServer, stopServer } from './server-manager';

// Module-level cache — safe because each Playwright worker is a separate process.
// Set by the auto workerServer fixture so createCliRunner() works in beforeAll hooks.
let _workerServerUrl: string | null = null;

/**
 * Standalone helper for CLI tests that need a runner outside of fixtures
 * (e.g. in test.beforeAll hooks). The URL is set automatically by the
 * worker-scoped workerServer fixture.
 */
export function createCliRunner(): CliRunner {
  const binaryPath = process.env.CLI_PATH || path.resolve(__dirname, '../../mr');
  const serverUrl = _workerServerUrl
    || process.env.CLI_BASE_URL
    || process.env.BASE_URL
    || 'http://localhost:8181';
  return new CliRunner(binaryPath, serverUrl);
}

type CliWorkerFixtures = {
  workerServer: ServerInfo;
  cli: CliRunner;
};

export const test = base.extend<{}, CliWorkerFixtures>({
  // ---- Worker-scoped: one ephemeral server per CLI worker ----
  // auto:true ensures it runs before any beforeAll hooks.
  workerServer: [async ({}, use) => {
    const externalUrl = process.env.CLI_BASE_URL || process.env.BASE_URL;
    if (externalUrl) {
      _workerServerUrl = externalUrl;
      const url = new URL(externalUrl);
      const shareUrl = process.env.SHARE_BASE_URL;
      await use({
        port: parseInt(url.port) || 8181,
        sharePort: shareUrl ? parseInt(new URL(shareUrl).port) || 8183 : 8183,
        proc: null,
      });
      _workerServerUrl = null;
      return;
    }

    const server = await startServer();
    _workerServerUrl = `http://127.0.0.1:${server.port}`;
    await use(server);
    _workerServerUrl = null;
    await stopServer(server.proc);
  }, { scope: 'worker', auto: true }],

  // Worker-scoped CLI runner reused across all tests in the worker
  cli: [async ({ workerServer }, use) => {
    const binaryPath = process.env.CLI_PATH || path.resolve(__dirname, '../../mr');
    const serverUrl = `http://127.0.0.1:${workerServer.port}`;
    await use(new CliRunner(binaryPath, serverUrl));
  }, { scope: 'worker' }],
});

export { expect };
