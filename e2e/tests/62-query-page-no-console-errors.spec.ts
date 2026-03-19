import { test, expect } from '../fixtures/base.fixture';

/**
 * Bug: The query creation page triggers 9 console errors (404s) because
 * Vite's modulepreload helper resolves dynamic chunk paths as absolute
 * URLs from the root (e.g. /assets/dist-XXXX.js) instead of relative to
 * the script location (/public/dist/assets/dist-XXXX.js).
 *
 * The code editor (CodeMirror) uses dynamic imports for its modules.
 * While the actual ES module imports work (because they use relative
 * "./assets/..." paths), the preload <link> elements fail because the
 * preload helper function prepends "/" to the chunk path, creating
 * "/assets/..." instead of "/public/dist/assets/...".
 *
 * This causes:
 * - 9 failed network requests (404) on every query page load
 * - Console errors that obscure real issues
 * - Degraded performance (no preloading of code-split chunks)
 */
test.describe('Query page should load without console errors', () => {
  test('query creation page should not produce 404 errors for JS chunks', async ({ page }) => {
    // Collect console errors during page load
    const consoleErrors: string[] = [];
    page.on('console', (msg) => {
      if (msg.type() === 'error') {
        consoleErrors.push(msg.text());
      }
    });

    // Also collect failed network requests for JS files
    const failedRequests: string[] = [];
    page.on('requestfailed', (request) => {
      const url = request.url();
      if (url.endsWith('.js')) {
        failedRequests.push(url);
      }
    });

    // Navigate to the query creation page which uses CodeMirror
    await page.goto('/query/new');

    // Wait for the code editor to initialize (it lazy-loads CodeMirror)
    // The CodeMirror editor container is created by the codeEditor Alpine
    // component, which uses dynamic imports for @codemirror/* packages.
    await page.waitForTimeout(3000);

    // Verify the code editors actually loaded (there should be 2: Query + Template)
    const editorCount = await page.locator('.cm-editor').count();
    expect(editorCount, 'Both CodeMirror editors (Query and Template) should load').toBe(2);

    // The main assertion: no JS chunk files should fail to load.
    // Currently, the preload helper resolves chunk paths incorrectly,
    // causing 404s for paths like /assets/dist-XXXX.js instead of
    // /public/dist/assets/dist-XXXX.js
    expect(
      failedRequests,
      `No JS files should fail to load. Failed requests:\n${failedRequests.join('\n')}`
    ).toHaveLength(0);

    // Additionally verify no console errors related to failed resource loads
    const chunkLoadErrors = consoleErrors.filter(
      (err) => err.includes('404') || err.includes('Failed to load')
    );
    expect(
      chunkLoadErrors,
      `No console errors for failed chunk loads. Errors:\n${chunkLoadErrors.join('\n')}`
    ).toHaveLength(0);
  });
});
