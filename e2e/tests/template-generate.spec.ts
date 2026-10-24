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

  test('a prompt submitted before the editors have loaded waits for them', async ({ page }) => {
    await page.route('**/v1/category/generateTemplate', async (route) => {
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
    // Hold CodeMirror back, so the prompt is certainly submitted before any editor exists.
    let releaseEditors!: () => void;
    const editorsReleased = new Promise<void>((resolve) => { releaseEditors = resolve; });
    await page.route('**/public/dist/assets/*.js', async (route) => {
      const response = await route.fetch();
      const body = await response.text();
      if (body.includes('cm-editor')) await editorsReleased;
      await route.fulfill({ response, body });
    });

    await page.goto('/category/new');
    await page.waitForLoadState('load');
    await expect(page.locator('.cm-editor')).toHaveCount(0);

    await page.getByTestId('generate-prompt-CustomHeader').fill('a header with the name');
    await page.getByTestId('generate-button-CustomHeader').click();
    await expect(page.getByTestId('generate-status-CustomHeader')).toHaveText('Waiting for the editor to load…');
    await expect(page.getByTestId('generate-button-CustomHeader')).toBeEnabled();
    await expect(page.getByTestId('generate-error-CustomHeader')).toHaveCount(0);

    releaseEditors();
    await expect(page.locator('input[name="CustomHeader"]')).toHaveValue('<h1>[property path="Name"]</h1>');
    await expect(page.locator('[data-template-cluster="CustomHeader"] input[name="CustomHeaderCSS"]')).toHaveValue('h1{color:red}');
    await expect(page.getByTestId('generate-status-CustomHeader')).toContainText('applied');
    await page.unrouteAll({ behavior: 'ignoreErrors' });
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

    // The draft is listed for review; nothing reaches an editor before Apply.
    const review = page.getByTestId('template-bundle-review');
    await expect(review).toBeVisible({ timeout: 10000 });
    await expect(review.getByTestId('template-bundle-review-slot-CustomHeader')).toContainText('<h1>Card</h1>');
    await expect(page.locator('input[name="CustomHeader"]')).toHaveValue('');
    await expect(page.locator('input[name="CustomCSS"]')).toHaveValue('');

    await page.getByTestId('template-bundle-apply').click();
    await expect(page.locator('input[name="CustomHeader"]')).toHaveValue('<h1>Card</h1>', { timeout: 10000 });
    await expect(page.locator('input[name="CustomCSS"]')).toHaveValue('.card{padding:1rem}');
    await expect(review).toHaveCount(0);
  });
});

// The server's issue for a reply cut off at max_tokens
// (application_context/template_generation.go). An empty reply and JSON that
// stops mid-value both reach the client as this result with nothing to apply;
// the Go tests cover the server telling them apart from a clean stop.
const truncationIssue = {
  severity: 'error',
  message: "The model's reply was cut off at its output limit, so this draft is incomplete. Try a shorter request, or generate fewer slots at once.",
};

test.describe('Truncated and oversize drafts', () => {
  for (const reply of ['empty-length', 'malformed-length']) {
    test(`${reply}: every generate surface shows the truncation issue and applies nothing`, async ({ page }) => {
      await page.route('**/v1/category/generateTemplate', async (route) => {
        const body = route.request().postDataJSON();
        const target = body.target === 'metaschema' ? 'metaschema' : body.target === 'slot' ? 'slot' : 'bundle';
        await route.fulfill({
          status: 200,
          contentType: 'application/json',
          body: JSON.stringify({ target, explanation: '', valid: false, issues: [truncationIssue] }),
        });
      });
      await page.goto('/category/new');
      await page.waitForLoadState('load');

      // CustomCSS, a pair, and the MetaSchema each use the shared controls.
      for (const field of ['CustomCSS', 'CustomHeader', 'MetaSchema']) {
        await expect(page.getByTestId(`generate-button-${field}`)).toBeVisible({ timeout: 10000 });
        await page.getByTestId(`generate-prompt-${field}`).fill('something long');
        await page.getByTestId(`generate-button-${field}`).click();
        await expect(page.getByTestId(`generate-error-${field}`)).toContainText('cut off at its output limit');
        await expect(page.getByTestId(`generate-error-${field}`)).not.toContainText('must return both');
        await expect(page.getByTestId(`generate-apply-${field}`)).toHaveCount(0);
        await expect(page.locator(`input[name="${field}"]`)).toHaveValue('');
      }
      await expect(page.locator('input[name="CustomHeaderCSS"]')).toHaveValue('');

      // The whole-template panel.
      await page.getByTestId('template-bundle-generate-prompt').fill('something long');
      await page.getByTestId('template-bundle-generate-button').click();
      await expect(page.getByTestId('template-bundle-tools').getByRole('status')).toContainText('cut off at its output limit');
      await expect(page.getByTestId('template-bundle-review')).toHaveCount(0);
      await expect(page.locator('input[name="CustomHeader"]')).toHaveValue('');
    });
  }

  test('a parseable truncated bundle draft is listed with its issue and applied only on Apply', async ({ page }) => {
    await page.route('**/v1/category/generateTemplate', async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          target: 'bundle',
          slots: { CustomHeader: '<h1>Cut short</h1>', CustomHeaderCSS: '' },
          explanation: 'Partial.',
          valid: false,
          issues: [truncationIssue],
        }),
      });
    });
    await page.goto('/category/new');
    await page.waitForLoadState('load');
    await page.getByTestId('template-bundle-generate-prompt').fill('a long template');
    await page.getByTestId('template-bundle-generate-button').click();

    const review = page.getByTestId('template-bundle-review');
    await expect(review.getByTestId('template-bundle-review-issues')).toContainText('cut off at its output limit');
    await expect(review.getByTestId('template-bundle-review-slot-CustomHeader')).toContainText('<h1>Cut short</h1>');
    await expect(page.locator('input[name="CustomHeader"]')).toHaveValue('');

    await page.getByTestId('template-bundle-apply').click();
    await expect(page.locator('input[name="CustomHeader"]')).toHaveValue('<h1>Cut short</h1>');
  });

  test('an over-cap bundle slot is listed with its issue, unsliced, and applied only on Apply', async ({ page }) => {
    const big = `<p>${'x'.repeat(20001)}</p>`;
    const oversize = { severity: 'error', message: `Output exceeded 20000 bytes in CustomHeader (${big.length} bytes). Nothing was cut; shorten it before saving.` };
    await page.route('**/v1/category/generateTemplate', async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          target: 'bundle',
          slots: { CustomHeader: big, CustomHeaderCSS: '' },
          explanation: 'Too big.',
          valid: false,
          issues: [oversize],
          slotIssues: { CustomHeader: [oversize] },
        }),
      });
    });
    await page.goto('/category/new');
    await page.waitForLoadState('load');
    await page.getByTestId('template-bundle-generate-prompt').fill('a huge header');
    await page.getByTestId('template-bundle-generate-button').click();

    const slot = page.getByTestId('template-bundle-review-slot-CustomHeader');
    await expect(slot).toContainText('Output exceeded 20000 bytes');
    // A slot's issue is listed with the slot, not again as a draft-wide issue.
    await expect(page.getByTestId('template-bundle-review-issues')).toBeHidden();
    await expect(page.locator('input[name="CustomHeader"]')).toHaveValue('');

    await page.getByTestId('template-bundle-apply').click();
    await expect(page.locator('input[name="CustomHeader"]')).toHaveValue(big);
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
