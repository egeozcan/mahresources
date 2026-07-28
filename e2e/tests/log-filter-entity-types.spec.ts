import { test, expect } from '../fixtures/base.fixture';

test.describe('Bug 2: Log filter dropdown should include all entity types that generate log entries', () => {
  test('Entity Type dropdown includes series, resourceCategory, and resource_version', async ({ page, baseURL }) => {
    await page.goto(`${baseURL}/logs`);

    const select = page.locator('select[name="EntityType"]');
    await expect(select).toBeVisible({ timeout: 10000 });

    // Get all option values from the Entity Type dropdown
    const optionValues = await select.locator('option').evaluateAll(
      (opts) => opts.map((o) => (o as HTMLOptionElement).value)
    );

    // These entity types generate log entries but were missing from the dropdown
    expect(optionValues).toContain('series');
    expect(optionValues).toContain('resourceCategory');
    expect(optionValues).toContain('resource_version');
  });

  test('Entity Type dropdown still includes existing entity types', async ({ page, baseURL }) => {
    await page.goto(`${baseURL}/logs`);

    const select = page.locator('select[name="EntityType"]');
    await expect(select).toBeVisible({ timeout: 10000 });

    const optionValues = await select.locator('option').evaluateAll(
      (opts) => opts.map((o) => (o as HTMLOptionElement).value)
    );

    // Verify existing entity types are still present
    expect(optionValues).toContain('tag');
    expect(optionValues).toContain('category');
    expect(optionValues).toContain('note');
    expect(optionValues).toContain('resource');
    expect(optionValues).toContain('group');
    expect(optionValues).toContain('query');
    expect(optionValues).toContain('relation');
  });
});

test.describe('Bug 3: Admin overview should show Series entity count', () => {
  test('Series card is visible in the Data Overview section', async ({ page, baseURL }) => {
    await page.goto(`${baseURL}/admin/overview`);

    const dataSection = page.locator('section[aria-label="Data overview"]');
    await expect(dataSection).toBeVisible({ timeout: 10000 });

    // Wait for entity count cards to load
    await expect(dataSection.locator('p:has-text("Resources")')).toBeVisible({ timeout: 10000 });

    // The Series card should be present
    await expect(dataSection.locator('p:has-text("Series")')).toBeVisible({ timeout: 10000 });
  });

  test('data-stats API response includes series field', async ({ request, baseURL }) => {
    const response = await request.get(`${baseURL}/v1/admin/data-stats`);
    expect(response.ok()).toBeTruthy();

    const data = await response.json();
    expect(data.entities).toHaveProperty('series');
    expect(typeof data.entities.series).toBe('number');
  });
});
