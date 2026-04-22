/**
 * Tests that typing into a text block in the block editor actually persists
 * the content.
 *
 * Bug: The template uses $parent.onInput() and $parent.save(), but $parent
 * doesn't exist in Alpine.js v3. The blockText.onInput() and save() methods
 * are never called, so text block content is silently lost.
 */
import { test, expect } from '../fixtures/base.fixture';

test.describe('Text block content saves via UI', () => {
  let categoryId: number;
  let ownerGroupId: number;
  let noteId: number;

  test.beforeAll(async ({ apiClient }) => {
    const category = await apiClient.createCategory(
      'Block Save Test Category',
      'Category for text block save test',
    );
    categoryId = category.ID;

    const ownerGroup = await apiClient.createGroup({
      name: 'Block Save Test Owner',
      categoryId,
    });
    ownerGroupId = ownerGroup.ID;

    const note = await apiClient.createNote({
      name: 'Block Save Test Note',
      description: 'Note for testing text block save',
      ownerId: ownerGroupId,
    });
    noteId = note.ID;
  });

  test('text typed into a text block should persist after leaving edit mode', async ({
    page,
    apiClient,
  }) => {
    // Navigate to note display page
    await page.goto(`/note?id=${noteId}`);
    await page.waitForLoadState('load');

    // Enter block edit mode
    await page.getByText('Edit Blocks').click();

    // Add a text block via the picker (BH-027: listbox with role=option items)
    await page.locator('[data-testid="add-block-trigger"]').click();
    await page.locator('[role="option"][data-block-type="text"]').click();

    // Type content into the textarea
    const textarea = page.locator('textarea[placeholder="Enter text..."]');
    await expect(textarea).toBeVisible({ timeout: 5000 });
    await textarea.fill('This content should be saved');

    // Blur the textarea to trigger save
    await page.locator('h1').first().click();

    // Give time for the save API call to complete
    await page.waitForTimeout(1500);

    // Verify content persisted via API
    const blocks = await apiClient.getBlocks(noteId);
    const textBlock = blocks.find((b: { type: string }) => b.type === 'text');
    expect(textBlock).toBeDefined();
    expect(textBlock!.content.text).toBe('This content should be saved');
  });

  test.afterAll(async ({ apiClient }) => {
    if (noteId) await apiClient.deleteNote(noteId);
    if (ownerGroupId) await apiClient.deleteGroup(ownerGroupId);
    if (categoryId) await apiClient.deleteCategory(categoryId);
  });
});
