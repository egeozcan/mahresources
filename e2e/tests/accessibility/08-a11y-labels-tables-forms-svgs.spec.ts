/**
 * Accessibility tests for missing labels, table names, form labels, and aria-hidden SVGs
 *
 * Bug 1: Query parameter inputs lack labels (displayQuery.tpl)
 * Bug 2: Tables missing accessible names (listLogs.tpl, listResourcesDetails.tpl)
 * Bug 3: Six filter forms missing aria-label
 * Bug 4: Decorative SVGs missing aria-hidden (displayRelationType.tpl, listRelations.tpl)
 */
import { test, expect } from '../../fixtures/a11y.fixture';

test.describe('Bug 1: Query parameter inputs have accessible names', () => {
  let queryWithParamsId: number;

  test.beforeAll(async ({ apiClient }) => {
    // Create a query that uses named parameters (e.g., $limit) so that
    // displayQuery.tpl renders input fields for each parameter.
    const query = await apiClient.createQuery({
      name: `A11y Param Query ${Date.now()}`,
      text: 'SELECT * FROM notes WHERE name LIKE :searchTerm LIMIT :maxResults',
      description: 'Query with parameters for a11y testing',
    });
    queryWithParamsId = query.ID;
  });

  test('query parameter inputs should have aria-label', async ({ page }) => {
    await page.goto(`/query?id=${queryWithParamsId}`);
    await page.waitForLoadState('load');

    // Wait for Alpine.js to render the template x-for loop
    // The inputs are rendered by Alpine from parsed query params (:searchTerm, :maxResults)
    // Alpine renders x-for children as siblings of the <template> tag
    const paramInputs = page.locator('[x-init*="parseQueryParams"] input[type="text"]');
    await paramInputs.first().waitFor({ state: 'visible', timeout: 5000 });
    const count = await paramInputs.count();

    // We expect at least 2 parameter inputs (from :searchTerm, :maxResults)
    expect(count).toBeGreaterThanOrEqual(2);

    // Each input should have an aria-label attribute
    for (let i = 0; i < count; i++) {
      const input = paramInputs.nth(i);
      const ariaLabel = await input.getAttribute('aria-label');
      expect(ariaLabel, `Query param input ${i} should have aria-label`).toBeTruthy();
    }
  });

  test.afterAll(async ({ apiClient }) => {
    if (queryWithParamsId) {
      await apiClient.deleteQuery(queryWithParamsId).catch(() => {});
    }
  });
});

test.describe('Bug 2: Tables have accessible names', () => {
  test('log entries table should have aria-label', async ({ page }) => {
    await page.goto('/logs');
    await page.waitForLoadState('load');

    const table = page.locator('table').first();
    await expect(table).toBeVisible();

    const ariaLabel = await table.getAttribute('aria-label');
    expect(ariaLabel, 'Log entries table should have aria-label').toBe('Log entries');
  });

  test('resources detail table should have aria-label', async ({ page }) => {
    await page.goto('/resources/details');
    await page.waitForLoadState('load');

    const table = page.locator('table.detail-table');
    await expect(table).toBeVisible();

    const ariaLabel = await table.getAttribute('aria-label');
    expect(ariaLabel, 'Resources detail table should have aria-label').toBe('Resources');
  });
});

test.describe('Bug 3: Filter forms have aria-label', () => {
  test('logs filter form should have aria-label', async ({ page }) => {
    await page.goto('/logs');
    await page.waitForLoadState('load');

    // The filter form is in the sidebar
    const filterForm = page.locator('form').first();
    await expect(filterForm).toBeVisible();

    const ariaLabel = await filterForm.getAttribute('aria-label');
    expect(ariaLabel, 'Logs filter form should have aria-label').toBe('Filter logs');
  });

  test('relations filter form should have aria-label', async ({ page }) => {
    await page.goto('/relations');
    await page.waitForLoadState('load');

    // The sidebar filter form
    const filterForm = page.locator('aside form, [class*="sidebar"] form').first();
    const ariaLabel = await filterForm.getAttribute('aria-label');
    expect(ariaLabel, 'Relations filter form should have aria-label').toBe('Filter relations');
  });

  test('relation types filter form should have aria-label', async ({ page }) => {
    await page.goto('/relationTypes');
    await page.waitForLoadState('load');

    const filterForm = page.locator('aside form, [class*="sidebar"] form').first();
    const ariaLabel = await filterForm.getAttribute('aria-label');
    expect(ariaLabel, 'Relation types filter form should have aria-label').toBe('Filter relation types');
  });

  test('note types filter form should have aria-label', async ({ page }) => {
    await page.goto('/noteTypes');
    await page.waitForLoadState('load');

    const filterForm = page.locator('aside form, [class*="sidebar"] form').first();
    const ariaLabel = await filterForm.getAttribute('aria-label');
    expect(ariaLabel, 'Note types filter form should have aria-label').toBe('Filter note types');
  });

  test('resource categories filter form should have aria-label', async ({ page }) => {
    await page.goto('/resourceCategories');
    await page.waitForLoadState('load');

    const filterForm = page.locator('aside form, [class*="sidebar"] form').first();
    const ariaLabel = await filterForm.getAttribute('aria-label');
    expect(ariaLabel, 'Resource categories filter form should have aria-label').toBe('Filter resource categories');
  });

  test('groups text view filter form should have aria-label', async ({ page }) => {
    await page.goto('/groups/text');
    await page.waitForLoadState('load');

    const filterForm = page.locator('aside form, [class*="sidebar"] form').first();
    const ariaLabel = await filterForm.getAttribute('aria-label');
    expect(ariaLabel, 'Groups text filter form should have aria-label').toBe('Filter groups');
  });
});

test.describe('Bug 4: Decorative SVGs have aria-hidden', () => {
  test('relation type detail page arrow SVGs should have aria-hidden', async ({ page, a11yTestData }) => {
    await page.goto(`/relationType?id=${a11yTestData.relationTypeId}`);
    await page.waitForLoadState('load');

    // The category flow section has arrow SVGs
    const svgs = page.locator('.detail-panel-body svg');
    const count = await svgs.count();
    expect(count).toBeGreaterThan(0);

    for (let i = 0; i < count; i++) {
      const svg = svgs.nth(i);
      const ariaHidden = await svg.getAttribute('aria-hidden');
      expect(ariaHidden, `SVG ${i} on relation type detail should have aria-hidden="true"`).toBe('true');
    }
  });

  test('relations list arrow SVGs should have aria-hidden', async ({ page }) => {
    await page.goto('/relations');
    await page.waitForLoadState('load');

    // Arrow SVGs in relation cards
    const svgs = page.locator('.relation-arrow svg');
    const count = await svgs.count();

    // If there are relations on the page, check SVGs
    if (count > 0) {
      for (let i = 0; i < count; i++) {
        const svg = svgs.nth(i);
        const ariaHidden = await svg.getAttribute('aria-hidden');
        expect(ariaHidden, `SVG ${i} in relations list should have aria-hidden="true"`).toBe('true');
      }
    }
  });
});
