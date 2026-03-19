/**
 * Tests that removing the Owner from a group via the edit form actually
 * clears the OwnerId in the database.
 *
 * Bug: The autocompleter hidden input uses `name="ownerId"` (lowercase 'o'),
 * but the backend handler checks `formHasField(request, "OwnerId")` (uppercase
 * 'O').  Because request.PostForm is case-sensitive, the sentinel field is
 * never detected, so the handler re-populates OwnerId from the existing
 * record — making it impossible to clear the owner via the edit form.
 *
 * The same case mismatch affects:
 *   - Group owner:     form sends "ownerId",    handler checks "OwnerId"
 *   - Group category:  form sends "categoryId", handler checks "CategoryId"
 *   - Note owner:      form sends "ownerId",    handler checks "OwnerId"
 *   - Resource owner:  form sends "ownerId",    handler checks "OwnerId"
 */
import { test, expect } from '../fixtures/base.fixture';

test.describe('Group edit clears owner', () => {
  let categoryId: number;
  let ownerGroupId: number;
  let childGroupId: number;

  test.beforeAll(async ({ apiClient }) => {
    const category = await apiClient.createCategory(
      'ClearOwnerCat',
      'Category for clear-owner test',
    );
    categoryId = category.ID;

    const ownerGroup = await apiClient.createGroup({
      name: 'TheOwner',
      description: 'This group is the owner',
      categoryId,
    });
    ownerGroupId = ownerGroup.ID;

    const childGroup = await apiClient.createGroup({
      name: 'TheChild',
      description: 'This group has an owner that should be clearable',
      categoryId,
      ownerId: ownerGroupId,
    });
    childGroupId = childGroup.ID;
  });

  test('removing owner via edit form clears OwnerId', async ({
    page,
    apiClient,
  }) => {
    // Verify the owner is set before editing
    const beforeEdit = await apiClient.getGroup(childGroupId);
    expect(beforeEdit.OwnerId).toBe(ownerGroupId);

    // Navigate to the edit form
    await page.goto(`/group/edit?id=${childGroupId}`);
    await page.waitForLoadState('load');

    // Verify the owner is shown as selected in the form
    const removeButton = page.locator('button').filter({
      hasText: /Remove.*TheOwner/i,
    });
    await expect(removeButton).toBeVisible();

    // Click the Remove button to deselect the owner
    await removeButton.click();

    // Verify the owner is no longer shown
    await expect(removeButton).not.toBeVisible();

    // Save the form
    await page.locator('button[type="submit"]').click();

    // Wait for redirect to group display page
    await page.waitForURL(/\/group\?id=/, { timeout: 5000 });

    // Verify via API that the OwnerId is now cleared (null/0)
    const afterEdit = await apiClient.getGroup(childGroupId);
    expect(afterEdit.OwnerId).toBeFalsy();
  });

  test.afterAll(async ({ apiClient }) => {
    if (childGroupId) {
      try { await apiClient.deleteGroup(childGroupId); } catch {}
    }
    if (ownerGroupId) {
      try { await apiClient.deleteGroup(ownerGroupId); } catch {}
    }
    if (categoryId) {
      try { await apiClient.deleteCategory(categoryId); } catch {}
    }
  });
});
