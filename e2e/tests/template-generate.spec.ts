/**
 * Natural-language generation of category template sections. The generate
 * button on each Custom* / MetaSchema editor (and the whole-template panel)
 * POSTs to /v1/{carrier}/generateTemplate and writes the result into the
 * CodeMirror-backed hidden input. The provider is stubbed via page.route so no
 * DeepSeek key is needed.
 */
import { test, expect } from '../fixtures/base.fixture';

test.describe('Template section generation', () => {
  test('a valid slot draft is applied to the editor', async ({ page }) => {
    let seenBody: Record<string, unknown> | null = null;
    await page.route('**/v1/category/generateTemplate', async (route) => {
      seenBody = route.request().postDataJSON();
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          target: 'slot',
          content: '<h1>[property path="Name"]</h1>',
          explanation: 'Shows the name.',
          valid: true,
        }),
      });
    });

    await page.goto('/category/new');
    await page.waitForLoadState('load');

    const genButton = page.getByTestId('generate-button-CustomHeader');
    await expect(genButton).toBeVisible({ timeout: 10000 });
    await page.getByTestId('generate-prompt-CustomHeader').fill('a header with the name');
    await genButton.click();

    await expect(page.locator('input[name="CustomHeader"]')).toHaveValue(
      '<h1>[property path="Name"]</h1>',
      { timeout: 10000 },
    );
    await expect(page.getByTestId('generate-status-CustomHeader')).toContainText('applied');

    // The request must carry the target + slot so the server routes correctly.
    expect(seenBody).toMatchObject({ target: 'slot', slot: 'CustomHeader', prompt: 'a header with the name' });
  });

  test('an invalid slot draft stays out of the editor until "Use anyway"', async ({ page }) => {
    await page.route('**/v1/category/generateTemplate', async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          target: 'slot',
          content: '<div>[meta]</div>',
          explanation: 'Broken.',
          valid: false,
          issues: [{ severity: 'error', message: '[meta] is missing required attribute "path"' }],
        }),
      });
    });

    await page.goto('/category/new');
    await page.waitForLoadState('load');

    await expect(page.getByTestId('generate-button-CustomHeader')).toBeVisible({ timeout: 10000 });
    await page.getByTestId('generate-prompt-CustomHeader').fill('a broken meta');
    await page.getByTestId('generate-button-CustomHeader').click();

    // Error is shown and nothing was applied.
    await expect(page.getByTestId('generate-error-CustomHeader')).toContainText('missing required attribute');
    await expect(page.locator('input[name="CustomHeader"]')).toHaveValue('');

    // Explicit opt-in applies it.
    await page.getByTestId('generate-apply-CustomHeader').click();
    await expect(page.locator('input[name="CustomHeader"]')).toHaveValue('<div>[meta]</div>');
  });

  test('a provider error leaves the editor unchanged', async ({ page }) => {
    await page.route('**/v1/category/generateTemplate', async (route) => {
      await route.fulfill({
        status: 503,
        contentType: 'application/json',
        body: JSON.stringify({ error: 'template generation is not configured' }),
      });
    });

    await page.goto('/category/new');
    await page.waitForLoadState('load');

    await expect(page.getByTestId('generate-button-CustomHeader')).toBeVisible({ timeout: 10000 });
    await page.getByTestId('generate-prompt-CustomHeader').fill('anything');
    await page.getByTestId('generate-button-CustomHeader').click();

    await expect(page.getByTestId('generate-error-CustomHeader')).toContainText('not configured');
    await expect(page.locator('input[name="CustomHeader"]')).toHaveValue('');
  });

  test('the whole-template panel fills multiple slots', async ({ page }) => {
    await page.route('**/v1/category/generateTemplate', async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          target: 'bundle',
          slots: {
            CustomHeader: '<h1>Card</h1>',
            CustomCSS: '.card{padding:1rem}',
          },
          explanation: 'A simple card.',
          valid: true,
        }),
      });
    });

    await page.goto('/category/new');
    await page.waitForLoadState('load');

    const bundleButton = page.getByTestId('template-bundle-generate-button');
    await expect(bundleButton).toBeVisible({ timeout: 10000 });
    await page.getByTestId('template-bundle-generate-prompt').fill('a compact card');
    await bundleButton.click();

    await expect(page.locator('input[name="CustomHeader"]')).toHaveValue('<h1>Card</h1>', { timeout: 10000 });
    await expect(page.locator('input[name="CustomCSS"]')).toHaveValue('.card{padding:1rem}');
  });
});
