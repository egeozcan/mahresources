import { test, expect } from '../../fixtures/base.fixture';
import * as path from 'path';

// Exports one group holding one resource and answers the archive's bytes and the
// group's id.
async function exportedArchive(
  apiClient: import('../../helpers/api-client').ApiClient,
  request: import('@playwright/test').APIRequestContext,
  baseURL: string,
  stamp: number,
): Promise<{ tar: Buffer; groupId: number }> {
  const category = await apiClient.createCategory(`ImportResumeCat_${stamp}`, 'test');
  const group = await apiClient.createGroup({ name: `ImportResumeGroup_${stamp}`, categoryId: category.ID });
  await apiClient.createResource({
    filePath: path.join(__dirname, '../../test-assets/sample-image-34.png'),
    name: `ImportResumeRes_${stamp}`,
    ownerId: group.ID,
  });
  const exportResp = await request.post(`${baseURL}/v1/groups/export`, {
    data: {
      rootGroupIds: [group.ID],
      scope: { subtree: true, owned_resources: true, owned_notes: true, related_m2m: true, group_relations: true },
      fidelity: { resource_blobs: true },
      schemaDefs: { categories_and_types: true, tags: true, group_relation_types: true },
    },
  });
  expect(exportResp.ok()).toBeTruthy();
  const { jobId } = await exportResp.json();
  await expect.poll(async () => {
    const resp = await request.get(`${baseURL}/v1/jobs/get?id=${jobId}`);
    return (await resp.json()).status;
  }, { timeout: 30000, intervals: [500, 1000, 2000] }).toBe('completed');
  const download = await request.get(`${baseURL}/v1/exports/${jobId}/download`);
  expect(download.ok()).toBeTruthy();
  return { tar: Buffer.from(await download.body()), groupId: group.ID };
}

test.describe('Admin Import: resuming a review', () => {
  test('a parsed import keeps its review across a reload, and says what each conflict policy will do', async ({ page, apiClient, request, baseURL }) => {
    const stamp = Date.now();
    const { tar } = await exportedArchive(apiClient, request, baseURL!, stamp);

    await page.goto('/admin/import');
    await page.getByTestId('import-file-input').setInputFiles({ name: `resume-${stamp}.tar`, mimeType: 'application/x-tar', buffer: tar });
    await page.getByTestId('import-upload-button').click();
    await expect(page.getByTestId('import-summary')).toBeVisible({ timeout: 30000 });

    // The handle is in the address, so leaving and coming back finds the review.
    await expect(page).toHaveURL(/\/admin\/import\?job=imp-/);
    const reviewURL = page.url();

    // The parse's Job links back to the review.
    const handle = new URL(reviewURL).searchParams.get('job')!;
    const legacy = await (await request.get(`${baseURL}/v1/jobs/get?id=${encodeURIComponent(handle)}`)).json();
    expect(legacy.canonicalJobId).toEqual(expect.any(String));
    await page.goto(`/job?id=${encodeURIComponent(legacy.canonicalJobId)}`);
    const reviewLink = page.getByRole('link', { name: 'View import review' });
    await expect(reviewLink).toBeVisible({ timeout: 30000 });
    await reviewLink.click();
    await expect(page).toHaveURL(new RegExp(`/admin/import\\?job=${handle}$`));
    await expect(page.getByTestId('import-summary')).toBeVisible({ timeout: 30000 });
    await expect(page.getByTestId('import-items')).toContainText(`ImportResumeGroup_${stamp}`);

    // Everything in the archive already exists here under its GUID, so the GUID
    // policy decides the resource and no content match is left for the other one.
    await expect(page.getByTestId('import-summary-guid-resources')).toHaveText('1 (will be merged into the existing rows)');
    await expect(page.getByTestId('import-summary-hash-resources')).toHaveText('0');
    await page.getByLabel('GUID Collision Policy').selectOption('skip');
    await expect(page.getByTestId('import-summary-guid-resources')).toHaveText('1 (will be skipped, keeping the existing rows)');

    await page.getByTestId('import-apply-button').click();
    await expect(page.getByTestId('import-apply-result')).toBeVisible({ timeout: 60000 });

    // Once applied there is no review to resume: the page shows the apply's report
    // and links to the import's Job instead.
    await page.goto(reviewURL);
    await expect(page.getByTestId('import-apply-result')).toContainText('Import completed', { timeout: 30000 });
    await expect(page.getByRole('link', { name: "Open the import's Job" })).toHaveAttribute('href', /^\/job\?id=/);
    await expect(page.getByTestId('import-summary')).toBeHidden();
  });

  test('an applied import that created a group shows it when reopened', async ({ page, apiClient, request, baseURL }) => {
    const stamp = Date.now();
    const { tar, groupId } = await exportedArchive(apiClient, request, baseURL!, stamp);
    // The group is gone here, so the import creates it again rather than merging.
    await apiClient.deleteGroup(groupId);

    await page.goto('/admin/import');
    await page.getByTestId('import-file-input').setInputFiles({ name: `created-${stamp}.tar`, mimeType: 'application/x-tar', buffer: tar });
    await page.getByTestId('import-upload-button').click();
    await expect(page.getByTestId('import-summary')).toBeVisible({ timeout: 30000 });
    const reviewURL = page.url();
    await page.getByTestId('import-apply-button').click();
    await expect(page.getByTestId('import-apply-result')).toBeVisible({ timeout: 60000 });

    await page.goto(reviewURL);
    const result = page.getByTestId('import-apply-result');
    await expect(result).toContainText('Import completed', { timeout: 30000 });
    const created = result.getByRole('link', { name: /^Group #\d+$/ });
    await expect(created).toHaveCount(1);
    await created.click();
    await expect(page).toHaveURL(/\/group\?id=\d+$/);
    await expect(page.locator('h1').first()).toContainText(`ImportResumeGroup_${stamp}`);
  });

  test('an unknown import handle says it could not be found', async ({ page }) => {
    await page.goto('/admin/import?job=imp-does-not-exist');
    await expect(page.getByTestId('import-resume-notice')).toContainText('could not be found');
    await expect(page.getByTestId('import-file-input')).toBeVisible();
  });
});
