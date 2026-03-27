import { test, expect } from '../fixtures/base.fixture';

test.describe('Query Description Field', () => {
  let queryId: number;

  test('should have a Description textarea on the create form', async ({ queryPage, page }) => {
    await queryPage.gotoNew();
    const descriptionField = page.locator('textarea[name="Description"]');
    await expect(descriptionField).toBeVisible();
  });

  test('should create a query with a description', async ({ queryPage }) => {
    queryId = await queryPage.create({
      name: 'Described Query',
      text: 'SELECT 1',
      description: 'This query is for testing descriptions',
    });
    expect(queryId).toBeGreaterThan(0);
  });

  test('should display description on the detail page', async ({ queryPage, page }) => {
    expect(queryId, 'Query must be created first').toBeGreaterThan(0);
    await queryPage.gotoDisplay(queryId);
    await expect(page.locator('text=This query is for testing descriptions')).toBeVisible();
  });

  test('should preserve description when editing a query', async ({ queryPage, page }) => {
    expect(queryId, 'Query must be created first').toBeGreaterThan(0);
    await queryPage.gotoEdit(queryId);
    const descriptionField = page.locator('textarea[name="Description"]');
    await expect(descriptionField).toHaveValue('This query is for testing descriptions');
  });

  test.afterAll(async ({ apiClient }) => {
    if (queryId) {
      await apiClient.deleteQuery(queryId);
    }
  });
});

test.describe('Query SQL Reference Completeness', () => {
  test('should list resource_versions table in SQL reference', async ({ queryPage, page }) => {
    await queryPage.gotoNew();
    // Expand the SQL reference panel
    await page.locator('button:has-text("SQL query reference")').click();
    await expect(page.locator('code:has-text("resource_versions")')).toBeVisible();
  });

  test('should list series table in SQL reference', async ({ queryPage, page }) => {
    await queryPage.gotoNew();
    await page.locator('button:has-text("SQL query reference")').click();
    await expect(page.locator('code:has-text("series")')).toBeVisible();
  });

  test('should list note_blocks table in SQL reference', async ({ queryPage, page }) => {
    await queryPage.gotoNew();
    await page.locator('button:has-text("SQL query reference")').click();
    await expect(page.locator('code:has-text("note_blocks")')).toBeVisible();
  });

  test('should list log_entries table in SQL reference', async ({ queryPage, page }) => {
    await queryPage.gotoNew();
    await page.locator('button:has-text("SQL query reference")').click();
    await expect(page.locator('code:has-text("log_entries")')).toBeVisible();
  });

  test('should list description column for queries table', async ({ queryPage, page }) => {
    await queryPage.gotoNew();
    await page.locator('button:has-text("SQL query reference")').click();
    // The queries table entry should mention description
    const queriesEntry = page.locator('div:has(> code:has-text("queries")) span');
    await expect(queriesEntry).toContainText('description');
  });
});
