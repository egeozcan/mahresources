/**
 * BH-015: export progress label overflows 100% for small-payload exports.
 *
 * Backend reports progressPercent = (bytes_written / totalSize) * 100,
 * where totalSize counts only unique blob bytes but bytes_written counts
 * everything in the tar (manifest + JSONs + padding). For a small export
 * (e.g., 1 tiny image) this blows past 100% — often reads "5140%".
 *
 * Fix: (a) clamp both label sites (adminExport.tpl and downloadCockpit.js)
 * to Math.min(100, ...). (b) Improve backend totalBytes estimate to
 * include JSON overhead so the raw number is accurate, not merely clamped.
 *
 * A separate Go unit test covers the backend estimate helper.
 */
import path from 'path';
import { test, expect } from '../fixtures/base.fixture';

test.describe('BH-015: progress label caps at 100%', () => {
  test('small export shows label <= 100% in adminExport', async ({ page, apiClient, baseURL }) => {
    const suffix = Date.now();
    const category = await apiClient.createCategory(`BH015-cat-${suffix}`);
    const group = await apiClient.createGroup({ name: `BH015-grp-${suffix}`, categoryId: category.ID });
    // Upload 1 tiny image so totalSize is small but tar overhead dominates.
    await apiClient.createResource({
      name: `BH015-r-${suffix}`,
      filePath: path.join(__dirname, '../test-assets/sample-image.png'),
      ownerId: group.ID,
    });

    await page.goto(`/admin/export?groups=${group.ID}`);
    // Wait for the pre-selected group chip to appear so submit is enabled.
    await expect(page.locator('[data-testid="export-group-chips"] span').first()).toBeVisible({ timeout: 5000 });

    await page.locator('[data-testid="export-submit-button"]').click();

    // Progress panel should appear immediately.
    await expect(page.locator('[data-testid="export-progress-panel"]')).toBeVisible({ timeout: 5000 });

    // Wait for completion by polling the jobs queue.
    await expect.poll(async () => {
      const resp = await apiClient.request.get(`${baseURL}/v1/jobs/queue`);
      if (!resp.ok()) return null;
      const data = await resp.json();
      const jobs: any[] = data.jobs || [];
      // Pick the most recent group-export job (there should only be one in this test).
      const job = jobs.filter((j: any) => j.source === 'group-export').pop();
      return job?.status ?? null;
    }, { timeout: 30_000, intervals: [500] }).toBe('completed');

    // Parse the "(N%)" badge text on the admin-export page.
    const bytesCounter = page.locator('[data-testid="export-bytes-counter"]');
    await expect(bytesCounter).toBeVisible();
    const text = await bytesCounter.textContent();
    const match = text?.match(/\((\d+)%\)/);
    expect(match, `expected (N%) in "${text}"`).not.toBeNull();
    const percent = parseInt(match![1], 10);
    expect(percent).toBeLessThanOrEqual(100);
    expect(percent).toBeGreaterThanOrEqual(0);
  });

  test('formatProgress in cockpit caps at 100% for overshoot input', async ({ page }) => {
    await page.goto('/');
    // The downloadCockpit factory is re-exposed via window.downloadCockpit (main.js) so we can unit-check formatProgress.
    const result = await page.evaluate(() => {
      const fn = (window as any).downloadCockpit;
      if (typeof fn !== 'function') return { error: 'window.downloadCockpit factory not found' };
      const inst = fn();
      return {
        ok: true,
        result: inst.formatProgress({ totalSize: 352, progress: 18096, progressPercent: 5140.9 }),
      };
    });
    expect(result).toMatchObject({ ok: true });
    // Cap at 100.0 — capped format still uses one decimal.
    expect((result as any).result).toMatch(/\(100\.0%\)/);
  });
});
