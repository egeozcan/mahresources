/**
 * E2E tests for template comments: [comment]...[/comment] is removed on the
 * server, so the commented text never reaches the rendered page: a template
 * slot, a Description and its list-card preview, or the live preview. The
 * editor dims the region and lints an unclosed opener.
 */
import { test, expect } from '../../fixtures/base.fixture';

test.describe('[comment] template comments', () => {
  let categoryId: number;
  let groupId: number;
  let groupName: string;
  let categoryName: string;

  test.beforeAll(async ({ apiClient }) => {
    const stamp = Date.now();
    categoryName = `Comment Cat ${stamp}`;
    const cat = await apiClient.createCategory(categoryName, 'shown-cat-desc [comment]hidden-cat-desc[/comment]', {
      CustomHeader:
        '<div class="comment-test">shown-header[comment reason="old"]hidden-header [property path="Name"][/comment]</div>',
    });
    categoryId = cat.ID;
    groupName = `Comment Group ${stamp}`;
    const group = await apiClient.createGroup({
      name: groupName,
      categoryId,
      description: 'shown-description\n\n[comment]hidden-description[/comment]',
    });
    groupId = group.ID;
  });

  test.afterAll(async ({ apiClient }) => {
    if (groupId) await apiClient.deleteGroup(groupId);
    if (categoryId) await apiClient.deleteCategory(categoryId);
  });

  // The page also carries the raw source for editing (the edit textarea and
  // the entity JSON Alpine reads), as it does for every template; what is
  // checked here is the rendered markup.
  test('commented text is absent from the rendered header and description', async ({ page }) => {
    await page.goto(`/group?id=${groupId}`);
    const headerHtml = await page.locator('.comment-test').innerHTML();
    expect(headerHtml).toBe('shown-header');

    const description = page.locator('[data-description-display]').first();
    await expect(description).toContainText('shown-description');
    const descriptionHtml = await description.innerHTML();
    expect(descriptionHtml).not.toContain('hidden-description');
    expect(descriptionHtml).not.toContain('[comment');
  });

  test('a list card preview drops the comment too', async ({ page }) => {
    await page.goto(`/categories?name=${encodeURIComponent(categoryName)}`);
    const preview = page.locator('.description-preview-content', { hasText: 'shown-cat-desc' });
    await expect(preview).toHaveCount(1);
    await expect(preview).not.toContainText('hidden-cat-desc');
    await expect(preview).not.toContainText('[comment');
  });

  test('the editor dims a comment, lints an unclosed one, and the preview drops it', async ({ page }) => {
    await page.goto('/category/new');
    await page.waitForLoadState('load');

    const entityInput = page.locator('#tp-entity-group');
    await expect(entityInput).toBeVisible({ timeout: 10000 });
    await expect(entityInput).not.toHaveValue('', { timeout: 10000 });

    const header = page.locator('.cm-content[aria-label="Custom Header"]');
    await expect(header).toBeVisible({ timeout: 10000 });
    await header.click();
    await page.keyboard.type('preview-shown[comment]preview-hidden');

    const scope = page.locator('div', { has: page.locator('input[name="CustomHeader"]') });
    await expect(scope.locator('.cm-template-comment').first()).toBeVisible({ timeout: 10000 });
    // Unclosed: hides to the end, flagged as an error.
    await expect(scope.locator('.cm-lint-marker-error').first()).toBeVisible({ timeout: 10000 });

    const frame = page.frameLocator('iframe[title="Template slot preview"]');
    await expect(frame.locator('body')).toContainText('preview-shown', { timeout: 10000 });
    await expect(frame.locator('body')).not.toContainText('preview-hidden');

    // Closing it clears the error.
    await page.keyboard.type('[/comment]');
    await expect(scope.locator('.cm-lint-marker-error')).toHaveCount(0, { timeout: 10000 });
    await expect(frame.locator('body')).not.toContainText('preview-hidden');
  });
});
