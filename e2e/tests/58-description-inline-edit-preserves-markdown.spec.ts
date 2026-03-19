/**
 * Tests that after inline description editing on a detail page, markdown
 * formatting is preserved in the displayed text (not shown as raw markdown).
 *
 * Bug: The description inline-edit handler (@click.away) saves the new
 * description via fetch(), then updates the display area using
 * `display.textContent = newValue`. This replaces the DOM content with
 * plain text, destroying any markdown rendering (bold, italic, links, etc.).
 * After saving, the user sees raw markdown like `**bold**` instead of
 * rendered bold text. A page reload correctly re-renders the markdown.
 *
 * Steps to reproduce:
 * 1. Create an entity with a markdown description (e.g., `**bold** and _italic_`)
 * 2. Go to the entity's detail page — markdown renders correctly
 * 3. Double-click the description to enter edit mode
 * 4. Change the description to new markdown content
 * 5. Click away to save
 * 6. Observe: raw markdown is displayed (e.g., `**new bold**`) instead of
 *    rendered HTML (e.g., bold text)
 *
 * Expected: After inline description edit, markdown should be rendered
 * (e.g., bold text appears bold, italic appears italic).
 *
 * Actual: After inline description edit, raw markdown syntax is shown
 * as plain text. Page reload fixes it.
 */
import { test, expect } from '../fixtures/base.fixture';

test.describe('Description inline edit preserves markdown rendering', () => {
  let tagId: number;

  test.beforeAll(async ({ apiClient }) => {
    const tag = await apiClient.createTag(
      'Markdown Render Tag',
      '**original bold** and _original italic_',
    );
    tagId = tag.ID;
  });

  test('after saving description with markdown, rendered HTML should appear (not raw markdown)', async ({
    page,
  }) => {
    // Navigate to the tag detail page
    await page.goto(`/tag?id=${tagId}`);
    await page.waitForLoadState('load');

    // Verify the original description is rendered with markdown
    const descriptionArea = page.locator('[title="Double-click to edit"]').first();
    await expect(descriptionArea).toBeVisible({ timeout: 5000 });

    // The markdown should be rendered: <strong> for bold, <em> for italic
    await expect(descriptionArea.locator('strong')).toContainText('original bold');
    await expect(descriptionArea.locator('em')).toContainText('original italic');

    // Double-click to enter edit mode
    await descriptionArea.dblclick();

    // Textarea should appear with the raw markdown
    const textarea = page.locator('textarea[name="description"]');
    await expect(textarea).toBeVisible({ timeout: 3000 });

    // Type a new description with markdown
    const newDescription = '**updated bold** and _updated italic_ plus plain';
    await textarea.fill(newDescription);

    // Wait for the POST request to complete when clicking away
    const savePromise = page.waitForResponse(
      (response) =>
        response.url().includes('/v1/tag/editDescription') &&
        response.status() === 200,
    );

    // Click away from the textarea to trigger the @click.away save
    await page.locator('h1').first().click();

    // Wait for the save to complete and page to reload
    await savePromise;
    await page.waitForLoadState('load');

    // After save + reload, the description should be rendered as markdown HTML
    const displayedDescription = page.locator('[title="Double-click to edit"]').first();
    await expect(displayedDescription.locator('strong')).toContainText('updated bold', {
      timeout: 5000,
    });
  });

  test.afterAll(async ({ apiClient }) => {
    if (tagId) await apiClient.deleteTag(tagId);
  });
});
