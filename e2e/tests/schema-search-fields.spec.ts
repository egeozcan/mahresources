/**
 * E2E Tests for Schema-Driven Search Fields
 *
 * Tests that the schemaSearchFields Alpine.js component renders the correct
 * filter inputs when a category (or resource category) with a MetaSchema is
 * selected in the list-view sidebar, and that form submission produces the
 * expected MetaQuery URL parameters.
 */
import { test, expect } from '../fixtures/base.fixture';
import {
  selectGroupCategory,
  removeGroupCategory,
  selectResourceCategory,
  schemaFieldsGroup,
  submitFilterForm,
} from '../helpers/schema-search-helpers';

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

// Schema with > 6 enum values — forces multi-select dropdown rendering
const largeEnumSchema = JSON.stringify({
  type: 'object',
  properties: {
    country: {
      type: 'string',
      enum: ['US', 'UK', 'CA', 'DE', 'FR', 'JP', 'AU', 'BR'],
    },
  },
});

// ── Test suite ────────────────────────────────────────────────────────────────

test.describe('Schema-Driven Search Fields', () => {
  // IDs created in beforeAll, cleaned up in afterAll
  let categoryWithSchemaId: number;
  let categoryNoSchemaId: number;
  let category2WithSchemaId: number;
  let categoryNoOverlapId: number;
  let categoryLargeEnumId: number;
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

    // Category with ZERO field overlap with schema A (for "no common fields" test)
    const noOverlapSchema = JSON.stringify({
      type: 'object',
      properties: {
        altitude: { type: 'number' },
        pressure: { type: 'number' },
      },
    });
    const catNoOverlap = await apiClient.createCategory(
      `Schema Cat NoOverlap ${runId}`,
      'Category with no field overlap',
      { MetaSchema: noOverlapSchema }
    );
    categoryNoOverlapId = catNoOverlap.ID;

    // Category with > 6 enum values — forces multi-select dropdown rendering
    const catLargeEnum = await apiClient.createCategory(
      `Schema Cat LargeEnum ${runId}`,
      'Category with large enum for dropdown rendering test',
      { MetaSchema: largeEnumSchema }
    );
    categoryLargeEnumId = catLargeEnum.ID;

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
    if (categoryNoOverlapId) await apiClient.deleteCategory(categoryNoOverlapId).catch(() => {});
    if (categoryLargeEnumId) await apiClient.deleteCategory(categoryLargeEnumId).catch(() => {});
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

  // ── 11. URL state restoration (pre-fill from MetaQuery params) ─────────────

  test('schema fields are pre-filled after form submit and page reload', async ({
    groupPage,
    page,
  }) => {
    await groupPage.gotoList();
    await selectGroupCategory(page, `Schema Cat A ${runId}`);

    const container = schemaFieldsGroup(page);

    // Fill in the weight field
    const weightInput = container.locator('input[type="number"]').first();
    await weightInput.fill('42');

    // Submit — page reloads with MetaQuery params in URL
    await submitFilterForm(page, 'Filter groups');

    // After reload, the category autocompleter should restore from URL params,
    // which triggers handleCategoryChange, which calls _findExistingValue.
    // Wait for schema fields to re-render after category restoration.
    const restoredContainer = schemaFieldsGroup(page);
    await expect(restoredContainer.locator('input[type="number"]').first()).toBeVisible({
      timeout: 5000,
    });

    // The weight input should be pre-filled with "42"
    await expect(
      restoredContainer.locator('input[type="number"]').first()
    ).toHaveValue('42');
  });

  // ── 12. Enum > 6 renders as multi-select dropdown ─────────────────────────

  test('enum field with > 6 values renders as a multi-select dropdown', async ({
    groupPage,
    page,
  }) => {
    await groupPage.gotoList();
    await selectGroupCategory(page, `Schema Cat LargeEnum ${runId}`);

    const container = schemaFieldsGroup(page);

    // Should render a <select multiple> rather than checkboxes
    const multiSelect = container.locator('select[multiple]');
    await expect(multiSelect).toBeVisible({ timeout: 5000 });

    // Should NOT render individual checkboxes for enum values
    await expect(container.locator('input[type="checkbox"]')).toHaveCount(0);

    // Verify options are present
    for (const country of ['US', 'UK', 'CA', 'DE', 'FR', 'JP', 'AU', 'BR']) {
      await expect(multiSelect.locator(`option[value="${country}"]`)).toHaveCount(1);
    }
  });

  // ── 13. Two categories with NO common fields → schema section hidden ───────

  test('selecting two categories with no common fields hides all schema fields', async ({
    groupPage,
    page,
  }) => {
    await groupPage.gotoList();

    // Category A has: color, weight, active, dimensions.*
    await selectGroupCategory(page, `Schema Cat A ${runId}`);
    // NoOverlap category has: altitude, pressure — zero overlap with A
    await selectGroupCategory(page, `Schema Cat NoOverlap ${runId}`);

    const container = schemaFieldsGroup(page);

    // No interactive inputs should be rendered when intersection is empty
    await expect(container.locator('input, select')).toHaveCount(0);
  });

  // ── 14. Multiple enum checkboxes → multiple MetaQuery entries (OR logic) ───

  test('checking multiple enum checkboxes produces multiple MetaQuery entries', async ({
    groupPage,
    page,
  }) => {
    await groupPage.gotoList();
    await selectGroupCategory(page, `Schema Cat A ${runId}`);

    const container = schemaFieldsGroup(page);

    // Check both "red" and "green"
    await container.getByRole('checkbox', { name: 'red' }).check();
    await container.getByRole('checkbox', { name: 'green' }).check();

    // Submit
    await submitFilterForm(page, 'Filter groups');

    const decoded = decodeURIComponent(page.url());
    expect(decoded).toContain('color:EQ:"red"');
    expect(decoded).toContain('color:EQ:"green"');
  });

  // ── 15. Keyboard focus management for operator toggle ─────────────────────

  test('keyboard: Tab to operator button, Enter opens select, choosing option closes it', async ({
    groupPage,
    page,
  }) => {
    await groupPage.gotoList();
    await selectGroupCategory(page, `Schema Cat A ${runId}`);

    const container = schemaFieldsGroup(page);

    // Find the first operator button and focus it via keyboard
    const operatorButton = container.locator('button[aria-label^="Change operator"]').first();
    await operatorButton.focus();
    await expect(operatorButton).toBeFocused();

    // Press Enter to open the operator dropdown
    await page.keyboard.press('Enter');

    // The operator select should now be visible
    const operatorSelect = container.locator('select[aria-label^="Operator for"]').first();
    await expect(operatorSelect).toBeVisible({ timeout: 3000 });

    // Select an option with keyboard (GT)
    await operatorSelect.selectOption('GT');

    // The dropdown should close after selection
    await expect(operatorSelect).not.toBeVisible({ timeout: 3000 });
  });

  // ── 16. Schema-claimed entries excluded from freeFields ─────────────────────

  test('freeFields does not show entries for paths owned by schema fields', async ({
    groupPage,
    page,
  }) => {
    await groupPage.gotoList();
    await selectGroupCategory(page, `Schema Cat A ${runId}`);

    const container = schemaFieldsGroup(page);

    // Fill weight in schema fields
    const weightInput = container.locator('input[type="number"]').first();
    await weightInput.fill('42');

    // Submit
    await submitFilterForm(page, 'Filter groups');

    // After reload, the freeFields section should NOT contain a "weight" entry
    // because the schema fields component owns that path.
    const freeFieldsGroup = page.locator('[role="group"][aria-label="Meta"]');
    // freeFields uses text inputs for field names — none should have value "weight"
    const freeFieldNameInputs = freeFieldsGroup.locator('input[type="text"]');
    const count = await freeFieldNameInputs.count();
    for (let i = 0; i < count; i++) {
      const val = await freeFieldNameInputs.nth(i).inputValue();
      expect(val, 'freeFields should not show entries owned by schema fields').not.toBe('weight');
    }
  });
});
