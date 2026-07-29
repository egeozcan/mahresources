/**
 * UI bug hunt 2026-07-29, finding 9 (WS2).
 *
 * Bug: every non-delete bulk operation on /resources/details raised a native
 * alert("Bulk operation failed: Could not find refreshed list") even though the
 * write had succeeded. src/components/bulkSelection.js resolves the element to
 * morph with document.querySelector(".list-container, .items-container");
 * templates/listResourcesDetails.tpl wraps its table in .detail-table-wrap and
 * matches neither, so the lookup returned null and the throw at :197 became the
 * alert at :210. Bulk delete escaped only because its form is class="no-ajax"
 * and does a full navigation.
 *
 * The alert is the visible symptom; the damage is that the failure path runs
 * before form.reset() and deselectAll(), so the row stays ticked and the editor
 * stays open — the page reads as "nothing happened" and invites a retry.
 *
 * Assertions therefore cover all three: no dialog, the tag actually persisted
 * (read back through the API, never off the DOM), and the list did refresh.
 * The /resources grid view runs the same steps as a positive control, since it
 * was never affected — if the control fails, the procedure is wrong, not the fix.
 */
import type { Dialog, Page } from '@playwright/test';
import { test, expect } from '../../fixtures/base.fixture';
import path from 'path';

const IMAGE = path.join(__dirname, '../../test-assets/sample-image.png');

test.describe('Bulk operations on the details view do not report a false failure', () => {
  let runId: string;
  let ownerGroupId: number;
  let categoryId: number;
  let resourceIds: number[] = [];
  let resourceNames: string[] = [];
  let tagId: number;
  let tagName: string;
  let gridTagId: number;
  let gridTagName: string;

  test.beforeAll(async ({ apiClient }) => {
    runId = `${Date.now()}-${Math.random().toString(36).substring(2, 8)}`;

    const category = await apiClient.createCategory(`WS2 Bulk Cat ${runId}`, 'finding 9');
    categoryId = category.ID;

    const owner = await apiClient.createGroup({
      name: `WS2 Bulk Owner ${runId}`,
      categoryId,
    });
    ownerGroupId = owner.ID;

    tagName = `ws2-bulk-details-${runId}`;
    tagId = (await apiClient.createTag(tagName, 'finding 9 details view')).ID;

    gridTagName = `ws2-bulk-grid-${runId}`;
    gridTagId = (await apiClient.createTag(gridTagName, 'finding 9 control')).ID;

    resourceIds = [];
    resourceNames = [];
    for (let i = 1; i <= 2; i++) {
      const name = `WS2 Bulk Resource ${i} ${runId}`;
      const resource = await apiClient.createResource({
        filePath: IMAGE,
        name,
        ownerId: ownerGroupId,
      });
      resourceIds.push(resource.ID);
      resourceNames.push(name);
    }
  });

  /**
   * Ticks one row, adds `tag` through the bulk Add Tag editor and returns every
   * dialog the page raised while doing it.
   */
  async function bulkAddTag(page: Page, listUrl: string, resourceName: string, tag: string) {
    const dialogs: string[] = [];
    page.on('dialog', async (dialog: Dialog) => {
      dialogs.push(dialog.message());
      await dialog.dismiss();
    });

    await page.goto(listUrl);
    await page.waitForLoadState('load');

    // Addressed by accessible name: `[x-data*="itemId: 1"]` also matches itemId 12.
    const checkbox = page.locator(`input[type="checkbox"][aria-label="Select ${resourceName}"]`);
    await expect(checkbox).toBeVisible();
    await checkbox.check();

    await page.getByRole('button', { name: 'Toggle Add Tag editor' }).click();
    const form = page.locator('form[action*="/v1/resources/addTags"]');
    await form.getByRole('combobox', { name: 'Add Tag' }).fill(tag);
    await page.locator('[role="option"]').filter({ hasText: tag }).click();
    await form.getByRole('button', { name: 'Add', exact: true }).click();

    return { dialogs, checkbox };
  }

  test('adding a tag from /resources/details neither alerts nor strands the selection', async ({
    page,
    apiClient,
  }) => {
    const { dialogs, checkbox } = await bulkAddTag(
      page,
      `/resources/details?ownerId=${ownerGroupId}`,
      resourceNames[0],
      tagName,
    );

    // The write must land — asserted through the API, because the whole failure
    // mode here is a UI that misreports a write it already completed.
    await expect
      .poll(async () => {
        const resource = await apiClient.getResource(resourceIds[0]);
        return (resource.Tags ?? []).some((t: { ID: number }) => t.ID === tagId);
      }, { timeout: 10000 })
      .toBe(true);

    // The submit handler is fire-and-forget, so wait for it to reach *some*
    // conclusion before judging which one. Without this the dialog assertion
    // can run before the alert has been raised and pass for the wrong reason.
    await expect
      .poll(async () => dialogs.length > 0 || !(await checkbox.isChecked()), { timeout: 15000 })
      .toBe(true);

    expect(dialogs, `the page raised: ${JSON.stringify(dialogs)}`).toEqual([]);

    // The success path resets the form and clears the selection; the failure
    // path throws before either, leaving the row ticked and the editor open.
    await expect(checkbox).not.toBeChecked();
    await expect(page.getByRole('button', { name: /Deselect All/i })).toBeHidden();
  });

  test('the same operation on the /resources grid stays clean (control)', async ({
    page,
    apiClient,
  }) => {
    const { dialogs, checkbox } = await bulkAddTag(
      page,
      `/resources?ownerId=${ownerGroupId}`,
      resourceNames[1],
      gridTagName,
    );

    await expect
      .poll(async () => {
        const resource = await apiClient.getResource(resourceIds[1]);
        return (resource.Tags ?? []).some((t: { ID: number }) => t.ID === gridTagId);
      }, { timeout: 10000 })
      .toBe(true);

    await expect
      .poll(async () => dialogs.length > 0 || !(await checkbox.isChecked()), { timeout: 15000 })
      .toBe(true);

    expect(dialogs, `the page raised: ${JSON.stringify(dialogs)}`).toEqual([]);
    await expect(checkbox).not.toBeChecked();
  });

  test.afterAll(async ({ apiClient }) => {
    for (const id of resourceIds) await apiClient.deleteResource(id);
    if (tagId) await apiClient.deleteTag(tagId);
    if (gridTagId) await apiClient.deleteTag(gridTagId);
    if (ownerGroupId) await apiClient.deleteGroup(ownerGroupId);
    if (categoryId) await apiClient.deleteCategory(categoryId);
  });
});
