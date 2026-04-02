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

  test('adds property to selected nested object, not root', async ({ page, apiClient }) => {
    const nestedSchema = JSON.stringify({
      type: 'object',
      properties: {
        name: { type: 'string' },
        address: {
          type: 'object',
          properties: {
            city: { type: 'string' },
          },
        },
      },
    });

    const cat = await apiClient.createCategory(
      `Nested Add Test ${runId}`,
      undefined,
      { MetaSchema: nestedSchema }
    );
    const catId = cat.ID;

    try {
      await page.goto(`/category/edit?id=${catId}`);
      await page.waitForLoadState('load');

      await page.locator('.visual-editor-btn').click();
      const dialog = schemaEditorDialog(page);
      await expect(dialog).toBeVisible({ timeout: 5000 });

      // Wait for the edit-mode schema-editor to be ready
      const schemaEditor = dialog.locator('schema-editor[mode="edit"]');
      await expect(schemaEditor).toBeVisible({ timeout: 5000 });

      // The address node should be in the tree (Playwright auto-pierces shadow roots)
      const addressNode = dialog.locator('[role="treeitem"]', { hasText: 'address' });
      await expect(addressNode).toBeVisible({ timeout: 5000 });

      // Click the address node to select it
      await addressNode.click();

      // Expand address by clicking the expand icon (triangle) or double-clicking
      // Double-click toggles expand on nodes that have children
      await addressNode.dblclick();

      // After expanding, the "city" child should appear
      const cityNode = dialog.locator('[role="treeitem"]', { hasText: 'city' });
      await expect(cityNode).toBeVisible({ timeout: 5000 });

      // Click address again to ensure it's selected (dblclick may have changed selection)
      await addressNode.click();

      // Now click "+ Property" — should add to address, not root
      const addPropertyBtn = dialog.getByRole('button', { name: '+ Property' });
      await addPropertyBtn.click();

      // Apply the schema
      await dialog.locator('button', { hasText: 'Apply Schema' }).click();
      await expect(dialog).not.toBeVisible({ timeout: 3000 });

      // Parse the result and verify nesting
      const value = await page.locator('#metaSchemaTextarea').inputValue();
      const result = JSON.parse(value);

      // Root should still have exactly 2 properties: name + address
      expect(Object.keys(result.properties)).toHaveLength(2);
      expect(result.properties).toHaveProperty('name');
      expect(result.properties).toHaveProperty('address');

      // The new property should be inside address.properties, not at root level
      const addressProps = result.properties.address.properties;
      expect(Object.keys(addressProps).length).toBeGreaterThanOrEqual(2); // city + newProperty
      expect(addressProps).toHaveProperty('city');
      expect(Object.keys(addressProps)).toContain('newProperty');
    } finally {
      await apiClient.deleteCategory(catId);
    }
  });
});

// ── Deletion tests for special node types ────────────────────────────────────

