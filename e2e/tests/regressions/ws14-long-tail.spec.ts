/**
 * UI bug hunt 2026-07-29, WS14: findings 57, 78, 131, 136, 149, 153.
 *
 * The server-visible half of WS14 lives in server/api_tests/ws14_long_tail_test.go,
 * because CI runs `go test` and does not run this suite. What is here is what only
 * a browser knows: the text of a window.confirm at submit time, which Alpine
 * `x-show` branch a live selection produced, and whether a page still shows a
 * value the server has already replaced.
 *
 * Two findings turned out to have a different cause than the plan states, and the
 * tests say so where it matters:
 *
 *  - 78/153 are one bug. The bulk toolbars did not "reuse the generic delete
 *    confirm"; all four authored their own message and `confirmAction` threw it
 *    away, because it destructures its argument and a string has no `.message`.
 *  - 136's POST does not "re-render the page with the OLD value". It answers
 *    {"ok":true} to a fetch and re-renders nothing; what the reader sees is the
 *    page as it was before the save, with the plugin's server-rendered output
 *    frozen at the previous setting.
 */
import { test, expect } from '../../fixtures/base.fixture';
import type { Page } from '@playwright/test';

/** Collect every native dialog, dismissing each so the page is left alone. */
function captureDialogs(page: Page): string[] {
  const seen: string[] = [];
  page.on('dialog', async (dialog) => {
    seen.push(dialog.message());
    await dialog.dismiss();
  });
  return seen;
}

/** Fail loudly on an uncaught page error rather than letting a spec pass around one. */
function failOnPageErrors(page: Page): string[] {
  const errors: string[] = [];
  page.on('pageerror', (e) => errors.push(String(e)));
  return errors;
}

/**
 * Tick the first `n` cards on a list page and wait for the store to agree.
 * Reading the store rather than counting checkboxes is deliberate: the bulk
 * toolbar's visibility, the confirm text and the Compare state are all derived
 * from it, so it is the precondition every assertion below depends on.
 */
async function selectCards(page: Page, n: number) {
  // Start from empty. A card checkbox toggles, so clicking nth(0) again after a
  // previous selection *deselects* it — the first version of this helper asked
  // for four and produced three.
  await page.evaluate(() => window.Alpine.store('bulkSelection').deselectAll());
  await expect
    .poll(() => page.evaluate(() => window.Alpine.store('bulkSelection').selectedIds.size))
    .toBe(0);

  const boxes = page.locator('.card-checkbox');
  for (let i = 0; i < n; i++) {
    await boxes.nth(i).click();
  }
  await expect
    .poll(() => page.evaluate(() => window.Alpine.store('bulkSelection').selectedIds.size))
    .toBe(n);
}

/**
 * Open one of the bulk toolbar's editors. Each editor form is `x-show`n only
 * while it is the active editor, so its fields are not interactable until the
 * toggle button bulkSelection generates for it is clicked. The button's label is
 * the form's first label or button text.
 */
async function openBulkEditor(page: Page, label: string) {
  const toggle = page.locator('.bulk-editors button', { hasText: label }).first();
  await expect(toggle).toBeVisible();
  await toggle.click();
}

/** Submit a bulk-toolbar form by its action, the way its own button does. */
async function submitBulkForm(page: Page, actionFragment: string) {
  await page.evaluate((fragment) => {
    const form = [...document.querySelectorAll('.bulk-editors form')]
      .find((f) => (f.getAttribute(':action') || (f as HTMLFormElement).action || '').includes(fragment));
    if (!form) throw new Error(`no bulk form matching ${fragment}`);
    (form as HTMLFormElement).requestSubmit();
  }, actionFragment);
}

