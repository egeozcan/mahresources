/**
 * Accessibility tests for Schema Editor
 *
 * Tests the schema editor modal (schemaEditorModal Alpine.js component and
 * schema-editor web component) for WCAG 2.1 Level AA compliance using
 * axe-core.  The modal is accessible via the "Visual Editor" button on the
 * category create/edit form.
 */
import { test, expect } from '../../fixtures/a11y.fixture';

// ── Shared test schema ────────────────────────────────────────────────────────

const testSchema = JSON.stringify({
  type: 'object',
  properties: {
    name: { type: 'string' },
    status: { type: 'string', enum: ['active', 'inactive'] },
  },
});

// ── Test suite ────────────────────────────────────────────────────────────────

test.describe('Schema Editor Accessibility', () => {
  let categoryId: number;
  const runId = `${Date.now()}-${Math.random().toString(36).substring(2, 8)}`;

  test.beforeAll(async ({ apiClient }) => {
    const cat = await apiClient.createCategory(
      `A11y Schema Editor Cat ${runId}`,
      'Category for schema editor accessibility tests',
      { MetaSchema: testSchema }
    );
    categoryId = cat.ID;
  });

  test.afterAll(async ({ apiClient }) => {
    if (categoryId) {
      await apiClient.deleteCategory(categoryId).catch(() => {});
    }
  });

  // ── 1. Schema editor modal has no axe violations ──────────────────────────

  test('schema editor modal has no axe violations', async ({
    page,
    checkA11y,
  }) => {
    await page.goto(`/category/edit?id=${categoryId}`);
    await page.waitForLoadState('load');

    // Open the visual editor modal
    await page.click('.visual-editor-btn');

    // Wait for the schema editor dialog specifically (not the lightbox or paste-upload dialogs)
    const schemaDialog = page.locator('[aria-label="Meta JSON Schema Editor"]');
    await expect(schemaDialog).toBeVisible({ timeout: 5000 });

    // Run axe on the full page (includes the open modal)
    await checkA11y();
  });

  // ── 2. Tree panel has proper ARIA roles ───────────────────────────────────

  test('tree panel has proper ARIA roles', async ({ page }) => {
    await page.goto(`/category/edit?id=${categoryId}`);
    await page.waitForLoadState('load');

    // Open the visual editor modal
    await page.click('.visual-editor-btn');

    // Wait for the schema editor dialog
    const schemaDialog = page.locator('[aria-label="Meta JSON Schema Editor"]');
    await expect(schemaDialog).toBeVisible({ timeout: 5000 });

    // The schema-editor web component uses shadow DOM; pierce into it to find
    // the tree role.  Playwright's >> pierce selector crosses shadow boundaries.
    const schemaEditorEl = page.locator('schema-editor[mode="edit"]');
    await expect(schemaEditorEl).toBeVisible({ timeout: 5000 });

    // Verify role="tree" exists inside the web component's shadow DOM
    const treeLocator = page.locator('schema-editor[mode="edit"] >> [role="tree"]');
    await expect(treeLocator).toBeVisible({ timeout: 5000 });

    // Verify at least one role="treeitem" exists (the root node)
    const firstTreeItem = page.locator('schema-editor[mode="edit"] >> [role="treeitem"]').first();
    await expect(firstTreeItem).toBeVisible({ timeout: 5000 });
  });

  // ── 3. Modal dialog has accessible name ──────────────────────────────────

  test('modal dialog has an accessible aria-label', async ({ page }) => {
    await page.goto(`/category/edit?id=${categoryId}`);
    await page.waitForLoadState('load');

    await page.click('.visual-editor-btn');

    // Target the schema editor dialog specifically
    const dialog = page.locator('[aria-label="Meta JSON Schema Editor"]');
    await expect(dialog).toBeVisible({ timeout: 5000 });

    const ariaLabel = await dialog.getAttribute('aria-label');
    expect(ariaLabel, 'Dialog must have an aria-label for screen readers').toBeTruthy();

    const ariaModal = await dialog.getAttribute('aria-modal');
    expect(ariaModal, 'Dialog must have aria-modal="true"').toBe('true');
  });
});
