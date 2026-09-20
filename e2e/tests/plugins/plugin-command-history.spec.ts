import AxeBuilder from '@axe-core/playwright';
import { test as base, expect, type APIRequestContext } from '@playwright/test';
import { execFileSync } from 'node:child_process';
import { mkdtempSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import path from 'node:path';
import {
  ServerDatabase,
  ServerInfo,
  startServer,
  stopServer,
} from '../../fixtures/server-manager';

const PROJECT_ROOT = path.resolve(__dirname, '../../..');
const COMMAND_PATH = path.join(PROJECT_ROOT, 'e2e/test-plugins/test-commands/bin');

type CommandServer = ServerInfo & { baseURL: string; tempDir: string; database: ServerDatabase };

const test = base.extend<{}, { commandServer: CommandServer }>({
  commandServer: [async ({}, use) => {
    const tempDir = mkdtempSync(path.join(tmpdir(), 'mahresources-command-e2e-'));
    const sqliteDsn = path.join(tempDir, 'commands.db');
    const server = await startServer(3, {
      sqliteDsn,
      pluginCommandPath: COMMAND_PATH,
    });
    if (!server.database) {
      throw new Error('managed command server did not expose its database target');
    }
    const commandServer = {
      ...server,
      database: server.database,
      baseURL: `http://127.0.0.1:${server.port}`,
      tempDir,
    };
    await use(commandServer);
    await stopServer(server.proc);
    rmSync(tempDir, { recursive: true, force: true });
  }, { scope: 'worker', auto: true }],

  baseURL: async ({ commandServer }, use) => {
    await use(commandServer.baseURL);
  },
});

test.describe.configure({ mode: 'serial' });

async function enableCommandFixture(request: APIRequestContext) {
  const response = await request.post('/v1/plugin/enable', {
    form: { name: 'test-commands', confirm_commands: '1' },
    headers: { Accept: 'application/json' },
  });
  expect(response.ok(), await response.text()).toBe(true);
}

async function submitFixture(request: APIRequestContext, mode: 'wait' | 'hostile-output'): Promise<string> {
  const response = await request.post('/v1/plugins/test-commands/run', {
    data: { mode },
    headers: { Accept: 'application/json' },
  });
  expect(response.status(), await response.text()).toBe(202);
  const payload = await response.json();
  expect(payload.run_id).toEqual(expect.any(String));
  return payload.run_id;
}

function pruneOutput(database: CommandServer['database'], runID: string) {
  if (!database.dsn) {
    throw new Error(`plugin command output pruning needs an addressable ${database.type} database`);
  }
  execFileSync('go', [
    'run',
    '-tags=json1,fts5,postgres',
    './e2e/helpers/prune-plugin-command-output',
    database.type,
    database.dsn,
    runID,
  ], { cwd: PROJECT_ROOT, stdio: 'pipe' });
}

async function commandRuns(request: APIRequestContext): Promise<any[]> {
  const response = await request.get('/v1/plugin/command-runs', {
    headers: { Accept: 'application/json' },
  });
  expect(response.ok(), await response.text()).toBe(true);
  return (await response.json()).runs;
}

test.describe('administrator plugin command history', () => {
  test('real queued and running commands keep durable authority through keyboard cancellation', async ({ page, request }) => {
    await enableCommandFixture(request);
    const first = await submitFixture(request, 'wait');
    const second = await submitFixture(request, 'wait');
    const queued = await submitFixture(request, 'wait');

    await expect.poll(async () => {
      const runs = await commandRuns(request);
      return [first, second, queued].map(id => runs.find(run => run.ID === id)?.Status);
    }).toEqual(['running', 'running', 'queued']);

    await page.goto(`/admin/plugin-command-runs?id=${queued}`);
    const queuedCancel = page.getByRole('button', { name: `Cancel queued run ${queued}`, exact: true });
    await queuedCancel.focus();
    await page.keyboard.press('Enter');
    await expect(page).toHaveURL(/\/admin\/plugin-command-runs\?notice=cancelled$/);
    const notice = page.getByTestId('command-cancel-notice');
    await expect(notice).toBeFocused();
    await expect(notice).toHaveText(/Cancellation requested/);
    await expect.poll(async () => {
      const runs = await commandRuns(request);
      return runs.find(run => run.ID === queued)?.Status;
    }).toBe('cancelled');

    await page.goto('/plugins/manage');
    await page.getByTestId('cockpit-trigger').click();
    const liveRow = page.getByTestId('cockpit-job').filter({ hasText: first });
    await expect(liveRow).toContainText('Running');
    await expect(liveRow.getByRole('button', { name: 'Cancel' })).toBeVisible();
    await expect(liveRow.getByRole('button', { name: /Pause|Resume|Retry/ })).toHaveCount(0);

    const liveCancel = liveRow.getByRole('button', { name: 'Cancel' });
    await liveCancel.focus();
    await expect(liveCancel).toBeFocused();
    const cancelResponsePromise = page.waitForResponse(response => response.url().includes('/v1/jobs/cancel'), { timeout: 30_000 });
    await page.keyboard.press('Enter');
    const cancelResponse = await cancelResponsePromise;
    expect(cancelResponse.ok(), await cancelResponse.text()).toBe(true);
    await expect.poll(async () => {
      const runs = await commandRuns(request);
      return runs.find(run => run.ID === first)?.Status;
    }).toBe('cancelled');
    await expect(liveRow).toContainText('Cancelled', { timeout: 10_000 });
    await expect(page.locator('[role="status"]').filter({ hasText: `Plugin command cancelled: ${first}` })).toBeVisible();

    const history = liveRow.getByRole('link', { name: 'View command history' });
    await expect(history).toHaveAttribute('href', `/admin/plugin-command-runs?id=${first}`);
    await history.click();
    await expect(page).toHaveURL(new RegExp(`/admin/plugin-command-runs\\?id=${first}$`));
    await expect(page.getByTestId('command-run-status')).toHaveText('cancelled');

    // Leave no process running when the worker-scoped server shuts down.
    const secondCancel = await request.post('/v1/plugin/command-run/cancel', {
      form: { id: second },
      headers: { Accept: 'application/json' },
    });
    expect(secondCancel.ok(), await secondCancel.text()).toBe(true);
  });

  test('terminal and pruned output use the real escaped detail page', async ({ page, request, commandServer }) => {
    const runID = await submitFixture(request, 'hostile-output');
    await expect.poll(async () => {
      const runs = await commandRuns(request);
      return runs.find(run => run.ID === runID)?.Status;
    }).toBe('succeeded');

    await page.goto(`/admin/plugin-command-runs?id=${runID}`);
    const output = page.getByTestId('command-run-output');
    await expect(output).toContainText('<script>window.commandFixtureSecret = true</script>');
    await expect(output).not.toContainText('\u001b[');
    expect(await page.evaluate(() => (window as any).commandFixtureSecret)).toBeUndefined();

    pruneOutput(commandServer.database, runID);
    await page.reload();
    await expect(page.getByTestId('command-run-output-pruned')).toHaveText('Output is no longer available.');
    await expect(page.getByText('The retained command/output row has expired; durable run and import history remains.')).toBeVisible();

    const scan = await new AxeBuilder({ page }).analyze();
    expect(scan.violations).toEqual([]);
  });

  test('unknown detail is a typed not-found response', async ({ request }) => {
    const response = await request.get('/v1/plugin/command-run?id=missing-command-run', {
      headers: { Accept: 'application/json' },
    });
    expect(response.status()).toBe(404);
  });
});
