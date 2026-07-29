/**
 * UI bug hunt 2026-07-29, finding 151 (WS2).
 *
 * Bug: src/webcomponents/inlineedit.js:191-193 updates its own display text and
 * document.title after a successful rename, but nothing else. The resource
 * detail page renders the same field again in the METADATA card
 * (templates/displayResource.tpl:33-42), server-side, so after an inline rename
 * one screen showed two different names for one resource until a manual reload.
 */
import { test, expect } from '../../fixtures/base.fixture';
import path from 'path';

test.describe('An inline rename refreshes every display of that field', () => {
  let runId: string;
  let categoryId: number;
  let ownerGroupId: number;
  let resourceId: number;
  let originalName: string;

  test.beforeAll(async ({ apiClient }) => {
    runId = `${Date.now()}-${Math.random().toString(36).substring(2, 8)}`;
    const category = await apiClient.createCategory(`WS2 Meta Cat ${runId}`, 'finding 151');
    categoryId = category.ID;
    const owner = await apiClient.createGroup({ name: `WS2 Meta Owner ${runId}`, categoryId });
    ownerGroupId = owner.ID;

    originalName = `ws2 metadata original ${runId}`;
    const resource = await apiClient.createResource({
      filePath: path.join(__dirname, '../../test-assets/sample-image.png'),
      name: originalName,
      ownerId: ownerGroupId,
    });
    resourceId = resource.ID;
  });

  test('the metadata card Name follows the heading without a reload', async ({
    page,
    apiClient,
  }) => {
    await page.goto(`/resource?id=${resourceId}`);
    await page.waitForLoadState('load');

    const metadataName = page.locator(
      '[aria-label="Resource metadata"] dt:text-is("Name") + dd',
    );
    await expect(metadataName).toContainText(originalName);

    const renamed = `ws2 metadata renamed ${runId}`;
    const inlineEdit = page.locator('h1 inline-edit').first();
    await inlineEdit.locator('button.edit-button').click();
    const input = inlineEdit.locator('input');
    await input.fill(renamed);
    await input.press('Enter');

    // The write has to land first, or "the card is stale" is unprovable.
    await expect
      .poll(async () => (await apiClient.getResource(resourceId)).Name, { timeout: 10000 })
      .toBe(renamed);

    // The heading updates optimistically today — it is the control that proves
    // the rename reached the client at all.
    await expect(page.locator('h1')).toContainText(renamed);

    // The card is the defect: it kept the old name until a manual reload.
    await expect(metadataName).toContainText(renamed);
    await expect(metadataName).not.toContainText(originalName);

    // No reload happened in between.
    expect(page.url()).toContain(`/resource?id=${resourceId}`);
  });

  test.afterAll(async ({ apiClient }) => {
    if (resourceId) await apiClient.deleteResource(resourceId);
    if (ownerGroupId) await apiClient.deleteGroup(ownerGroupId);
    if (categoryId) await apiClient.deleteCategory(categoryId);
  });
});
