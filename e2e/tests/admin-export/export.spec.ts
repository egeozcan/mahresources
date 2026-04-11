import path from 'path';
import { test, expect } from '../../fixtures/base.fixture';
import { AdminExportPage } from '../../pages/AdminExportPage';

test.describe('Admin export', () => {
  test('runs an estimate and starts a download', async ({ page, apiClient }) => {
    const testRunId = Date.now();

    // A group requires a category first.
    const category = await apiClient.createCategory(`Export Test Category ${testRunId}`);
    const group = await apiClient.createGroup({
      name: `ExportRoot_${testRunId}`,
      categoryId: category.ID,
    });

    // Upload a tiny resource owned by the group.
    const testFilePath = path.join(__dirname, '../../test-assets/sample-image.png');
    await apiClient.createResource({
      filePath: testFilePath,
      name: `export-cover-${testRunId}.png`,
      ownerId: group.ID,
    });

    const exportPage = new AdminExportPage(page);
    await exportPage.goto([group.ID]);

    // Wait for the chip to materialise (Alpine preselect fetch is async).
    await expect(exportPage.chips.getByText(new RegExp(`ExportRoot_${testRunId}`))).toBeVisible({ timeout: 10000 });

    // Estimate.
    await exportPage.estimateButton.click();
    await expect(exportPage.estimateOutput).toBeVisible({ timeout: 10000 });
    await expect(exportPage.estimateOutput).toContainText('Groups: 1');
    await expect(exportPage.estimateOutput).toContainText('Resources: 1');

    await exportPage.submitButton.click();

    // Progress panel should appear immediately.
    await expect(exportPage.progressPanel).toBeVisible({ timeout: 5000 });

    // Wait for the download link to become visible (Alpine shows it when job.status === 'completed').
    await expect(exportPage.downloadLink).toBeVisible({ timeout: 60000 });

    // Verify the download link points to a valid tar file by reading its href.
    const href = await exportPage.downloadLink.getAttribute('href');
    expect(href).toMatch(/\/v1\/exports\/.+\/download/);
  });
});