test.describe('WS14 finding 57: a fixed field stops looking broken', () => {
  test('the "select at least 1 value" message clears once a category is chosen', async ({ page }) => {
    const errors = failOnPageErrors(page);
    await page.goto('/relationType/new');

    const name = page.locator('#Name, input[name="name"]').first();
    await expect(name).toBeVisible();
    await name.fill(`ws14 rt ${Date.now()}`);

    // Submit with both required category pickers empty.
    await page.evaluate(() => (document.querySelector('form') as HTMLFormElement).requestSubmit());

    const errorMessages = page.locator('[id$="-error"]:visible');
    await expect(errorMessages.first()).toHaveText('Please select at least 1 value');
    const combos = page.locator('input[role="combobox"]');
    await expect(combos.first()).toHaveAttribute('aria-invalid', 'true');

    // Now pick a From Category. The page must stop claiming the field is empty.
    await combos.first().click();
    await combos.first().fill('a');
    const options = page.locator('[data-selector-field="FromCategory"] [role="option"]');
    await expect(options.first()).toBeVisible();
    await options.first().click();

    const fromField = page.locator('[data-selector-field="FromCategory"]');
    await expect(fromField.locator('[id$="-error"]')).toBeHidden();
    await expect(combos.first()).not.toHaveAttribute('aria-invalid', 'true');

    // Positive control: the *other* field is still empty and still says so, so
    // this is not "the fix cleared every message on the page".
    const toField = page.locator('[data-selector-field="ToCategory"]');
    await expect(toField.locator('[id$="-error"]')).toHaveText('Please select at least 1 value');

    expect(errors).toEqual([]);
  });
});

test.describe('WS14 findings 78/153: a destructive confirm states its blast radius', () => {
  test('bulk delete names how many resources it will destroy', async ({ page, apiClient }) => {
    const suffix = Date.now();
    for (let i = 0; i < 4; i++) {
      await apiClient.createResource({
        filePath: 'test-assets/sample-image.png',
        name: `ws14-del-${suffix}-${i}`,
      });
    }
    const dialogs = captureDialogs(page);
    await page.goto(`/resources?name=ws14-del-${suffix}`);

    await selectCards(page, 1);
    await submitBulkForm(page, 'resources/delete');
    await expect.poll(() => dialogs.length).toBe(1);
    expect(dialogs[0]).toBe('Delete 1 resource? This cannot be undone.');

    await selectCards(page, 4);
    await submitBulkForm(page, 'resources/delete');
    await expect.poll(() => dialogs.length).toBe(2);
    expect(dialogs[1]).toBe('Delete 4 resources? This cannot be undone.');
  });

  test('bulk merge names the count and the winner, and refuses an empty winner', async ({ page, apiClient }) => {
    const suffix = Date.now();
    await apiClient.createTag(`ws14-merge-${suffix}-a`);
    await apiClient.createTag(`ws14-merge-${suffix}-b`);
    const winner = await apiClient.createTag(`ws14-winner-${suffix}`);

    const dialogs = captureDialogs(page);
    await page.goto(`/tags?Name=ws14-merge-${suffix}`);
    await selectCards(page, 2);

    // No winner picked yet: the guard must stop the submit before the confirm,
    // and before the request that used to answer 400 (findings 16/92's class on
    // the one surface that fix missed).
    await submitBulkForm(page, 'tags/merge');
    await page.waitForTimeout(500);
    expect(dialogs, 'an empty selection is not something to confirm').toEqual([]);
    await expect(page).toHaveURL(/\/tags\?Name=ws14-merge-/);

    // Pick the winner, then submit again.
    await openBulkEditor(page, 'Merge Winner');
    const mergeCombo = page.locator('.bulk-editors [data-selector-field="winner"] input[role="combobox"]');
    await mergeCombo.click();
    await mergeCombo.fill(`ws14-winner-${suffix}`);
    const option = page.locator('.bulk-editors [data-selector-field="winner"] [role="option"]').first();
    await expect(option).toBeVisible();
    await option.click();

    await submitBulkForm(page, 'tags/merge');
    await expect.poll(() => dialogs.length).toBe(1);
    expect(dialogs[0]).toBe(
      `Merge 2 tags into ws14-winner-${suffix}? The merged tags will be deleted.`,
    );

    // The dialog was dismissed, so nothing may have been merged. Before this
    // batch the AJAX bulk handler ran regardless of what the reader answered.
    await page.waitForTimeout(500);
    const stillThere = await apiClient.request.get(`/v1/tags?Name=ws14-merge-${suffix}`);
    expect((await stillThere.json()).length, 'a dismissed confirm merged the tags anyway').toBe(2);
    void winner;
  });
});

