/**
 * E2E tests for meta edit preservation when switching between
 * schema-driven (schema-form-mode) and free-form (freeFields) editors.
 *
 * Bug 1: freeFields -> schema switch loses edits because freeFields never
 *         syncs back to currentMeta.
 * Bug 2: schema -> freeFields switch partially works because freeFields
 *         re-initializes from the static server-rendered fromJSON, ignoring
 *         currentMeta.
 */
import { test, expect } from '../fixtures/base.fixture';

test.describe('Meta edit preservation across editor switches', () => {
  const runId = Date.now();

  // Two categories: one without schema (freeFields mode), one with schema
  let noSchemaCatId: number;
  let noSchemaCatName: string;
  let schemaCatId: number;
  let schemaCatName: string;

  const schema = JSON.stringify({
    type: 'object',
    properties: {
      color: { type: 'string' },
      size: { type: 'string' },
    },
  });

  test.beforeAll(async ({ apiClient }) => {
    noSchemaCatName = `No Schema Cat ${runId}`;
    const noSchemaCat = await apiClient.createCategory(noSchemaCatName, 'No schema');
    noSchemaCatId = noSchemaCat.ID;

    schemaCatName = `Schema Cat ${runId}`;
    const schemaCat = await apiClient.createCategory(schemaCatName, 'With schema', {
      MetaSchema: schema,
    });
    schemaCatId = schemaCat.ID;
  });

  test.afterAll(async ({ apiClient }) => {
    if (noSchemaCatId) await apiClient.deleteCategory(noSchemaCatId);
    if (schemaCatId) await apiClient.deleteCategory(schemaCatId);
  });

  test('freeFields edits survive switch to schema-driven editor', async ({ page }) => {
    // Navigate to group create page
    await page.goto('/group/new');
    await page.waitForLoadState('load');

    // Select the no-schema category first to get freeFields mode
    const categoryInput = page.getByRole('combobox', { name: 'Category' });
    await categoryInput.click();
    await categoryInput.fill(noSchemaCatName);
    const noSchemaOption = page.locator('div[role="option"]:visible').filter({ hasText: noSchemaCatName }).first();
    await noSchemaOption.waitFor({ timeout: 10000 });
    await noSchemaOption.click();
    await noSchemaOption.waitFor({ state: 'hidden', timeout: 3000 }).catch(() => {});
    await page.waitForTimeout(500);

    // freeFields should be visible (no schema-form-mode)
    await expect(page.locator('schema-form-mode')).not.toBeVisible();

    // Add a meta field in freeFields: name="myKey", value="myValue"
    const freeFieldsContainer = page.locator('[x-data*="freeFields"]').filter({ hasText: 'Meta' });
    await expect(freeFieldsContainer).toBeVisible({ timeout: 5000 });

    // The freeFields component starts with fields=[] so we need to add a field
    const addFieldBtn = freeFieldsContainer.getByRole('button', { name: 'Add new field' });
    await addFieldBtn.click();

    // Fill in the field name and value
    const fieldNameInput = freeFieldsContainer.locator('input[type="text"]').first();
    await fieldNameInput.fill('myKey');
    const fieldValueInput = freeFieldsContainer.locator('input[type="text"]').nth(1);
    await fieldValueInput.fill('myValue');

    // Wait for Alpine effects to propagate
    await page.waitForTimeout(300);

    // Now switch to the schema category (removes the no-schema category first)
    // Clear the category by clicking the remove button
    const removeBtn = page.locator('button[aria-label*="Remove"]').first();
    if (await removeBtn.isVisible()) {
      await removeBtn.click();
      await page.waitForTimeout(200);
    }

    await categoryInput.click();
    await categoryInput.fill(schemaCatName);
    const schemaOption = page.locator('div[role="option"]:visible').filter({ hasText: schemaCatName }).first();
    await schemaOption.waitFor({ timeout: 10000 });
    await schemaOption.click();
    await schemaOption.waitFor({ state: 'hidden', timeout: 3000 }).catch(() => {});
    await page.waitForTimeout(500);

    // schema-form-mode should now be visible
    const formMode = page.locator('schema-form-mode');
    await expect(formMode).toBeVisible({ timeout: 5000 });

    // The hidden input in schema-form-mode should contain our freeFields edits
    // (the color field from the schema may be empty, but myKey should be preserved)
    const hiddenInput = formMode.locator('input[type="hidden"]');
    const hiddenValue = await hiddenInput.inputValue();
    const meta = JSON.parse(hiddenValue);
    expect(meta.myKey).toBe('myValue');
  });

  test('schema edits survive switch to freeFields editor', async ({ page }) => {
    // Navigate to group create page
    await page.goto('/group/new');
    await page.waitForLoadState('load');

    // Select the schema category first
    const categoryInput = page.getByRole('combobox', { name: 'Category' });
    await categoryInput.click();
    await categoryInput.fill(schemaCatName);
    const schemaOption = page.locator('div[role="option"]:visible').filter({ hasText: schemaCatName }).first();
    await schemaOption.waitFor({ timeout: 10000 });
    await schemaOption.click();
    await schemaOption.waitFor({ state: 'hidden', timeout: 3000 }).catch(() => {});
    await page.waitForTimeout(500);

    // schema-form-mode should be visible
    const formMode = page.locator('schema-form-mode');
    await expect(formMode).toBeVisible({ timeout: 5000 });

    // Fill in the "color" field
    const colorInput = formMode.locator('input[type="text"]').first();
    await expect(colorInput).toBeVisible({ timeout: 3000 });
    await colorInput.fill('red');

    // Wait for the schema-form-mode to dispatch value-change
    await page.waitForTimeout(300);

    // Verify current value is stored
    const hiddenInput = formMode.locator('input[type="hidden"]');
    const hiddenValue = await hiddenInput.inputValue();
    const parsed = JSON.parse(hiddenValue);
    expect(parsed.color).toBe('red');

    // Now switch to the no-schema category
    const removeBtn = page.locator('button[aria-label*="Remove"]').first();
    if (await removeBtn.isVisible()) {
      await removeBtn.click();
      await page.waitForTimeout(200);
    }

    await categoryInput.click();
    await categoryInput.fill(noSchemaCatName);
    const noSchemaOption = page.locator('div[role="option"]:visible').filter({ hasText: noSchemaCatName }).first();
    await noSchemaOption.waitFor({ timeout: 10000 });
    await noSchemaOption.click();
    await noSchemaOption.waitFor({ state: 'hidden', timeout: 3000 }).catch(() => {});
    await page.waitForTimeout(500);

    // freeFields should be visible now (no schema-form-mode)
    await expect(page.locator('schema-form-mode')).not.toBeVisible();

    // The freeFields component should have initialized from currentMeta,
    // which includes the color=red edit from schema-form-mode
    const freeFieldsContainer = page.locator('[x-data*="freeFields"]').filter({ hasText: 'Meta' });
    await expect(freeFieldsContainer).toBeVisible({ timeout: 5000 });

    // Verify the hidden input contains the preserved meta data
    const freeFieldsHiddenInput = freeFieldsContainer.locator('input[type="hidden"][name="Meta"]');
    // Wait for the freeFields component to initialize and compute jsonText
    await page.waitForTimeout(300);
    const freeFieldsValue = await freeFieldsHiddenInput.inputValue();
    const freeFieldsMeta = JSON.parse(freeFieldsValue);
    expect(freeFieldsMeta.color).toBe('red');
  });

  test('round-trip: freeFields -> schema -> freeFields preserves all edits', async ({ page }) => {
    await page.goto('/group/new');
    await page.waitForLoadState('load');

    const categoryInput = page.getByRole('combobox', { name: 'Category' });

    // Step 1: Start in freeFields mode, add a custom field
    await categoryInput.click();
    await categoryInput.fill(noSchemaCatName);
    const noSchemaOption = page.locator('div[role="option"]:visible').filter({ hasText: noSchemaCatName }).first();
    await noSchemaOption.waitFor({ timeout: 10000 });
    await noSchemaOption.click();
    await noSchemaOption.waitFor({ state: 'hidden', timeout: 3000 }).catch(() => {});
    await page.waitForTimeout(500);

    const freeFieldsContainer = page.locator('[x-data*="freeFields"]').filter({ hasText: 'Meta' });
    await expect(freeFieldsContainer).toBeVisible({ timeout: 5000 });

    const addFieldBtn = freeFieldsContainer.getByRole('button', { name: 'Add new field' });
    await addFieldBtn.click();
    const fieldNameInput = freeFieldsContainer.locator('input[type="text"]').first();
    await fieldNameInput.fill('custom');
    const fieldValueInput = freeFieldsContainer.locator('input[type="text"]').nth(1);
    await fieldValueInput.fill('data');
    await page.waitForTimeout(300);

    // Step 2: Switch to schema mode — edits should carry over
    const removeBtn = page.locator('button[aria-label*="Remove"]').first();
    if (await removeBtn.isVisible()) {
      await removeBtn.click();
      await page.waitForTimeout(200);
    }

    await categoryInput.click();
    await categoryInput.fill(schemaCatName);
    const schemaOption = page.locator('div[role="option"]:visible').filter({ hasText: schemaCatName }).first();
    await schemaOption.waitFor({ timeout: 10000 });
    await schemaOption.click();
    await schemaOption.waitFor({ state: 'hidden', timeout: 3000 }).catch(() => {});
    await page.waitForTimeout(500);

    const formMode = page.locator('schema-form-mode');
    await expect(formMode).toBeVisible({ timeout: 5000 });

    // Add a schema-driven edit
    const colorInput = formMode.locator('input[type="text"]').first();
    await expect(colorInput).toBeVisible({ timeout: 3000 });
    await colorInput.fill('blue');
    await page.waitForTimeout(300);

    // Verify both custom and color are in the hidden input
    const hiddenInput = formMode.locator('input[type="hidden"]');
    const hiddenValue = await hiddenInput.inputValue();
    const meta = JSON.parse(hiddenValue);
    expect(meta.custom).toBe('data');
    expect(meta.color).toBe('blue');

    // Step 3: Switch back to freeFields — all edits should still be there
    const removeBtn2 = page.locator('button[aria-label*="Remove"]').first();
    if (await removeBtn2.isVisible()) {
      await removeBtn2.click();
      await page.waitForTimeout(200);
    }

    await categoryInput.click();
    await categoryInput.fill(noSchemaCatName);
    const noSchemaOption2 = page.locator('div[role="option"]:visible').filter({ hasText: noSchemaCatName }).first();
    await noSchemaOption2.waitFor({ timeout: 10000 });
    await noSchemaOption2.click();
    await noSchemaOption2.waitFor({ state: 'hidden', timeout: 3000 }).catch(() => {});
    await page.waitForTimeout(500);

    const freeFieldsContainer2 = page.locator('[x-data*="freeFields"]').filter({ hasText: 'Meta' });
    await expect(freeFieldsContainer2).toBeVisible({ timeout: 5000 });

    const freeFieldsHiddenInput = freeFieldsContainer2.locator('input[type="hidden"][name="Meta"]');
    await page.waitForTimeout(300);
    const freeFieldsValue = await freeFieldsHiddenInput.inputValue();
    const freeFieldsMeta = JSON.parse(freeFieldsValue);
    expect(freeFieldsMeta.custom).toBe('data');
    expect(freeFieldsMeta.color).toBe('blue');
  });
});
