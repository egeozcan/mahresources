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
          target: 'bundle',
          slots: { CustomHeader: '<h1>[property path="Name"]</h1>', CustomHeaderCSS: 'h1{color:red}' },
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
    await expect(page.locator('[data-template-cluster="CustomHeader"] input[name="CustomHeaderCSS"]')).toHaveValue('h1{color:red}');

    // The request must carry the target + slot so the server routes correctly.
    expect(seenBody).toMatchObject({ target: 'cluster', slot: 'CustomHeader', prompt: 'a header with the name' });
  });

  test('an invalid slot draft stays out of the editor until "Use anyway"', async ({ page }) => {
    await page.route('**/v1/category/generateTemplate', async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          target: 'bundle',
          slots: { CustomHeader: '<div>[meta]</div>', CustomHeaderCSS: '.meta{color:red}' },
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
    await expect(page.locator('input[name="CustomHeaderCSS"]')).toHaveValue('.meta{color:red}');
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
            CustomHeaderCSS: '',
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

for (const carrier of ['category', 'resourceCategory', 'noteType']) {
  test(`${carrier} has one generation prompt per template/CSS pair`, async ({ page }) => {
    await page.route(`**/v1/${carrier}/generateTemplate`, async (route) => {
      expect(route.request().postDataJSON()).toMatchObject({ target: 'cluster', slot: 'CustomMRQLResult' });
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({
        target: 'bundle', valid: true, explanation: 'Styles the card.',
        slots: { CustomMRQLResult: '<p class="result">Card</p>', CustomMRQLResultCSS: '.result{color:red}' },
      }) });
    });
    await page.goto(`/${carrier}/new`);
    const clusters = page.locator('[data-template-cluster]');
    await expect(clusters.first()).toBeVisible();
    for (const cluster of await clusters.all()) {
      await expect(cluster.locator('textarea[data-testid^="generate-prompt-"]')).toHaveCount(1);
      await expect(cluster.locator('button[data-testid^="generate-button-"]')).toHaveCount(1);
      await expect(cluster.locator('[x-ref="editorContainer"]')).toHaveCount(2);
    }
    const pair = page.locator('[data-template-cluster="CustomMRQLResult"]');
    await pair.getByTestId('generate-prompt-CustomMRQLResult').fill('Style the result card');
    await pair.getByTestId('generate-button-CustomMRQLResult').click();
    await expect(pair.locator('input[name="CustomMRQLResult"]')).toHaveValue('<p class="result">Card</p>');
    await expect(pair.locator('input[name="CustomMRQLResultCSS"]')).toHaveValue('.result{color:red}');
  });
}
