import { test, expect } from '../fixtures/base.fixture';
import { getWorkerBaseUrl } from '../fixtures/base.fixture';
import { ApiClient } from '../helpers/api-client';
import { MRQLPage } from '../pages/MRQLPage';
import { request as playwrightRequest } from '@playwright/test';

test.describe('MRQL Page', () => {
  // Track created entity IDs for cleanup
  let categoryId: number;
  let tagId: number;
  let groupId: number;
  let noteTypeId: number;
  let noteId: number;

  test.beforeAll(async () => {
    const baseUrl = getWorkerBaseUrl();
    const ctx = await playwrightRequest.newContext({ baseURL: baseUrl });
    const api = new ApiClient(ctx, baseUrl);
    const suffix = `mrql-${Date.now()}`;

    // Create test data in dependency order
    const category = await api.createCategory(`MRQL Test Category ${suffix}`);
    categoryId = category.ID;

    const tag = await api.createTag(`mrql-test-tag-${suffix}`);
    tagId = tag.ID;

    const group = await api.createGroup({
      name: `MRQL Test Group ${suffix}`,
      description: 'A group created for MRQL E2E tests',
      categoryId: categoryId,
    });
    groupId = group.ID;

    // Add tag to the group
    await api.addTagsToGroups([groupId], [tagId]);

    const noteType = await api.createNoteType(`MRQL Test NoteType ${suffix}`);
    noteTypeId = noteType.ID;

    const note = await api.createNote({
      name: `MRQL Test Note ${suffix}`,
      description: 'A note for MRQL testing',
      noteTypeId: noteTypeId,
    });
    noteId = note.ID;

    await ctx.dispose();
  });

  test.afterAll(async () => {
    const baseUrl = getWorkerBaseUrl();
    const ctx = await playwrightRequest.newContext({ baseURL: baseUrl });
    const api = new ApiClient(ctx, baseUrl);

    // Cleanup in reverse dependency order
    try { if (noteId) await api.deleteNote(noteId); } catch { /* ignore */ }
    try { if (groupId) await api.deleteGroup(groupId); } catch { /* ignore */ }
    try { if (noteTypeId) await api.deleteNoteType(noteTypeId); } catch { /* ignore */ }
    try { if (tagId) await api.deleteTag(tagId); } catch { /* ignore */ }
    try { if (categoryId) await api.deleteCategory(categoryId); } catch { /* ignore */ }

    await ctx.dispose();
  });

  test('page loads with CodeMirror editor visible', async ({ page }) => {
    const mrql = new MRQLPage(page);
    await mrql.navigate();

    // Verify the editor container is visible
    await expect(mrql.editorContainer).toBeVisible();

    // Verify CodeMirror is initialized (has .cm-editor child)
    await expect(mrql.editorContainer.locator('.cm-editor')).toBeVisible();
    await expect(mrql.editorContainer.locator('.cm-content')).toBeVisible();

    // Verify Run button exists
    await expect(mrql.runButton).toBeVisible();

    // Verify Save button exists
    await expect(mrql.saveButton).toBeVisible();

    // Verify Saved Queries section exists
    await expect(mrql.savedQueriesSection).toBeVisible();
  });

  test('enter a simple query, execute, and verify results appear', async ({ page }) => {
    const mrql = new MRQLPage(page);
    await mrql.navigate();

    // Enter a query that should match our test data
    await mrql.enterQuery('name ~ "MRQL Test"');
    await mrql.executeQuery();

    // Verify results section shows up with results
    const count = await mrql.getResultCount();
    expect(count).toBeGreaterThan(0);

    // Verify result links are present
    const results = await mrql.getResults();
    await expect(results.first()).toBeVisible();
  });

  test('enter invalid query and verify error feedback', async ({ page }) => {
    const mrql = new MRQLPage(page);
    await mrql.navigate();

    // Enter a syntactically invalid query
    await mrql.enterQuery('INVALID $$$ SYNTAX !!!');

    // Wait for validation to trigger (debounced at 500ms)
    await page.waitForTimeout(800);

    // Validation error should appear
    const validationErr = await mrql.getValidationError();
    expect(validationErr).toBeTruthy();

    // Try executing it — should show execution error
    await mrql.executeQuery();
    const execError = await mrql.getErrors();
    expect(execError).toBeTruthy();
  });

  test('save a query and verify it appears in saved list', async ({ page }) => {
    const mrql = new MRQLPage(page);
    await mrql.navigate();

    const queryName = `Saved E2E Query ${Date.now()}`;
    const queryText = 'name ~ "test"';

    // Enter a query
    await mrql.enterQuery(queryText);

    // Save it
    await mrql.saveQuery(queryName, 'E2E test saved query');

    // Verify it appears in the saved queries list
    const savedNames = await mrql.getSavedQueryNames();
    expect(savedNames).toContain(queryName);
  });

  test('load and run a saved query', async ({ page }) => {
    const mrql = new MRQLPage(page);
    await mrql.navigate();

    const queryName = `Load Test Query ${Date.now()}`;
    const queryText = 'name ~ "MRQL Test"';

    // First save a query
    await mrql.enterQuery(queryText);
    await mrql.saveQuery(queryName);

    // Clear the editor by entering an empty-ish placeholder
    await mrql.enterQuery('');

    // Load the saved query
    await mrql.loadSavedQuery(queryName);

    // Verify the editor now contains the saved query text
    const editorContent = await mrql.getEditorContent();
    expect(editorContent.trim()).toBe(queryText);

    // Execute it and verify results
    await mrql.executeQuery();
    const count = await mrql.getResultCount();
    expect(count).toBeGreaterThan(0);
  });

  test('keyboard shortcut (Ctrl/Meta+Enter) executes query', async ({ page }) => {
    const mrql = new MRQLPage(page);
    await mrql.navigate();

    // Enter a query
    await mrql.enterQuery('name ~ "MRQL Test"');

    // Execute using keyboard shortcut
    await mrql.executeQueryWithKeyboard();

    // Verify results appear
    const count = await mrql.getResultCount();
    expect(count).toBeGreaterThan(0);
  });
});
