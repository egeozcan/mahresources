import { test, expect } from '../fixtures/base.fixture';
import path from 'path';

/**
 * Bug: In listResourcesDetails.tpl, the <thead> row uses a bare <td></td> for
 * the checkbox column instead of <th scope="col">.  WCAG 1.3.1 (Info and
 * Relationships) requires that table header cells use <th> so assistive
 * technology can associate data cells with their column headers.
 *
 * Screen readers announce column headers when a user navigates table cells.
 * An empty <td> in <thead> means the checkbox column has no programmatic
 * header, making it impossible for screen-reader users to understand what that
 * column contains.
 *
 * The fix is to replace the <td></td> in the <thead> with:
 *   <th scope="col"><span class="sr-only">Select</span></th>
 */
test.describe('Detail table column headers use <th> elements', () => {
  let categoryId: number;
  let groupId: number;
  let resourceId: number;
  const testRunId = `${Date.now()}`;

  test.beforeAll(async ({ apiClient }) => {
    // Create prerequisite data so the details table has at least one row
    const category = await apiClient.createCategory(
      `DetailTH Cat ${testRunId}`,
      'Category for detail table th test'
    );
    categoryId = category.ID;

    const group = await apiClient.createGroup({
      name: `DetailTH Group ${testRunId}`,
      categoryId: category.ID,
    });
    groupId = group.ID;

    const resource = await apiClient.createResource({
      filePath: path.join(__dirname, '../test-assets/sample-image.png'),
      name: `DetailTH Resource ${testRunId}`,
      ownerId: group.ID,
    });
    resourceId = resource.ID;
  });

  test('every column in the resources detail table <thead> should be a <th>, not a <td>', async ({
    page,
  }) => {
    // Navigate to the resources detail/table view
    await page.goto('/resources/details');
    await page.waitForLoadState('load');

    // Ensure the table is present and has at least one data row
    const table = page.locator('table');
    await expect(table).toBeVisible({ timeout: 10000 });

    const bodyRows = table.locator('tbody tr');
    await expect(bodyRows.first()).toBeVisible({ timeout: 10000 });

    // Count <td> elements inside <thead> — there should be zero.
    // Every cell in the header row must be a <th>.
    const theadTdCount = await table.locator('thead td').count();

    expect(
      theadTdCount,
      `Found ${theadTdCount} <td> element(s) inside <thead>. ` +
        'All header cells should use <th scope="col"> for WCAG 1.3.1 compliance.'
    ).toBe(0);
  });

  test('the first header cell (checkbox column) should be a <th> with an accessible label', async ({
    page,
  }) => {
    await page.goto('/resources/details');
    await page.waitForLoadState('load');

    const table = page.locator('table');
    await expect(table).toBeVisible({ timeout: 10000 });

    // The first cell in the header row is the checkbox column.
    // It must be a <th> (not <td>) AND have an accessible label.
    const firstHeaderCell = table.locator('thead tr').first().locator('th, td').first();
    await expect(firstHeaderCell).toBeVisible();

    // Verify it is a <th>, not a <td>
    const tagName = await firstHeaderCell.evaluate(el => el.tagName.toLowerCase());
    expect(
      tagName,
      'The first header cell should be a <th>, not a <td>.'
    ).toBe('th');

    // Verify it has an accessible name (text content, sr-only span, or aria-label)
    const textContent = (await firstHeaderCell.textContent())?.trim() ?? '';
    const ariaLabel = await firstHeaderCell.getAttribute('aria-label');
    const hasAccessibleName =
      textContent.length > 0 ||
      (ariaLabel !== null && ariaLabel.trim().length > 0);

    expect(
      hasAccessibleName,
      'The checkbox column <th> must have an accessible name ' +
        '(e.g., <span class="sr-only">Select</span> or aria-label="Select").'
    ).toBe(true);
  });

  test.afterAll(async ({ apiClient }) => {
    try {
      if (resourceId) await apiClient.deleteResource(resourceId);
    } catch { /* ignore cleanup errors */ }
    try {
      if (groupId) await apiClient.deleteGroup(groupId);
    } catch { /* ignore cleanup errors */ }
    try {
      if (categoryId) await apiClient.deleteCategory(categoryId);
    } catch { /* ignore cleanup errors */ }
  });
});
