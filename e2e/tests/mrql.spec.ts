import { test, expect } from '../fixtures/base.fixture';
import { getWorkerBaseUrl } from '../fixtures/base.fixture';
import { ApiClient } from '../helpers/api-client';
import { MRQLPage } from '../pages/MRQLPage';
import { request as playwrightRequest } from '@playwright/test';
import * as path from 'path';

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

    // Enter a query that should match our test data (use * wildcard for contains)
    await mrql.enterQuery('name ~ "*MRQL Test*"');
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
    const queryText = 'name ~ "*MRQL Test*"';

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

    // Enter a query (use * wildcard for contains)
    await mrql.enterQuery('name ~ "*MRQL Test*"');

    // Execute using keyboard shortcut
    await mrql.executeQueryWithKeyboard();

    // Verify results appear
    const count = await mrql.getResultCount();
    expect(count).toBeGreaterThan(0);
  });
});

test.describe('MRQL Docs Panel', () => {
  const expectedHeadings = [
    'Syntax Overview',
    'Entity Types',
    'Common Fields',
    'Resource Fields',
    'Note Fields',
    'Group Fields',
    'Operators',
    'Wildcards in',
    'Full-Text Search',
    'Existence Checks',
    'Set Membership',
    'Boolean Logic',
    'Relative Dates',
    'Date Functions',
    'File Size Units',
    'String Escaping',
    'Traversal',
    'Cross-Entity Queries',
    'ORDER BY / LIMIT / OFFSET',
    'GROUP BY',
    'Examples',
  ];

  test('docs panel shows all section headings', async ({ page }) => {
    const mrql = new MRQLPage(page);
    await mrql.navigate();
    await mrql.openDocs();

    const headings = mrql.docsPanel.locator('h3');
    const count = await headings.count();
    expect(count).toBe(expectedHeadings.length);

    for (const heading of expectedHeadings) {
      await expect(mrql.docsPanel.locator('h3', { hasText: heading }).first()).toBeVisible();
    }
  });

  test('docs panel renders example code blocks', async ({ page }) => {
    const mrql = new MRQLPage(page);
    await mrql.navigate();
    await mrql.openDocs();

    const preBlocks = mrql.docsPanel.locator('pre');
    const preCount = await preBlocks.count();
    // At least 20 pre blocks across all sections
    expect(preCount).toBeGreaterThanOrEqual(20);

    // Spot-check a few example blocks
    await expect(mrql.docsPanel.locator('pre', { hasText: 'contentType ~ "image/*"' })).toBeVisible();
    await expect(mrql.docsPanel.locator('pre', { hasText: 'TEXT ~ "quarterly earnings"' })).toBeVisible();
  });

  test('openDocs is idempotent', async ({ page }) => {
    const mrql = new MRQLPage(page);
    await mrql.navigate();

    // Call openDocs twice — panel should stay open
    await mrql.openDocs();
    await mrql.openDocs();
    await expect(mrql.docsPanel).toBeVisible();
  });
});

