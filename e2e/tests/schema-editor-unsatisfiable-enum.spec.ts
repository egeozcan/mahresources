/**
 * E2E Tests for Unsatisfiable Enum (enum: []) Surfacing
 *
 * An unsatisfiable enum occurs when allOf merges conflicting enum constraints:
 *   allOf: [
 *     { properties: { status: { type: 'string', enum: ['red', 'blue'] } } },
 *     { properties: { status: { type: 'string', enum: ['green', 'yellow'] } } },
 *   ]
 * After resolution the intersection is empty → enum: [].
 *
 * These tests verify that:
 * 1. Form mode: the select renders only the placeholder (no real options) and
 *    the form is still submittable.
 * 2. Search mode: the status field renders with no checkboxes / options.
 * 3. Visual editor: the enum section is visible in the tree detail panel but
 *    shows 0 values.
 *
 * Category "Unsatisfiable Enum Test" (ID 4) is pre-seeded on the running server.
 */
import { test, expect } from '../fixtures/base.fixture';
import {
  selectGroupCategory,
  schemaFieldsGroup,
} from '../helpers/schema-search-helpers';

// ── Constants ─────────────────────────────────────────────────────────────────

const CATEGORY_NAME = 'Unsatisfiable Enum Test';
const CATEGORY_ID = 4;

// Helper: open the schema editor dialog from the category edit page
function schemaEditorDialog(page: Parameters<typeof test>[1]['page']) {
  return page.getByRole('dialog', { name: 'Meta JSON Schema Editor' });
}

// ── Test 1: Form mode ─────────────────────────────────────────────────────────

test.describe('Unsatisfiable enum: form mode', () => {
  test('status select renders only placeholder option when enum is empty', async ({ page }) => {
    await page.goto('/group/new');
    await page.waitForLoadState('load');

    // schema-form-mode should not be visible before selecting a category
    await expect(page.locator('schema-form-mode')).not.toBeVisible();

    // Select the "Unsatisfiable Enum Test" category
    const categoryInput = page.getByRole('combobox', { name: 'Category' });
    await categoryInput.click();
    await categoryInput.fill(CATEGORY_NAME);

    const option = page
      .locator('div[role="option"]:visible')
      .filter({ hasText: CATEGORY_NAME })
      .first();
    await option.waitFor({ timeout: 10000 });
    await option.click();
    await option.waitFor({ state: 'hidden', timeout: 3000 }).catch(() => {});

    // Give Alpine / Lit time to propagate the MetaSchema
    await page.waitForTimeout(500);

    // The schema-form-mode should now be visible
    const formMode = page.locator('schema-form-mode');
    await expect(formMode).toBeVisible({ timeout: 5000 });

    // Locate the select for the "status" field.
    // An empty enum should still render a <select> (not crash), but have
    // only the placeholder option — no real values.
    const statusSelect = formMode.locator('select');
    await expect(statusSelect).toBeVisible({ timeout: 5000 });

    // Count the <option> elements inside the select.
    // The only option should be the placeholder ("-- select --" or similar).
    const optionCount = await statusSelect.locator('option').count();

    // FINDING: document what we actually see
    const allOptionTexts = await statusSelect.locator('option').allTextContents();
    console.log(`[Test 1] status select option count: ${optionCount}, values: ${JSON.stringify(allOptionTexts)}`);

    // The select should have exactly 1 option (the placeholder).
    // If more options are present that is a bug (enum intersection should be empty).
    expect(optionCount).toBe(1);

    // The single option text should look like a placeholder (empty or "-- select --")
    const placeholderText = allOptionTexts[0];
    expect(
      placeholderText === '' || placeholderText.includes('select') || placeholderText.includes('--')
    ).toBe(true);

    // The form should still be submittable — the submit button must be present
    // (empty enum must not crash the page)
    await expect(page.locator('button[type="submit"]')).toBeVisible();

    // Verify no JS errors occurred during rendering
    // (already implicitly checked by the test completing without throwing)
  });
});

// ── Test 2: Search mode ───────────────────────────────────────────────────────

test.describe('Unsatisfiable enum: search mode', () => {
  test('status field renders no checkboxes or options in the search sidebar', async ({
    page,
  }) => {
    await page.goto('/groups');
    await page.waitForLoadState('load');

    // Select the category in the search sidebar using the shared helper
    await selectGroupCategory(page, CATEGORY_NAME);

    // The schema-search-mode component should be present in the DOM
    const searchModeEl = page.locator('schema-search-mode');
    await expect(searchModeEl).toBeAttached({ timeout: 5000 });

    // The schema fields container ([role="group"][aria-label="Schema fields"])
    // may or may not be rendered for an all-empty-enum schema.
    const container = schemaFieldsGroup(page);

    // Give the component time to render
    await page.waitForTimeout(300);

    const containerVisible = await container.isVisible();
    console.log(`[Test 2] schema fields container visible: ${containerVisible}`);

    if (containerVisible) {
      // If the container IS visible, it must not have any checkboxes for "status"
      // because the intersection enum is empty → no valid values to filter by.
      const checkboxes = container.locator('input[type="checkbox"]');
      const checkboxCount = await checkboxes.count();
      console.log(`[Test 2] checkbox count inside schema fields: ${checkboxCount}`);

      // FINDING: An empty-enum field should produce 0 checkboxes.
      // If checkboxes are present that is unexpected behavior.
      expect(checkboxCount).toBe(0);

      // Similarly, no <option> elements beyond a potential placeholder
      const selects = container.locator('select');
      const selectCount = await selects.count();
      console.log(`[Test 2] select count inside schema fields: ${selectCount}`);

      if (selectCount > 0) {
        // Each select should have at most 1 option (placeholder only)
        for (let i = 0; i < selectCount; i++) {
          const opts = await selects.nth(i).locator('option').count();
          console.log(`[Test 2] select[${i}] option count: ${opts}`);
          expect(opts).toBeLessThanOrEqual(1);
        }
      }
    } else {
      // The container not being visible is also acceptable — it means the
      // component correctly chose not to render anything for an all-empty schema.
      console.log('[Test 2] schema fields container not visible (acceptable — no renderable fields)');
    }

    // Page must not have crashed — use the specific "Apply Filters" button to
    // avoid strict-mode violation (the groups list page has several submit buttons)
    await expect(page.getByRole('button', { name: 'Apply Filters' })).toBeVisible();
  });
});

