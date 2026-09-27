/**
 * What a plugin action's Job says about how it ended: the reason a plugin gave
 * for a failure, and a Cancel for a running handler whose action declares
 * `cancel = true`.
 */
import { test, expect } from '../../fixtures/base.fixture';

type Job = {
  id: string;
  state: string;
  version: number;
  failure?: { code?: string; class?: string; message?: string };
  commands?: Array<{ key: string }>;
};

async function runAction(request: import('@playwright/test').APIRequestContext, action: string): Promise<string> {
  const response = await request.post('/v1/jobs/action/run', {
    data: { plugin: 'test-outcomes', action, entity_ids: [99999], params: {} },
  });
  expect(response.status(), await response.text()).toBe(202);
  const id = (await response.json()).canonicalJobId as string;
  expect(id).toEqual(expect.any(String));
  return id;
}

async function readJob(request: import('@playwright/test').APIRequestContext, id: string): Promise<Job> {
  const response = await request.get(`/v1/jobs/${encodeURIComponent(id)}`);
  expect(response.ok(), await response.text()).toBe(true);
  return response.json();
}

test.describe('plugin action outcomes', () => {
  // One plugin runs one handler at a time, so these share its lane: run in order
  // rather than each waiting behind another test's long action.
  test.describe.configure({ mode: 'serial' });

  test.beforeEach(async ({ apiClient }) => {
    await apiClient.enablePlugin('test-outcomes');
  });

  test('a failed action shows the reason the plugin gave', async ({ page, request }) => {
    const id = await runAction(request, 'fail-demo');
    await expect.poll(async () => (await readJob(request, id)).state, { timeout: 20_000 }).toBe('failed');
    const job = await readJob(request, id);
    expect(job.failure).toMatchObject({
      code: 'plugin-action-failed',
      class: 'dependency',
      message: 'Upstream rejected chunk 6: HTTP 502 Bad Gateway',
    });

    await page.goto(`/job?id=${encodeURIComponent(id)}`);
    const failure = page.getByRole('region', { name: 'Failure' });
    await expect(failure).toContainText('Upstream rejected chunk 6: HTTP 502 Bad Gateway');
    await expect(failure).toContainText('Class: dependency');
  });

  test('a running action that declares cancel can be cancelled from its page', async ({ page, request }) => {
    const id = await runAction(request, 'cancellable-wait');
    await expect.poll(async () => (await readJob(request, id)).state, { timeout: 20_000 }).toBe('running');

    await page.goto(`/job?id=${encodeURIComponent(id)}`);
    const commands = page.getByRole('group', { name: 'Job actions' });
    await commands.getByRole('button', { name: 'Cancel', exact: true }).click();
    const confirmation = page.getByRole('alertdialog');
    await expect(confirmation).toContainText('Stop this plugin action?');
    // The dismiss button does not read "Cancel" too, or the two could not be told
    // apart.
    await expect(confirmation.getByRole('button', { name: 'Go back', exact: true })).toBeVisible();
    await confirmation.getByRole('button', { name: 'Cancel', exact: true }).click();

    await expect.poll(async () => (await readJob(request, id)).state, { timeout: 20_000 }).toBe('cancelled');
  });

  test('a queued action can be cancelled before it starts, whatever it declares', async ({ request }) => {
    const running = await runAction(request, 'cancellable-wait');
    await expect.poll(async () => (await readJob(request, running)).state, { timeout: 20_000 }).toBe('running');
    // Queued behind the running one in the plugin's lane: none of it has run, so
    // it offers Cancel although quick does not declare cancel = true.
    const queued = await runAction(request, 'quick');
    expect((await readJob(request, queued)).state).toBe('queued');

    for (const id of [queued, running]) {
      const job = await readJob(request, id);
      expect(job.commands?.map(command => command.key)).toContain('cancel');
      const key = `plugin-action-outcomes-${id}`;
      const response = await request.post(`/v1/jobs/${encodeURIComponent(id)}/commands/cancel`, {
        headers: { 'Idempotency-Key': key },
        data: { expectedVersion: job.version, idempotencyKey: key },
      });
      expect(response.ok(), await response.text()).toBe(true);
    }
    await expect.poll(async () => (await readJob(request, queued)).state, { timeout: 20_000 }).toBe('cancelled');
    await expect.poll(async () => (await readJob(request, running)).state, { timeout: 20_000 }).toBe('cancelled');
  });
});
