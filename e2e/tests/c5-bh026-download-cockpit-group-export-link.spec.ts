import { test, expect } from '../fixtures/base.fixture';

test('BH-026: completed group export has title + download link in cockpit', async ({ page, apiClient, baseURL }) => {
  const suffix = Date.now();
  const category = await apiClient.createCategory(`bh026-cat-${suffix}`);
  const group = await apiClient.createGroup({ name: `bh026-grp-${suffix}`, categoryId: category.ID });

  // Submit export via API
  const exportResp = await apiClient.request.post(`${baseURL}/v1/groups/export`, {
    headers: { 'Content-Type': 'application/json' },
    data: JSON.stringify({ rootGroupIds: [group.ID] }),
  });
  expect(exportResp.ok()).toBeTruthy();
  const { jobId } = await exportResp.json();

  // Poll the jobs queue REST endpoint until the export completes
  await expect.poll(async () => {
    const resp = await apiClient.request.get(`${baseURL}/v1/jobs/queue`);
    if (!resp.ok()) return null;
    const data = await resp.json();
    const jobs: any[] = data.jobs || [];
    const job = jobs.find((j: any) => j.id === jobId);
    return job?.status ?? null;
  }, { timeout: 30000, intervals: [1000] }).toBe('completed');

  await page.goto('/');
  // Wait for the cockpit trigger to be present
  await page.waitForSelector('[data-testid="cockpit-trigger"]', { timeout: 5000 });

  // Open the download cockpit panel
  await page.locator('[data-testid="cockpit-trigger"]').click();

  const panel = page.locator('[data-testid="cockpit-panel"]');
  await expect(panel).toBeVisible({ timeout: 5000 });

  // Wait for the SSE connection to be established (ensures init payload is processed)
  await expect(
    panel.locator('[data-testid="cockpit-connection-status"][aria-label*="connected"]')
  ).toBeVisible({ timeout: 10000 });

  // Wait for the specific job's download link, identified by jobId in the href.
  // This avoids ambiguity when other export jobs from concurrent tests are also in the panel.
  const downloadLink = panel.locator(`a[href*="/exports/${jobId}/download"]`);
  await expect(downloadLink).toBeVisible({ timeout: 5000 });

  // The job row containing the download link must have a non-empty title.
  // Navigate from the link up to its parent li[data-testid="cockpit-job"] via DOM evaluation.
  const titleText = await downloadLink.evaluate((el) => {
    const row = el.closest('[data-testid="cockpit-job"]');
    if (!row) return null;
    return row.querySelector('[data-testid="cockpit-job-title"]')?.textContent?.trim() ?? null;
  });
  expect(titleText, 'job title must not be empty').toBeTruthy();
});
