/**
 * E2E tests for schema editor focus management and enum drag reordering.
 *
 * Bug 1: After switching from "Preview Form" back to "Edit Schema",
 *         all inputs in the edit mode become non-functional (can't type/focus).
 * Bug 2: Tab key should move focus among inputs in the schema editor detail panel.
 * Bug 3: Enum drag handles should allow reordering enum values.
 */
import { test, expect } from '../fixtures/base.fixture';

function schemaEditorDialog(page: Parameters<typeof test>[1]['page']) {
  return page.getByRole('dialog', { name: 'Meta JSON Schema Editor' });
}

// ── Bug 1: Inputs non-functional after Preview→Edit switch ──────────────────

test.describe('Schema editor inputs after tab switch', () => {
  let categoryId: number;

  test.beforeAll(async ({ apiClient }) => {
    const schema = JSON.stringify({
      type: 'object',
      properties: {
        name: { type: 'string', title: 'Full Name' },
        age: { type: 'integer' },
      },
    });
    const cat = await apiClient.createCategory(
      `Focus Test ${Date.now()}`,
      'Testing focus after tab switch',
      { MetaSchema: schema },
    );
    categoryId = cat.ID;
  });

  test.afterAll(async ({ apiClient }) => {
    if (categoryId) await apiClient.deleteCategory(categoryId);
  });

  test('inputs remain editable after switching from Preview Form back to Edit Schema', async ({ page }) => {
    await page.goto(`/category/edit?id=${categoryId}`);
    await page.waitForLoadState('load');

    // Open visual editor
    await page.locator('.visual-editor-btn').click();
    const dialog = schemaEditorDialog(page);
    await expect(dialog).toBeVisible({ timeout: 5000 });

    // Switch to Preview Form then back to Edit Schema
    await dialog.getByRole('tab', { name: 'Preview Form' }).click();
    await expect(dialog.locator('schema-editor[mode="form"]')).toBeVisible({ timeout: 3000 });
    await dialog.getByRole('tab', { name: 'Edit Schema' }).click();
    await expect(dialog.locator('schema-editor[mode="edit"]')).toBeVisible({ timeout: 3000 });

    // Click on the "name" property in the tree to select it
    await dialog.getByRole('treeitem', { name: /name\s+string/ }).click();

    // The detail panel should show the Title input with "Full Name"
    const titleInput = dialog.getByRole('textbox', { name: 'Title' });
    await expect(titleInput).toBeVisible({ timeout: 3000 });
    await expect(titleInput).toHaveValue('Full Name');

    // Click the input and type via keyboard (not fill — fill bypasses focus-trap)
    await titleInput.click();
    await page.keyboard.press('End');
    await page.keyboard.type('XYZ', { delay: 20 });
    await expect(titleInput).toHaveValue('Full NameXYZ');
  });

  test('inputs remain editable after multiple rapid tab switches', async ({ page }) => {
    await page.goto(`/category/edit?id=${categoryId}`);
    await page.waitForLoadState('load');

    await page.locator('.visual-editor-btn').click();
    const dialog = schemaEditorDialog(page);
    await expect(dialog).toBeVisible({ timeout: 5000 });

    // Rapid switches: Edit → Preview → Raw → Edit
    await dialog.getByRole('tab', { name: 'Preview Form' }).click();
    await dialog.getByRole('tab', { name: 'Raw JSON' }).click();
    await dialog.getByRole('tab', { name: 'Edit Schema' }).click();
    await expect(dialog.locator('schema-editor[mode="edit"]')).toBeVisible({ timeout: 3000 });

    // Select the "name" property
    await dialog.getByRole('treeitem', { name: /name\s+string/ }).click();

    // Title input should be editable via keyboard (not fill — fill bypasses focus-trap)
    const titleInput = dialog.getByRole('textbox', { name: 'Title' });
    await expect(titleInput).toBeVisible({ timeout: 3000 });
    await titleInput.click();
    await page.keyboard.press('End');
    await page.keyboard.type('ABC', { delay: 20 });
    await expect(titleInput).toHaveValue('Full NameABC');
  });
});

// ── Bug 2: Tab navigation among schema editor inputs ────────────────────────

