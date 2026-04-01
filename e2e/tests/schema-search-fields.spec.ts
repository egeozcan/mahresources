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
  let categoryPlainStringId: number;
  let categoryIntegerWeightId: number;
  let categoryCoercibleEnumId: number;
  let categoryNumericEnumId: number;
  let categoryRefId: number;
  let categoryAllOfId: number;
  let categoryOneOfId: number;
  let categoryNoTypeId: number;
  let categoryIntRatingId: number;
  let categoryRefSiblingId: number;
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

    // Category with weight as integer (not number) for type merge test
    const integerWeightSchema = JSON.stringify({
      type: 'object',
      properties: {
        weight: { type: 'integer' },
      },
    });
    const catIntWeight = await apiClient.createCategory(
      `Schema Cat IntWeight ${runId}`,
      'Category with integer weight',
      { MetaSchema: integerWeightSchema }
    );
    categoryIntegerWeightId = catIntWeight.ID;

    // Category with enum values that look like numbers/bools/null
    const coercibleEnumSchema = JSON.stringify({
      type: 'object',
      properties: {
        code: { type: 'string', enum: ['007', 'true', 'null', 'abc'] },
      },
    });
    const catCoercible = await apiClient.createCategory(
      `Schema Cat CoercibleEnum ${runId}`,
      'Category with coercible enum values',
      { MetaSchema: coercibleEnumSchema }
    );
    categoryCoercibleEnumId = catCoercible.ID;

    // Category with a numeric enum
    const numericEnumSchema = JSON.stringify({
      type: 'object',
      properties: {
        rating: { type: 'number', enum: [1, 2, 3, 4, 5] },
      },
    });
    const catNumericEnum = await apiClient.createCategory(
      `Schema Cat NumEnum ${runId}`,
      'Category with numeric enum',
      { MetaSchema: numericEnumSchema }
    );
    categoryNumericEnumId = catNumericEnum.ID;

    // Category with $ref — uses definitions to define a reusable address type
    const refSchema = JSON.stringify({
      type: 'object',
      definitions: {
        address: {
          type: 'object',
          properties: {
            city: { type: 'string' },
            zip: { type: 'string' },
          },
        },
      },
      properties: {
        name: { type: 'string' },
        home: { $ref: '#/definitions/address' },
      },
    });
    const catRef = await apiClient.createCategory(
      `Schema Cat Ref ${runId}`,
      'Category with $ref schema',
      { MetaSchema: refSchema }
    );
    categoryRefId = catRef.ID;

    // Category with allOf — merges two schema fragments at top level
    const allOfSchema = JSON.stringify({
      allOf: [
        {
          type: 'object',
          properties: {
            firstName: { type: 'string' },
          },
        },
        {
          type: 'object',
          properties: {
            age: { type: 'integer' },
          },
        },
      ],
    });
    const catAllOf = await apiClient.createCategory(
      `Schema Cat AllOf ${runId}`,
      'Category with allOf schema',
      { MetaSchema: allOfSchema }
    );
    categoryAllOfId = catAllOf.ID;

    // Category with oneOf — two variant objects, search should see union of fields
    const oneOfSchema = JSON.stringify({
      type: 'object',
      properties: {
        kind: { type: 'string' },
      },
      oneOf: [
        { properties: { wingspan: { type: 'number' } } },
        { properties: { topSpeed: { type: 'number' } } },
      ],
    });
    const catOneOf = await apiClient.createCategory(
      `Schema Cat OneOf ${runId}`,
      'Category with oneOf schema',
      { MetaSchema: oneOfSchema }
    );
    categoryOneOfId = catOneOf.ID;

    // Category that omits type:"object" but has properties (valid JSON Schema shorthand)
    const noTypeSchema = JSON.stringify({
      properties: {
        nickname: { type: 'string' },
        score: { type: 'integer' },
      },
    });
    const catNoType = await apiClient.createCategory(
      `Schema Cat NoType ${runId}`,
      'Category with properties but no explicit type',
      { MetaSchema: noTypeSchema }
    );
    categoryNoTypeId = catNoType.ID;

    // Category with rating as plain integer (no enum) — for numeric enum merge test
    const intRatingSchema = JSON.stringify({
      type: 'object',
      properties: {
        rating: { type: 'integer' },
      },
    });
    const catIntRating = await apiClient.createCategory(
      `Schema Cat IntRating ${runId}`,
      'Category with plain integer rating',
      { MetaSchema: intRatingSchema }
    );
    categoryIntRatingId = catIntRating.ID;

    // Category with allOf containing $ref + sibling properties
    const refSiblingSchema = JSON.stringify({
      definitions: {
        base: {
          type: 'object',
          properties: {
            id: { type: 'integer' },
          },
        },
      },
      allOf: [
        {
          $ref: '#/definitions/base',
          properties: {
            extra: { type: 'string' },
          },
        },
      ],
    });
    const catRefSibling = await apiClient.createCategory(
      `Schema Cat RefSibling ${runId}`,
      'Category with $ref + sibling properties in allOf',
      { MetaSchema: refSiblingSchema }
    );
    categoryRefSiblingId = catRefSibling.ID;

    // Category with a plain string field (no enum) for string-quoting tests
    const plainStringSchema = JSON.stringify({
      type: 'object',
      properties: {
        label: { type: 'string' },
      },
    });
    const catPlainString = await apiClient.createCategory(
      `Schema Cat PlainStr ${runId}`,
      'Category with plain string field',
      { MetaSchema: plainStringSchema }
    );
    categoryPlainStringId = catPlainString.ID;

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
    if (categoryPlainStringId) await apiClient.deleteCategory(categoryPlainStringId).catch(() => {});
    if (categoryIntegerWeightId) await apiClient.deleteCategory(categoryIntegerWeightId).catch(() => {});
    if (categoryCoercibleEnumId) await apiClient.deleteCategory(categoryCoercibleEnumId).catch(() => {});
    if (categoryNumericEnumId) await apiClient.deleteCategory(categoryNumericEnumId).catch(() => {});
    if (categoryRefId) await apiClient.deleteCategory(categoryRefId).catch(() => {});
    if (categoryAllOfId) await apiClient.deleteCategory(categoryAllOfId).catch(() => {});
    if (categoryOneOfId) await apiClient.deleteCategory(categoryOneOfId).catch(() => {});
    if (categoryNoTypeId) await apiClient.deleteCategory(categoryNoTypeId).catch(() => {});
    if (categoryIntRatingId) await apiClient.deleteCategory(categoryIntRatingId).catch(() => {});
    if (categoryRefSiblingId) await apiClient.deleteCategory(categoryRefSiblingId).catch(() => {});
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

  test('mixing a schema category with a category without MetaSchema hides schema fields', async ({
    groupPage,
    page,
  }) => {
    await groupPage.gotoList();
    await selectGroupCategory(page, `Schema Cat A ${runId}`);

    const container = schemaFieldsGroup(page);
    await expect(container.locator('input, select')).not.toHaveCount(0);

    await selectGroupCategory(page, `Schema Cat None ${runId}`);

    // A missing schema anywhere in the selected set should suppress
    // schema-driven filters rather than imply they apply to all categories.
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

  // ── 16b. Single enum value restored after reload ────────────────────────────

  test('single enum checkbox is checked after form submit and reload', async ({
    groupPage,
    page,
  }) => {
    await groupPage.gotoList();
    await selectGroupCategory(page, `Schema Cat A ${runId}`);

    const container = schemaFieldsGroup(page);

    // Check only "red"
    await container.getByRole('checkbox', { name: 'red' }).check();

    // Submit
    await submitFilterForm(page, 'Filter groups');

    // After reload, "red" checkbox should be checked
    const restoredContainer = schemaFieldsGroup(page);
    await expect(restoredContainer.getByRole('checkbox', { name: 'red' })).toBeVisible({ timeout: 5000 });
    await expect(restoredContainer.getByRole('checkbox', { name: 'red' })).toBeChecked();
    await expect(restoredContainer.getByRole('checkbox', { name: 'green' })).not.toBeChecked();
  });

  // ── 16c. Category change preserves in-progress field values ──────────────────

  test('adding a second category preserves in-progress field values for common fields', async ({
    groupPage,
    page,
  }) => {
    await groupPage.gotoList();

    // Select category A (has: color, weight, active, dimensions.width/height)
    await selectGroupCategory(page, `Schema Cat A ${runId}`);

    const container = schemaFieldsGroup(page);

    // Fill weight with a value the user hasn't submitted yet
    const weightInput = container.locator('input[type="number"]').first();
    await weightInput.fill('99');

    // Now add category B (has: weight, active, score)
    // "weight" is common to both — its value should be preserved
    await selectGroupCategory(page, `Schema Cat B ${runId}`);

    // Weight should still show "99" (preserved from before category change)
    const updatedWeightInput = container.locator('input[type="number"]').first();
    await expect(updatedWeightInput).toHaveValue('99');
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

  // ── 17. Deselecting last schema category restores freeFields entries ────────

  test('removing the last schema category restores claimed entries to freeFields', async ({
    groupPage,
    page,
  }) => {
    await groupPage.gotoList();
    await selectGroupCategory(page, `Schema Cat A ${runId}`);

    const container = schemaFieldsGroup(page);

    // Fill weight in schema fields
    const weightInput = container.locator('input[type="number"]').first();
    await weightInput.fill('42');

    // Submit so weight is in the URL
    await submitFilterForm(page, 'Filter groups');

    // After reload, weight should be in schema fields (not freeFields)
    const restoredContainer = schemaFieldsGroup(page);
    await expect(restoredContainer.locator('input[type="number"]').first()).toBeVisible({ timeout: 5000 });

    // Now remove the category
    await removeGroupCategory(page, `Schema Cat A ${runId}`);

    // Schema fields should be gone
    await expect(restoredContainer.locator('input[type="number"]')).toHaveCount(0);

    // freeFields should now show the "weight" entry again (restored)
    const freeFieldsGroup = page.locator('[role="group"][aria-label="Meta"]');
    const freeFieldNameInputs = freeFieldsGroup.locator('input[type="text"]');
    const count = await freeFieldNameInputs.count();
    let hasWeight = false;
    for (let i = 0; i < count; i++) {
      const val = await freeFieldNameInputs.nth(i).inputValue();
      if (val === 'weight') hasWeight = true;
    }
    expect(hasWeight, 'freeFields should restore "weight" after schema category is removed').toBe(true);
  });

  // ── 18. Range queries (multi-match non-enum) stay in freeFields ─────────────

  test('range query (weight:GT:5 + weight:LT:10) is preserved in freeFields, not claimed by schema', async ({
    groupPage,
    page,
  }) => {
    // Navigate directly with a range query in the URL
    await page.goto(`/groups?categories=${categoryWithSchemaId}&MetaQuery.0=${encodeURIComponent('weight:GT:5')}&MetaQuery.1=${encodeURIComponent('weight:LT:10')}`);
    await page.waitForLoadState('load');

    // Wait for Alpine to initialise
    await page.waitForTimeout(500);

    // The schema weight field should NOT pre-fill (it can't represent a range)
    const container = schemaFieldsGroup(page);
    // Schema fields may or may not render — if they do, weight should be empty
    const schemaWeightInputs = container.locator('input[type="number"]');
    const schemaCount = await schemaWeightInputs.count();
    if (schemaCount > 0) {
      // If schema field is shown, it should be empty (not pre-filled with one of the range values)
      const firstVal = await schemaWeightInputs.first().inputValue();
      expect(firstVal, 'schema weight field should be empty for range queries').toBe('');
    }

    // The freeFields section should still show the two weight entries
    const freeFieldsGroup = page.locator('[role="group"][aria-label="Meta"]');
    const freeFieldNameInputs = freeFieldsGroup.locator('input[type="text"]');
    const count = await freeFieldNameInputs.count();
    let weightCount = 0;
    for (let i = 0; i < count; i++) {
      const val = await freeFieldNameInputs.nth(i).inputValue();
      if (val === 'weight') weightCount++;
    }
    expect(weightCount, 'freeFields should keep both weight range entries').toBe(2);
  });

  // ── 19. User-added freeFields rows survive schema selection changes ──────────

  test('user-added freeFields rows are not erased by schema category changes', async ({
    groupPage,
    page,
  }) => {
    await groupPage.gotoList();

    // Select a category so schema fields appear
    await selectGroupCategory(page, `Schema Cat A ${runId}`);
    const container = schemaFieldsGroup(page);
    await expect(container.locator('input, select')).not.toHaveCount(0);

    // Add a custom free-form field (click "+ Add Field" in freeFields)
    const freeFieldsGroup = page.locator('[role="group"][aria-label="Meta"]');
    await freeFieldsGroup.getByRole('button', { name: 'Add new field' }).click();

    // Fill in the new field
    const freeFieldNameInputs = freeFieldsGroup.locator('input[type="text"]');
    const lastIdx = await freeFieldNameInputs.count() - 1;
    await freeFieldNameInputs.nth(lastIdx).fill('custom_key');

    // Now add a second category — this triggers schema-fields-claimed again
    await selectGroupCategory(page, `Schema Cat B ${runId}`);

    // The user-added "custom_key" should still be in freeFields
    const updatedNameInputs = freeFieldsGroup.locator('input[type="text"]');
    const count = await updatedNameInputs.count();
    let hasCustomKey = false;
    for (let i = 0; i < count; i++) {
      const val = await updatedNameInputs.nth(i).inputValue();
      if (val === 'custom_key') hasCustomKey = true;
    }
    expect(hasCustomKey, 'user-added freeFields row should survive schema category changes').toBe(true);
  });

  // ── 20. Repeated boolean MetaQuery entries stay in freeFields ───────────────

  test('repeated boolean entries (active:EQ:true + active:EQ:false) are preserved in freeFields', async ({
    groupPage,
    page,
  }) => {
    await page.goto(`/groups?categories=${categoryWithSchemaId}&MetaQuery.0=${encodeURIComponent('active:EQ:true')}&MetaQuery.1=${encodeURIComponent('active:EQ:false')}`);
    await page.waitForLoadState('load');
    await page.waitForTimeout(500);

    // freeFields should still show both "active" entries
    const freeFieldsGroup = page.locator('[role="group"][aria-label="Meta"]');
    const freeFieldNameInputs = freeFieldsGroup.locator('input[type="text"]');
    const count = await freeFieldNameInputs.count();
    let activeCount = 0;
    for (let i = 0; i < count; i++) {
      const val = await freeFieldNameInputs.nth(i).inputValue();
      if (val === 'active') activeCount++;
    }
    expect(activeCount, 'freeFields should keep both repeated boolean entries').toBe(2);
  });

  // ── 21. Non-EQ enum/boolean entries stay in freeFields ──────────────────────

  test('non-EQ enum entry (color:NE:"red") is preserved in freeFields, not rewritten as EQ', async ({
    groupPage,
    page,
  }) => {
    await page.goto(`/groups?categories=${categoryWithSchemaId}&MetaQuery=${encodeURIComponent('color:NE:"red"')}`);
    await page.waitForLoadState('load');
    await page.waitForTimeout(500);

    const freeFieldsGroup = page.locator('[role="group"][aria-label="Meta"]');
    const freeFieldNameInputs = freeFieldsGroup.locator('input[type="text"]');
    const count = await freeFieldNameInputs.count();
    let hasColor = false;
    for (let i = 0; i < count; i++) {
      if (await freeFieldNameInputs.nth(i).inputValue() === 'color') hasColor = true;
    }
    expect(hasColor, 'freeFields should keep color:NE entry that schema cannot represent').toBe(true);
  });

  test('non-EQ boolean entry (active:NE:true) is preserved in freeFields, not rewritten as EQ', async ({
    groupPage,
    page,
  }) => {
    await page.goto(`/groups?categories=${categoryWithSchemaId}&MetaQuery=${encodeURIComponent('active:NE:true')}`);
    await page.waitForLoadState('load');
    await page.waitForTimeout(500);

    const freeFieldsGroup = page.locator('[role="group"][aria-label="Meta"]');
    const freeFieldNameInputs = freeFieldsGroup.locator('input[type="text"]');
    const count = await freeFieldNameInputs.count();
    let hasActive = false;
    for (let i = 0; i < count; i++) {
      if (await freeFieldNameInputs.nth(i).inputValue() === 'active') hasActive = true;
    }
    expect(hasActive, 'freeFields should keep active:NE entry that schema cannot represent').toBe(true);
  });

  // ── 22. Resource timeline sidebar preserves bare MetaQuery params ────────────

  test('resource timeline view preserves bare MetaQuery params in the sidebar', async ({
    page,
  }) => {
    // Navigate to resource timeline with a bare MetaQuery + resource category
    await page.goto(`/resources/timeline?ResourceCategoryId=${resourceCategoryId}&MetaQuery=${encodeURIComponent('weight:EQ:42')}`);
    await page.waitForLoadState('load');
    await page.waitForTimeout(500);

    // The freeFields or schema fields sidebar should show the weight filter.
    // Check freeFields first (it should have the weight entry since
    // parsedQuery.MetaQuery should be populated).
    const freeFieldsGroup = page.locator('[role="group"][aria-label="Meta"]');
    const schemaGroup = page.locator('[role="group"][aria-label="Schema fields"]');

    // Either schema fields pre-filled weight, or freeFields has it
    const schemaWeightInputs = schemaGroup.locator('input[type="number"]');
    const freeFieldNameInputs = freeFieldsGroup.locator('input[type="text"]');

    const schemaCount = await schemaWeightInputs.count();
    let weightVisible = false;

    if (schemaCount > 0) {
      const val = await schemaWeightInputs.first().inputValue();
      if (val === '42') weightVisible = true;
    }

    if (!weightVisible) {
      const freeCount = await freeFieldNameInputs.count();
      for (let i = 0; i < freeCount; i++) {
        if (await freeFieldNameInputs.nth(i).inputValue() === 'weight') {
          weightVisible = true;
          break;
        }
      }
    }

    expect(weightVisible, 'weight:EQ:42 should be visible in timeline sidebar (schema or freeFields)').toBe(true);
  });

  // ── 23. Schema string fields preserve string type for numeric-looking values ─

  test('schema string field value "007" is submitted as quoted string, not as number', async ({
    groupPage,
    page,
  }) => {
    await groupPage.gotoList();

    // Use the category with a plain string field (no enum)
    await selectGroupCategory(page, `Schema Cat PlainStr ${runId}`);

    const container = schemaFieldsGroup(page);

    // Type a numeric-looking string into the "label" text input
    const labelInput = container.locator('input[type="text"]').first();
    await labelInput.fill('007');

    // Submit
    await submitFilterForm(page, 'Filter groups');

    // The URL should contain label:LI:"007" (quoted string), not label:LI:7 (number)
    const decoded = decodeURIComponent(page.url());
    expect(decoded).toContain('label:LI:"007"');
  });

  // ── 24. integer/number intersection stays numeric ───────────────────────────

  test('intersecting integer and number fields produces a number input, not text', async ({
    groupPage,
    page,
  }) => {
    await groupPage.gotoList();

    // Cat A has weight:number, Cat IntWeight has weight:integer
    await selectGroupCategory(page, `Schema Cat A ${runId}`);
    await selectGroupCategory(page, `Schema Cat IntWeight ${runId}`);

    const container = schemaFieldsGroup(page);

    // Weight should remain a number input (not downgraded to text)
    const numberInputs = container.locator('input[type="number"]');
    await expect(numberInputs).not.toHaveCount(0);

    // Fill and submit — should use EQ (number default), not LI (string default)
    await numberInputs.first().fill('42');
    await submitFilterForm(page, 'Filter groups');

    const decoded = decodeURIComponent(page.url());
    expect(decoded).toContain('weight:EQ:42');
    expect(decoded).not.toContain('weight:LI:');
  });

  // ── 25. Enum values with coercible strings are quoted ───────────────────────

  test('enum value "007" is submitted as quoted string, not coerced to number', async ({
    groupPage,
    page,
  }) => {
    await groupPage.gotoList();
    await selectGroupCategory(page, `Schema Cat CoercibleEnum ${runId}`);

    const container = schemaFieldsGroup(page);

    // Check the "007" checkbox
    await container.getByRole('checkbox', { name: '007' }).check();

    await submitFilterForm(page, 'Filter groups');

    const decoded = decodeURIComponent(page.url());
    // Should be code:EQ:"007" (quoted string), not code:EQ:7
    expect(decoded).toContain('code:EQ:"007"');
  });

  // ── 26. Numeric enum values are not quoted ──────────────────────────────────

  test('numeric enum value is submitted without quotes', async ({
    groupPage,
    page,
  }) => {
    await groupPage.gotoList();
    await selectGroupCategory(page, `Schema Cat NumEnum ${runId}`);

    const container = schemaFieldsGroup(page);
    await container.getByRole('checkbox', { name: '3' }).check();

    await submitFilterForm(page, 'Filter groups');

    const decoded = decodeURIComponent(page.url());
    // Should be rating:EQ:3 (unquoted number), not rating:EQ:"3"
    expect(decoded).toContain('rating:EQ:3');
    expect(decoded).not.toContain('rating:EQ:"3"');
  });

  // ── 27b. schema-fields-claimed only affects MetaQuery freeFields ─────────────

  test('schema-fields-claimed event is ignored by non-MetaQuery freeFields instances', async ({
    groupPage,
    page,
  }) => {
    await groupPage.gotoList();

    // Find a non-MetaQuery freeFields (bulk editor style, name="Meta") via Alpine,
    // inject a field whose name matches a schema path, then fire schema-fields-claimed
    // and verify the field survives.
    const survived = await page.evaluate(() => {
      const allFreeFieldEls = document.querySelectorAll('[x-data*="freeFields"]');
      for (const el of allFreeFieldEls) {
        const data = (window as any).Alpine.$data(el);
        if (!data || data.name !== 'Meta') continue;

        // Inject a field that matches a claimed path
        data.fields = [{ name: 'color', value: 'test', operation: 'EQ' }];

        // Fire the event claiming "color"
        window.dispatchEvent(new CustomEvent('schema-fields-claimed', {
          detail: { paths: ['color'] },
        }));

        // The field should still be there (non-MetaQuery freeFields ignores the event)
        return data.fields.some((f: any) => f.name === 'color');
      }
      // No non-MetaQuery freeFields found — skip (vacuously true)
      return true;
    });

    expect(survived, 'non-MetaQuery freeFields should ignore schema-fields-claimed').toBe(true);
  });

  // ── 27. $ref properties are resolved and flattened ──────────────────────────

  test('$ref property is resolved into nested fields', async ({
    groupPage,
    page,
  }) => {
    await groupPage.gotoList();
    await selectGroupCategory(page, `Schema Cat Ref ${runId}`);

    const container = schemaFieldsGroup(page);

    // "name" is a direct string property
    const nameInputs = container.locator('input[type="text"]');
    await expect(nameInputs).not.toHaveCount(0);

    // "home" references definitions.address which has city and zip
    // These should flatten as home.city and home.zip text inputs
    // Look for labels containing "Home" and "City" or "Zip"
    await expect(container.locator('label:has-text("City")')).toBeVisible();
    await expect(container.locator('label:has-text("Zip")')).toBeVisible();
  });

  // ── 28. Top-level allOf is merged and flattened ─────────────────────────────

  test('top-level allOf schema produces merged fields', async ({
    groupPage,
    page,
  }) => {
    await groupPage.gotoList();
    await selectGroupCategory(page, `Schema Cat AllOf ${runId}`);

    const container = schemaFieldsGroup(page);

    // allOf merges firstName (string) and age (integer)
    await expect(container.locator('input[type="text"]')).not.toHaveCount(0);
    await expect(container.locator('input[type="number"]')).not.toHaveCount(0);
  });

  // ── 29. oneOf/anyOf sub-schemas produce union of fields ─────────────────────

  test('oneOf schema produces fields from all variants', async ({
    groupPage,
    page,
  }) => {
    await groupPage.gotoList();
    await selectGroupCategory(page, `Schema Cat OneOf ${runId}`);

    const container = schemaFieldsGroup(page);

    // "kind" is a direct property (outside oneOf)
    await expect(container.locator('input[type="text"]')).not.toHaveCount(0);

    // "wingspan" from variant 1 and "topSpeed" from variant 2 should both appear
    await expect(container.locator('label:has-text("Wingspan")')).toBeVisible();
    await expect(container.locator('label:has-text("Top Speed")')).toBeVisible();
  });

  // ── 30. Schema without explicit type:"object" still flattens ────────────────

  test('schema with properties but no type:"object" still produces fields', async ({
    groupPage,
    page,
  }) => {
    await groupPage.gotoList();
    await selectGroupCategory(page, `Schema Cat NoType ${runId}`);

    const container = schemaFieldsGroup(page);
    await expect(container.locator('input[type="text"]')).not.toHaveCount(0);
    await expect(container.locator('input[type="number"]')).not.toHaveCount(0);
  });

  // ── 31. Enum dropped when numeric types merge with non-enum variant ─────────

  test('numeric enum is dropped when intersected with a non-enum integer variant', async ({
    groupPage,
    page,
  }) => {
    await groupPage.gotoList();

    // NumEnum has rating: {type:"number", enum:[1,2,3,4,5]}
    await selectGroupCategory(page, `Schema Cat NumEnum ${runId}`);
    // IntRating has rating: {type:"integer"} (no enum)
    await selectGroupCategory(page, `Schema Cat IntRating ${runId}`);

    const container = schemaFieldsGroup(page);

    // Rating should render as a plain number input (enum dropped), not checkboxes
    await expect(container.locator('input[type="number"]')).not.toHaveCount(0);
    await expect(container.locator('input[type="checkbox"]')).toHaveCount(0);
  });

  // ── 32. $ref siblings preserved in allOf items ──────────────────────────────

  test('allOf item with $ref and sibling properties includes both', async ({
    groupPage,
    page,
  }) => {
    await groupPage.gotoList();
    await selectGroupCategory(page, `Schema Cat RefSibling ${runId}`);

    const container = schemaFieldsGroup(page);

    // "id" comes from the $ref (definitions.base)
    await expect(container.locator('label:has-text("Id")')).toBeVisible();
    // "extra" is a sibling property alongside the $ref
    await expect(container.locator('label:has-text("Extra")')).toBeVisible();
  });
});
