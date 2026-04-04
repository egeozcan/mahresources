/**
 * E2E tests for three confirmed schema-editor bugs (TDD red-green-refactor).
 *
 * Bug 1 (P1): Stored XSS via MetaSchema injection into Alpine x-data
 * Bug 2 (P1): Category/schema change drops in-progress Meta edits
 * Bug 3 (P2): Opening modal on already-invalid MetaSchema lets Apply through
 */
import { test, expect } from '../fixtures/base.fixture';

// ── Bug 1: MetaSchema injection into Alpine x-data ─────────────────────────

test.describe('Bug 1: MetaSchema injection into Alpine x-data', () => {
  test('group edit page does not crash when category has JS-breaking MetaSchema', async ({ page, apiClient }) => {
    // Create a category with MetaSchema that would break JavaScript if
    // injected unescaped into an x-data expression
    const cat = await apiClient.createCategory(
      `XSS Schema Test ${Date.now()}`,
      'Category with JS-breaking MetaSchema',
      { MetaSchema: "'; alert('xss'); '" }
    );

    // Create a group with that category so the group edit page will render
    // the MetaSchema in the initial template (server-side)
    const group = await apiClient.createGroup({
      name: `XSS Test Group ${Date.now()}`,
      categoryId: cat.ID,
    });

    try {
      const jsErrors: string[] = [];
      page.on('pageerror', (err) => jsErrors.push(err.message));

      // Navigate to the group edit page -- the server renders the MetaSchema
      // directly into the x-data attribute at template-render time
      await page.goto(`/group/edit?id=${group.ID}`);
      await page.waitForLoadState('load');

      // The page should NOT have any JS errors
      expect(jsErrors).toHaveLength(0);

      // Page should be functional
      await expect(page.locator('button[type="submit"]')).toBeVisible();
    } finally {
      await apiClient.deleteGroup(group.ID);
      await apiClient.deleteCategory(cat.ID);
    }
  });

  test('group edit page does not crash with non-JSON MetaSchema', async ({ page, apiClient }) => {
    const cat = await apiClient.createCategory(
      `XSS Schema NonJSON Test ${Date.now()}`,
      'Category with non-JSON MetaSchema',
      { MetaSchema: 'this is not json' }
    );

    const group = await apiClient.createGroup({
      name: `XSS Test Group2 ${Date.now()}`,
      categoryId: cat.ID,
    });

    try {
      const jsErrors: string[] = [];
      page.on('pageerror', (err) => jsErrors.push(err.message));

      await page.goto(`/group/edit?id=${group.ID}`);
      await page.waitForLoadState('load');

      expect(jsErrors).toHaveLength(0);
      await expect(page.locator('button[type="submit"]')).toBeVisible();
    } finally {
      await apiClient.deleteGroup(group.ID);
      await apiClient.deleteCategory(cat.ID);
    }
  });

  test('dynamic category selection handles invalid MetaSchema without crashing', async ({ page, apiClient }) => {
    // Test the dynamic selection path (via autocompleter) as well
    const cat = await apiClient.createCategory(
      `XSS Dynamic Test ${Date.now()}`,
      'Category with non-JSON MetaSchema',
      { MetaSchema: 'this is not json' }
    );

    try {
      const jsErrors: string[] = [];
      page.on('pageerror', (err) => jsErrors.push(err.message));

      await page.goto('/group/new');
      await page.waitForLoadState('load');

      const categoryInput = page.getByRole('combobox', { name: 'Category' });
      await categoryInput.click();
      await categoryInput.fill(cat.Name);

      const option = page.locator('div[role="option"]:visible').filter({ hasText: cat.Name }).first();
      await option.waitFor({ timeout: 10000 });
      await option.click();
      await option.waitFor({ state: 'hidden', timeout: 3000 }).catch(() => {});
      await page.waitForTimeout(500);

      expect(jsErrors).toHaveLength(0);
      await expect(page.locator('button[type="submit"]')).toBeVisible();
    } finally {
      await apiClient.deleteCategory(cat.ID);
    }
  });
});

// ── Bug 2: Category/schema change drops in-progress Meta edits ─────────────

