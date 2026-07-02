import { test, expect } from '../fixtures/base.fixture';
import path from 'path';

// The resource search's Owner filter has an "Include subgroups" checkbox that
// widens the ownerId match to the whole group subtree (recursive descendants).
test.describe('Resource owner filter: include subgroups', () => {
  let categoryId: number;
  let parentGroupId: number;
  let childGroupId: number;
  let parentResourceId: number;
  let childResourceId: number;
  let testRunId: number;

  test.beforeAll(async ({ apiClient }) => {
    testRunId = Date.now() + Math.floor(Math.random() * 100000);

    const category = await apiClient.createCategory(
      `Subtree Category ${testRunId}`,
      'Category for owner-subtree filter tests'
    );
    categoryId = category.ID;

    const parent = await apiClient.createGroup({
      name: `Subtree Parent ${testRunId}`,
      categoryId,
    });
    parentGroupId = parent.ID;
    const child = await apiClient.createGroup({
      name: `Subtree Child ${testRunId}`,
      categoryId,
      ownerId: parentGroupId,
    });
    childGroupId = child.ID;

    const assetPath = path.join(__dirname, '../test-assets/sample-image-9.png');
    const parentRes = await apiClient.createResource({
      filePath: assetPath,
      name: `Subtree Parent Res ${testRunId}`,
      ownerId: parentGroupId,
    });
    parentResourceId = parentRes.ID;
    const childRes = await apiClient.createResource({
      filePath: assetPath,
      name: `Subtree Child Res ${testRunId}`,
      ownerId: childGroupId,
    });
    childResourceId = childRes.ID;
  });

  test('checkbox widens the owner filter to descendant subgroups', async ({ page }) => {
    // The gallery links carry the resource name in their title attribute; the
    // hidden lightbox header link does not, so this stays strict-mode safe.
    const parentLink = page.locator(`a[title="Subtree Parent Res ${testRunId}"]`);
    const childLink = page.locator(`a[title="Subtree Child Res ${testRunId}"]`);

    // Without the flag: exact owner match only.
    await page.goto(`/resources?ownerId=${parentGroupId}`);
    await expect(parentLink).toBeVisible();
    await expect(childLink).not.toBeVisible();

    // Check "Include subgroups" and re-apply the filters.
    const checkbox = page.locator('input[name="IncludeSubgroups"]');
    await checkbox.check();
    await page.locator('button[type="submit"]:has-text("Apply Filters")').click();
    await page.waitForLoadState('load');

    // Both the parent's and the child's resources are now listed.
    await expect(parentLink).toBeVisible();
    await expect(childLink).toBeVisible();

    // The checkbox state survives the round-trip.
    await expect(page.locator('input[name="IncludeSubgroups"]')).toBeChecked();
  });

  test.afterAll(async ({ apiClient }) => {
    if (parentResourceId) await apiClient.deleteResource(parentResourceId).catch(() => {});
    if (childResourceId) await apiClient.deleteResource(childResourceId).catch(() => {});
    if (childGroupId) await apiClient.deleteGroup(childGroupId).catch(() => {});
    if (parentGroupId) await apiClient.deleteGroup(parentGroupId).catch(() => {});
    if (categoryId) await apiClient.deleteCategory(categoryId).catch(() => {});
  });
});
