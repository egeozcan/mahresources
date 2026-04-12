import { test, expect } from '../../fixtures/base.fixture';
import * as path from 'path';

test.describe('Admin Import — Apply', () => {
  test('full round-trip: export → upload → parse → apply → verify', async ({ page, apiClient, request, baseURL }) => {
    const testRunId = Date.now();

    // 1. Seed: category + group with a resource
    const category = await apiClient.createCategory(`ImportApplyCat_${testRunId}`, 'test');
    const group = await apiClient.createGroup({
      name: `ImportApplyGroup_${testRunId}`,
      categoryId: category.ID,
    });

    const testFile = path.join(__dirname, '../../test-assets/sample-image-34.png');
    const resource = await apiClient.createResource({
      filePath: testFile,
      name: `ImportApplyRes_${testRunId}`,
      ownerId: group.ID,
    });
    expect(resource.ID).toBeGreaterThan(0);

    // 2. Export via API
    const exportResp = await request.post(`${baseURL}/v1/groups/export`, {
      data: {
        rootGroupIds: [group.ID],
        scope: {
          subtree: true,
          owned_resources: true,
          owned_notes: true,
          related_m2m: true,
          group_relations: true,
        },
        fidelity: { resource_blobs: true },
        schemaDefs: {
          categories_and_types: true,
          tags: true,
          group_relation_types: true,
        },
      },
    });
    expect(exportResp.ok()).toBeTruthy();
    const { jobId: exportJobId } = await exportResp.json();
    expect(exportJobId).toBeTruthy();

    // Poll until the export job completes
    await expect.poll(
      async () => {
        const resp = await request.get(`${baseURL}/v1/jobs/get?id=${exportJobId}`);
        const data = await resp.json();
        return data.status;
      },
      { timeout: 30000, intervals: [500, 1000, 2000] },
    ).toBe('completed');

    // Download the tar bytes
    const downloadResp = await request.get(`${baseURL}/v1/exports/${exportJobId}/download`);
    expect(downloadResp.ok()).toBeTruthy();
    const tarBuffer = Buffer.from(await downloadResp.body());
    expect(tarBuffer.length).toBeGreaterThan(0);

    // 3. Navigate to import page, upload tar, wait for parse
    await page.goto('/admin/import');
    await expect(page.getByTestId('import-file-input')).toBeVisible();

    await page.getByTestId('import-file-input').setInputFiles({
      name: 'test-export.tar',
      mimeType: 'application/x-tar',
      buffer: tarBuffer,
    });
    await page.getByTestId('import-upload-button').click();

    // Wait for parse to complete and plan to render
    await expect(page.getByTestId('import-summary')).toBeVisible({ timeout: 30000 });

    // Verify the summary shows at least 1 group
    const summaryText = await page.getByTestId('import-summary').textContent();
    expect(summaryText).toContain('1');

    // The item tree should contain the group name
    await expect(page.getByTestId('import-items')).toContainText(`ImportApplyGroup_${testRunId}`);

    // 4. Click apply button, wait for result
    const applyButton = page.getByTestId('import-apply-button');
    await expect(applyButton).toBeVisible();
    await expect(applyButton).toBeEnabled();
    await applyButton.click();

    // 5. Verify success result
    await expect(page.getByTestId('import-apply-result')).toBeVisible({ timeout: 60000 });

    const resultText = await page.getByTestId('import-apply-result').textContent();
    expect(resultText).toContain('Import completed');

    // 6. Verify imported group exists via API (the resource was hash-matched so
    //    the group was created but the resource was skipped/reused)
    const groups = await apiClient.getGroups();
    const importedGroups = groups.filter(g => g.Name.includes(`ImportApplyGroup_${testRunId}`));
    // Original + imported copy
    expect(importedGroups.length).toBeGreaterThanOrEqual(2);
  });
});
