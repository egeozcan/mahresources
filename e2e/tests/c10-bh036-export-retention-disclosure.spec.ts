/**
 * BH-036: export UI does not disclose the 24 h (default) retention window.
 * Completed tars vanish with no prior warning, compounding BH-025 and BH-026.
 *
 * Fix:
 *   a) Static helper text on /admin/export referencing config.ExportRetention.
 *   b) Per-completed-export expiry timestamp in the downloadCockpit panel.
 */
import { test, expect } from '../fixtures/base.fixture';

test.describe('BH-036: retention disclosure', () => {
  test('/admin/export shows retention helper text', async ({ page }) => {
    await page.goto('/admin/export');
    const helper = page.getByTestId('export-retention-helper');
    await expect(helper).toBeVisible();
    await expect(helper).toContainText(/Completed exports/i);
    await expect(helper).toContainText(/\d+\s*(h|m|hour|min)/i);
  });

  test('cockpit shows expiry timestamp on completed group-export rows', async ({ page, apiClient, baseURL }) => {
    const suffix = Date.now();
    const category = await apiClient.createCategory(`BH036-cat-${suffix}`);
    const group = await apiClient.createGroup({ name: `BH036-grp-${suffix}`, categoryId: category.ID });

    // Submit export via API so completion is deterministic.
    const exportResp = await apiClient.request.post(`${baseURL}/v1/groups/export`, {
      headers: { 'Content-Type': 'application/json' },
      data: JSON.stringify({ rootGroupIds: [group.ID] }),
    });
    expect(exportResp.ok()).toBeTruthy();
    const { jobId } = await exportResp.json();

    // Poll until completion.
    await expect.poll(async () => {
      const resp = await apiClient.request.get(`${baseURL}/v1/jobs/queue`);
      if (!resp.ok()) return null;
      const data = await resp.json();
      const jobs: any[] = data.jobs || [];
      const job = jobs.find((j: any) => j.id === jobId);
      return job?.status ?? null;
    }, { timeout: 30_000, intervals: [500] }).toBe('completed');

    await page.goto('/');
    await page.waitForSelector('[data-testid="cockpit-trigger"]', { timeout: 5000 });
    await page.locator('[data-testid="cockpit-trigger"]').click();

    const panel = page.locator('[data-testid="cockpit-panel"]');
    await expect(panel).toBeVisible({ timeout: 5000 });

    // Wait for the SSE init payload.
    await expect(
      panel.locator('[data-testid="cockpit-connection-status"][aria-label*="connected"]')
    ).toBeVisible({ timeout: 10000 });

    // Find the row for the specific job via its download link, then inspect siblings for the expiry line.
    const downloadLink = panel.locator(`a[href*="/exports/${jobId}/download"]`);
    await expect(downloadLink).toBeVisible({ timeout: 5000 });

    const expiryText = await downloadLink.evaluate((el) => {
      const row = el.closest('[data-testid="cockpit-job"]');
      if (!row) return null;
      const expiry = row.querySelector('[data-testid="cockpit-job-expiry"]');
      return expiry?.textContent?.trim() ?? null;
    });
    expect(expiryText, 'expected expiry row on completed group-export').toBeTruthy();
    expect(expiryText).toMatch(/expire/i);
  });
});
