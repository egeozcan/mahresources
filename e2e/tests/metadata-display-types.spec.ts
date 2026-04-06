/**
 * Tests for type-aware metadata rendering in the redesigned metadata table.
 */
import { test, expect } from '../fixtures/base.fixture';

test.describe.serial('Metadata Display Type Rendering', () => {
  let categoryId: number;
  let groupId: number;
  const testRunId = Date.now();

  const testMeta = {
    id: 3360,
    data: 1623661253000,         // millisecond timestamp → date
    name: 'testuser',
    active: 1,                   // boolean-like key
    is_verified: 0,              // boolean-like key
    parent_id: 1948,             // ID field
    count: 42,                   // plain number (not timestamp, not boolean)
    website: 'https://example.com/user/profile',
    tags: [],                    // empty array
    settings: { theme: 'dark', lang: 'en' },  // non-empty object
    bio: 'This is a long biography text that should be truncated after thirty characters',
  };

  test.beforeAll(async ({ apiClient }) => {
    const category = await apiClient.createCategory(
      `Meta Type Test Category ${testRunId}`,
      'Category for metadata type rendering test'
    );
    categoryId = category.ID;

    const group = await apiClient.createGroup({
      name: `Meta Type Test Group ${testRunId}`,
      description: 'Group for metadata type rendering test',
      categoryId: category.ID,
    });
    groupId = group.ID;

    const formData = new URLSearchParams();
    formData.append('ID', group.ID.toString());
    formData.append('Name', `Meta Type Test Group ${testRunId}`);
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

  test('timestamps render as formatted dates', async ({ page }) => {
    await page.goto(`/group?id=${groupId}`);
    const table = page.locator('.jsonTable').first();
    await expect(table).toBeVisible();

    // The epoch 1623661253000 is Jun 14, 2021 — should render as date text, not raw number
    const dataCell = table.locator('tr', { has: page.locator('th', { hasText: /^data$/ }) }).locator('td .metaVal--date');
    await expect(dataCell).toBeVisible();
    await expect(dataCell).toContainText('2021');
    // Raw number should NOT appear
    await expect(table.locator('td', { hasText: '1623661253000' })).toHaveCount(0);
  });

  test('boolean-like fields render with dot indicator', async ({ page }) => {
    await page.goto(`/group?id=${groupId}`);
    const table = page.locator('.jsonTable').first();

    // "active: 1" should render as boolean (green dot + "yes")
    const activeRow = table.locator('tr', { has: page.locator('th', { hasText: /^active$/ }) });
    const activeBool = activeRow.locator('.metaVal--bool');
    await expect(activeBool).toBeVisible();
    await expect(activeBool).toContainText('yes');
    await expect(activeRow.locator('.metaVal--bool-dot--on')).toBeVisible();

    // "is_verified: 0" should render as boolean (gray dot + "no")
    const verifiedRow = table.locator('tr', { has: page.locator('th', { hasText: /^is_verified$/ }) });
    const verifiedBool = verifiedRow.locator('.metaVal--bool');
    await expect(verifiedBool).toBeVisible();
    await expect(verifiedBool).toContainText('no');
    await expect(verifiedRow.locator('.metaVal--bool-dot--off')).toBeVisible();
  });

  test('ID fields render in muted style', async ({ page }) => {
    await page.goto(`/group?id=${groupId}`);
    const table = page.locator('.jsonTable').first();

    const idCell = table.locator('tr', { has: page.locator('th', { hasText: /^id$/ }) }).locator('.metaVal--id');
    await expect(idCell).toBeVisible();
    await expect(idCell).toContainText('3360');

    const parentCell = table.locator('tr', { has: page.locator('th', { hasText: /^parent_id$/ }) }).locator('.metaVal--id');
    await expect(parentCell).toBeVisible();
    await expect(parentCell).toContainText('1948');
  });

  test('URLs render as clickable links', async ({ page }) => {
    await page.goto(`/group?id=${groupId}`);
    const table = page.locator('.jsonTable').first();

    const urlLink = table.locator('.metaVal--url');
    await expect(urlLink).toBeVisible();
    await expect(urlLink).toHaveAttribute('href', 'https://example.com/user/profile');
    await expect(urlLink).toContainText('example.com');
  });

  test('empty arrays show "empty — show" button', async ({ page }) => {
    await page.goto(`/group?id=${groupId}`);
    const table = page.locator('.jsonTable').first();

    // hasSubTable rows have the toggler inside the th, so the th text includes
    // both the key name and the toggler label — match loosely.
    const tagsRow = table.locator('tr.hasSubTable', { has: page.locator('th', { hasText: 'tags' }) });
    const toggler = tagsRow.locator('.metaToggler');
    await expect(toggler).toBeVisible();
    await expect(toggler).toContainText('empty');
    await expect(toggler).toContainText('show');
  });

  test('non-empty objects show "N keys — show" and expand on click', async ({ page }) => {
    await page.goto(`/group?id=${groupId}`);
    const table = page.locator('.jsonTable').first();

    const settingsRow = table.locator('tr.hasSubTable', { has: page.locator('th', { hasText: 'settings' }) });
    const toggler = settingsRow.locator('.metaToggler');
    await expect(toggler).toBeVisible();
    await expect(toggler).toContainText('2 keys');
    await expect(toggler).toContainText('show');

    // Click to expand
    await toggler.click();
    await expect(toggler).toContainText('hide');
    await expect(toggler).toHaveClass(/expanded/);

    // Nested table should be visible
    const nestedTable = settingsRow.locator('.jsonTable');
    await expect(nestedTable).toBeVisible();
    await expect(nestedTable.locator('th', { hasText: 'theme' })).toBeVisible();
  });

  test('expand button toggles fullscreen', async ({ page }) => {
    await page.goto(`/group?id=${groupId}`);

    const expandBtn = page.locator('.metaExpandBtn');
    await expect(expandBtn).toBeVisible();
    await expect(expandBtn).toContainText('Expand');

    await expandBtn.click();

    const container = page.locator('.tableContainer');
    await expect(container).toHaveClass(/expanded/);
    await expect(expandBtn).toContainText('Minimize');

    // Click minimize
    await expandBtn.click();
    await expect(container).not.toHaveClass(/expanded/);
  });

  test('plain numbers are not converted to dates or booleans', async ({ page }) => {
    await page.goto(`/group?id=${groupId}`);
    const table = page.locator('.jsonTable').first();

    // count: 42 should render as plain text, not a date and not a boolean
    const countRow = table.locator('tr', { has: page.locator('th', { hasText: /^count$/ }) });
    await expect(countRow.locator('.metaVal--date')).toHaveCount(0);
    await expect(countRow.locator('.metaVal--bool')).toHaveCount(0);
    await expect(countRow.locator('td')).toContainText('42');
  });

  test('copy-on-click shows flash and tooltip', async ({ page, context }) => {
    await context.grantPermissions(['clipboard-read', 'clipboard-write']);
    await page.goto(`/group?id=${groupId}`);

    const table = page.locator('.jsonTable').first();
    // Click on count value cell (42) — plain number, renders as text in td
    const countCell = table.locator('tr', { has: page.locator('th', { hasText: /^count$/ }) }).locator('td');
    await countCell.click();

    // Tooltip should appear
    const tooltip = countCell.locator('.copyTooltip');
    await expect(tooltip).toBeVisible();
    await expect(tooltip).toHaveText('Copied!');

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
