/**
 * Accessibility tests for Schema Search Fields
 *
 * Tests the schemaSearchFields Alpine.js component for WCAG 2.1 Level AA
 * compliance using axe-core. The component renders schema-driven filter
 * inputs in list-view sidebars when a category with a MetaSchema is selected.
 */
import { test, expect } from '../../fixtures/a11y.fixture';

// ── Shared test schema covering all field types ───────────────────────────────

const testSchema = JSON.stringify({
  type: 'object',
  properties: {
    color: { type: 'string', enum: ['red', 'green', 'blue'] },
    weight: { type: 'number' },
    active: { type: 'boolean' },
    title: { type: 'string' },
    dimensions: {
      type: 'object',
      properties: {
        width: { type: 'number' },
        height: { type: 'number' },
      },
    },
  },
});

// ── Helper: select a category from the groups list sidebar ────────────────────

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

// ── Test suite ────────────────────────────────────────────────────────────────

test.describe('Schema Search Fields Accessibility', () => {
  let categoryId: number;
  const runId = `${Date.now()}-${Math.random().toString(36).substring(2, 8)}`;

  test.beforeAll(async ({ apiClient }) => {
    const cat = await apiClient.createCategory(
      `A11y Schema Cat ${runId}`,
      'Category with MetaSchema for accessibility tests',
      { MetaSchema: testSchema }
    );
    categoryId = cat.ID;
  });

  test.afterAll(async ({ apiClient }) => {
    if (categoryId) {
      await apiClient.deleteCategory(categoryId).catch(() => {});
    }
  });

  // ── 1. Full page a11y with schema fields visible ──────────────────────────

  test('groups list page with schema fields visible should have no axe violations', async ({
    page,
    checkA11y,
  }) => {
    await page.goto('/groups');
    await page.waitForLoadState('load');

    // Select the category so the schema fields render
    await selectGroupCategory(page, `A11y Schema Cat ${runId}`);

    // Wait until schema fields container has rendered at least one input
    const schemaGroup = page.locator('[role="group"][aria-label="Schema fields"]');
    await expect(schemaGroup.locator('input, select, fieldset')).not.toHaveCount(0, {
      timeout: 5000,
    });

    // Run axe on the full page
    await checkA11y();
  });

  // ── 2. Component-scoped a11y on the schema fields container ───────────────

  test('schema fields container should have no axe violations (component scope)', async ({
    page,
    checkComponentA11y,
  }) => {
    await page.goto('/groups');
    await page.waitForLoadState('load');

    await selectGroupCategory(page, `A11y Schema Cat ${runId}`);

    const schemaGroup = page.locator('[role="group"][aria-label="Schema fields"]');
    await expect(schemaGroup.locator('input, select, fieldset')).not.toHaveCount(0, {
      timeout: 5000,
    });

    await checkComponentA11y('[role="group"][aria-label="Schema fields"]');
  });

  // ── 3. Component-scoped a11y on the whole sidebar filter form ─────────────

  test('filter sidebar form should have no axe violations when schema fields are visible', async ({
    page,
    checkComponentA11y,
  }) => {
    await page.goto('/groups');
    await page.waitForLoadState('load');

    await selectGroupCategory(page, `A11y Schema Cat ${runId}`);

    const schemaGroup = page.locator('[role="group"][aria-label="Schema fields"]');
    await expect(schemaGroup.locator('input, select, fieldset')).not.toHaveCount(0, {
      timeout: 5000,
    });

    await checkComponentA11y('form[aria-label="Filter groups"]');
  });

  // ── 4. Boolean fieldset has accessible name ───────────────────────────────

  test('boolean field renders inside a fieldset with an accessible aria-label', async ({
    page,
  }) => {
    await page.goto('/groups');
    await page.waitForLoadState('load');

    await selectGroupCategory(page, `A11y Schema Cat ${runId}`);

    const schemaGroup = page.locator('[role="group"][aria-label="Schema fields"]');
    // The boolean "active" field wraps radios in a fieldset
    const boolFieldset = schemaGroup.locator('fieldset').filter({ has: page.getByRole('radio', { name: 'Any' }) }).first();
    await expect(boolFieldset).toBeVisible({ timeout: 5000 });

    const ariaLabel = await boolFieldset.getAttribute('aria-label');
    expect(ariaLabel, 'Boolean fieldset must have an aria-label for screen readers').toBeTruthy();
  });

  // ── 5. Enum checkboxes fieldset has accessible name ───────────────────────

  test('enum checkbox field renders inside a fieldset with an accessible aria-label', async ({
    page,
  }) => {
    await page.goto('/groups');
    await page.waitForLoadState('load');

    await selectGroupCategory(page, `A11y Schema Cat ${runId}`);

    const schemaGroup = page.locator('[role="group"][aria-label="Schema fields"]');
    // The "color" enum field wraps checkboxes in a fieldset
    const enumFieldset = schemaGroup.locator('fieldset').filter({ has: page.getByRole('checkbox', { name: 'red' }) }).first();
    await expect(enumFieldset).toBeVisible({ timeout: 5000 });

    const ariaLabel = await enumFieldset.getAttribute('aria-label');
    expect(ariaLabel, 'Enum fieldset must have an aria-label for screen readers').toBeTruthy();
  });

  // ── 6. String/number inputs have associated <label> elements ─────────────

  test('string and number inputs have associated label elements', async ({ page }) => {
    await page.goto('/groups');
    await page.waitForLoadState('load');

    await selectGroupCategory(page, `A11y Schema Cat ${runId}`);

    const schemaGroup = page.locator('[role="group"][aria-label="Schema fields"]');
    await expect(schemaGroup.locator('input[type="number"], input[type="text"]')).not.toHaveCount(0, {
      timeout: 5000,
    });

    const inputs = schemaGroup.locator('input[type="number"], input[type="text"]');
    const inputCount = await inputs.count();
    expect(inputCount).toBeGreaterThan(0);

    for (let i = 0; i < inputCount; i++) {
      const input = inputs.nth(i);
      const id = await input.getAttribute('id');
      expect(id, `Input ${i + 1} must have an id for label association`).toBeTruthy();

      const label = page.locator(`label[for="${id}"]`);
      const labelCount = await label.count();
      expect(
        labelCount,
        `Input with id="${id}" must have an associated <label for="${id}">. WCAG 1.3.1 / 4.1.2.`
      ).toBeGreaterThan(0);
    }
  });

  // ── 7. Operator button has accessible aria-label ─────────────────────────

  test('operator toggle button has an accessible aria-label', async ({ page }) => {
    await page.goto('/groups');
    await page.waitForLoadState('load');

    await selectGroupCategory(page, `A11y Schema Cat ${runId}`);

    const schemaGroup = page.locator('[role="group"][aria-label="Schema fields"]');
    await expect(schemaGroup.locator('input[type="number"]')).not.toHaveCount(0, {
      timeout: 5000,
    });

    // The operator toggle is an icon-like button showing a symbol (e.g. "=")
    const operatorButton = schemaGroup.locator('button[aria-label^="Change operator"]').first();
    await expect(operatorButton).toBeVisible({ timeout: 5000 });

    const ariaLabel = await operatorButton.getAttribute('aria-label');
    expect(ariaLabel, 'Operator toggle button must have an aria-label. WCAG 4.1.2.').toBeTruthy();
  });

  // ── 8. Operator expanded select has accessible aria-label ────────────────

  test('expanded operator select has an accessible aria-label', async ({ page }) => {
    await page.goto('/groups');
    await page.waitForLoadState('load');

    await selectGroupCategory(page, `A11y Schema Cat ${runId}`);

    const schemaGroup = page.locator('[role="group"][aria-label="Schema fields"]');
    await expect(schemaGroup.locator('input[type="number"]')).not.toHaveCount(0, {
      timeout: 5000,
    });

    // Click the first operator button to expand it
    const operatorButton = schemaGroup.locator('button[aria-label^="Change operator"]').first();
    await operatorButton.click();

    // The expanded <select> should have an aria-label
    const operatorSelect = schemaGroup.locator('select[aria-label^="Operator for"]').first();
    await expect(operatorSelect).toBeVisible({ timeout: 3000 });

    const ariaLabel = await operatorSelect.getAttribute('aria-label');
    expect(ariaLabel, 'Expanded operator select must have an aria-label. WCAG 4.1.2.').toBeTruthy();
  });

  // ── 9. aria-live region announces field count ─────────────────────────────

  test('schema fields group has an aria-live polite status region', async ({ page }) => {
    await page.goto('/groups');
    await page.waitForLoadState('load');

    // The aria-live span is present in the DOM even before selection
    const schemaGroup = page.locator('[role="group"][aria-label="Schema fields"]');
    await expect(schemaGroup).toBeAttached();

    const liveRegion = schemaGroup.locator('[aria-live="polite"]');
    await expect(liveRegion).toBeAttached();

    const ariaAtomic = await liveRegion.getAttribute('aria-atomic');
    expect(ariaAtomic, 'aria-live region should be atomic to announce the full message').toBe('true');
  });
});
