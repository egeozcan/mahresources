/**
 * BH-036: export UI does not disclose the 24 h (default) retention window.
 * Completed tars vanish with no prior warning, compounding BH-025 and BH-026.
 *
 * Fix:
 *   a) Static helper text on /admin/export referencing config.ExportRetention.
 *   b) Per-completed-export expiry timestamp in Job Center detail.
 */
import { test, expect } from '../../fixtures/base.fixture';

test.describe('BH-036: retention disclosure', () => {
  test('/admin/export shows retention helper text', async ({ page }) => {
    await page.goto('/admin/export');
    const helper = page.getByTestId('export-retention-helper');
    await expect(helper).toBeVisible();
    await expect(helper).toContainText(/Completed exports/i);
    await expect(helper).toContainText(/\d+\s*(h|m|hour|min)/i);
  });

  test('Job Center detail shows expiry timestamp on completed group-export outputs', async ({ page, apiClient, baseURL }) => {
    const suffix = Date.now();
    const category = await apiClient.createCategory(`BH036-cat-${suffix}`);
    const group = await apiClient.createGroup({ name: `BH036-grp-${suffix}`, categoryId: category.ID });

    // Submit export via API so completion is deterministic.
    const exportResp = await apiClient.request.post(`${baseURL}/v1/groups/export`, {
      headers: { 'Content-Type': 'application/json' },
      data: JSON.stringify({ rootGroupIds: [group.ID] }),
    });
    expect(exportResp.ok()).toBeTruthy();
    const { canonicalJobId } = await exportResp.json();
    expect(canonicalJobId).toEqual(expect.any(String));

    // Poll the canonical record until the completed output is available.
    await expect.poll(async () => {
      const resp = await apiClient.request.get(`${baseURL}/v1/jobs/${encodeURIComponent(canonicalJobId)}`);
      if (!resp.ok()) return null;
      const job = await resp.json();
      return job.state === 'succeeded' && job.outputs?.some((output: any) => output.expiresAt)
        ? job.state
        : null;
    }, { timeout: 30_000, intervals: [500] }).toBe('succeeded');

    await page.goto(`/job?id=${encodeURIComponent(canonicalJobId)}`);
    const outputSection = page.locator('[data-testid="job-detail"] section[aria-labelledby="job-outputs-heading"]');
    const expiry = outputSection.locator('time');
    await expect(expiry.first()).toBeVisible();
    await expect(expiry.first()).toContainText(/expire/i);
  });
});
