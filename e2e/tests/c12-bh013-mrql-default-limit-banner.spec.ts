/**
 * BH-013: Default MRQL LIMIT is silently applied. The UI shows no affordance
 * that a default limit kicked in, so users see partial results without any
 * signal they should paginate.
 *
 * Fix: MRQL response gains default_limit_applied + applied_limit fields.
 * The mrql editor renders a banner whenever the flag is true, and no banner
 * when the query supplies an explicit LIMIT.
 */
import { test, expect } from '../fixtures/base.fixture';

test.describe('BH-013: MRQL default-limit banner', () => {
  test('query without LIMIT shows the default-limit banner', async ({ page }) => {
    await page.goto('/mrql');
    await page.locator('[data-testid="mrql-input"] .cm-editor').waitFor({ state: 'visible', timeout: 15000 });

    // Fill the editor with a LIMIT-less query.
    await page.evaluate(() => {
      const container = document.querySelector('[data-testid="mrql-input"]') as any;
      const view = container._cmView;
      view.dispatch({ changes: { from: 0, to: view.state.doc.length, insert: 'type = resource' } });
    });

    // Click Run.
    const runBtn = page.getByRole('button', { name: /^Run/ });
    await runBtn.click();

    // Wait for execution to complete.
    await expect(runBtn).toContainText('Run', { timeout: 15000 });

    // Banner should appear.
    const banner = page.getByTestId('mrql-default-limit-banner');
    await expect(banner).toBeVisible({ timeout: 5000 });
    await expect(banner).toContainText(/Default limit applied/);
    await expect(banner).toContainText(/LIMIT/);
  });

  test('query WITH explicit LIMIT does not show the banner', async ({ page }) => {
    await page.goto('/mrql');
    await page.locator('[data-testid="mrql-input"] .cm-editor').waitFor({ state: 'visible', timeout: 15000 });

    await page.evaluate(() => {
      const container = document.querySelector('[data-testid="mrql-input"]') as any;
      const view = container._cmView;
      view.dispatch({ changes: { from: 0, to: view.state.doc.length, insert: 'type = resource LIMIT 5' } });
    });

    const runBtn = page.getByRole('button', { name: /^Run/ });
    await runBtn.click();
    await expect(runBtn).toContainText('Run', { timeout: 15000 });

    // Give the banner template a moment to settle, then assert it is absent.
    await page.waitForTimeout(300);
    const banner = page.getByTestId('mrql-default-limit-banner');
    await expect(banner).toBeHidden();
  });
});
