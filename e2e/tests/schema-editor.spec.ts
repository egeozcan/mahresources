/**
 * E2E Tests for the Schema Editor Modal and Form Mode
 *
 * Covers:
 * 1. Opening the modal from the category edit form
 * 2. Building a schema visually and applying it back to the textarea
 * 3. Tab switching (Edit / Preview Form / Raw JSON)
 * 4. Escape key closes the modal
 * 5. Form mode renders on the group create page when a category with MetaSchema is selected
 */
import { test, expect } from '../fixtures/base.fixture';

// ── Shared schema for form-mode tests ────────────────────────────────────────

const testSchema = JSON.stringify({
  type: 'object',
  properties: {
    name: { type: 'string', minLength: 1 },
    status: { type: 'string', enum: ['active', 'inactive'] },
    age: { type: 'integer', minimum: 0 },
  },
  required: ['name'],
});

/**
 * Helper: returns the schema editor modal dialog locator.
 * Uses getByRole with the exact aria-label to avoid matching the lightbox
 * or paste-upload dialogs that are also present in the DOM.
 */
function schemaEditorDialog(page: Parameters<typeof test>[1]['page']) {
  return page.getByRole('dialog', { name: 'Meta JSON Schema Editor' });
}

// ── Modal integration tests ───────────────────────────────────────────────────

test.describe('Schema Editor Modal', () => {
  let categoryId: number;
  const runId = Date.now();

  test.beforeAll(async ({ apiClient }) => {
    const cat = await apiClient.createCategory(
      `Schema Editor Modal Test ${runId}`,
      'Category for schema editor modal E2E tests'
    );
    categoryId = cat.ID;
  });

  test.afterAll(async ({ apiClient }) => {
    if (categoryId) await apiClient.deleteCategory(categoryId);
  });

  test('opens modal from category edit form', async ({ page }) => {
    await page.goto(`/category/edit?id=${categoryId}`);
    await page.waitForLoadState('load');

    // Click the "Visual Editor" button
    await page.locator('.visual-editor-btn').click();

    // The modal dialog should appear (identified by its aria-label)
    const dialog = schemaEditorDialog(page);
    await expect(dialog).toBeVisible({ timeout: 5000 });

    // Modal header should show the editor title
    await expect(dialog).toContainText('Meta JSON Schema');
  });

  test('builds schema visually and applies it', async ({ page }) => {
    await page.goto(`/category/edit?id=${categoryId}`);
    await page.waitForLoadState('load');

    // Open the visual editor
    await page.locator('.visual-editor-btn').click();
    const dialog = schemaEditorDialog(page);
    await expect(dialog).toBeVisible({ timeout: 5000 });

    // The edit tab should be active by default — the schema-editor should render
    const schemaEditor = dialog.locator('schema-editor[mode="edit"]');
    await expect(schemaEditor).toBeVisible({ timeout: 5000 });

    // Click "+ Property" to add a property (inside the tree panel shadow DOM)
    // Playwright auto-pierces shadow roots when using locator()
    const addPropertyBtn = dialog.getByRole('button', { name: '+ Property' });
    await expect(addPropertyBtn).toBeVisible({ timeout: 5000 });
    await addPropertyBtn.click();

    // A new tree item with the default name should appear
    const newPropertyItem = dialog.locator('[role="treeitem"]', { hasText: 'newProperty' });
    await expect(newPropertyItem).toBeVisible({ timeout: 5000 });

    // Click "Apply Schema" to write schema back to the textarea
    await dialog.locator('button', { hasText: 'Apply Schema' }).click();

    // Modal should close
    await expect(dialog).not.toBeVisible({ timeout: 3000 });

    // The hidden textarea should now contain "newProperty"
    const textarea = page.locator('#metaSchemaTextarea');
    const value = await textarea.inputValue();
    expect(value).toContain('newProperty');
  });

  test('tab switching works', async ({ page }) => {
    await page.goto(`/category/edit?id=${categoryId}`);
    await page.waitForLoadState('load');

    // Pre-fill the textarea with a known schema
    await page.locator('#metaSchemaTextarea').fill(
      '{"type":"object","properties":{"name":{"type":"string"}}}'
    );

    // Open the visual editor
    await page.locator('.visual-editor-btn').click();
    const dialog = schemaEditorDialog(page);
    await expect(dialog).toBeVisible({ timeout: 5000 });

    // Default tab is Edit — the edit-mode schema-editor should be visible
    await expect(dialog.locator('schema-editor[mode="edit"]')).toBeVisible({ timeout: 5000 });

    // Switch to "Preview Form" tab
    await dialog.locator('button', { hasText: 'Preview Form' }).click();
    await expect(dialog.locator('schema-editor[mode="form"]')).toBeVisible({ timeout: 5000 });
    // Edit mode schema-editor should no longer be rendered
    await expect(dialog.locator('schema-editor[mode="edit"]')).not.toBeVisible();

    // Switch to "Raw JSON" tab
    await dialog.locator('button', { hasText: 'Raw JSON' }).click();
    const rawTextarea = dialog.locator('textarea');
    await expect(rawTextarea).toBeVisible({ timeout: 5000 });
    const rawContent = await rawTextarea.inputValue();
    expect(rawContent).toContain('"name"');

    // Close via "Cancel"
    await dialog.locator('button', { hasText: 'Cancel' }).click();
    await expect(dialog).not.toBeVisible({ timeout: 3000 });
  });

  test('escape closes modal', async ({ page }) => {
    await page.goto(`/category/edit?id=${categoryId}`);
    await page.waitForLoadState('load');

    // Open the modal
    await page.locator('.visual-editor-btn').click();
    const dialog = schemaEditorDialog(page);
    await expect(dialog).toBeVisible({ timeout: 5000 });

    // Press Escape
    await page.keyboard.press('Escape');
    await expect(dialog).not.toBeVisible({ timeout: 3000 });
  });

  test('cancel button closes modal without applying changes', async ({ page }) => {
    await page.goto(`/category/edit?id=${categoryId}`);
    await page.waitForLoadState('load');

    // Note the current textarea value
    const before = await page.locator('#metaSchemaTextarea').inputValue();

    // Open the modal
    await page.locator('.visual-editor-btn').click();
    const dialog = schemaEditorDialog(page);
    await expect(dialog).toBeVisible({ timeout: 5000 });

    // Add a property
    const addPropertyBtn = dialog.getByRole('button', { name: '+ Property' });
    await expect(addPropertyBtn).toBeVisible({ timeout: 5000 });
    await addPropertyBtn.click();
    await expect(dialog.locator('[role="treeitem"]', { hasText: 'newProperty' })).toBeVisible({ timeout: 3000 });

    // Cancel instead of applying
    await dialog.locator('button', { hasText: 'Cancel' }).click();
    await expect(dialog).not.toBeVisible({ timeout: 3000 });

    // Textarea should be unchanged
    const after = await page.locator('#metaSchemaTextarea').inputValue();
    expect(after).toBe(before);
  });
});

