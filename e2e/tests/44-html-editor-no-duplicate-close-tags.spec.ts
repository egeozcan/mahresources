/**
 * Tests that the HTML CodeMirror editor does not duplicate closing tags
 * when the user types HTML.
 *
 * Bug: The @codemirror/lang-html auto-close feature inserts closing tags
 * automatically, but there's no skip-over behavior. When a user types
 * a closing tag that was auto-inserted, it gets duplicated.
 */
import { test, expect } from '../fixtures/base.fixture';

test.describe('HTML editor does not duplicate closing tags', () => {
  test('typing HTML in the query template editor should not produce extra closing tags', async ({
    page,
  }) => {
    await page.goto('/query/new');
    await page.waitForLoadState('load');

    // Fill in required fields
    await page.locator('input[name="name"]').fill('HTML Test Query');

    // Wait for CodeMirror editors to initialize (they lazy-load)
    // The SQL editor appears first (data-language="sql" on .cm-content)
    const sqlContent = page.locator('.cm-content[data-language="sql"]');
    await expect(sqlContent).toBeVisible({ timeout: 10000 });
    await sqlContent.click();
    await page.keyboard.type('SELECT 1');

    // Wait for the HTML editor to initialize
    const htmlContent = page.locator('.cm-content[data-language="html"]');
    await expect(htmlContent).toBeVisible({ timeout: 10000 });
    await htmlContent.click();

    // Type an opening tag — the editor may auto-insert the closing tag
    await page.keyboard.type('<p>Hello</p>');

    // Check what's in the hidden input (the actual value that will be saved)
    const hiddenValue = await page.locator('input[name="Template"]').inputValue();

    // The value should be exactly <p>Hello</p>, not <p>Hello</p></p>
    expect(hiddenValue).toBe('<p>Hello</p>');
  });
});