test.describe('MRQL GROUP BY', () => {
  // Track created entity IDs for cleanup
  let categoryId: number;
  let groupId: number;
  const resourceIds: number[] = [];

  test.beforeAll(async () => {
    const baseUrl = getWorkerBaseUrl();
    const ctx = await playwrightRequest.newContext({ baseURL: baseUrl });
    const api = new ApiClient(ctx, baseUrl);
    const suffix = `gb-${Date.now()}`;

    // Create a category and group (needed for resource ownership)
    const category = await api.createCategory(`GB Test Category ${suffix}`);
    categoryId = category.ID;

    const group = await api.createGroup({
      name: `GB Test Group ${suffix}`,
      description: 'Group for GROUP BY E2E tests',
      categoryId: categoryId,
    });
    groupId = group.ID;

    // Create resources with different content types:
    // 2 images (image/png) and 1 text file (text/plain)
    const imgPath = path.join(__dirname, '../test-assets/sample-image.png');
    const imgPath2 = path.join(__dirname, '../test-assets/sample-image-2.png');
    const txtPath = path.join(__dirname, '../test-assets/sample-document.txt');

    const r1 = await api.createResource({
      filePath: imgPath,
      name: `GB Image 1 ${suffix}`,
      ownerId: groupId,
    });
    resourceIds.push(r1.ID);

    const r2 = await api.createResource({
      filePath: imgPath2,
      name: `GB Image 2 ${suffix}`,
      ownerId: groupId,
    });
    resourceIds.push(r2.ID);

    const r3 = await api.createResource({
      filePath: txtPath,
      name: `GB Doc ${suffix}`,
      ownerId: groupId,
    });
    resourceIds.push(r3.ID);

    await ctx.dispose();
  });

  test.afterAll(async () => {
    const baseUrl = getWorkerBaseUrl();
    const ctx = await playwrightRequest.newContext({ baseURL: baseUrl });
    const api = new ApiClient(ctx, baseUrl);

    for (const id of resourceIds) {
      try { await api.deleteResource(id); } catch { /* ignore */ }
    }
    try { if (groupId) await api.deleteGroup(groupId); } catch { /* ignore */ }
    try { if (categoryId) await api.deleteCategory(categoryId); } catch { /* ignore */ }

    await ctx.dispose();
  });

  test('aggregated mode renders table', async ({ page }) => {
    const mrql = new MRQLPage(page);
    await mrql.navigate();

    await mrql.enterQuery('type = resource GROUP BY contentType COUNT()');
    await mrql.executeQuery();

    // Results heading should mention "rows" (aggregated mode)
    const heading = mrql.resultsSection.locator('h2');
    await expect(heading).toContainText('rows');

    // A <table> element should be present
    const table = mrql.resultsSection.locator('table');
    await expect(table).toBeVisible();

    // Table should have <th> headers including contentType and count
    const headers = table.locator('thead th');
    const headerCount = await headers.count();
    expect(headerCount).toBeGreaterThanOrEqual(2);

    const headerTexts: string[] = [];
    for (let i = 0; i < headerCount; i++) {
      const text = await headers.nth(i).textContent();
      if (text) headerTexts.push(text.trim().toLowerCase());
    }
    expect(headerTexts).toContain('contenttype');
    expect(headerTexts).toContain('count');

    // At least one data row should exist
    const dataRows = table.locator('tbody tr');
    const rowCount = await dataRows.count();
    expect(rowCount).toBeGreaterThan(0);
  });

  test('bucketed mode renders groups', async ({ page }) => {
    const mrql = new MRQLPage(page);
    await mrql.navigate();

    await mrql.enterQuery('type = resource GROUP BY contentType LIMIT 5');
    await mrql.executeQuery();

    // Results heading should mention "groups" (bucketed mode)
    const heading = mrql.resultsSection.locator('h2');
    await expect(heading).toContainText('groups');

    // Bucket headers (bg-stone-100 divs inside bordered containers) should have key labels
    const bucketHeaders = mrql.resultsSection.locator('.bg-stone-100');
    await expect(bucketHeaders.first()).toBeVisible();
    const firstHeaderText = await bucketHeaders.first().textContent();
    expect(firstHeaderText).toContain('contentType');

    // Entity cards should appear within groups (links to entity pages)
    const entityCards = mrql.resultsSection.locator('a[href*="?id="]');
    const cardCount = await entityCards.count();
    expect(cardCount).toBeGreaterThan(0);
  });

  test('aggregated with multiple aggregates', async ({ page }) => {
    const mrql = new MRQLPage(page);
    await mrql.navigate();

    await mrql.enterQuery('type = resource GROUP BY contentType COUNT() SUM(fileSize)');
    await mrql.executeQuery();

    const table = mrql.resultsSection.locator('table');
    await expect(table).toBeVisible();

    // Table headers should include count and sum_filesize (or similar)
    const headers = table.locator('thead th');
    const headerCount = await headers.count();
    expect(headerCount).toBeGreaterThanOrEqual(3); // contentType, count, sum_fileSize

    const headerTexts: string[] = [];
    for (let i = 0; i < headerCount; i++) {
      const text = await headers.nth(i).textContent();
      if (text) headerTexts.push(text.trim().toLowerCase());
    }
    expect(headerTexts).toContain('count');
    // The sum column name varies by implementation — check for any header containing "sum" or "filesize"
    const hasSumColumn = headerTexts.some(h => h.includes('sum') || h.includes('filesize'));
    expect(hasSumColumn).toBe(true);
  });

  test('GROUP BY with traversal (owner.name)', async ({ page }) => {
    const mrql = new MRQLPage(page);
    await mrql.navigate();

    await mrql.enterQuery('type = resource GROUP BY owner.name COUNT()');
    await mrql.executeQuery();

    // Should not show an error — either results or empty state
    const errorText = await mrql.getErrors();
    expect(errorText).toBeFalsy();

    // Results heading should be visible
    const heading = mrql.resultsSection.locator('h2');
    await expect(heading).toBeVisible();
  });

  test('GROUP BY validation error without entity type', async ({ page }) => {
    const mrql = new MRQLPage(page);
    await mrql.navigate();

    await mrql.enterQuery('name ~ "test" GROUP BY name COUNT()');
    await mrql.executeQuery();

    // Should show an error about requiring entity type
    const errorText = await mrql.getErrors();
    expect(errorText).toBeTruthy();
    expect(errorText!.toLowerCase()).toContain('type');
  });

  test('GROUP BY meta field', async ({ page }) => {
    const mrql = new MRQLPage(page);
    await mrql.navigate();

    await mrql.enterQuery('type = resource GROUP BY meta.source COUNT()');
    await mrql.executeQuery();

    // Should not crash — either results table or empty state
    const errorText = await mrql.getErrors();
    expect(errorText).toBeFalsy();

    // Results section should be visible
    const heading = mrql.resultsSection.locator('h2');
    await expect(heading).toBeVisible();
  });
});

