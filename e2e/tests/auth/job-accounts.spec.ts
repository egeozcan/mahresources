import { test, expect, loginAs } from '../../fixtures/auth.fixture';
import type { Page } from '@playwright/test';

// A loopback address every fetch policy refuses, so the download fails at once.
const DEAD_URL = 'http://127.0.0.1:9/';

async function csrfOf(page: Page): Promise<string> {
  const me = await page.request.get('/v1/auth/me');
  expect(me.ok()).toBe(true);
  return (await me.json()).csrfToken as string;
}

async function failedDownload(page: Page, groupId: number, name: string): Promise<string> {
  const response = await page.request.post('/v1/download/submit', {
    headers: { 'X-CSRF-Token': await csrfOf(page) },
    data: { URL: `${DEAD_URL}${name}`, OwnerId: groupId, Name: name },
  });
  expect(response.status(), await response.text()).toBe(202);
  const jobId = (await response.json()).jobs[0].canonicalJobId as string;
  await expect.poll(async () => {
    const detail = await page.request.get(`/v1/jobs/${encodeURIComponent(jobId)}`);
    return detail.ok() ? (await detail.json()).state : null;
  }, { timeout: 15000 }).toBe('failed');
  return jobId;
}

test('an administrator sees whose job is whose, and an owner is told when another account retried theirs', async ({ browser, baseURL, authSeed }) => {
  const name = `owner-named-${Date.now()}.bin`;
  const userContext = await browser.newContext({ baseURL });
  const adminContext = await browser.newContext({ baseURL });
  try {
    const user = await userContext.newPage();
    await loginAs(user, authSeed.user);
    const jobId = await failedDownload(user, authSeed.scopeGroupId, name);

    const admin = await adminContext.newPage();
    await loginAs(admin, authSeed.admin);

    await admin.goto('/jobs?view=all&dismissed=any');
    const card = admin.locator(`[data-testid="job-center"] [data-job-id="${jobId}"]`);
    await expect(card.getByTestId('job-owner')).toContainText(authSeed.user.username);

    await admin.goto(`/job?id=${encodeURIComponent(jobId)}`);
    await expect(admin.locator('[data-job-owner] dd')).toHaveText(authSeed.user.username);

    await admin.getByRole('button', { name: 'Open Jobs panel' }).click();
    const drawer = admin.getByRole('dialog', { name: 'Jobs' });
    await drawer.getByText('Everyone\'s', { exact: true }).click();
    const row = admin.locator(`[data-job-panel-row][data-job-id="${jobId}"]`);
    await expect(row.locator('[data-job-panel-owner]')).toHaveText(`Owner: ${authSeed.user.username}`);
    await drawer.getByText('My jobs', { exact: true }).click();

    // The administrator retries the user's job; the successor is theirs.
    const detail = await (await admin.request.get(`/v1/jobs/${encodeURIComponent(jobId)}`)).json();
    const retried = await admin.request.post(`/v1/jobs/${encodeURIComponent(jobId)}/commands/retry`, {
      headers: { 'X-CSRF-Token': await csrfOf(admin) },
      data: { expectedVersion: detail.version, idempotencyKey: `retry-${name}`, origin: 'ui' },
    });
    expect(retried.ok(), await retried.text()).toBe(true);

    await user.goto(`/job?id=${encodeURIComponent(jobId)}`);
    await expect(user.locator('[data-job-retried-elsewhere]')).toBeVisible();
    await expect(user.getByRole('group', { name: 'Job actions' }).getByRole('button', { name: 'Retry', exact: true })).toHaveCount(0);
    // An owner's own drawer names no owner on their own rows.
    await user.getByRole('button', { name: 'Open Jobs panel' }).click();
    await expect(user.locator(`[data-job-panel-row][data-job-id="${jobId}"] [data-job-panel-owner]`)).toBeHidden();
  } finally {
    await userContext.close();
    await adminContext.close();
  }
});

test('an administrator listing their own jobs keeps that choice when they change another filter', async ({ browser, baseURL, authSeed }) => {
  const stamp = Date.now();
  const userContext = await browser.newContext({ baseURL });
  const adminContext = await browser.newContext({ baseURL });
  try {
    const user = await userContext.newPage();
    await loginAs(user, authSeed.user);
    const theirs = await failedDownload(user, authSeed.scopeGroupId, `mine-theirs-${stamp}.bin`);

    const admin = await adminContext.newPage();
    await loginAs(admin, authSeed.admin);
    const mine = await failedDownload(admin, authSeed.scopeGroupId, `mine-own-${stamp}.bin`);

    const list = admin.locator('[data-testid="job-center"]');
    await admin.goto('/jobs?owner=me&dismissed=any');
    const form = admin.getByRole('form', { name: 'Filter jobs' });
    await expect(form.getByRole('combobox', { name: 'Owner' })).toHaveValue('me');
    await expect(list.locator(`[data-job-id="${mine}"]`)).toHaveCount(1);
    await expect(list.locator(`[data-job-id="${theirs}"]`)).toHaveCount(0);

    await form.getByRole('checkbox', { name: 'failed' }).check();
    await form.getByRole('button', { name: 'Apply Filters' }).click();
    await expect(admin).toHaveURL(/[?&]state=failed/);
    expect(new URL(admin.url()).searchParams.get('owner')).toBe('me');
    await expect(form.getByRole('combobox', { name: 'Owner' })).toHaveValue('me');
    await expect(list.locator(`[data-job-id="${mine}"]`)).toHaveCount(1);
    await expect(list.locator(`[data-job-id="${theirs}"]`)).toHaveCount(0);
  } finally {
    await userContext.close();
    await adminContext.close();
  }
});

