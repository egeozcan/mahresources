/**
 * E2E tests for the data-views plugin shortcodes.
 * Tests that data viewing shortcodes render correctly with sample meta data.
 */
import { test, expect } from '../../fixtures/base.fixture';

test.describe('Data-views plugin shortcodes', () => {
  let categoryId: number;
  let groupId: number;
  let childGroupIds: number[] = [];
  let noteIds: number[] = [];

  test.beforeAll(async ({ apiClient }) => {
    await apiClient.enablePlugin('data-views');

    const cat = await apiClient.createCategory(
      `DataViews Test ${Date.now()}`,
      'Category for data-views E2E tests',
      {
        CustomHeader: [
          '[plugin:data-views:badge path="status" values="draft,review,done" colors="#9ca3af,#f59e0b,#22c55e" labels="Draft,Review,Done"]',
          '[plugin:data-views:format path="price" type="currency" decimals="2"]',
          '[plugin:data-views:stat-card path="visitors" label="Visitors" type="number" icon="users"]',
          '[plugin:data-views:meter path="health" min="0" max="100" low="30" high="70" label="Health"]',
          '[plugin:data-views:sparkline path="history" type="line" height="32" width="120"]',
          '[plugin:data-views:bar-chart path="scores" label-key="name" value-key="score"]',
          '[plugin:data-views:pie-chart path="distribution" label-key="category" value-key="amount" size="100"]',
          '[conditional path="featured" eq="true"]Featured Item[/conditional]',
        ].join('\n'),
        CustomSidebar: [
          '[plugin:data-views:list path="ingredients" style="bullet"]',
          '[plugin:data-views:count-badge path="tasks" count-where="done" eq="false" label="remaining"]',
          '[plugin:data-views:link-preview path="website"]',
          '[plugin:data-views:json-tree path="config"]',
          '[plugin:data-views:image path="avatar" width="64" height="64" rounded="true"]',
          '[plugin:data-views:table type="notes" cols="name,updated_at" labels="Name,Updated"]',
          '[plugin:data-views:barcode path="website" size="80"]',
          '[plugin:data-views:qr-code path="website" size="100"]',
        ].join('\n'),
      },
    );
    categoryId = cat.ID;

    const group = await apiClient.createGroup({
      name: `DataViews Demo ${Date.now()}`,
      categoryId: cat.ID,
      meta: JSON.stringify({
        status: 'review',
        price: 1234.5,
        visitors: 98432,
        health: 72,
        featured: true,
        history: [10, 25, 18, 42, 35, 50, 45, 60, 55, 70],
        scores: [
          { name: 'Alice', score: 92 },
          { name: 'Bob', score: 78 },
          { name: 'Carol', score: 85 },
        ],
        distribution: [
          { category: 'Web', amount: 45 },
          { category: 'Mobile', amount: 30 },
          { category: 'Desktop', amount: 25 },
        ],
        ingredients: ['flour', 'sugar', 'butter', 'eggs'],
        tasks: [
          { text: 'Task 1', done: true },
          { text: 'Task 2', done: false },
          { text: 'Task 3', done: false },
        ],
        website: 'https://github.com/egeozcan/mahresources',
        config: { theme: 'dark', notifications: true, maxItems: 50 },
        avatar: 'https://via.placeholder.com/64',
      }),
    });
    groupId = group.ID;

    // Create child entities for the table shortcode
    const n1 = await apiClient.createNote({
      name: `Test Note Alpha ${Date.now()}`,
      ownerId: group.ID,
    });
    noteIds.push(n1.ID);

    const n2 = await apiClient.createNote({
      name: `Test Note Beta ${Date.now()}`,
      ownerId: group.ID,
    });
    noteIds.push(n2.ID);
  });

  test.afterAll(async ({ apiClient }) => {
    for (const id of noteIds) await apiClient.deleteNote(id);
    for (const id of childGroupIds) await apiClient.deleteGroup(id);
    if (groupId) await apiClient.deleteGroup(groupId);
    if (categoryId) await apiClient.deleteCategory(categoryId);
  });

  test('badge renders styled pill for meta value', async ({ page }) => {
    await page.goto(`/group?id=${groupId}`);
    await page.waitForLoadState('load');

    // "Review" badge should be visible with amber color
    const badge = page.locator('.rounded-full', { hasText: 'Review' });
    await expect(badge).toBeVisible({ timeout: 5000 });
  });

  test('format renders formatted currency value', async ({ page }) => {
    await page.goto(`/group?id=${groupId}`);
    await page.waitForLoadState('load');

    // Should show "$1,234.50" formatted
    await expect(page.locator('text=$1,234.50')).toBeVisible({ timeout: 5000 });
  });

  test('stat-card renders KPI card with large number', async ({ page }) => {
    await page.goto(`/group?id=${groupId}`);
    await page.waitForLoadState('load');

    // Should show the visitor count and label in a stat card
    await expect(page.locator('main >> text=98,432')).toBeVisible({ timeout: 5000 });
    await expect(page.locator('main >> text=Visitors')).toBeVisible();
  });

  test('meter renders gauge bar', async ({ page }) => {
    await page.goto(`/group?id=${groupId}`);
    await page.waitForLoadState('load');

    // Should show "Health" label with the gauge value
    await expect(page.locator('main >> text=Health')).toBeVisible({ timeout: 5000 });
    await expect(page.locator('main >> text=72')).toBeVisible();
  });

  test('sparkline renders inline SVG', async ({ page }) => {
    await page.goto(`/group?id=${groupId}`);
    await page.waitForLoadState('load');

    // Should have an SVG element for the sparkline
    const svg = page.locator('main svg').first();
    await expect(svg).toBeVisible({ timeout: 5000 });
  });

  test('bar-chart renders horizontal bars', async ({ page }) => {
    await page.goto(`/group?id=${groupId}`);
    await page.waitForLoadState('load');

    // Should show the score labels from the bar chart
    await expect(page.locator('main >> text=Alice')).toBeVisible({ timeout: 5000 });
    await expect(page.locator('main >> text=Bob')).toBeVisible();
  });

  test('pie-chart renders SVG with legend', async ({ page }) => {
    await page.goto(`/group?id=${groupId}`);
    await page.waitForLoadState('load');

    // Legend entries include value in parens, e.g. "Web (45)"
    await expect(page.locator('main >> text=/Web/')).toBeVisible({ timeout: 5000 });
    await expect(page.locator('main >> text=/Mobile/')).toBeVisible();
  });

  test('conditional shows content when condition is met', async ({ page }) => {
    await page.goto(`/group?id=${groupId}`);
    await page.waitForLoadState('load');

    // "Featured Item" should be visible (featured=true, eq="true")
    await expect(page.locator('text=Featured Item')).toBeVisible({ timeout: 5000 });
  });

  test('list renders bullet list from meta array', async ({ page }) => {
    await page.goto(`/group?id=${groupId}`);
    await page.waitForLoadState('load');

    // Should show ingredients as list items
    await expect(page.locator('li', { hasText: 'flour' })).toBeVisible({ timeout: 5000 });
    await expect(page.locator('li', { hasText: 'eggs' })).toBeVisible();
  });

  test('count-badge shows count of items matching condition', async ({ page }) => {
    await page.goto(`/group?id=${groupId}`);
    await page.waitForLoadState('load');

    // 2 tasks have done=false, so "2 remaining"
    await expect(page.locator('text=remaining')).toBeVisible({ timeout: 5000 });
  });

  test('link-preview renders URL card', async ({ page }) => {
    await page.goto(`/group?id=${groupId}`);
    await page.waitForLoadState('load');

    // Should show the GitHub URL somewhere in the sidebar
    await expect(page.locator('a[href*="github.com"]').first()).toBeVisible({ timeout: 5000 });
  });

  test('json-tree renders keys from config object', async ({ page }) => {
    await page.goto(`/group?id=${groupId}`);
    await page.waitForLoadState('load');

    // json-tree renders key-value pairs — look for maxItems which is simpler text
    await expect(page.locator('text=maxItems: 50')).toBeVisible({ timeout: 5000 });
  });

  test('table renders owned notes', async ({ page }) => {
    await page.goto(`/group?id=${groupId}`);
    await page.waitForLoadState('load');

    // The table shortcode renders a table with note links
    const noteLink = page.locator('table a[href*="/note?id="]');
    await expect(noteLink.first()).toBeVisible({ timeout: 5000 });
  });

  test('qr-code renders barcode or visual code', async ({ page }) => {
    await page.goto(`/group?id=${groupId}`);
    await page.waitForLoadState('load');

    // Should have an SVG element for the barcode
    const barcode = page.locator('.sidebar-group svg').first();
    await expect(barcode).toBeVisible({ timeout: 5000 });
  });

  test('bar-chart accepts x-axis/y-axis as attribute aliases', async ({
    page,
    apiClient,
  }) => {
    const cat2 = await apiClient.createCategory(
      `BarAlias Test ${Date.now()}`,
      '',
      {
        CustomHeader:
          '[plugin:data-views:bar-chart path="scores" x-axis="name" y-axis="score"]',
      },
    );
    const grp2 = await apiClient.createGroup({
      name: `BarAlias Group ${Date.now()}`,
      categoryId: cat2.ID,
      meta: JSON.stringify({
        scores: [
          { name: 'Alpha', score: 90 },
          { name: 'Beta', score: 60 },
        ],
      }),
    });

    await page.goto(`/group?id=${grp2.ID}`);
    await page.waitForLoadState('load');

    // Should show label names from x-axis attr, not numeric indices
    await expect(page.locator('main >> text=Alpha')).toBeVisible({ timeout: 5000 });
    await expect(page.locator('main >> text=Beta')).toBeVisible();

    await apiClient.deleteGroup(grp2.ID);
    await apiClient.deleteCategory(cat2.ID);
  });

  test('pie-chart accepts label-field/value-field as attribute aliases', async ({
    page,
    apiClient,
  }) => {
    const cat2 = await apiClient.createCategory(
      `PieAlias Test ${Date.now()}`,
      '',
      {
        CustomHeader:
          '[plugin:data-views:pie-chart path="dist" label-field="cat" value-field="amt" size="100"]',
      },
    );
    const grp2 = await apiClient.createGroup({
      name: `PieAlias Group ${Date.now()}`,
      categoryId: cat2.ID,
      meta: JSON.stringify({
        dist: [
          { cat: 'Red', amt: 40 },
          { cat: 'Blue', amt: 60 },
        ],
      }),
    });

    await page.goto(`/group?id=${grp2.ID}`);
    await page.waitForLoadState('load');

    // Should render pie chart legend with category names (not "No data")
    await expect(page.locator('main >> text=/Red/')).toBeVisible({ timeout: 5000 });
    await expect(page.locator('main >> text=/Blue/')).toBeVisible();

    await apiClient.deleteGroup(grp2.ID);
    await apiClient.deleteCategory(cat2.ID);
  });
});
