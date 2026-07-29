/**
 * UI bug hunt 2026-07-29, findings 16 / 92 (tag merge) and 56 / 91 (Add Tags).
 *
 * Bug: both are plain HTML form POSTs to `/v1/…` carrying a `?redirect=`, so
 * submitting one with nothing selected navigated the whole page to the API URL
 * and rendered an error. The merge form was worse: it fired
 * `confirm('Selected tags will be deleted and merged to X. Are you sure?')`
 * first, so the reader accepted a destructive-sounding prompt and was then
 * ejected from the app with the message "one or more losers required".
 *
 * The server-rendered half (the guard is declared, the submit is bound to the
 * selection, the message no longer says "losers") is covered by
 * server/api_tests/ws3_error_surface_test.go. This spec covers the behaviour:
 * the button is unavailable, no dialog fires, the page does not move, and the
 * happy path still works.
 */
import type { Page } from '@playwright/test';
import { test, expect } from '../../fixtures/base.fixture';

/** Fails the test on any uncaught page error, per docs/lessons.md. */
function failOnPageError(page: Page, sink: string[]) {
  page.on('pageerror', (error) => sink.push(error.message));
}

test.describe('An empty selection cannot submit a merge or an Add Tags', () => {
  let runId: string;
  let categoryId: number;

  test.beforeEach(async ({ apiClient }) => {
    runId = `${Date.now()}-${Math.random().toString(36).substring(2, 8)}`;
    categoryId = (await apiClient.createCategory(`ws3 cat ${runId}`)).ID;
  });

  test.afterEach(async ({ apiClient }) => {
    if (categoryId) await apiClient.deleteCategory(categoryId);
  });

  test('the tag Merge button is unavailable, fires no confirm, and stays put', async ({
    page,
    apiClient,
  }) => {
    const errors: string[] = [];
    failOnPageError(page, errors);

    const winner = await apiClient.createTag(`ws3 merge winner ${runId}`);
    const dialogs: string[] = [];
    page.on('dialog', async (dialog) => {
      dialogs.push(dialog.message());
      await dialog.dismiss();
    });

    try {
      await page.goto(`/tag?id=${winner.ID}`);
      await page.waitForLoadState('load');

      const mergeButton = page.getByRole('button', { name: 'Merge' });
      await expect(mergeButton).toBeVisible();
      await expect(mergeButton).toBeDisabled();
      await expect(page.getByText('Choose at least one tag to merge into this one.')).toBeVisible();

      // The keyboard path bypasses the button: the selector calls
      // form.requestSubmit() on Enter, which a disabled button does not stop.
      // Located through the field rather than through action=, because Alpine's
      // :action binding rewrites that attribute to the ?redirect= form at init.
      const form = page.locator('form:has([data-selector-field="losers"])');
      await form.evaluate((el: HTMLFormElement) => el.requestSubmit());
      await page.waitForTimeout(300);

      expect(dialogs).toEqual([]);
      expect(page.url()).toContain(`/tag?id=${winner.ID}`);
      expect(errors).toEqual([]);
    } finally {
      await apiClient.deleteTag(winner.ID);
    }
  });

  test('the tag Merge button still works once something is chosen', async ({
    page,
    apiClient,
  }) => {
    const errors: string[] = [];
    failOnPageError(page, errors);

    const winner = await apiClient.createTag(`ws3 keep ${runId}`);
    const loser = await apiClient.createTag(`ws3 gone ${runId}`);
    page.on('dialog', (dialog) => dialog.accept());

    await page.goto(`/tag?id=${winner.ID}`);
    await page.waitForLoadState('load');

    const field = page.locator('[data-selector-field="losers"] input[role="combobox"]');
    await field.fill(`ws3 gone ${runId}`);
    await page.locator(`[role="option"]:has-text("ws3 gone ${runId}")`).first().click();

    const mergeButton = page.getByRole('button', { name: 'Merge' });
    await expect(mergeButton).toBeEnabled();
    await mergeButton.click();
    await page.waitForLoadState('load');

    // Assert the persisted state, not the page: the loser must be gone.
    await expect
      .poll(async () => {
        const tags = await apiClient.getTags();
        return tags.some((t: { ID: number }) => t.ID === loser.ID);
      })
      .toBe(false);
    expect(errors).toEqual([]);

    await apiClient.deleteTag(winner.ID);
  });

  test('the group Add Tags button is unavailable while nothing is chosen', async ({
    page,
    apiClient,
  }) => {
    const errors: string[] = [];
    failOnPageError(page, errors);

    const group = await apiClient.createGroup({ name: `ws3 addtags ${runId}`, categoryId });

    try {
      await page.goto(`/group?id=${group.ID}`);
      await page.waitForLoadState('load');

      const addTags = page.getByRole('button', { name: 'Add Tags' });
      await expect(addTags).toBeVisible();
      await expect(addTags).toBeDisabled();
      await expect(page.getByText('Choose at least one tag first.')).toBeVisible();

      const form = page.locator('form:has(button:text("Add Tags"))');
      await form.evaluate((el: HTMLFormElement) => el.requestSubmit());
      await page.waitForTimeout(300);

      expect(page.url()).toContain(`/group?id=${group.ID}`);
      expect(errors).toEqual([]);
    } finally {
      await apiClient.deleteGroup(group.ID);
    }
  });

  test('the group Add Tags button still adds a tag once one is chosen', async ({
    page,
    apiClient,
  }) => {
    const errors: string[] = [];
    failOnPageError(page, errors);

    const group = await apiClient.createGroup({ name: `ws3 addtags ok ${runId}`, categoryId });
    const tag = await apiClient.createTag(`ws3 attach ${runId}`);

    try {
      await page.goto(`/group?id=${group.ID}`);
      await page.waitForLoadState('load');

      const field = page.locator('[data-selector-field="editedId"] input[role="combobox"]');
      await field.fill(`ws3 attach ${runId}`);
      await page.locator(`[role="option"]:has-text("ws3 attach ${runId}")`).first().click();

      const addTags = page.getByRole('button', { name: 'Add Tags' });
      await expect(addTags).toBeEnabled();
      await addTags.click();
      await page.waitForLoadState('load');

      // Persisted state, not the rendered chip.
      await expect
        .poll(async () => {
          const fresh = (await apiClient.getGroup(group.ID)) as unknown as {
            Tags?: { ID: number }[];
          };
          return (fresh.Tags ?? []).some((t) => t.ID === tag.ID);
        })
        .toBe(true);
      expect(errors).toEqual([]);
    } finally {
      await apiClient.deleteGroup(group.ID);
      await apiClient.deleteTag(tag.ID);
    }
  });
});