test.describe('Bug 2: Category change drops in-progress Meta edits', () => {
  test('preserves in-progress meta edits when schema property is re-set', async ({ page, apiClient }) => {
    // The bug: when Alpine re-evaluates `:schema="currentSchema"` (e.g.,
    // because of a re-render or schema object identity change), willUpdate()
    // rehydrates _data from the stale `this.value` property, wiping edits.
    const schema = JSON.stringify({
      type: 'object',
      properties: {
        color: { type: 'string' },
        size: { type: 'string' },
      },
    });

    const cat = await apiClient.createCategory(
      `Edit Preserve Test ${Date.now()}`,
      undefined,
      { MetaSchema: schema }
    );

    try {
      await page.goto('/group/new');
      await page.waitForLoadState('load');

      // Select the category
      const categoryInput = page.getByRole('combobox', { name: 'Category' });
      await categoryInput.click();
      await categoryInput.fill(cat.Name);
      const option = page.locator('div[role="option"]:visible').filter({ hasText: cat.Name }).first();
      await option.waitFor({ timeout: 10000 });
      await option.click();
      await option.waitFor({ state: 'hidden', timeout: 3000 }).catch(() => {});
      await page.waitForTimeout(500);

      // The schema-form-mode should be visible
      const formMode = page.locator('schema-form-mode');
      await expect(formMode).toBeVisible({ timeout: 5000 });

      // Fill in the "color" field
      const colorInput = formMode.locator('input[type="text"]').first();
      await expect(colorInput).toBeVisible({ timeout: 3000 });
      await colorInput.fill('red');

      // Verify the hidden input reflects our edit
      const hiddenInput = formMode.locator('input[type="hidden"]');
      const hiddenValue = await hiddenInput.inputValue();
      const parsed = JSON.parse(hiddenValue);
      expect(parsed.color).toBe('red');

      // Simulate a schema property re-set (as Alpine would do on re-render).
      // Re-assign the same schema string to trigger willUpdate with schema change.
      await page.evaluate((schemaStr) => {
        const el = document.querySelector('schema-form-mode') as any;
        if (el) {
          // Force schema change by setting it to a new string (same content)
          el.schema = schemaStr;
        }
      }, schema);

      // Wait for Lit to process the update
      await page.waitForTimeout(300);

      // The hidden input should STILL contain our edits
      const hiddenValue2 = await formMode.locator('input[type="hidden"]').inputValue();
      const parsed2 = JSON.parse(hiddenValue2);
      expect(parsed2.color).toBe('red');
    } finally {
      await apiClient.deleteCategory(cat.ID);
    }
  });
});

// ── Bug 3: Modal Apply enabled on invalid MetaSchema ───────────────────────

test.describe('Bug 3: Modal Apply enabled on invalid MetaSchema', () => {
  test('disables Apply when opened on invalid MetaSchema', async ({ page, apiClient }) => {
    const cat = await apiClient.createCategory(
      `Invalid Schema Modal Test ${Date.now()}`,
      undefined,
      { MetaSchema: 'this is not valid json {{{' }
    );

    try {
      await page.goto(`/category/edit?id=${cat.ID}`);
      await page.waitForLoadState('load');

      // The MetaSchema textarea should contain the invalid content
      const textarea = page.locator('#metaSchemaTextarea');
      const value = await textarea.inputValue();
      expect(value).toBe('this is not valid json {{{');

      // Open the visual editor modal
      await page.locator('.visual-editor-btn').click();
      const dialog = page.getByRole('dialog', { name: 'Meta JSON Schema Editor' });
      await expect(dialog).toBeVisible({ timeout: 5000 });

      // The Apply button should be disabled because the schema is invalid
      const applyBtn = dialog.locator('button', { hasText: 'Apply Schema' });
      await expect(applyBtn).toBeVisible();
      await expect(applyBtn).toBeDisabled();
    } finally {
      await apiClient.deleteCategory(cat.ID);
    }
  });

  test('enables Apply after fixing invalid MetaSchema in raw tab', async ({ page, apiClient }) => {
    const cat = await apiClient.createCategory(
      `Fix Schema Modal Test ${Date.now()}`,
      undefined,
      { MetaSchema: 'broken json!!!' }
    );

    try {
      await page.goto(`/category/edit?id=${cat.ID}`);
      await page.waitForLoadState('load');

      // Open the visual editor modal
      await page.locator('.visual-editor-btn').click();
      const dialog = page.getByRole('dialog', { name: 'Meta JSON Schema Editor' });
      await expect(dialog).toBeVisible({ timeout: 5000 });

      // Apply should be disabled initially
      const applyBtn = dialog.locator('button', { hasText: 'Apply Schema' });
      await expect(applyBtn).toBeDisabled();

      // Switch to Raw JSON tab and fix the content
      await dialog.locator('button', { hasText: 'Raw JSON' }).click();
      const rawTextarea = dialog.locator('textarea');
      await expect(rawTextarea).toBeVisible({ timeout: 5000 });

      // Clear and type valid JSON
      await rawTextarea.fill('{"type":"object","properties":{"name":{"type":"string"}}}');

      // Apply should now be enabled
      await expect(applyBtn).toBeEnabled({ timeout: 3000 });
    } finally {
      await apiClient.deleteCategory(cat.ID);
    }
  });
});