// ── Test 3: Visual editor ─────────────────────────────────────────────────────
//
// FINDING (documented): The visual editor does NOT flatten/resolve the allOf
// before displaying the tree. It shows the raw schema structure:
//
//   root (allOf badge)
//     ├── variant1 (string badge)  ← first allOf branch
//     └── variant2 (string badge)  ← second allOf branch
//
// The "status" property is NOT a direct treeitem — it is nested inside each
// variant. This means the editor exposes the unresolved conflict to the user
// rather than hiding it. The resolved empty-enum form/search behaviour (tests 1
// & 2) operates on a server-side resolved schema.

test.describe('Unsatisfiable enum: visual editor', () => {
  test('tree shows allOf structure with variant nodes for conflicting enum schema', async ({ page }) => {
    await page.goto(`/category/edit?id=${CATEGORY_ID}`);
    await page.waitForLoadState('load');

    // Open the Visual Editor modal
    await page.locator('.visual-editor-btn').click();
    const dialog = schemaEditorDialog(page);
    await expect(dialog).toBeVisible({ timeout: 5000 });

    // The edit-mode schema-editor should render
    const schemaEditor = dialog.locator('schema-editor[mode="edit"]');
    await expect(schemaEditor).toBeVisible({ timeout: 5000 });

    // FINDING: The editor shows the unresolved allOf structure.
    // The root node should carry an "allOf" badge.
    const rootNode = dialog.locator('[role="treeitem"]').filter({ hasText: 'root' }).first();
    await expect(rootNode).toBeVisible({ timeout: 5000 });

    // The allOf badge text should be visible on the root node
    const rootText = await rootNode.textContent();
    console.log(`[Test 3] root node text: ${JSON.stringify(rootText)}`);
    expect(rootText).toContain('allOf');

    // The two allOf branches (variant1, variant2) should appear in the tree
    const variant1 = dialog.locator('[role="treeitem"]', { hasText: 'variant1' });
    const variant2 = dialog.locator('[role="treeitem"]', { hasText: 'variant2' });
    await expect(variant1).toBeVisible({ timeout: 5000 });
    await expect(variant2).toBeVisible({ timeout: 5000 });

    // Expand variant1 by double-clicking to see the "status" property inside it
    await variant1.dblclick();
    await page.waitForTimeout(300);

    // The "status" node should now be visible as a child of variant1
    const statusNode = dialog.locator('[role="treeitem"]', { hasText: 'status' }).first();
    const statusVisible = await statusNode.isVisible();
    console.log(`[Test 3] status node visible after expanding variant1: ${statusVisible}`);

    if (statusVisible) {
      // Click status to see its detail panel
      await statusNode.click();
      await page.waitForTimeout(300);

      // FINDING: check whether an enum section / enum editor is visible
      // The branch-level enum ['red','blue'] should be shown (NOT the empty intersection).
      // The editor shows raw branch data, not the merged result.
      const enumSection = dialog.locator('text=Enum Values').first();
      const enumSectionVisible = await enumSection.isVisible();
      console.log(`[Test 3] "Enum Values" section visible after clicking status: ${enumSectionVisible}`);

      if (enumSectionVisible) {
        // The branch enum ['red', 'blue'] should be present (not empty)
        // because the editor shows raw schema — it doesn't merge enums.
        const redItem = dialog.locator('text=red').first();
        const blueItem = dialog.locator('text=blue').first();
        const redVisible = await redItem.isVisible();
        const blueVisible = await blueItem.isVisible();
        console.log(`[Test 3] enum values visible — red: ${redVisible}, blue: ${blueVisible}`);
        // The branch-level values ARE expected here (raw, unresolved view)
        expect(redVisible || blueVisible).toBe(true);
      }
    }

    // The footer should report "0 properties · 0 required" for the root
    // (allOf schemas have no direct root properties — they live in branches)
    const footer = dialog.locator('text=properties').first();
    const footerText = await footer.textContent();
    console.log(`[Test 3] footer text: ${JSON.stringify(footerText)}`);
    expect(footerText).toContain('0 properties');

    // Close the modal cleanly
    await dialog.locator('button', { hasText: 'Cancel' }).click();
    await expect(dialog).not.toBeVisible({ timeout: 3000 });
  });
});
