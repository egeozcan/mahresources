/**
 * Tests that clearing the Template field on the query edit page actually
 * removes the template.
 *
 * Bug: The backend UpdateQuery function uses `if queryQuery.Template != ""`
 * to decide whether to update the Template field. When the user clears the
 * Template in the edit form, the empty string is treated as "no change"
 * instead of "clear the template", so the old template silently persists.
 *
 * Steps to reproduce:
 * 1. Create a query with a non-empty Template
 * 2. Go to the query edit page
 * 3. Clear the Template field
 * 4. Click Save
 * 5. Verify the template was actually removed
 *
 * Expected: Template is cleared (empty string)
 * Actual: Template retains its old value
 */
import { test, expect } from '../fixtures/base.fixture';

test.describe('Query edit: clearing Template field should persist', () => {
  let queryId: number;

  test.beforeAll(async ({ apiClient }) => {
    // Create a query WITH a template
    const query = await apiClient.createQuery({
      name: 'Template Clear Test Query',
      text: 'SELECT id, name FROM tags LIMIT 5',
      template: '<p x-text="results.length + \' rows\'"></p>',
    });
    queryId = query.ID;

    // Verify the template was saved
    const queries = await apiClient.getQueries();
    const saved = queries.find(q => q.ID === queryId);
    expect(saved).toBeDefined();
    expect(saved!.Text).toBe('SELECT id, name FROM tags LIMIT 5');
  });

  test('clearing the Template field in the edit form should remove it', async ({
    queryPage,
    page,
    apiClient,
  }) => {
    // Navigate to the query edit page
    await queryPage.gotoEdit(queryId);
    await expect(page.locator('h1')).toContainText('Edit Query');

    // The Template code editor should contain the template text
    const templateInput = page.locator('input[type="hidden"][name="Template"]');
    await templateInput.waitFor({ state: 'attached' });

    // Verify the template is currently set (non-empty)
    const currentTemplate = await templateInput.inputValue();
    expect(currentTemplate).not.toBe('');

    // Clear the Template field via the CodeMirror editor API
    await page.evaluate(() => {
      const input = document.querySelector(
        'input[type="hidden"][name="Template"]'
      ) as HTMLInputElement;
      if (!input) throw new Error('Template hidden input not found');

      // Clear the hidden input value
      input.value = '';

      // Also clear the CodeMirror editor if present
      const container = input.parentElement as any;
      if (container?._cmView) {
        const view = container._cmView;
        view.dispatch({
          changes: {
            from: 0,
            to: view.state.doc.length,
            insert: '',
          },
        });
      }
    });

    // Verify the hidden input is now empty before submitting
    const clearedValue = await templateInput.inputValue();
    expect(clearedValue).toBe('');

    // Save the form
    await queryPage.save();

    // Should redirect to the query display page
    await expect(page).toHaveURL(/\/query\?id=\d+/);

    // Verify the template was actually cleared via API
    const queries = await apiClient.getQueries();
    const updatedQuery = queries.find(q => q.ID === queryId);
    expect(updatedQuery).toBeDefined();
    expect(updatedQuery!.Template).toBe('');
  });

  test.afterAll(async ({ apiClient }) => {
    if (queryId) {
      await apiClient.deleteQuery(queryId);
    }
  });
});
