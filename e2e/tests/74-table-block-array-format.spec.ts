import { test, expect } from '../fixtures/base.fixture';

test.describe('Table block with array-format data', () => {
  let categoryId: number;
  let ownerGroupId: number;
  let noteId: number;

  test.beforeAll(async ({ apiClient }) => {
    const category = await apiClient.createCategory('ArrayTableCat', 'Cat for array table test');
    categoryId = category.ID;

    const ownerGroup = await apiClient.createGroup({
      name: 'ArrayTableOwner',
      categoryId: categoryId,
    });
    ownerGroupId = ownerGroup.ID;

    const note = await apiClient.createNote({
      name: 'ArrayTableNote',
      description: 'Note with array-format table block',
      ownerId: ownerGroupId,
    });
    noteId = note.ID;
  });

  test('should render table with array-format columns and rows without JS errors', async ({
    page,
    baseURL,
    apiClient,
  }) => {
    // Create a table block with array-format data (simple string columns, array rows)
    await apiClient.createBlock(noteId, 'table', 'a', {
      columns: ['Name', 'Value', 'Status'],
      rows: [
        ['Alice', '100', 'Active'],
        ['Bob', '200', 'Inactive'],
      ],
    });

    // Collect JS errors during page load
    const jsErrors: string[] = [];
    page.on('pageerror', (error) => {
      jsErrors.push(error.message);
    });

    await page.goto(`${baseURL}/note?id=${noteId}`);
    await page.waitForLoadState('load');

    // The table should render without JS errors
    expect(jsErrors).toHaveLength(0);

    // The block table should display (use the specific class from the block editor)
    const table = page.locator('table.min-w-full');
    await expect(table).toBeVisible();

    // Check column headers are rendered
    await expect(table.locator('th', { hasText: 'Name' })).toBeVisible();
    await expect(table.locator('th', { hasText: 'Value' })).toBeVisible();
    await expect(table.locator('th', { hasText: 'Status' })).toBeVisible();

    // Check row data is rendered
    await expect(table.locator('td', { hasText: 'Alice' })).toBeVisible();
    await expect(table.locator('td', { hasText: '100' })).toBeVisible();
    await expect(table.locator('td', { hasText: 'Bob' })).toBeVisible();
    await expect(table.locator('td', { hasText: '200' })).toBeVisible();
  });

  test.afterAll(async ({ apiClient }) => {
    if (noteId) await apiClient.deleteNote(noteId);
    if (ownerGroupId) await apiClient.deleteGroup(ownerGroupId);
    if (categoryId) await apiClient.deleteCategory(categoryId);
  });
});
