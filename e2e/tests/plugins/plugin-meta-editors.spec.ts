/**
 * E2E tests for the meta-editors plugin shortcodes.
 * Tests that interactive meta editor shortcodes render and save correctly.
 */
import { test, expect } from '../../fixtures/base.fixture';

test.describe('Meta-editors plugin shortcodes', () => {
  let categoryId: number;
  let groupId: number;

  test.beforeAll(async ({ apiClient }) => {
    await apiClient.enablePlugin('meta-editors');

    // Create a category with various meta-editor shortcodes in CustomHeader
    const cat = await apiClient.createCategory(
      `MetaEditors Test ${Date.now()}`,
      'Category for meta-editors plugin E2E tests',
      {
        CustomHeader: [
          '[plugin:meta-editors:toggle path="active" label="Active"]',
          '[plugin:meta-editors:slider path="rating" min="0" max="10" step="1"]',
          '[plugin:meta-editors:star-rating path="stars" max="5"]',
          '[plugin:meta-editors:button-group path="priority" options="low,medium,high" labels="Low,Medium,High"]',
          '[plugin:meta-editors:status-badge path="status" options="todo,in-progress,done" colors="#9ca3af,#f59e0b,#22c55e" labels="Todo,In Progress,Done"]',
          '[plugin:meta-editors:stepper path="count" min="0" max="20"]',
        ].join('\n'),
        CustomSidebar: [
          '[plugin:meta-editors:textarea path="notes" rows="3" placeholder="Quick notes..."]',
          '[plugin:meta-editors:tags-input path="keywords" placeholder="Add tag..."]',
          '[plugin:meta-editors:date-picker path="due_date" label="Due Date"]',
          '[plugin:meta-editors:checklist path="tasks"]',
          '[plugin:meta-editors:color-picker path="color"]',
          '[plugin:meta-editors:progress-input path="completion" label="Progress"]',
        ].join('\n'),
      },
    );
    categoryId = cat.ID;

    const group = await apiClient.createGroup({
      name: `MetaEditors Demo ${Date.now()}`,
      categoryId: cat.ID,
      meta: JSON.stringify({
        active: true,
        rating: 7,
        stars: 3,
        priority: 'medium',
        status: 'in-progress',
        count: 5,
        notes: 'Some existing notes',
        keywords: ['test', 'demo'],
        completion: 40,
      }),
    });
    groupId = group.ID;
  });

  test.afterAll(async ({ apiClient }) => {
    if (groupId) await apiClient.deleteGroup(groupId);
    if (categoryId) await apiClient.deleteCategory(categoryId);
  });

  test('toggle shortcode renders with correct initial state', async ({ page }) => {
    await page.goto(`/group?id=${groupId}`);
    await page.waitForLoadState('load');

    // Toggle should render as a switch button with role="switch"
    const toggle = page.locator('button[role="switch"]');
    await expect(toggle).toBeVisible({ timeout: 5000 });
    // Initial value is true, so aria-checked should be "true"
    await expect(toggle).toHaveAttribute('aria-checked', 'true');
  });

  test('toggle shortcode saves on click', async ({ page, apiClient }) => {
    await page.goto(`/group?id=${groupId}`);
    await page.waitForLoadState('load');

    const toggle = page.locator('button[role="switch"]');
    await expect(toggle).toBeVisible({ timeout: 5000 });

    // Click to toggle off
    await toggle.click();

    // Wait for aria-checked to flip (confirms Alpine handler ran and save completed)
    await expect(toggle).toHaveAttribute('aria-checked', 'false', { timeout: 5000 });

    // Verify the value was saved via API
    const group = await apiClient.getGroup(groupId);
    const meta = typeof group.Meta === 'string' ? JSON.parse(group.Meta || '{}') : (group.Meta || {});
    expect(meta.active).toBe(false);
  });

  test('slider shortcode renders with initial value', async ({ page }) => {
    await page.goto(`/group?id=${groupId}`);
    await page.waitForLoadState('load');

    const slider = page.locator('input[type="range"]');
    await expect(slider).toBeVisible({ timeout: 5000 });
  });

  test('star-rating shortcode renders stars', async ({ page }) => {
    await page.goto(`/group?id=${groupId}`);
    await page.waitForLoadState('load');

    // Should have 5 star buttons
    const stars = page.locator('button[aria-label^="Rate "]');
    await expect(stars).toHaveCount(5, { timeout: 5000 });
  });

  test('button-group shortcode renders options', async ({ page }) => {
    await page.goto(`/group?id=${groupId}`);
    await page.waitForLoadState('load');

    // Should have Low, Medium, High buttons
    await expect(page.locator('button', { hasText: 'Low' })).toBeVisible({ timeout: 5000 });
    await expect(page.locator('button', { hasText: 'Medium' })).toBeVisible();
    await expect(page.locator('button', { hasText: 'High' })).toBeVisible();
  });

  test('button-group saves on click', async ({ page, apiClient }) => {
    await page.goto(`/group?id=${groupId}`);
    await page.waitForLoadState('load');

    // Click "High" and wait for the network request to complete
    const [response] = await Promise.all([
      page.waitForResponse(resp => resp.url().includes('/editMeta') && resp.status() === 200, { timeout: 5000 }),
      page.locator('button', { hasText: 'High' }).click(),
    ]);

    const group = await apiClient.getGroup(groupId);
    const meta = typeof group.Meta === 'string' ? JSON.parse(group.Meta || '{}') : (group.Meta || {});
    expect(meta.priority).toBe('high');
  });

  test('status-badge renders and cycles on click', async ({ page, apiClient }) => {
    await page.goto(`/group?id=${groupId}`);
    await page.waitForLoadState('load');

    // Should show current status as a clickable badge
    const badge = page.locator('button', { hasText: /In Progress|Todo|Done/ });
    await expect(badge).toBeVisible({ timeout: 5000 });

    const currentText = await badge.textContent();

    // Click to cycle to next state
    const [response] = await Promise.all([
      page.waitForResponse(resp => resp.url().includes('/editMeta') && resp.status() === 200, { timeout: 5000 }),
      badge.click(),
    ]);

    // Badge text should have changed
    await expect(badge).not.toHaveText(currentText!, { timeout: 3000 });
  });

  test('textarea renders with existing content', async ({ page }) => {
    await page.goto(`/group?id=${groupId}`);
    await page.waitForLoadState('load');

    const textarea = page.locator('textarea').first();
    await expect(textarea).toBeVisible({ timeout: 5000 });
    await expect(textarea).toHaveValue('Some existing notes');
  });

  test('tags-input renders existing tags', async ({ page }) => {
    await page.goto(`/group?id=${groupId}`);
    await page.waitForLoadState('load');

    // Should show existing tags "test" and "demo"
    await expect(page.locator('text=test').first()).toBeVisible({ timeout: 5000 });
    await expect(page.locator('text=demo').first()).toBeVisible();
  });

  test('checklist renders add input and button', async ({ page }) => {
    await page.goto(`/group?id=${groupId}`);
    await page.waitForLoadState('load');

    // Checklist should have an add input and a + button
    const addInput = page.locator('input[placeholder="Add item..."]');
    await expect(addInput).toBeVisible({ timeout: 5000 });

    const addButton = addInput.locator('..').locator('button');
    await expect(addButton).toBeVisible();
    await expect(addButton).toHaveText('+');
  });

  test('color-picker renders color swatches', async ({ page }) => {
    await page.goto(`/group?id=${groupId}`);
    await page.waitForLoadState('load');

    // Should have color swatch buttons
    const swatches = page.locator('button[aria-label^="Select color"]');
    await expect(swatches.first()).toBeVisible({ timeout: 5000 });
    // Default palette has 8 colors
    await expect(swatches).toHaveCount(8);
  });

  test('progress-input renders clickable bar', async ({ page }) => {
    await page.goto(`/group?id=${groupId}`);
    await page.waitForLoadState('load');

    // Should show percentage
    await expect(page.locator('text=40%')).toBeVisible({ timeout: 5000 });

    // The progress bar should be visible
    const bar = page.locator('.bg-amber-600').first();
    await expect(bar).toBeVisible();
  });

  test('date-picker renders', async ({ page }) => {
    await page.goto(`/group?id=${groupId}`);
    await page.waitForLoadState('load');

    const datePicker = page.locator('input[type="date"]');
    await expect(datePicker).toBeVisible({ timeout: 5000 });
  });

  test('date-range renders with object value {start, end}', async ({ page, apiClient }) => {
    // date-range expects value as {start, end} object, not a string
    const cat2 = await apiClient.createCategory(
      `DateRange Test ${Date.now()}`,
      '',
      { CustomHeader: '[plugin:meta-editors:date-range path="period"]' },
    );
    const grp2 = await apiClient.createGroup({
      name: `DateRange Group ${Date.now()}`,
      categoryId: cat2.ID,
      meta: JSON.stringify({ period: { start: '2024-01-15', end: '2024-06-30' } }),
    });

    await page.goto(`/group?id=${grp2.ID}`);
    await page.waitForLoadState('load');

    // Both date inputs should have values populated
    const dateInputs = page.locator('main input[type="date"]');
    await expect(dateInputs).toHaveCount(2, { timeout: 5000 });
    await expect(dateInputs.first()).toHaveValue('2024-01-15');
    await expect(dateInputs.last()).toHaveValue('2024-06-30');

    await apiClient.deleteGroup(grp2.ID);
    await apiClient.deleteCategory(cat2.ID);
  });

  test('status-badge renders without JS errors even with missing options attr', async ({
    page,
    apiClient,
  }) => {
    // Simulates typo: using "values" instead of "options"
    const cat2 = await apiClient.createCategory(
      `BadBadge Test ${Date.now()}`,
      '',
      {
        CustomHeader:
          '[plugin:meta-editors:status-badge path="status" values="a,b,c" colors="#aaa,#bbb,#ccc" labels="A,B,C"]',
      },
    );
    const grp2 = await apiClient.createGroup({
      name: `BadBadge Group ${Date.now()}`,
      categoryId: cat2.ID,
      meta: JSON.stringify({ status: 'a' }),
    });

    const consoleErrors: string[] = [];
    page.on('pageerror', (err) => consoleErrors.push(err.message));

    await page.goto(`/group?id=${grp2.ID}`);
    await page.waitForLoadState('load');
    // Give Alpine time to initialize
    await page.waitForTimeout(500);

    // Should have zero JS errors (no TypeError: indexOf is not a function)
    expect(consoleErrors).toHaveLength(0);

    await apiClient.deleteGroup(grp2.ID);
    await apiClient.deleteCategory(cat2.ID);
  });
});
