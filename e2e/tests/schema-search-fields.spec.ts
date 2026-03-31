/**
 * E2E Tests for Schema-Driven Search Fields
 *
 * Tests that the schemaSearchFields Alpine.js component renders the correct
 * filter inputs when a category (or resource category) with a MetaSchema is
 * selected in the list-view sidebar, and that form submission produces the
 * expected MetaQuery URL parameters.
 */
import { test, expect } from '../fixtures/base.fixture';

// ── Shared test schema ───────────────────────────────────────────────────────

const testSchema = JSON.stringify({
  type: 'object',
  properties: {
    color: { type: 'string', enum: ['red', 'green', 'blue'] },
    weight: { type: 'number' },
    active: { type: 'boolean' },
    dimensions: {
      type: 'object',
      properties: {
        width: { type: 'number' },
        height: { type: 'number' },
      },
    },
  },
});

// ── Helpers ───────────────────────────────────────────────────────────────────

/**
 * Select a value from the Categories autocompleter on the groups list page.
 * The autocompleter is labelled "Categories".
 */
async function selectGroupCategory(page: any, searchText: string) {
  const input = page.getByRole('combobox', { name: 'Categories' });
  await input.click();
  await input.fill(searchText);
  const option = page.locator(`div[role="option"]:visible:has-text("${searchText}")`).first();
  await option.waitFor({ timeout: 10000 });
  await option.click();
  await option.waitFor({ state: 'hidden', timeout: 3000 }).catch(() => {});
  // Give Alpine time to propagate the selection and re-render schema fields
  await page.waitForTimeout(300);
}

/**
 * Remove a previously-selected category chip by clicking its remove button.
 */
async function removeGroupCategory(page: any, categoryName: string) {
  const removeBtn = page
    .locator(`[x-data*="autocompleter"] button[aria-label="Remove ${categoryName}"]`)
    .first();
  await removeBtn.click();
  // Give Alpine time to re-render
  await page.waitForTimeout(200);
}

/**
 * Select a value from the Resource Category autocompleter on the resources list page.
 */
async function selectResourceCategory(page: any, searchText: string) {
  const input = page.getByRole('combobox', { name: 'Resource Category' });
  await input.click();
  await input.fill(searchText);
  const option = page.locator(`div[role="option"]:visible:has-text("${searchText}")`).first();
  await option.waitFor({ timeout: 10000 });
  await option.click();
  await option.waitFor({ state: 'hidden', timeout: 3000 }).catch(() => {});
  // Give Alpine time to propagate and render schema fields
  await page.waitForTimeout(300);
}

/**
 * The schema fields container — anchored by role="group" / aria-label="Schema fields".
 */
function schemaFieldsGroup(page: any) {
  return page.locator('[role="group"][aria-label="Schema fields"]');
}

/**
 * Submit the filter form on a list page. Finds the submit button within the
 * sidebar filter form specifically (using aria-label) to avoid ambiguity.
 */
async function submitFilterForm(page: any, formAriaLabel = 'Filter groups') {
  const form = page.locator(`form[aria-label="${formAriaLabel}"]`);
  const submitBtn = form.getByRole('button', { name: 'Apply Filters' });
  await submitBtn.scrollIntoViewIfNeeded();
  await submitBtn.click();
  await page.waitForLoadState('load');
}

// ── Test suite ────────────────────────────────────────────────────────────────

