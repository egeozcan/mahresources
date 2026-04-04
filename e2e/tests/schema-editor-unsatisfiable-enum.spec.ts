/**
 * E2E Tests for Unsatisfiable Enum (enum: []) Surfacing
 *
 * An unsatisfiable enum occurs when allOf merges conflicting enum constraints:
 *   allOf: [
 *     { properties: { status: { type: 'string', enum: ['red', 'blue'] } } },
 *     { properties: { status: { type: 'string', enum: ['green', 'yellow'] } } },
 *   ]
 * After resolution the intersection is empty → enum: [].
 */
import { test, expect } from '../fixtures/base.fixture';
import {
  selectGroupCategory,
  schemaFieldsGroup,
} from '../helpers/schema-search-helpers';

const SCHEMA = JSON.stringify({
  type: 'object',
  allOf: [
    { properties: { status: { type: 'string', enum: ['red', 'blue'] } } },
    { properties: { status: { type: 'string', enum: ['green', 'yellow'] } } },
  ],
});

let categoryId: number;
const categoryName = 'Unsatisfiable Enum ' + Date.now();

test.beforeAll(async ({ apiClient }) => {
  const cat = await apiClient.createCategory(categoryName, 'Test category for empty enum intersection', { MetaSchema: SCHEMA });
  categoryId = cat.ID;
});

test.afterAll(async ({ apiClient }) => {
  await apiClient.deleteCategory(categoryId);
});

function schemaEditorDialog(page: any) {
  return page.getByRole('dialog', { name: 'Meta JSON Schema Editor' });
}

// ── Test 1: Form mode ─────────────────────────────────────────────────────────

test.describe('Unsatisfiable enum: form mode', () => {
  test('status select renders only placeholder option when enum is empty', async ({ page }) => {
    await page.goto('/group/new');
    await page.waitForLoadState('load');

    const categoryInput = page.getByRole('combobox', { name: 'Category' });
    await categoryInput.click();
    await categoryInput.fill(categoryName);

    const option = page.locator('div[role="option"]:visible').filter({ hasText: categoryName }).first();
    await option.waitFor({ timeout: 10000 });
    await option.click();
    await option.waitFor({ state: 'hidden', timeout: 3000 }).catch(() => {});
    await page.waitForTimeout(500);

    const formMode = page.locator('schema-form-mode');
    await expect(formMode).toBeVisible({ timeout: 5000 });

    const statusSelect = formMode.locator('select');
    await expect(statusSelect).toBeVisible({ timeout: 5000 });

    const optionCount = await statusSelect.locator('option').count();
    expect(optionCount).toBe(1);

    await expect(page.locator('button[type="submit"]')).toBeVisible();
  });
});

// ── Test 2: Search mode ───────────────────────────────────────────────────────

test.describe('Unsatisfiable enum: search mode', () => {
  test('status field renders no checkboxes or options in the search sidebar', async ({ page }) => {
    await page.goto('/groups');
    await page.waitForLoadState('load');

    await selectGroupCategory(page, categoryName);

    const searchModeEl = page.locator('schema-search-mode');
    await expect(searchModeEl).toBeAttached({ timeout: 5000 });

    const container = schemaFieldsGroup(page);
    await page.waitForTimeout(300);

    const containerVisible = await container.isVisible();

    if (containerVisible) {
      const checkboxCount = await container.locator('input[type="checkbox"]').count();
      expect(checkboxCount).toBe(0);
    }

    await expect(page.getByRole('button', { name: 'Apply Filters' })).toBeVisible();
  });
});

// ── Test 3: Visual editor ─────────────────────────────────────────────────────

test.describe('Unsatisfiable enum: visual editor', () => {
  test('tree shows allOf structure with variant nodes for conflicting enum schema', async ({ page }) => {
    await page.goto(`/category/edit?id=${categoryId}`);
    await page.waitForLoadState('load');

    await page.locator('.visual-editor-btn').click();
    const dialog = schemaEditorDialog(page);
    await expect(dialog).toBeVisible({ timeout: 5000 });

    const rootNode = dialog.locator('[role="treeitem"]').filter({ hasText: 'root' }).first();
    await expect(rootNode).toBeVisible({ timeout: 5000 });

    const rootText = await rootNode.textContent();
    expect(rootText).toContain('allOf');

    await dialog.locator('button', { hasText: 'Cancel' }).click();
    await expect(dialog).not.toBeVisible({ timeout: 3000 });
  });
});
