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
      if (path === '/account') {
        await expect(page.getByRole('button', { name: 'Refresh tokens' })).toBeEnabled();
        await expect(page.locator('#tok-name')).toBeEnabled();
      }
      await expectNoViolations(page);
    }

    await page.setViewportSize({ width: 390, height: 844 });
    for (const path of [`/admin/users/edit?id=${userID}`, '/account']) {
      const response = await page.goto(path);
      expect(response?.status(), `${path} should render at mobile viewport`).toBe(200);
      const primaryAction = path === '/account'
        ? page.getByRole('button', { name: 'Update password' })
        : page.getByRole('button', { name: 'Save', exact: true });
      if (path === '/account') {
        await expect(page.getByRole('button', { name: 'Refresh tokens' })).toBeEnabled();
      } else {
        await expect(page.locator('#ue-username')).toBeVisible();
      }
      await expect(primaryAction).toBeVisible();
      const actionBox = await primaryAction.boundingBox();
      expect(actionBox, `${path} primary action should have a layout box`).not.toBeNull();
      expect(actionBox!.x, `${path} primary action should start inside the viewport`).toBeGreaterThanOrEqual(0);
      expect(actionBox!.x + actionBox!.width, `${path} primary action should fit the viewport`).toBeLessThanOrEqual(391);
      const horizontalOverflow = await page.evaluate(() => document.documentElement.scrollWidth - window.innerWidth);
      expect(horizontalOverflow, `${path} should not have page-level horizontal overflow`).toBeLessThanOrEqual(1);
      await expectNoViolations(page);
    }
  });
});