test('an administrator\'s drawer lists their own jobs until they choose everyone\'s, and remembers the choice', async ({ browser, baseURL, authSeed }) => {
  const stamp = Date.now();
  const userContext = await browser.newContext({ baseURL });
  const adminContext = await browser.newContext({ baseURL });
  try {
    const user = await userContext.newPage();
    await loginAs(user, authSeed.user);
    const theirs = await failedDownload(user, authSeed.scopeGroupId, `drawer-theirs-${stamp}.bin`);
    const admin = await adminContext.newPage();
    await loginAs(admin, authSeed.admin);
    const mine = await failedDownload(admin, authSeed.scopeGroupId, `drawer-mine-${stamp}.bin`);

    await admin.goto('/dashboard');
    await admin.getByRole('button', { name: 'Open Jobs panel' }).click();
    const drawer = admin.getByRole('dialog', { name: 'Jobs' });
    const choice = drawer.getByRole('group', { name: 'Whose jobs to show' });
    // Their own work by default: another account's failure does not ask this
    // administrator for attention.
    await expect(choice.getByRole('radio', { name: 'My jobs' })).toBeChecked();
    await expect(drawer.locator(`[data-job-panel-row][data-job-id="${mine}"]`)).toHaveCount(1);
    await expect(drawer.locator(`[data-job-panel-row][data-job-id="${theirs}"]`)).toHaveCount(0);

    // A radio pair: the keyboard moves the choice with the arrow keys.
    await choice.getByRole('radio', { name: 'My jobs' }).focus();
    await admin.keyboard.press('ArrowRight');
    await expect(choice.getByRole('radio', { name: 'Everyone\'s' })).toBeChecked();
    await expect(drawer.locator(`[data-job-panel-row][data-job-id="${theirs}"]`)).toHaveCount(1);

    // Kept for the next page, opened straight away.
    await admin.reload();
    await admin.getByRole('button', { name: 'Open Jobs panel' }).click();
    await expect(choice.getByRole('radio', { name: 'Everyone\'s' })).toBeChecked();
    await expect(drawer.locator(`[data-job-panel-row][data-job-id="${theirs}"]`)).toHaveCount(1);

    // Switched back and away in the same moment: the next page still lists
    // only this administrator's jobs, and the choice is stored.
    await choice.getByText('My jobs', { exact: true }).click();
    await admin.goto('/dashboard');
    await admin.getByRole('button', { name: 'Open Jobs panel' }).click();
    await expect(choice.getByRole('radio', { name: 'My jobs' })).toBeChecked();
    await expect(drawer.locator(`[data-job-panel-row][data-job-id="${mine}"]`)).toHaveCount(1);
    await expect(drawer.locator(`[data-job-panel-row][data-job-id="${theirs}"]`)).toHaveCount(0);
    await expect.poll(async () => (await (await admin.request.get('/v1/account/settings')).json()).jobsPanelScope).toBe('mine');

    // Only an administrator is offered the choice: anyone else sees only their own.
    await user.goto('/dashboard');
    await user.getByRole('button', { name: 'Open Jobs panel' }).click();
    await expect(user.getByRole('dialog', { name: 'Jobs' }).getByRole('group', { name: 'Whose jobs to show' })).toHaveCount(0);
  } finally {
    await userContext.close();
    await adminContext.close();
  }
});

test('an owner\'s open drawer lets go of a failure another account retried', async ({ browser, baseURL, authSeed }) => {
  const name = `retried-elsewhere-${Date.now()}.bin`;
  const userContext = await browser.newContext({ baseURL });
  const adminContext = await browser.newContext({ baseURL });
  try {
    const user = await userContext.newPage();
    await loginAs(user, authSeed.user);
    const jobId = await failedDownload(user, authSeed.scopeGroupId, name);
    await user.goto('/dashboard');
    await user.getByRole('button', { name: 'Open Jobs panel' }).click();
    const attention = user.locator(`[data-job-panel-group="attention"] [data-job-panel-row][data-job-id="${jobId}"]`);
    await expect(attention).toHaveCount(1);
    // Every event of the failure has reached the drawer and no list read is
    // pending, so what follows can only come from the retry.
    await expect.poll(async () => {
      const events = (await (await user.request.get(`/v1/jobs/${encodeURIComponent(jobId)}/events`)).json()).events as Array<{ deliverySequence?: number }>;
      return events.length > 0 && events.every(event => event.deliverySequence);
    }, { timeout: 15_000 }).toBe(true);
    await user.waitForTimeout(2500);
    await expect.poll(() => user.evaluate(() => {
      const panel = (window as any).Alpine.$data(document.querySelector('[data-testid="job-panel-root"]'));
      return !!panel.streamCaughtUp && !panel._panelRefreshPromise && !panel._panelRefreshTimer && !panel._panelRefreshMaxTimer;
    }), { timeout: 15_000 }).toBe(true);

    const admin = await adminContext.newPage();
    await loginAs(admin, authSeed.admin);
    const detail = await (await admin.request.get(`/v1/jobs/${encodeURIComponent(jobId)}`)).json();
    const retried = await admin.request.post(`/v1/jobs/${encodeURIComponent(jobId)}/commands/retry`, {
      headers: { 'X-CSRF-Token': await csrfOf(admin) },
      data: { expectedVersion: detail.version, idempotencyKey: `retry-${name}`, origin: 'ui' },
    });
    expect(retried.ok(), await retried.text()).toBe(true);

    // Nothing on the owner's page changes or reloads: the retried Job tells
    // its viewers, and the drawer reads its lists again.
    await expect(attention).toHaveCount(0, { timeout: 15_000 });
  } finally {
    await userContext.close();
    await adminContext.close();
  }
});
