/**
 * Security test: template literal injection in confirmAction messages
 *
 * Bug: displayGroup.tpl and displayTag.tpl embed entity names inside JS
 * backtick template literals in x-data="confirmAction({ message: `...` })".
 * The |json filter passes strings unchanged, so a name containing ${...}
 * is evaluated as a JS expression, enabling stored XSS.
 *
 * Fix: Replace backtick strings with single-quoted strings using |escapejs
 * to properly escape the entity name for a JS string context.
 */
import { test, expect } from '../fixtures/base.fixture';

test.describe('XSS: Template Literal Injection in confirmAction', () => {
  let categoryId: number;
  let groupId: number;
  let tagId: number;

  const XSS_PAYLOAD = '${document.title="XSS_GROUP"}';
  const XSS_TAG_PAYLOAD = '${document.title="XSS_TAG"}';

  test.beforeAll(async ({ apiClient }) => {
    const category = await apiClient.createCategory('XSS Test Category', 'For XSS tests');
    categoryId = category.ID;

    const group = await apiClient.createGroup({
      name: XSS_PAYLOAD,
      categoryId: categoryId,
    });
    groupId = group.ID;

    const tag = await apiClient.createTag(XSS_TAG_PAYLOAD);
    tagId = tag.ID;
  });

  test('group page: template literal in name must not execute as JS', async ({ page }) => {
    await page.goto(`/group?id=${groupId}`);
    await page.waitForLoadState('load');

    // If the XSS fires, document.title would be changed to just "XSS_GROUP"
    // If safe, the title retains the normal " - mahresources" suffix
    const title = await page.title();
    expect(title).toContain('- mahresources');

    // The merge button should exist without JS errors
    const mergeButton = page.locator('button:has-text("Merge")');
    await expect(mergeButton).toBeVisible();

    // The literal text ${document.title="XSS_GROUP"} should appear safely
    // in the page (as the group name in h1), not be evaluated
    await expect(page.locator('h1')).toContainText('${');
  });

  test('tag page: template literal in name must not execute as JS', async ({ page }) => {
    await page.goto(`/tag?id=${tagId}`);
    await page.waitForLoadState('load');

    const title = await page.title();
    expect(title).toContain('- mahresources');

    const mergeButton = page.locator('button:has-text("Merge")');
    await expect(mergeButton).toBeVisible();
  });

  test.afterAll(async ({ apiClient }) => {
    if (groupId) await apiClient.deleteGroup(groupId);
    if (tagId) await apiClient.deleteTag(tagId);
    if (categoryId) await apiClient.deleteCategory(categoryId);
  });
});
