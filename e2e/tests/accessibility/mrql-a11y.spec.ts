/**
 * Accessibility tests for the MRQL query page
 *
 * Tests the MRQL page for WCAG 2.1 Level AA compliance using axe-core.
 */
import { test, expect } from '../../fixtures/a11y.fixture';

test.describe('MRQL Page Accessibility', () => {
  test('MRQL page should have no critical accessibility violations', async ({ page, checkA11y }) => {
    await page.goto('/mrql');
    await page.waitForLoadState('load');

    // Wait for CodeMirror to initialize
    await page.locator('.cm-editor').waitFor({ state: 'visible', timeout: 15000 });

    // Run accessibility check
    await checkA11y();
  });

  test('MRQL page should have proper heading structure', async ({ page }) => {
    await page.goto('/mrql');
    await page.waitForLoadState('load');

    // Wait for CodeMirror to initialize
    await page.locator('.cm-editor').waitFor({ state: 'visible', timeout: 15000 });

    // The page should have an h1 (from the base layout) and h2 headings for sections
    const h1 = page.locator('h1');
    await expect(h1.first()).toBeVisible();

    // Check for section headings (Query, Saved Queries are h2 elements)
    const h2s = page.locator('h2');
    const h2Count = await h2s.count();
    expect(h2Count).toBeGreaterThanOrEqual(2);

    // Verify the MRQL-specific sections have proper aria-label attributes
    const editorSection = page.locator('section[aria-label="MRQL query editor"]');
    await expect(editorSection).toBeVisible();

    const resultsSection = page.locator('section[aria-label="Query results"]');
    await expect(resultsSection).toBeAttached();

    const savedSection = page.locator('section[aria-label="Saved queries"]');
    await expect(savedSection).toBeVisible();
  });
});
