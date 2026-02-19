import { test, expect } from '../fixtures/base.fixture';

test.describe.serial('Tag Merge Operations', () => {
  let winnerTagId: number;
  let loserTag1Id: number;
  let loserTag2Id: number;
  let groupId: number;
  let categoryId: number;
  let testRunId: string;

  test.beforeAll(async ({ apiClient }) => {
    testRunId = `${Date.now()}-${Math.random().toString(36).substring(2, 8)}`;

    const category = await apiClient.createCategory(
      `Merge Test Category ${testRunId}`,
      'Category for tag merge tests'
    );
    categoryId = category.ID;

    const winner = await apiClient.createTag(`Winner Tag ${testRunId}`, 'The winner');
    winnerTagId = winner.ID;

    const loser1 = await apiClient.createTag(`Loser Tag 1 ${testRunId}`, 'First loser');
    loserTag1Id = loser1.ID;

    const loser2 = await apiClient.createTag(`Loser Tag 2 ${testRunId}`, 'Second loser');
    loserTag2Id = loser2.ID;

    const group = await apiClient.createGroup({
      name: `Merge Test Group ${testRunId}`,
      categoryId: categoryId,
      tags: [loserTag1Id, loserTag2Id],
    });
    groupId = group.ID;
  });

  test('should merge tags and transfer associations', async ({ apiClient, tagPage, groupPage, page }) => {
    await apiClient.mergeTags(winnerTagId, [loserTag1Id, loserTag2Id]);

    await tagPage.verifyTagNotInList(`Loser Tag 1 ${testRunId}`);
    await tagPage.verifyTagNotInList(`Loser Tag 2 ${testRunId}`);

    await tagPage.verifyTagInList(`Winner Tag ${testRunId}`);

    await groupPage.gotoDisplay(groupId);
    await expect(page.locator(`a:has-text("Winner Tag ${testRunId}")`).first()).toBeVisible();
  });

  test('should show merge form on tag detail page', async ({ tagPage, page }) => {
    await tagPage.gotoDisplay(winnerTagId);

    await expect(page.locator('text=Merge others with this tag?')).toBeVisible();
    await expect(page.locator('text=Tags To Merge')).toBeVisible();
  });

  test('should show meta with merge backups', async ({ tagPage, page }) => {
    await tagPage.gotoDisplay(winnerTagId);

    await expect(page.locator('text=backups')).toBeVisible();
  });

  test.afterAll(async ({ apiClient }) => {
    try { await apiClient.deleteTag(winnerTagId); } catch {}
    try { await apiClient.deleteGroup(groupId); } catch {}
    try { await apiClient.deleteCategory(categoryId); } catch {}
  });
});

test.describe('Tag List Bulk Selection', () => {
  let tag1Id: number;
  let tag2Id: number;
  let testRunId: string;

  test.beforeAll(async ({ apiClient }) => {
    testRunId = `${Date.now()}-${Math.random().toString(36).substring(2, 8)}`;

    const tag1 = await apiClient.createTag(`Bulk Tag A ${testRunId}`, 'First bulk tag');
    tag1Id = tag1.ID;

    const tag2 = await apiClient.createTag(`Bulk Tag B ${testRunId}`, 'Second bulk tag');
    tag2Id = tag2.ID;
  });

  test('should show bulk editor when tag selected', async ({ tagPage, page }) => {
    await tagPage.gotoList();

    await page.locator(`[x-data*="itemId: ${tag1Id}"] input[type="checkbox"]`).check();

    await expect(page.locator('button:has-text("Deselect All"), button:has-text("Deselect")')).toBeVisible();
  });

  test('should bulk delete tags via API', async ({ apiClient, tagPage }) => {
    await apiClient.bulkDeleteTags([tag1Id, tag2Id]);
    await tagPage.verifyTagNotInList(`Bulk Tag A ${testRunId}`);
    await tagPage.verifyTagNotInList(`Bulk Tag B ${testRunId}`);
  });

  test.afterAll(async ({ apiClient }) => {
    try { await apiClient.deleteTag(tag1Id); } catch {}
    try { await apiClient.deleteTag(tag2Id); } catch {}
  });
});
