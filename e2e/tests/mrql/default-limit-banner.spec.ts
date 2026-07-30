/**
 * BH-013: Default MRQL LIMIT is silently applied. The UI shows no affordance
 * that a default limit kicked in, so users see partial results without any
 * signal they should paginate.
 *
 * Fix: MRQL response gains default_limit_applied + applied_limit fields.
 * The mrql editor renders a banner when the default limit actually cut rows off,
 * and no banner when the query supplies an explicit LIMIT.
 *
 * Narrowed by finding 125 (2026-07-29 UI bug hunt): the banner used to render
 * whenever the flag was true, which is *every* query without an explicit LIMIT —
 * so a single-row result carried a full-width warning telling the reader to
 * paginate. It now requires the row count to have reached the applied limit, so
 * the first test below drives the limit down rather than relying on the flag
 * alone.
 */
import { test, expect } from '../../fixtures/base.fixture';

test.describe('BH-013: MRQL default-limit banner', () => {
  test('query without LIMIT shows the default-limit banner once rows are cut off', async ({ page, apiClient }) => {
    // Finding 125: the banner is a truncation warning, so the run has to actually
    // be truncated. Two notes against a default limit of 1 is the smallest fixture
    // that genuinely is — and the notes have to be created here, because the
    // original `type = resource` returned nothing at all on a worker whose server
    // this file never seeds, which is exactly how a truncation assertion passes for
    // the wrong reason.
    const marker = `bh013-banner-${Date.now()}`;
    await apiClient.createNote({ name: `${marker}-a`, description: 'banner fixture' });
    await apiClient.createNote({ name: `${marker}-b`, description: 'banner fixture' });

    await page.request.put('/v1/admin/settings/mrql_default_limit', { data: { value: '1' } });
    await page.goto('/mrql');
    await page.locator('[data-testid="mrql-input"] .cm-editor').waitFor({ state: 'visible', timeout: 15000 });

    // Fill the editor with a LIMIT-less query.
    await page.evaluate((q) => {
      const container = document.querySelector('[data-testid="mrql-input"]') as any;
      const view = container._cmView;
      view.dispatch({ changes: { from: 0, to: view.state.doc.length, insert: q } });
    }, `type = note AND name ~ "${marker}"`);

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

    await page.request.delete('/v1/admin/settings/mrql_default_limit');
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
