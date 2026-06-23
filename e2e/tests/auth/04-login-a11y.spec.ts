import { test } from '../../fixtures/auth.fixture';
import { expectNoViolations } from '../../helpers/accessibility/axe-helper';

test.describe('auth: login page accessibility', () => {
  test('login page has no WCAG violations', async ({ page }) => {
    await page.goto('/login');
    await page.waitForLoadState('load');
    await expectNoViolations(page);
  });

  test('login page with an error has no WCAG violations', async ({ page }) => {
    await page.goto('/login?error=1');
    await page.waitForLoadState('load');
    await expectNoViolations(page);
  });
});