test.describe('Schema-Driven Search Fields', () => {
  // IDs created in beforeAll, cleaned up in afterAll
  let categoryWithSchemaId: number;
  let categoryNoSchemaId: number;
  let category2WithSchemaId: number;
  let resourceCategoryId: number;
  const runId = Date.now();

  test.beforeAll(async ({ apiClient }) => {
    // Category with full test schema
    const catA = await apiClient.createCategory(
      `Schema Cat A ${runId}`,
      'Category with MetaSchema for search fields tests',
      { MetaSchema: testSchema }
    );
    categoryWithSchemaId = catA.ID;

    // Category with NO schema
    const catNone = await apiClient.createCategory(
      `Schema Cat None ${runId}`,
      'Category without MetaSchema'
    );
    categoryNoSchemaId = catNone.ID;

    // Second category — shares only "weight" + "active" with schema A (color dropped, extra "score")
    const overlappingSchema = JSON.stringify({
      type: 'object',
      properties: {
        weight: { type: 'number' },
        active: { type: 'boolean' },
        score: { type: 'number' },
      },
    });
    const catB = await apiClient.createCategory(
      `Schema Cat B ${runId}`,
      'Category with partial-overlap MetaSchema',
      { MetaSchema: overlappingSchema }
    );
    category2WithSchemaId = catB.ID;

    // Resource category with schema
    const rc = await apiClient.createResourceCategory(
      `Schema RC ${runId}`,
      'Resource category with MetaSchema for search fields tests',
      { MetaSchema: testSchema }
    );
    resourceCategoryId = rc.ID;
  });

  test.afterAll(async ({ apiClient }) => {
    if (categoryWithSchemaId) await apiClient.deleteCategory(categoryWithSchemaId).catch(() => {});
    if (categoryNoSchemaId) await apiClient.deleteCategory(categoryNoSchemaId).catch(() => {});
    if (category2WithSchemaId) await apiClient.deleteCategory(category2WithSchemaId).catch(() => {});
    if (resourceCategoryId) await apiClient.deleteResourceCategory(resourceCategoryId).catch(() => {});
  });

  // ── 1. Schema fields appear when selecting a category with MetaSchema ──────

  test('schema fields appear after selecting a category with MetaSchema on groups list', async ({
    groupPage,
    page,
  }) => {
    await groupPage.gotoList();
    const container = schemaFieldsGroup(page);

    // Before selecting — no schema fields visible
    await expect(container.locator('input, select')).toHaveCount(0);

    await selectGroupCategory(page, `Schema Cat A ${runId}`);

    // Schema fields container should now have rendered inputs
    await expect(container).toBeVisible();
    await expect(container.locator('input, select')).not.toHaveCount(0);
  });

  // ── 2. Schema fields disappear when deselecting the category ───────────────

  test('schema fields disappear after removing the selected category', async ({
    groupPage,
    page,
  }) => {
    await groupPage.gotoList();
    await selectGroupCategory(page, `Schema Cat A ${runId}`);

    const container = schemaFieldsGroup(page);
    await expect(container.locator('input, select')).not.toHaveCount(0);

    await removeGroupCategory(page, `Schema Cat A ${runId}`);

    // Fields should be gone
    await expect(container.locator('input, select')).toHaveCount(0);
  });

  // ── 3. Enum ≤ 6 renders as checkboxes ─────────────────────────────────────

  test('enum field with ≤ 6 values renders as checkboxes', async ({
    groupPage,
    page,
  }) => {
    await groupPage.gotoList();
    await selectGroupCategory(page, `Schema Cat A ${runId}`);

    const container = schemaFieldsGroup(page);
    // The "color" enum has 3 values — should render as checkboxes
    const checkboxes = container.locator('input[type="checkbox"]');
    await expect(checkboxes).not.toHaveCount(0);

    // Verify the three enum values are present
    for (const val of ['red', 'green', 'blue']) {
      await expect(container.getByRole('checkbox', { name: val })).toBeVisible();
    }
  });

  // ── 4. Boolean renders as three-state radio (Any / Yes / No) ──────────────

  test('boolean field renders as three-state radio (Any / Yes / No)', async ({
    groupPage,
    page,
  }) => {
    await groupPage.gotoList();
    await selectGroupCategory(page, `Schema Cat A ${runId}`);

    const container = schemaFieldsGroup(page);
    await expect(container.getByRole('radio', { name: 'Any' })).toBeVisible();
    await expect(container.getByRole('radio', { name: 'Yes' })).toBeVisible();
    await expect(container.getByRole('radio', { name: 'No' })).toBeVisible();
  });

  // ── 5. Number field renders a number input ─────────────────────────────────

  test('number field renders a number input', async ({
    groupPage,
    page,
  }) => {
    await groupPage.gotoList();
    await selectGroupCategory(page, `Schema Cat A ${runId}`);

    const container = schemaFieldsGroup(page);
    // The "weight" field should produce a <input type="number">
    const numberInputs = container.locator('input[type="number"]');
    await expect(numberInputs).not.toHaveCount(0);
  });

  // ── 6. Submitting schema fields produces correct MetaQuery URL params ──────

  test('filling a number field and submitting produces MetaQuery param in URL', async ({
    groupPage,
    page,
  }) => {
    await groupPage.gotoList();
    await selectGroupCategory(page, `Schema Cat A ${runId}`);

    const container = schemaFieldsGroup(page);

    // Fill in the weight number input
    const weightInput = container.locator('input[type="number"]').first();
    await weightInput.fill('42');

    // Submit the search form (targets the sidebar form specifically)
    await submitFilterForm(page, 'Filter groups');

    const url = page.url();
    // MetaQuery should contain the encoded param for weight:EQ:42
    expect(url).toContain('MetaQuery=');
    expect(decodeURIComponent(url)).toContain('weight:EQ:42');
  });

  test('checking an enum checkbox and submitting produces MetaQuery param in URL', async ({
    groupPage,
    page,
  }) => {
    await groupPage.gotoList();
    await selectGroupCategory(page, `Schema Cat A ${runId}`);

    const container = schemaFieldsGroup(page);

    // Check the "red" color checkbox
    await container.getByRole('checkbox', { name: 'red' }).check();

    // Submit
    await submitFilterForm(page, 'Filter groups');

    const url = page.url();
    expect(url).toContain('MetaQuery=');
    expect(decodeURIComponent(url)).toContain('color:EQ:"red"');
  });

  test('setting boolean radio to Yes and submitting produces MetaQuery param in URL', async ({
    groupPage,
    page,
  }) => {
    await groupPage.gotoList();
    await selectGroupCategory(page, `Schema Cat A ${runId}`);

    const container = schemaFieldsGroup(page);

    // Select the "Yes" radio for the boolean field
    await container.getByRole('radio', { name: 'Yes' }).click();

    // Submit
    await submitFilterForm(page, 'Filter groups');

    const url = page.url();
    expect(url).toContain('MetaQuery=');
    expect(decodeURIComponent(url)).toContain('active:EQ:true');
  });

  // ── 7. Multi-category intersection — only common fields shown ─────────────

  test('selecting two categories shows only fields common to both', async ({
    groupPage,
    page,
  }) => {
    await groupPage.gotoList();

    // Select category A (has: color, weight, active, dimensions.width, dimensions.height)
    await selectGroupCategory(page, `Schema Cat A ${runId}`);
    // Select category B (has: weight, active, score)
    await selectGroupCategory(page, `Schema Cat B ${runId}`);

    const container = schemaFieldsGroup(page);

    // "weight" and "active" are common — they should appear
    // Weight → number input; active → radio buttons
    await expect(container.locator('input[type="number"]')).not.toHaveCount(0);
    await expect(container.getByRole('radio', { name: 'Any' })).toBeVisible();

    // "color" is only in A → checkboxes for red/green/blue should NOT appear
    await expect(container.getByRole('checkbox', { name: 'red' })).toHaveCount(0);
    await expect(container.getByRole('checkbox', { name: 'green' })).toHaveCount(0);
  });

  // ── 8. Category without MetaSchema shows no schema fields ─────────────────

  test('selecting a category without MetaSchema renders no schema fields', async ({
    groupPage,
    page,
  }) => {
    await groupPage.gotoList();
    await selectGroupCategory(page, `Schema Cat None ${runId}`);

    const container = schemaFieldsGroup(page);
    // The container renders but should have no interactive inputs
    await expect(container.locator('input, select')).toHaveCount(0);
  });

  // ── 9. Operator override ───────────────────────────────────────────────────

  test('clicking the operator symbol, changing it, then submitting reflects new operator in URL', async ({
    groupPage,
    page,
  }) => {
    await groupPage.gotoList();
    await selectGroupCategory(page, `Schema Cat A ${runId}`);

    const container = schemaFieldsGroup(page);

    // The weight field shows the default operator symbol (≈ for string, = for number)
    // For numbers the default is EQ (=). Click the operator button to expand the dropdown.
    const operatorButton = container.locator('button[aria-label^="Change operator"]').first();
    await operatorButton.click();

    // The operator dropdown should now be visible; select GT
    const operatorSelect = container.locator('select[aria-label^="Operator for"]').first();
    await operatorSelect.waitFor({ timeout: 3000 });
    await operatorSelect.selectOption('GT');

    // Dropdown should close (showOperator = false) after change
    await expect(operatorSelect).not.toBeVisible({ timeout: 3000 });

    // Fill a value for the field
    const numberInput = container.locator('input[type="number"]').first();
    await numberInput.fill('10');

    // Submit
    await submitFilterForm(page, 'Filter groups');

    const url = page.url();
    expect(decodeURIComponent(url)).toContain('weight:GT:10');
  });

  // ── 10. Resources list view: schema fields from resource category ──────────

  test('schema fields appear when selecting a resource category with MetaSchema on resources list', async ({
    resourcePage,
    page,
  }) => {
    await resourcePage.gotoList();

    const container = schemaFieldsGroup(page);

    // Before selecting — no schema fields
    await expect(container.locator('input, select')).toHaveCount(0);

    await selectResourceCategory(page, `Schema RC ${runId}`);

    // Schema fields should now render
    await expect(container).toBeVisible();
    await expect(container.locator('input, select')).not.toHaveCount(0);

    // Spot-check: enum checkboxes (color), radios (active), number input (weight)
    await expect(container.getByRole('checkbox', { name: 'red' })).toBeVisible();
    await expect(container.getByRole('radio', { name: 'Any' })).toBeVisible();
    await expect(container.locator('input[type="number"]')).not.toHaveCount(0);
  });
});