test.describe('Schema Editor Delete Special Nodes', () => {
  const runId = Date.now();

  test('can delete a composition (oneOf) node from the editor', async ({ page, apiClient }) => {
    const compositionSchema = JSON.stringify({
      type: 'object',
      properties: {
        name: { type: 'string' },
        contact: {
          oneOf: [
            { type: 'string', title: 'Email' },
            { type: 'string', title: 'Phone' },
          ],
        },
      },
    });

    const cat = await apiClient.createCategory(
      `Composition Delete Test ${runId}`,
      undefined,
      { MetaSchema: compositionSchema }
    );
    const catId = cat.ID;

    try {
      await page.goto(`/category/edit?id=${catId}`);
      await page.waitForLoadState('load');

      // Open the visual editor
      await page.locator('.visual-editor-btn').click();
      const dialog = schemaEditorDialog(page);
      await expect(dialog).toBeVisible({ timeout: 5000 });

      // Wait for the edit-mode schema-editor to be ready
      await expect(dialog.locator('schema-editor[mode="edit"]')).toBeVisible({ timeout: 5000 });

      // Click the "contact" node in the tree (it should show a composition badge)
      const contactNode = dialog.locator('[role="treeitem"]', { hasText: 'contact' });
      await expect(contactNode).toBeVisible({ timeout: 5000 });
      await contactNode.click();

      // Verify the "Delete Property" button is visible in the detail panel
      const deleteBtn = dialog.getByRole('button', { name: 'Delete Property' });
      await expect(deleteBtn).toBeVisible({ timeout: 5000 });

      // Click "Delete Property"
      await deleteBtn.click();

      // Apply the schema
      await dialog.locator('button', { hasText: 'Apply Schema' }).click();
      await expect(dialog).not.toBeVisible({ timeout: 3000 });

      // Parse the textarea value and verify `contact` is gone but `name` remains
      const value = await page.locator('#metaSchemaTextarea').inputValue();
      const schema = JSON.parse(value);
      expect(schema.properties).toHaveProperty('name');
      expect(schema.properties).not.toHaveProperty('contact');
    } finally {
      await apiClient.deleteCategory(catId);
    }
  });

  test('can delete a $ref node from the editor', async ({ page, apiClient }) => {
    const refSchema = JSON.stringify({
      type: 'object',
      $defs: {
        addr: { type: 'object', properties: { city: { type: 'string' } } },
      },
      properties: {
        name: { type: 'string' },
        home: { $ref: '#/$defs/addr' },
      },
    });

    const cat = await apiClient.createCategory(
      `Ref Delete Test ${runId}`,
      undefined,
      { MetaSchema: refSchema }
    );
    const catId = cat.ID;

    try {
      await page.goto(`/category/edit?id=${catId}`);
      await page.waitForLoadState('load');

      // Open the visual editor
      await page.locator('.visual-editor-btn').click();
      const dialog = schemaEditorDialog(page);
      await expect(dialog).toBeVisible({ timeout: 5000 });

      // Wait for the edit-mode schema-editor to be ready
      await expect(dialog.locator('schema-editor[mode="edit"]')).toBeVisible({ timeout: 5000 });

      // Click the "home" node in the tree (should show $ref badge)
      const homeNode = dialog.locator('[role="treeitem"]', { hasText: 'home' });
      await expect(homeNode).toBeVisible({ timeout: 5000 });
      await homeNode.click();

      // Verify the "Delete Property" button is visible in the detail panel
      const deleteBtn = dialog.getByRole('button', { name: 'Delete Property' });
      await expect(deleteBtn).toBeVisible({ timeout: 5000 });

      // Click "Delete Property"
      await deleteBtn.click();

      // Apply the schema
      await dialog.locator('button', { hasText: 'Apply Schema' }).click();
      await expect(dialog).not.toBeVisible({ timeout: 3000 });

      // Parse the textarea value and verify `home` is gone, `name` remains, `$defs` still has `addr`
      const value = await page.locator('#metaSchemaTextarea').inputValue();
      const schema = JSON.parse(value);
      expect(schema.properties).toHaveProperty('name');
      expect(schema.properties).not.toHaveProperty('home');
      expect(schema.$defs).toHaveProperty('addr');
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

    // Before selecting a category, the schema-form-mode should not be present
    await expect(page.locator('schema-form-mode')).not.toBeVisible();

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

    // The schema-form-mode should now be visible
    await expect(page.locator('schema-form-mode')).toBeVisible({ timeout: 5000 });

    // It should contain the field names from our schema (rendered in light DOM by form-mode)
    const schemaEditor = page.locator('schema-form-mode');
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

    const schemaEditor = page.locator('schema-form-mode');
    await expect(schemaEditor).toBeVisible({ timeout: 5000 });

    // "name" (string) — should render a text input
    const nameInput = schemaEditor.locator('input[type="text"]').first();
    await expect(nameInput).toBeVisible({ timeout: 3000 });

    // "status" (string enum) — should render a select or radio inputs
    // The form-mode renders enums as <select> or radio depending on count
    const statusControl = schemaEditor.locator('select, input[type="radio"]').first();
    await expect(statusControl).toBeVisible({ timeout: 3000 });
  });

  test('required $ref and allOf fields get native validation and aria attributes', async ({ page, apiClient }) => {
    // Descriptions are placed at the property level (sibling to $ref / allOf)
    // so that propSchema.description is truthy and the description element is rendered.
    const schema = JSON.stringify({
      type: 'object',
      $defs: {
        email: { type: 'string', format: 'email' },
      },
      properties: {
        contact: {
          $ref: '#/$defs/email',
          description: 'Valid email address',
        },
        name: {
          description: 'Full legal name',
          allOf: [
            { type: 'string' },
            { minLength: 1 },
          ],
        },
      },
      required: ['contact', 'name'],
    });

    const cat = await apiClient.createCategory(
      'RefAllOf Attr Test ' + Date.now(),
      undefined,
      { MetaSchema: schema },
    );

    try {
      await page.goto('/group/new');
      // Select the category to trigger schema-driven form
      const categoryInput = page.getByRole('combobox', { name: 'Category' });
      await categoryInput.click();
      await categoryInput.fill(cat.Name);
      const option = page.locator('div[role="option"]:visible').filter({ hasText: cat.Name }).first();
      await option.waitFor({ timeout: 10000 });
      await option.click();
      await page.waitForTimeout(500);

      // Find the schema-form-mode element
      const formMode = page.locator('schema-form-mode');
      await expect(formMode).toBeVisible({ timeout: 5000 });

      // ── contact field ($ref to email, description at property level) ──────

      // The $ref resolves to { type: 'string', format: 'email' } → renders as
      // input[type="email"]. Required + aria attributes must be present.
      const contactInput = formMode.locator('input[type="email"]');
      await expect(contactInput).toBeVisible({ timeout: 3000 });
      await expect(contactInput).toHaveAttribute('required', '');
      await expect(contactInput).toHaveAttribute('aria-required', 'true');
      // Must have a non-empty id for label association
      const contactId = await contactInput.getAttribute('id');
      expect(contactId).toBeTruthy();
      // aria-describedby may include multiple IDs (e.g. "field-contact-desc field-contact-error").
      // Extract the description ID (the "-desc" suffixed one) and locate the element by id attribute.
      const contactAriaDesc = await contactInput.getAttribute('aria-describedby');
      expect(contactAriaDesc).toBeTruthy();
      const contactDescId = contactAriaDesc!.split(' ').find(id => id.endsWith('-desc'));
      expect(contactDescId).toBeTruthy();
      const contactDesc = formMode.locator(`[id="${contactDescId}"]`);
      await expect(contactDesc).toContainText('Valid email address');

      // ── name field (allOf resolved to string, description at property level) ─

      // allOf merges to { type: 'string', minLength: 1 } → renders as input[type="text"].
      const nameInput = formMode.locator('input[type="text"]').first();
      await expect(nameInput).toBeVisible({ timeout: 3000 });
      await expect(nameInput).toHaveAttribute('required', '');
      await expect(nameInput).toHaveAttribute('aria-required', 'true');
      const nameId = await nameInput.getAttribute('id');
      expect(nameId).toBeTruthy();
      const nameAriaDesc = await nameInput.getAttribute('aria-describedby');
      expect(nameAriaDesc).toBeTruthy();
      const nameDescId = nameAriaDesc!.split(' ').find(id => id.endsWith('-desc'));
      expect(nameDescId).toBeTruthy();
      const nameDesc = formMode.locator(`[id="${nameDescId}"]`);
      await expect(nameDesc).toContainText('Full legal name');
    } finally {
      await apiClient.deleteCategory(cat.ID);
    }
  });

  test('persists Meta data through form submission', async ({ page, apiClient }) => {
    // Create a category with a simple MetaSchema
    const catName = `Meta Persist Test ${runId}`;
    const simpleSchema = JSON.stringify({
      type: 'object',
      properties: { color: { type: 'string' } },
    });
    const cat = await apiClient.createCategory(catName, undefined, { MetaSchema: simpleSchema });
    const catId = cat.ID;

    let createdGroupId: number | undefined;

    try {
      // Navigate to group create page
      await page.goto('/group/new');
      await page.waitForLoadState('load');

      // Select the category
      const categoryInput = page.getByRole('combobox', { name: 'Category' });
      await categoryInput.click();
      await categoryInput.fill(catName);
      const option = page.locator('div[role="option"]:visible').filter({ hasText: catName }).first();
      await option.waitFor({ timeout: 10000 });
      await option.click();
      await option.waitFor({ state: 'hidden', timeout: 3000 }).catch(() => {});
      await page.waitForTimeout(500);

      // The schema-driven form should appear
      const formMode = page.locator('schema-form-mode');
      await expect(formMode).toBeVisible({ timeout: 5000 });

      // Fill in the group name (required)
      await page.locator('#form-name').fill(`Meta Test Group ${runId}`);

      // Fill in the "color" field rendered by schema-form-mode
      const colorInput = formMode.locator('input[type="text"]').first();
      await expect(colorInput).toBeVisible({ timeout: 3000 });
      await colorInput.fill('blue');

      // Submit the form
      await page.locator('button[type="submit"]').click();

      // Should redirect to the created group's detail page
      await page.waitForURL(/\/group\?id=\d+/, { timeout: 10000 });
      const url = page.url();
      const idMatch = url.match(/id=(\d+)/);
      expect(idMatch).not.toBeNull();
      createdGroupId = parseInt(idMatch![1], 10);

      // Fetch the group via API to verify Meta was persisted
      const group = await apiClient.getGroup(createdGroupId);
      const meta = typeof group.Meta === 'string' ? JSON.parse(group.Meta) : group.Meta;
      expect(meta).toHaveProperty('color', 'blue');
    } finally {
      if (createdGroupId) await apiClient.deleteGroup(createdGroupId);
      await apiClient.deleteCategory(catId);
    }
  });
});
