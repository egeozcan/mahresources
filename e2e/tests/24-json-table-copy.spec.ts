/**
 * Regression test: clicking value cells in JSON tables should copy the JSON path.
 *
 * Background: tableMaker uses event delegation with findTitledAncestor to walk
 * up from the click target to the nearest element with a title attribute (the
 * JSON path). In object tables, value td cells have no title — only the th and
 * tr have one. Without the ancestor walk-up, clicking value cells copies nothing.
 */
import { test, expect } from '../fixtures/base.fixture';

test.describe.serial('JSON Table Copy on Click', () => {
  let categoryId: number;
  let groupId: number;
  const testRunId = Date.now();

  const testMeta = {
    color: 'blue',
    count: 42,
    nested: { inner: 'value' },
  };

  test.beforeAll(async ({ apiClient }) => {
    const category = await apiClient.createCategory(
      `JSON Table Test Category ${testRunId}`,
      'Category for JSON table copy test'
    );
    categoryId = category.ID;

    const group = await apiClient.createGroup({
      name: `JSON Table Test Group ${testRunId}`,
      description: 'Group with metadata for JSON table copy testing',
      categoryId: category.ID,
    });
    groupId = group.ID;

    // Add metadata to the group via the API
    const formData = new URLSearchParams();
    formData.append('ID', group.ID.toString());
    formData.append('Name', `JSON Table Test Group ${testRunId}`);
    formData.append('categoryId', category.ID.toString());
    formData.append('Meta', JSON.stringify(testMeta));

    const response = await apiClient['request'].post(
      `${apiClient['baseUrl']}/v1/group`,
      {
        headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
        data: formData.toString(),
      }
    );
    expect(response.ok()).toBeTruthy();
  });

  test('should render JSON table with metadata on group page', async ({ page }) => {
    await page.goto(`/group?id=${groupId}`);

    // The JSON table should be rendered with the metadata
    const jsonTable = page.locator('.jsonTable').first();
    await expect(jsonTable).toBeVisible();

    // Should contain our metadata keys
    await expect(jsonTable.locator('th')).toContainText(['color']);
  });

  test('should copy JSON path when clicking a value cell (td without title)', async ({ page, context }) => {
    await context.grantPermissions(['clipboard-read', 'clipboard-write']);
    await page.goto(`/group?id=${groupId}`);

    const jsonTable = page.locator('.jsonTable').first();
    await expect(jsonTable).toBeVisible();

    // Find a value cell (td) that contains "blue" — this td has no title,
    // but its parent tr has title="$.color"
    const valueCell = jsonTable.locator('td', { hasText: 'blue' }).first();
    await expect(valueCell).toBeVisible();

    // Verify the td itself has no title (the bug scenario)
    const tdTitle = await valueCell.getAttribute('title');
    expect(tdTitle).toBeFalsy();

    // The parent row should have the title
    const rowTitle = await valueCell.locator('..').getAttribute('title');
    expect(rowTitle).toBe('$.color');

    // Click the value cell — should copy the path from the ancestor row
    await valueCell.click();

    // Verify clipboard contains the JSON path
    const clipboardText = await page.evaluate(() => navigator.clipboard.readText());
    expect(clipboardText).toBe('$.color');
  });

  test('should copy JSON path when clicking a header cell (th with title)', async ({ page, context }) => {
    await context.grantPermissions(['clipboard-read', 'clipboard-write']);
    await page.goto(`/group?id=${groupId}`);

    const jsonTable = page.locator('.jsonTable').first();
    await expect(jsonTable).toBeVisible();

    // Find the "count" header cell — th elements have their own title
    const headerCell = jsonTable.locator('th', { hasText: 'count' }).first();
    await expect(headerCell).toBeVisible();

    // Click the header cell
    await headerCell.click();

    // Verify clipboard contains the JSON path
    const clipboardText = await page.evaluate(() => navigator.clipboard.readText());
    expect(clipboardText).toBe('$.count');
  });

  test.afterAll(async ({ apiClient }) => {
    try {
      if (groupId) await apiClient.deleteGroup(groupId);
    } catch { /* ignore */ }
    try {
      if (categoryId) await apiClient.deleteCategory(categoryId);
    } catch { /* ignore */ }
  });
});
