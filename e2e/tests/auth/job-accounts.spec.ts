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
    const row = admin.locator(`[data-job-panel-row][data-job-id="${jobId}"]`);
    await expect(row.locator('[data-job-panel-owner]')).toHaveText(`Owner: ${authSeed.user.username}`);

    // The administrator retries the user's job; the successor is theirs.
    const detail = await (await admin.request.get(`/v1/jobs/${encodeURIComponent(jobId)}`)).json();
    const retried = await admin.request.post(`/v1/jobs/${encodeURIComponent(jobId)}/commands/retry`, {
      headers: { 'X-CSRF-Token': await csrfOf(admin) },
      data: { expectedVersion: detail.version, idempotencyKey: `retry-${name}`, origin: 'ui' },
    });
    expect(retried.ok(), await retried.text()).toBe(true);

    await user.goto(`/job?id=${encodeURIComponent(jobId)}`);
    await expect(user.locator('[data-job-retried-elsewhere]')).toBeVisible();
    await expect(user.getByRole('group', { name: 'Advertised job commands' }).getByRole('button', { name: 'Retry' })).toHaveCount(0);
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