test.describe('MRQL SCOPE', () => {
  // Track created entity IDs for cleanup
  let categoryId: number;
  let parentGroupId: number;
  let childGroupId: number;
  let grandchildGroupId: number;
  let scopedResourceId: number;
  let unscopedResourceId: number;
  let duplicateGroupId: number;

  const suffix = `scope-${Date.now()}`;

  test.beforeAll(async () => {
    const baseUrl = getWorkerBaseUrl();
    const ctx = await playwrightRequest.newContext({ baseURL: baseUrl });
    const api = new ApiClient(ctx, baseUrl);

    // Create a category (required for groups)
    const category = await api.createCategory(`Scope Test Category ${suffix}`);
    categoryId = category.ID;

    // Create group hierarchy: parent → child → grandchild
    const parent = await api.createGroup({
      name: `Scope Parent ${suffix}`,
      description: 'Parent group for SCOPE tests',
      categoryId: categoryId,
    });
    parentGroupId = parent.ID;

    const child = await api.createGroup({
      name: `Scope Child ${suffix}`,
      description: 'Child group for SCOPE tests',
      categoryId: categoryId,
      ownerId: parentGroupId,
    });
    childGroupId = child.ID;

    const grandchild = await api.createGroup({
      name: `Scope Grandchild ${suffix}`,
      description: 'Grandchild group for SCOPE tests',
      categoryId: categoryId,
      ownerId: childGroupId,
    });
    grandchildGroupId = grandchild.ID;

    // Create a resource owned by the child group (scoped)
    const scopedRes = await api.createResource({
      filePath: path.join(__dirname, '../test-assets/sample-image-38.png'),
      name: `Scoped Resource ${suffix}`,
      ownerId: childGroupId,
    });
    scopedResourceId = scopedRes.ID;

    // Create a resource with no owner (unscoped — outside the hierarchy)
    // Use a different image file to avoid duplicate hash conflict
    const unscopedRes = await api.createResource({
      filePath: path.join(__dirname, '../test-assets/sample-image-39.png'),
      name: `Unscoped Resource ${suffix}`,
    });
    unscopedResourceId = unscopedRes.ID;

    await ctx.dispose();
  });

  test.afterAll(async () => {
    const baseUrl = getWorkerBaseUrl();
    const ctx = await playwrightRequest.newContext({ baseURL: baseUrl });
    const api = new ApiClient(ctx, baseUrl);

    // Cleanup in reverse dependency order
    try { if (scopedResourceId) await api.deleteResource(scopedResourceId); } catch { /* ignore */ }
    try { if (unscopedResourceId) await api.deleteResource(unscopedResourceId); } catch { /* ignore */ }
    try { if (duplicateGroupId) await api.deleteGroup(duplicateGroupId); } catch { /* ignore */ }
    try { if (grandchildGroupId) await api.deleteGroup(grandchildGroupId); } catch { /* ignore */ }
    try { if (childGroupId) await api.deleteGroup(childGroupId); } catch { /* ignore */ }
    try { if (parentGroupId) await api.deleteGroup(parentGroupId); } catch { /* ignore */ }
    try { if (categoryId) await api.deleteCategory(categoryId); } catch { /* ignore */ }

    await ctx.dispose();
  });

  test('SCOPE by ID returns subtree resources', async ({ page }) => {
    const mrql = new MRQLPage(page);
    await mrql.navigate();

    await mrql.enterQuery(`type = "resource" SCOPE ${parentGroupId}`);
    await mrql.executeQuery();

    // Should not show an error
    const errorText = await mrql.getErrors();
    expect(errorText).toBeFalsy();

    // The scoped resource should appear
    const resultsText = await mrql.resultsSection.textContent();
    expect(resultsText).toContain(`Scoped Resource ${suffix}`);

    // The unscoped resource should NOT appear
    expect(resultsText).not.toContain(`Unscoped Resource ${suffix}`);
  });

  test('SCOPE by name returns subtree resources', async ({ page }) => {
    const mrql = new MRQLPage(page);
    await mrql.navigate();

    await mrql.enterQuery(`type = "resource" SCOPE "Scope Parent ${suffix}"`);
    await mrql.executeQuery();

    // Should not show an error
    const errorText = await mrql.getErrors();
    expect(errorText).toBeFalsy();

    // The scoped resource should appear
    const resultsText = await mrql.resultsSection.textContent();
    expect(resultsText).toContain(`Scoped Resource ${suffix}`);
  });

  test('SCOPE for groups includes the scoped group itself', async ({ page }) => {
    const mrql = new MRQLPage(page);
    await mrql.navigate();

    await mrql.enterQuery(`type = "group" SCOPE ${parentGroupId}`);
    await mrql.executeQuery();

    // Should not show an error
    const errorText = await mrql.getErrors();
    expect(errorText).toBeFalsy();

    // All 3 groups in the hierarchy should appear
    const resultsText = await mrql.resultsSection.textContent();
    expect(resultsText).toContain(`Scope Parent ${suffix}`);
    expect(resultsText).toContain(`Scope Child ${suffix}`);
    expect(resultsText).toContain(`Scope Grandchild ${suffix}`);
  });

  test('SCOPE with nonexistent ID shows error', async ({ page }) => {
    const mrql = new MRQLPage(page);
    await mrql.navigate();

    await mrql.enterQuery('type = "resource" SCOPE 999999');
    await mrql.executeQuery();

    // Should show an error about the scope group not being found
    const errorText = await mrql.getErrors();
    expect(errorText).toBeTruthy();
    expect(errorText!.toLowerCase()).toContain('scope group not found');
  });

  test('SCOPE with ambiguous name shows error', async ({ page }) => {
    // Create a second group with the same name to trigger ambiguity
    const baseUrl = getWorkerBaseUrl();
    const ctx = await playwrightRequest.newContext({ baseURL: baseUrl });
    const api = new ApiClient(ctx, baseUrl);

    const dupGroup = await api.createGroup({
      name: `Scope Parent ${suffix}`,
      description: 'Duplicate group to test ambiguous scope',
      categoryId: categoryId,
    });
    duplicateGroupId = dupGroup.ID;
    await ctx.dispose();

    const mrql = new MRQLPage(page);
    await mrql.navigate();

    await mrql.enterQuery(`type = "resource" SCOPE "Scope Parent ${suffix}"`);
    await mrql.executeQuery();

    // Should show an error about ambiguous scope
    const errorText = await mrql.getErrors();
    expect(errorText).toBeTruthy();
    expect(errorText!.toLowerCase()).toContain('ambiguous');
  });
});