test.describe('WS14 finding 131: Compare explains itself instead of vanishing', () => {
  test('the Compare action stays visible and disabled at three selections', async ({ page, apiClient }) => {
    const suffix = Date.now();
    const category = await apiClient.createCategory(`ws14-cmp-cat-${suffix}`);
    for (let i = 0; i < 3; i++) {
      await apiClient.createGroup({ name: `ws14-cmp-${suffix}-${i}`, categoryId: category.ID });
    }
    await page.goto(`/groups?name=ws14-cmp-${suffix}`);

    const compare = page.getByTestId('bulk-compare-action');
    const hint = page.locator('#bulk-compare-groups-hint');

    await selectCards(page, 2);
    await expect(compare).toBeVisible();
    await expect(compare).toBeEnabled();
    await expect(hint).toBeHidden();

    await selectCards(page, 3);
    await expect(compare, 'the control disappeared instead of explaining itself').toBeVisible();
    await expect(compare).toBeDisabled();
    await expect(hint).toHaveText('Select exactly two groups to compare.');
    await expect(compare).toHaveAttribute('aria-describedby', 'bulk-compare-groups-hint');
  });

  test('the enabled Compare action still navigates to the comparison', async ({ page, apiClient }) => {
    const suffix = Date.now();
    const category = await apiClient.createCategory(`ws14-cmpgo-cat-${suffix}`);
    for (let i = 0; i < 2; i++) {
      await apiClient.createGroup({ name: `ws14-cmpgo-${suffix}-${i}`, categoryId: category.ID });
    }
    await page.goto(`/groups?name=ws14-cmpgo-${suffix}`);
    await selectCards(page, 2);
    await page.getByTestId('bulk-compare-action').click();
    await expect(page).toHaveURL(/\/group\/compare\?g1=\d+&g2=\d+/);
  });
});

test.describe('WS14 finding 149: no editor is advertised where none can open', () => {
  test('a list card offers no double-click tooltip; the detail page still does', async ({ page, apiClient }) => {
    const suffix = Date.now();
    const tag = await apiClient.createTag(`ws14-tip-${suffix}`, 'a description that lands on the card');

    await page.goto(`/tags?Name=ws14-tip-${suffix}`);
    const cardDescription = page.locator('.card .description [data-description-display]').first();
    await expect(cardDescription).toBeVisible();
    await expect(cardDescription).not.toHaveAttribute('title', /Double-click/);

    // Positive control on the other side: the detail page can edit, and says so.
    await page.goto(`/tag?id=${tag.ID}`);
    const detailDescription = page.locator('.description [data-description-display]').first();
    await expect(detailDescription).toHaveAttribute('title', 'Double-click to edit');
    await detailDescription.dblclick();
    await expect(page.locator('textarea[name="description"]')).toBeVisible();
  });
});

test.describe('WS14 finding 136: a saved plugin setting is visible on the page that saved it', () => {
  test('the injected output updates without a manual reload', async ({ page, apiClient }) => {
    await apiClient.savePluginSettings('test-banner', {
      api_key: 'ws14-key',
      banner_text: 'ws14 before',
      show_banner: true,
      mode: 'simple',
      count: 1,
    });
    await apiClient.enablePlugin('test-banner');

    await page.goto('/plugins/manage');
    const banner = page.getByTestId('plugin-banner');
    await expect(banner).toContainText('ws14 before');

    const form = page.getByTestId('plugin-settings-test-banner');
    await form.getByTestId('setting-banner_text').fill('ws14 after');
    await page.getByTestId('save-settings-test-banner').click();

    // The value the plugin injects is server-rendered, so only a fresh render can
    // show it. Before the fix the input said "ws14 after" and the banner still
    // said "ws14 before", which reads as a failed save.
    await expect(banner).toContainText('ws14 after', { timeout: 10_000 });
    await expect(form.getByTestId('setting-banner_text')).toHaveValue('ws14 after');

    await apiClient.disablePlugin('test-banner');
  });
});