// ── Edit mode comprehensive tests ────────────────────────────────────────────

test.describe('Schema Editor Edit Mode', () => {
  const runId = Date.now();

  test('creates schema with multiple property types', async ({ page, apiClient }) => {
    const cat = await apiClient.createCategory(`Multi Type Test ${runId}`);
    const catId = cat.ID;

    try {
      await page.goto(`/category/edit?id=${catId}`);
      await page.waitForLoadState('load');

      await page.locator('.visual-editor-btn').click();
      const dialog = schemaEditorDialog(page);
      await expect(dialog).toBeVisible({ timeout: 5000 });

      // Add first property
      const addPropertyBtn = dialog.getByRole('button', { name: '+ Property' });
      await expect(addPropertyBtn).toBeVisible({ timeout: 5000 });
      await addPropertyBtn.click();
      await expect(dialog.locator('[role="treeitem"]', { hasText: 'newProperty' })).toBeVisible({ timeout: 5000 });

      // Add second property — the name will be auto-incremented (newProperty1 etc.)
      await addPropertyBtn.click();

      // Apply and verify both properties exist in the schema
      await dialog.locator('button', { hasText: 'Apply Schema' }).click();
      await expect(dialog).not.toBeVisible({ timeout: 3000 });

      const textarea = page.locator('#metaSchemaTextarea');
      const value = await textarea.inputValue();
      const schema = JSON.parse(value);
      expect(Object.keys(schema.properties).length).toBeGreaterThanOrEqual(2);
    } finally {
      await apiClient.deleteCategory(catId);
    }
  });

  test('preserves complex schema through edit round-trip', async ({ page, apiClient }) => {
    const complexSchema = JSON.stringify({
      type: 'object',
      properties: {
        name: { type: 'string', minLength: 1, title: 'Full Name' },
        status: { type: 'string', enum: ['active', 'inactive'] },
        age: { type: 'integer', minimum: 0, maximum: 150 },
      },
      required: ['name'],
    });

    const cat = await apiClient.createCategory(
      `Round Trip Test ${runId}`,
      'Round-trip fidelity test category',
      { MetaSchema: complexSchema }
    );
    const catId = cat.ID;

    try {
      await page.goto(`/category/edit?id=${catId}`);
      await page.waitForLoadState('load');

      // Open editor, switch to Raw JSON to verify schema loaded correctly
      await page.locator('.visual-editor-btn').click();
      const dialog = schemaEditorDialog(page);
      await expect(dialog).toBeVisible({ timeout: 5000 });

      await dialog.locator('button', { hasText: 'Raw JSON' }).click();
      const rawTextarea = dialog.locator('textarea');
      await expect(rawTextarea).toBeVisible({ timeout: 5000 });
      const rawContent = await rawTextarea.inputValue();
      const loaded = JSON.parse(rawContent);
      expect(loaded.properties.name.minLength).toBe(1);
      expect(loaded.properties.status.enum).toEqual(['active', 'inactive']);
      expect(loaded.required).toEqual(['name']);

      // Apply without changes
      await dialog.locator('button', { hasText: 'Apply Schema' }).click();
      await expect(dialog).not.toBeVisible({ timeout: 3000 });

      // Verify textarea preserved the schema
      const finalValue = await page.locator('#metaSchemaTextarea').inputValue();
      const final = JSON.parse(finalValue);
      expect(final.properties.name.minLength).toBe(1);
      expect(final.properties.status.enum).toEqual(['active', 'inactive']);
      expect(final.required).toEqual(['name']);
    } finally {
      await apiClient.deleteCategory(catId);
    }
  });
});

