import { test, expect } from '../../fixtures/a11y.fixture';

test.describe('Job Center panel accessibility', () => {
  async function openPanel(page: import('@playwright/test').Page) {
    await page.goto('/dashboard');
    const trigger = page.getByRole('button', { name: 'Open Jobs panel' });
    await expect(trigger).toBeVisible();
    await trigger.click();
    const panel = page.getByRole('dialog', { name: 'Jobs' });
    await expect(panel).toBeVisible();
    return { trigger, panel };
  }

  test('the panel has dialog semantics and moves focus inside on open', async ({ page }) => {
    const { panel } = await openPanel(page);
    await expect(panel).toHaveAttribute('aria-modal', 'true');
    await expect.poll(() => page.evaluate(() =>
      document.activeElement?.closest('#job-center-panel') !== null,
    )).toBe(true);
  });

  test('the close control and status announcement have accessible names', async ({ page }) => {
    const { panel } = await openPanel(page);
    const close = panel.getByRole('button', { name: 'Close Jobs panel', exact: true });
    await expect(close).toBeVisible();
    await expect(close).toHaveAttribute('aria-label', 'Close Jobs panel');

    const status = panel.getByRole('status');
    await expect(status).toBeVisible();
    await expect(status).toHaveAttribute('aria-live', 'polite');
    await expect(status).not.toBeEmpty();
  });

  test('axe finds no serious or critical violations in the open panel', async ({ page, checkComponentA11y }) => {
    await openPanel(page);
    await checkComponentA11y('#job-center-panel', {
      // The dialog uses local header/footer sections; these are not the page's
      // banner or contentinfo landmarks.
      disableRules: ['landmark-no-duplicate-banner', 'landmark-no-duplicate-contentinfo'],
    });
  });

  test('decorative icons in the trigger and close control are hidden from assistive technology', async ({ page }) => {
    const { trigger, panel } = await openPanel(page);
    await expect(trigger.locator('svg')).toHaveAttribute('aria-hidden', 'true');
    await expect(panel.getByRole('button', { name: 'Close Jobs panel' }).locator('svg'))
      .toHaveAttribute('aria-hidden', 'true');
  });
});
