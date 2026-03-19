/**
 * Tests that after a successful inline description edit (double-click to edit,
 * change text, click away to save), the UI shows the NEWLY saved text, not the
 * old server-rendered content.
 *
 * Bug: The description inline edit template (@click.away handler) saves the new
 * description to the server via fetch(), then sets `editing = false`. When
 * `editing` becomes false, Alpine.js removes the <textarea> (x-if="editing")
 * and shows the original server-rendered HTML (x-if="!editing"). Since the
 * server-rendered HTML was not updated, the user sees the OLD description text
 * even though the new value was successfully saved.
 *
 * Steps to reproduce:
 * 1. Create an entity (tag, group, note, etc.) with a description
 * 2. Go to the entity's detail page
 * 3. Double-click the description to enter edit mode
 * 4. Change the description text
 * 5. Click away from the textarea to trigger save (via @click.away)
 * 6. Observe: the displayed description reverts to the OLD text
 * 7. Reload the page: the NEW text appears (confirming save was successful)
 *
 * Expected: After clicking away, the displayed description should show the
 * newly saved text.
 *
 * Actual: After clicking away, the displayed description shows the old
 * server-rendered text. The user thinks their edit was lost.
 */
import { test, expect } from '../fixtures/base.fixture';

test.describe('Description inline edit shows updated value after save', () => {
  let tagId: number;

  test.beforeAll(async ({ apiClient }) => {
    const tag = await apiClient.createTag(
      'Desc Update Display Tag',
      'Original description before edit',
    );
    tagId = tag.ID;
  });

  test('after saving description via click-away, the new text should be displayed', async ({
    page,
  }) => {
    // Navigate to the tag detail page
    await page.goto(`/tag?id=${tagId}`);
    await page.waitForLoadState('load');

    // Verify the original description is shown
    const descriptionArea = page.locator('[title="Double-click to edit"]').first();
    await expect(descriptionArea).toBeVisible({ timeout: 5000 });
    await expect(descriptionArea).toContainText('Original description before edit');

    // Double-click to enter edit mode
    await descriptionArea.dblclick();

    // Textarea should appear with the original description
    const textarea = page.locator('textarea[name="description"]');
    await expect(textarea).toBeVisible({ timeout: 3000 });
    await expect(textarea).toHaveValue('Original description before edit');

    // Type a new description
    const newDescription = 'Updated description after inline edit';
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

    // After save + reload, the description should show the NEW text
    const displayedDescription = page.locator('[title="Double-click to edit"]').first();
    await expect(displayedDescription).toContainText(newDescription, {
      timeout: 5000,
    });
  });

  test.afterAll(async ({ apiClient }) => {
    if (tagId) await apiClient.deleteTag(tagId);
  });
});
