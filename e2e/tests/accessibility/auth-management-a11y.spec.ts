import { test, expect, loginAs } from '../../fixtures/auth.fixture';
import { expectNoViolations } from '../../helpers/accessibility/axe-helper';

test.describe('auth management accessibility', () => {
  test('admin users, user edit, and account pages pass axe', async ({ page, authSeed }) => {
    await loginAs(page, authSeed.admin);
    const csrf = (await (await page.request.get('/v1/auth/me')).json()).csrfToken as string;
    const suffix = `${Date.now()}_${Math.floor(Math.random() * 1e6)}`;

    const userResponse = await page.request.post('/v1/users', {
      headers: { 'X-CSRF-Token': csrf },
      data: {
        username: `axe_account_${suffix}`,
        password: 'password1',
        role: 'editor',
      },
    });
    expect(userResponse.ok(), `creating axe account: ${userResponse.status()}`).toBe(true);
    const userID = (await userResponse.json()).ID as number;

    const tokenResponse = await page.request.post('/v1/account/tokens', {
      headers: { 'X-CSRF-Token': csrf },
      data: { name: `axe token ${suffix}` },
    });
    expect(tokenResponse.ok(), `creating axe token: ${tokenResponse.status()}`).toBe(true);

    for (const path of ['/admin/users', `/admin/users/edit?id=${userID}`, '/account']) {
      const response = await page.goto(path);
      expect(response?.status(), `${path} should render`).toBe(200);
      await page.waitForLoadState('load');
      await page.locator('body[x-data-ready="true"], body').first().waitFor({ state: 'visible' });
      await expectNoViolations(page);
    }
  });
});
