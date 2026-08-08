import type { Page } from '@playwright/test';
import { test, expect, loginAs, type AuthSeed, type RoleCreds } from '../../fixtures/auth.fixture';
import { acceptConfirm, dismissConfirm } from '../../helpers/confirm-dialog';

async function createOwnEditor(page: Page, authSeed: AuthSeed, label: string): Promise<RoleCreds> {
  await loginAs(page, authSeed.admin);
  const csrf = (await (await page.request.get('/v1/auth/me')).json()).csrfToken as string;
  const creds: RoleCreds = {
    username: `${label}_${Date.now()}_${Math.floor(Math.random() * 1e6)}`,
    password: 'password1',
    role: 'editor',
  };
  const response = await page.request.post('/v1/users', {
    headers: { 'X-CSRF-Token': csrf },
    data: creds,
  });
  expect(response.ok(), `creating account: ${response.status()} ${await response.text()}`).toBe(true);
  await page.context().clearCookies();
  await loginAs(page, creds);
  return creds;
}

async function createToken(page: Page, name: string): Promise<string> {
  const csrf = (await (await page.request.get('/v1/auth/me')).json()).csrfToken as string;
  const response = await page.request.post('/v1/account/tokens', {
    headers: { 'X-CSRF-Token': csrf },
    data: { name },
  });
  expect(response.ok(), `creating token ${name}: ${response.status()}`).toBe(true);
  return ((await response.json()).token as string);
}

test.describe('auth: account management', () => {
  test('duplicate token creation is blocked and a recoverable error is announced', async ({ page, authSeed }) => {
    await createOwnEditor(page, authSeed, 'token_state');
    await page.goto('/account');

    let requests = 0;
    await page.route('**/v1/account/tokens', async (route, request) => {
      if (request.method() !== 'POST') {
        await route.continue();
        return;
      }
      requests += 1;
      await new Promise((resolve) => setTimeout(resolve, 250));
      await route.continue();
    });

    await page.locator('#tok-name').fill('rapid laptop');
    const create = page.getByRole('button', { name: 'Create' });
    await Promise.all([
      create.dispatchEvent('click'),
      create.dispatchEvent('click'),
    ]);
    const status = page.locator('[role="status"]', { hasText: /copy it now/i });
    await expect(status).toBeVisible();
    await expect(status.locator('code')).not.toBeEmpty();
    expect(requests).toBe(1);

    await page.getByRole('button', { name: 'I have saved this token' }).click();
    await expect(status).toBeHidden();
    await page.unroute('**/v1/account/tokens');

    let rejectOnce = true;
    await page.route('**/v1/account/tokens', async (route, request) => {
      if (request.method() === 'POST' && rejectOnce) {
        rejectOnce = false;
        await route.fulfill({
          status: 409,
          contentType: 'application/json',
          body: JSON.stringify({ errors: [{ message: 'token limit reached for this test' }] }),
        });
        return;
      }
      await route.continue();
    });

    await page.locator('#tok-name').fill('recovery token');
    await page.getByRole('button', { name: 'Create' }).click();
    const alert = page.locator('[role="alert"]', { hasText: 'token limit reached for this test' });
    await expect(alert).toBeVisible();
    await expect(page.getByRole('button', { name: 'Create' })).toBeEnabled();

    await page.getByRole('button', { name: 'Create' }).click();
    await expect(alert).toBeHidden();
    await expect(status).toBeVisible();
    await expect(page.locator('tbody')).toContainText('recovery token');
  });

  test('successful self-password change preserves this session, revokes a peer session, and preserves API tokens', async ({ page, browser, authSeed }) => {
    const creds = await createOwnEditor(page, authSeed, 'password_success');
    const rawToken = await createToken(page, 'password-change-positive-control');
    const baseURL = new URL(page.url()).origin;
    const peerContext = await browser.newContext({ baseURL });
    const peerPage = await peerContext.newPage();
    await loginAs(peerPage, creds);

    await page.goto('/account');
    await page.locator('#cur-pw').fill(creds.password);
    await page.locator('#new-pw').fill('newpassword1');
    await page.locator('#confirm-pw').fill('newpassword1');
    const endpointRequest = page.waitForRequest((request) => request.method() === 'POST' && request.url().endsWith('/v1/account/password'));
    await page.getByRole('button', { name: 'Update password' }).click();
    await endpointRequest;

    await expect(page.getByText('Password updated. Other browser sessions were signed out; this session remains active.')).toBeVisible();
    expect((await page.request.get('/v1/auth/me')).status(), 'current browser session').toBe(200);
    expect((await peerPage.request.get('/v1/auth/me')).status(), 'peer browser session').toBe(401);
    const tokenResponse = await page.request.get('/v1/auth/me', {
      headers: { Authorization: `Bearer ${rawToken}` },
    });
    expect(tokenResponse.status(), 'existing API token').toBe(200);
    await peerContext.close();
  });

  test('password confirmation reports a mismatch without submitting', async ({ page, authSeed }) => {
    await createOwnEditor(page, authSeed, 'password_confirm');
    await page.goto('/account');

    let passwordRequests = 0;
    page.on('request', (request) => {
      if (request.method() === 'POST' && request.url().endsWith('/v1/account/password')) passwordRequests += 1;
    });

    await page.locator('#cur-pw').fill('password1');
    await page.locator('#new-pw').fill('newpassword1');
    await page.locator('#confirm-pw').fill('different1');
    await page.getByRole('button', { name: 'Update password' }).click();

    await expect(page.locator('#account-password-mismatch')).toHaveText(/do not match/i);
    expect(passwordRequests).toBe(0);
  });

  test('keyboard token revocation names and removes only the selected token', async ({ page, authSeed }) => {
    await createOwnEditor(page, authSeed, 'token_revoke');
    const first = `desktop ${Date.now()}`;
    const second = `automation ${Date.now()}`;
    await createToken(page, first);
    await createToken(page, second);
    await page.goto('/account');

    const firstButton = page.getByRole('button', { name: `Revoke API token ${first}` });
    const secondButton = page.getByRole('button', { name: `Revoke API token ${second}` });
    await expect(firstButton).toBeVisible();
    await expect(secondButton).toBeVisible();

    await secondButton.focus();
    await page.keyboard.press('Enter');
    const declined = await dismissConfirm(page);
    expect(declined).toContain(second);
    expect(declined).not.toContain(first);
    await expect(secondButton).toBeFocused();

    await page.keyboard.press('Enter');
    const accepted = await acceptConfirm(page);
    expect(accepted).toContain(second);
    await page.waitForLoadState('load');

    await expect(page.getByRole('button', { name: `Revoke API token ${second}` })).toHaveCount(0);
    await expect(page.getByRole('button', { name: `Revoke API token ${first}` })).toBeVisible();
  });
});
