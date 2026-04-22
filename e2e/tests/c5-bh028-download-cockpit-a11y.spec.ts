import { test, expect } from '../fixtures/base.fixture';
import AxeBuilder from '@axe-core/playwright';

test.describe('BH-028: download cockpit panel a11y', () => {
  test('panel has dialog ARIA and initial focus lands inside', async ({ page }) => {
    await page.goto('/');
    await page.waitForSelector('[data-testid="cockpit-trigger"]', { timeout: 5000 });
    await page.locator('[data-testid="cockpit-trigger"]').click();

    const panel = page.locator('[data-testid="cockpit-panel"]');
    await expect(panel).toBeVisible({ timeout: 5000 });
    await expect(panel).toHaveAttribute('role', 'dialog');
    await expect(panel).toHaveAttribute('aria-modal', 'true');

    // Focus must land inside the panel on open
    const focusInsidePanel = await page.evaluate(() => {
      return document.activeElement?.closest('[data-testid="cockpit-panel"]') !== null;
    });
    expect(focusInsidePanel, 'focus must move into the panel on open').toBe(true);
  });

  test('connection status has accessible name', async ({ page }) => {
    await page.goto('/');
    await page.waitForSelector('[data-testid="cockpit-trigger"]', { timeout: 5000 });
    await page.locator('[data-testid="cockpit-trigger"]').click();

    const panel = page.locator('[data-testid="cockpit-panel"]');
    await expect(panel).toBeVisible({ timeout: 5000 });

    const dot = panel.locator('[data-testid="cockpit-connection-status"]');
    await expect(dot).toBeVisible();
    const ariaLabel = await dot.getAttribute('aria-label');
    expect(ariaLabel).toBeTruthy();
    expect(ariaLabel).toMatch(/connection/i);
  });

  test('axe finds zero Serious+ violations on open panel', async ({ page }) => {
    await page.goto('/');
    await page.waitForSelector('[data-testid="cockpit-trigger"]', { timeout: 5000 });
    await page.locator('[data-testid="cockpit-trigger"]').click();

    await page.waitForSelector('[data-testid="cockpit-panel"]', { timeout: 5000 });

    const scan = await new AxeBuilder({ page })
      .include('[data-testid="cockpit-panel"]')
      .disableRules(['region'])
      .analyze();

    const seriousPlus = scan.violations.filter(
      (v) => v.impact === 'serious' || v.impact === 'critical'
    );
    if (seriousPlus.length > 0) {
      console.error('Axe violations:', JSON.stringify(seriousPlus, null, 2));
    }
    expect(seriousPlus).toEqual([]);
  });
});
