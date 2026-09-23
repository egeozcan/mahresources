import { test, expect, loginAs, type RoleCreds } from '../../fixtures/auth.fixture';
import type { APIRequestContext, BrowserContext, Page } from '@playwright/test';

const DEAD_URL = 'http://127.0.0.1:9/';

async function submitJob(page: Page, groupId: number, name: string): Promise<string> {
  const me = await page.request.get('/v1/auth/me');
  expect(me.ok()).toBe(true);
  const csrf = (await me.json()).csrfToken as string;
  const response = await page.request.post('/v1/download/submit', {
    headers: { 'X-CSRF-Token': csrf },
    data: { URL: `${DEAD_URL}${name}`, OwnerId: groupId, Name: name },
  });
  expect(response.status(), await response.text()).toBe(202);
  const body = await response.json();
  return body.jobs[0].canonicalJobId as string;
}

async function loginAndSubmit(
  context: BrowserContext,
  creds: RoleCreds,
  groupId: number,
  name: string,
): Promise<{ page: Page; jobId: string }> {
  const page = await context.newPage();
  await loginAs(page, creds);
  const jobId = await submitJob(page, groupId, name);
  return { page, jobId };
}

async function getJobState(request: APIRequestContext, id: string) {
  const response = await request.get(`/v1/jobs/${encodeURIComponent(id)}`);
  if (!response.ok()) return null;
  return (await response.json()).state as string;
}

test('Job Center lists only the current user’s jobs while administrators can inspect both', async ({ browser, baseURL, authSeed }) => {
  const suffix = Date.now();
  const adminName = `admin-owned-job-${suffix}.bin`;
  const userName = `user-owned-job-${suffix}.bin`;
  const adminContext = await browser.newContext({ baseURL });
  const userContext = await browser.newContext({ baseURL });
  try {
    const admin = await loginAndSubmit(adminContext, authSeed.admin, authSeed.scopeGroupId, adminName);
    const user = await loginAndSubmit(userContext, authSeed.user, authSeed.scopeGroupId, userName);

    const userMe = await user.page.request.get('/v1/auth/me');
    const userID = (await userMe.json()).userId as number;
    const adminMe = await admin.page.request.get('/v1/auth/me');
    const adminID = (await adminMe.json()).userId as number;
    expect(userID).not.toBe(adminID);

    await user.page.goto('/jobs?view=all');
    await expect(user.page.locator(`[data-testid="job-center"] [data-job-id="${user.jobId}"]`)).toBeVisible();
    await expect(user.page.locator(`[data-testid="job-center"] [data-job-id="${admin.jobId}"]`)).toHaveCount(0);
    await expect.poll(() => getJobState(user.page.request, admin.jobId)).toBeNull();

    await admin.page.goto('/jobs?view=all');
    await expect(admin.page.locator(`[data-testid="job-center"] [data-job-id="${admin.jobId}"]`)).toBeVisible();
    await expect(admin.page.locator(`[data-testid="job-center"] [data-job-id="${user.jobId}"]`)).toBeVisible();
    await expect.poll(() => getJobState(admin.page.request, user.jobId)).not.toBeNull();
  } finally {
    await adminContext.close();
    await userContext.close();
  }
});
