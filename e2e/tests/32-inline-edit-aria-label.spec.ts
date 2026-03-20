/**
 * Tests that the inline-edit web component on entity display pages provides
 * a specific, meaningful aria-label for its edit button — not the generic
 * default "Edit Edit value" that screen readers would announce when the
 * `label` attribute is not passed to the component.
 *
 * WCAG 2.4.6 (Headings and Labels): Labels must describe the purpose.
 * A button labelled "Edit Edit value" is ambiguous when multiple inline-edit
 * elements exist on the same page (e.g., group list cards).
 */
import { test, expect } from '../fixtures/base.fixture';

test.describe('Inline-edit aria-label specificity', () => {
  let categoryId: number;
  let groupId: number;

  test.beforeAll(async ({ apiClient }) => {
    const category = await apiClient.createCategory(
      'Inline Edit A11y Test Category',
      'Category for inline-edit aria-label tests',
    );
    categoryId = category.ID;

    const group = await apiClient.createGroup({
      name: 'Inline Edit Test Group',
      description: 'Group for testing inline-edit aria labels',
      categoryId,
    });
    groupId = group.ID;
  });

  test('inline-edit button on note display page should not have generic aria-label', async ({
    apiClient,
    page,
  }) => {
    // Create a note to display
    const note = await apiClient.createNote({
      name: 'A11y Inline Edit Note',
      description: 'Testing inline-edit label',
      ownerId: groupId,
    });

    await page.goto(`/note?id=${note.ID}`);
    await page.waitForLoadState('load');

    // The inline-edit renders inside shadow DOM. Locate the custom element first.
    const inlineEdit = page.locator('inline-edit').first();
    await expect(inlineEdit).toBeVisible({ timeout: 5000 });

    // Pierce into shadow DOM to find the edit button
    const editButton = inlineEdit.locator('button.edit-button');

    // The edit button should have an aria-label that identifies WHAT is being edited.
    // Bug: without a `label` attribute on <inline-edit>, the component defaults to
    // label="Edit value", producing aria-label="Edit Edit value" — which is generic
    // and grammatically nonsensical.
    const ariaLabel = await editButton.getAttribute('aria-label');

    expect(ariaLabel).not.toBe('Edit Edit value');

    // Clean up
    await apiClient.deleteNote(note.ID);
  });

  test.afterAll(async ({ apiClient }) => {
    if (groupId) await apiClient.deleteGroup(groupId);
    if (categoryId) await apiClient.deleteCategory(categoryId);
  });
});
