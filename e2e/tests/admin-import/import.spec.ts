import { test, expect } from '../../fixtures/base.fixture';

test.describe('Admin Import', () => {
  test('upload tar and view parse plan', async ({ page, apiClient, request, baseURL }) => {
    const testRunId = Date.now();

    // 1. Seed data: category + group
    const category = await apiClient.createCategory(`ImportTestCat_${testRunId}`, 'test');
    const group = await apiClient.createGroup({
      name: `ImportTestGroup_${testRunId}`,
      categoryId: category.ID,
    });

    // 2. Kick off an export job via the API
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

    // 3. Poll until the export job completes
    await expect.poll(
      async () => {
        const resp = await request.get(`${baseURL}/v1/jobs/get?id=${exportJobId}`);
        const data = await resp.json();
        return data.status;
      },
      { timeout: 30000, intervals: [500, 1000, 2000] },
    ).toBe('completed');

    // 4. Download the tar bytes
    const downloadResp = await request.get(`${baseURL}/v1/exports/${exportJobId}/download`);
    expect(downloadResp.ok()).toBeTruthy();
    const tarBuffer = Buffer.from(await downloadResp.body());
    expect(tarBuffer.length).toBeGreaterThan(0);

    // 5. Navigate to import page and verify the upload form is visible
    await page.goto('/admin/import');
    await expect(page.getByTestId('import-file-input')).toBeVisible();

    // 6. Upload the tar via the file input
    await page.getByTestId('import-file-input').setInputFiles({
      name: 'test-export.tar',
      mimeType: 'application/x-tar',
      buffer: tarBuffer,
    });
    await page.getByTestId('import-upload-button').click();

    // 7. Wait for the plan review section to appear (Alpine renders it when
    //    the parse job completes and the plan is fetched).
    await expect(page.getByTestId('import-summary')).toBeVisible({ timeout: 30000 });

    // 8. Verify the summary shows at least 1 group
    const summaryText = await page.getByTestId('import-summary').textContent();
    expect(summaryText).toContain('1');

    // The item tree should contain the group name
    await expect(page.getByTestId('import-items')).toContainText(`ImportTestGroup_${testRunId}`);

    // The options section (collision policy, parent group picker) should be visible
    await expect(page.getByTestId('import-options')).toBeVisible();

    // The collision-policy select should be present
    const collisionSelect = page.locator('#collision-policy');
    await expect(collisionSelect).toBeVisible();
    await expect(collisionSelect).toHaveValue('skip');
  });
});