test.describe('Schema editor Tab key navigation', () => {
  let categoryId: number;

  test.beforeAll(async ({ apiClient }) => {
    const schema = JSON.stringify({
      type: 'object',
      properties: {
        fullName: { type: 'string', title: 'Some Title' },
      },
    });
    const cat = await apiClient.createCategory(
      `Tab Nav Test ${Date.now()}`,
      'Testing Tab navigation among inputs',
      { MetaSchema: schema },
    );
    categoryId = cat.ID;
  });

  test.afterAll(async ({ apiClient }) => {
    if (categoryId) await apiClient.deleteCategory(categoryId);
  });

  test('Tab key moves focus between inputs after Preview→Edit switch', async ({ page }) => {
    await page.goto(`/category/edit?id=${categoryId}`);
    await page.waitForLoadState('load');

    await page.locator('.visual-editor-btn').click();
    const dialog = schemaEditorDialog(page);
    await expect(dialog).toBeVisible({ timeout: 5000 });

    // Switch to Preview then back to Edit
    await dialog.getByRole('tab', { name: 'Preview Form' }).click();
    await expect(dialog.locator('schema-editor[mode="form"]')).toBeVisible({ timeout: 3000 });
    await dialog.getByRole('tab', { name: 'Edit Schema' }).click();
    await expect(dialog.locator('schema-editor[mode="edit"]')).toBeVisible({ timeout: 3000 });

    // Select the "fullName" property to show its detail panel
    await dialog.getByRole('treeitem', { name: /fullName\s+string/ }).click();

    // Focus the Title input via click, type to verify it's editable
    const titleInput = dialog.getByRole('textbox', { name: 'Title' });
    await expect(titleInput).toBeVisible({ timeout: 3000 });
    await titleInput.click();
    await page.keyboard.type('tab test', { delay: 20 });
    await expect(titleInput).toHaveValue(/tab test/);

    // Press Tab — should move to the Description textbox
    await page.keyboard.press('Tab');
    const descInput = dialog.getByRole('textbox', { name: 'Description' });
    await expect(descInput).toBeFocused({ timeout: 3000 });
  });
});

// ── Bug 3: Enum drag reordering ─────────────────────────────────────────────

test.describe('Schema editor enum drag reordering', () => {
  let categoryId: number;

  test.beforeAll(async ({ apiClient }) => {
    const schema = JSON.stringify({
      type: 'object',
      properties: {
        status: { type: 'string', enum: ['alpha', 'beta', 'gamma'] },
      },
    });
    const cat = await apiClient.createCategory(
      `Enum Drag Test ${Date.now()}`,
      'Testing enum value reordering',
      { MetaSchema: schema },
    );
    categoryId = cat.ID;
  });

  test.afterAll(async ({ apiClient }) => {
    if (categoryId) await apiClient.deleteCategory(categoryId);
  });

  test('drag handle reorders enum values', async ({ page }) => {
    await page.goto(`/category/edit?id=${categoryId}`);
    await page.waitForLoadState('load');

    await page.locator('.visual-editor-btn').click();
    const dialog = schemaEditorDialog(page);
    await expect(dialog).toBeVisible({ timeout: 5000 });

    // Select the "status" property to see enum editor
    await dialog.getByRole('treeitem', { name: /status\s+enum/ }).click();

    // Verify initial enum order: alpha, beta, gamma
    const enumInputs = dialog.getByRole('textbox', { name: /Enum value \d+/ });
    await expect(enumInputs).toHaveCount(3, { timeout: 3000 });
    await expect(enumInputs.nth(0)).toHaveValue('alpha');
    await expect(enumInputs.nth(1)).toHaveValue('beta');
    await expect(enumInputs.nth(2)).toHaveValue('gamma');

    // Drag the first item (alpha) down past the second (beta)
    // The drag handle is the ☰ span before each input
    const firstDragHandle = dialog.locator('.drag').nth(0);
    const secondRow = enumInputs.nth(1);
    await firstDragHandle.dragTo(secondRow);

    // After drag: beta should be first, alpha second
    await expect(enumInputs.nth(0)).toHaveValue('beta');
    await expect(enumInputs.nth(1)).toHaveValue('alpha');
    await expect(enumInputs.nth(2)).toHaveValue('gamma');

    // Apply and verify the schema was updated
    await dialog.getByRole('button', { name: 'Apply Schema' }).click();
    await expect(dialog).not.toBeVisible({ timeout: 3000 });

    const textarea = page.locator('#metaSchemaTextarea');
    const schema = JSON.parse(await textarea.inputValue());
    expect(schema.properties.status.enum).toEqual(['beta', 'alpha', 'gamma']);
  });
});