// ── Form mode tests ───────────────────────────────────────────────────────────

test.describe('Schema Editor Form Mode', () => {
  let categoryId: number;
  let categoryName: string;
  const runId = Date.now();

  test.beforeAll(async ({ apiClient }) => {
    categoryName = `Form Mode Test Cat ${runId}`;
    const cat = await apiClient.createCategory(
      categoryName,
      'Category with MetaSchema for form mode E2E tests',
      { MetaSchema: testSchema }
    );
    categoryId = cat.ID;
  });

  test.afterAll(async ({ apiClient }) => {
    if (categoryId) await apiClient.deleteCategory(categoryId);
  });

  test('renders schema-driven form when category selected on group create', async ({ page }) => {
    await page.goto('/group/new');
    await page.waitForLoadState('load');

    // Before selecting a category, the schema-editor form mode should not be present
    await expect(page.locator('schema-editor[mode="form"]')).not.toBeVisible();

    // Select the category using the autocompleter
    const categoryInput = page.getByRole('combobox', { name: 'Category' });
    await categoryInput.click();
    await categoryInput.fill(categoryName);

    // Wait for and click the dropdown option
    const option = page.locator(`div[role="option"]:visible`).filter({ hasText: categoryName }).first();
    await option.waitFor({ timeout: 10000 });
    await option.click();
    await option.waitFor({ state: 'hidden', timeout: 3000 }).catch(() => {});

    // Give Alpine time to propagate the MetaSchema and re-render
    await page.waitForTimeout(500);

    // The schema-editor in form mode should now be visible
    await expect(page.locator('schema-editor[mode="form"]')).toBeVisible({ timeout: 5000 });

    // It should contain the field names from our schema (rendered in light DOM by form-mode)
    const schemaEditor = page.locator('schema-editor[mode="form"]');
    await expect(schemaEditor).toContainText('name');
  });

  test('form mode renders all expected field types from schema', async ({ page }) => {
    await page.goto('/group/new');
    await page.waitForLoadState('load');

    // Select the category
    const categoryInput = page.getByRole('combobox', { name: 'Category' });
    await categoryInput.click();
    await categoryInput.fill(categoryName);
    const option = page.locator(`div[role="option"]:visible`).filter({ hasText: categoryName }).first();
    await option.waitFor({ timeout: 10000 });
    await option.click();
    await option.waitFor({ state: 'hidden', timeout: 3000 }).catch(() => {});
    await page.waitForTimeout(500);

    const schemaEditor = page.locator('schema-editor[mode="form"]');
    await expect(schemaEditor).toBeVisible({ timeout: 5000 });

    // "name" (string) — should render a text input
    const nameInput = schemaEditor.locator('input[type="text"]').first();
    await expect(nameInput).toBeVisible({ timeout: 3000 });

    // "status" (string enum) — should render a select or radio inputs
    // The form-mode renders enums as <select> or radio depending on count
    const statusControl = schemaEditor.locator('select, input[type="radio"]').first();
    await expect(statusControl).toBeVisible({ timeout: 3000 });
  });
});
