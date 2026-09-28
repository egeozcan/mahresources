/**
 * UI bug hunt 2026-07-29, findings 16 / 92 (tag merge) and 56 / 91 (Add Tags).
 *
 * Bug: both are plain HTML form POSTs to `/v1/…` carrying a `?redirect=`, so
 * submitting one with nothing selected navigated the whole page to the API URL
 * and rendered an error. The merge form was worse: it raised
 * 'Selected tags will be deleted and merged to X. Are you sure?' first, so the
 * reader accepted a destructive-sounding prompt and was then ejected from the
 * app with the message "one or more losers required".
 *
 * The server-rendered half (the merge guard is declared, its submit is bound to
 * the selection, the message no longer says "losers") is covered by
 * server/api_tests/ws3_error_surface_test.go. This spec covers the behaviour:
 * the merge button is unavailable, no confirmation fires, the page does not move,
 * the happy path still works, and the detail tag editor — no form, no submit
 * button, persisted as each tag is chosen — neither reloads the page nor offers
 * a tag the entity already has. That confirmation is the in-app
 * `role="alertdialog"` rather than `window.confirm`, so it is driven through
 * e2e/helpers/confirm-dialog.ts and asserted as ordinary DOM.
 */
import type { Page } from '@playwright/test';
import { test, expect } from '../../fixtures/base.fixture';
import { acceptConfirm, expectNoConfirm } from '../../helpers/confirm-dialog';

/** Fails the test on any uncaught page error, per docs/lessons.md. */
function failOnPageError(page: Page, sink: string[]) {
  page.on('pageerror', (error) => sink.push(error.message));
}

test.describe('Merge guard and the in-place tag editor', () => {
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

    try {
      await page.goto(`/tag?id=${winner.ID}`);
      await page.waitForLoadState('load');

      const mergeButton = page.getByRole('button', { name: 'Merge', exact: true });
      await expect(mergeButton).toBeVisible();
      await expect(mergeButton).toBeDisabled();
      await expect(page.getByText('Choose at least one tag to merge into this one.')).toBeVisible();

      // The keyboard path bypasses the button: the selector calls
      // form.requestSubmit() on Enter, which a disabled button does not stop.
      // Located through the field rather than through action=, because Alpine's
      // :action binding rewrites that attribute to the ?redirect= form at init.
      const form = page.locator('form:has([data-selector-field="losers"])');
      await form.evaluate((el: HTMLFormElement) => el.requestSubmit());
      // The window in which a confirm would have opened, or a navigation started.
      // The guard runs before the confirm in confirmAction's submit handler, so
      // both must still be absent when it closes.
      await page.waitForTimeout(300);

      await expectNoConfirm(page);
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

    await page.goto(`/tag?id=${winner.ID}`);
    await page.waitForLoadState('load');

    const field = page.locator('[data-selector-field="losers"] input[role="combobox"]');
    await field.fill(`ws3 gone ${runId}`);
    await page.locator(`[role="option"]:has-text("ws3 gone ${runId}")`).first().click();

    const mergeButton = page.getByRole('button', { name: 'Merge', exact: true });
    await expect(mergeButton).toBeEnabled();
    await mergeButton.click();

    // The click only opens the confirm; the POST is re-issued from the dialog's
    // Confirm, so the load wait has to come after the answer, not after the click.
    const message = await acceptConfirm(page);
    // The confirm has to say what it is about to destroy and where it goes —
    // findings 78 and 153 were confirmations that could not. Nothing else checks
    // this wording: ws3_error_surface_test.go asserts the `requireSelection`
    // declaration is rendered, not what the reader is asked.
    expect(message).toContain('deleted');
    expect(message).toContain(`ws3 keep ${runId}`);
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

  test('a tag already on the entity is not offered by the editor', async ({
    page,
    apiClient,
  }) => {
    const errors: string[] = [];
    failOnPageError(page, errors);

    const group = await apiClient.createGroup({ name: `ws3 filter ${runId}`, categoryId });
    const attached = await apiClient.createTag(`ws3 attached ${runId}`, 'already on the group');
    await apiClient.addTagsToGroups([group.ID], [attached.ID]);

    try {
      await page.goto(`/group?id=${group.ID}`);
      await page.waitForLoadState('load');

      const field = page.locator('[data-selector-field="editedId"] input[role="combobox"]');
      const search = page.waitForResponse((r) => r.url().includes('/v1/tags/suggest'));
      await field.fill(attached.Name);
      const suggestions = (await (await search).json()) as { ID: number }[];

      // Positive control: the server did return the tag (it is the only one
      // matching); the selector is what keeps an already-attached tag out of the
      // list — and seeding it with the entity's tags is what makes that happen.
      // Without the seed it is offered back as a result, and as a create row for
      // the very name the entity already has.
      expect(suggestions.some((t) => t.ID === attached.ID)).toBe(true);
      await expect(page.getByRole('option', { name: attached.Name, exact: true })).toHaveCount(0);
      await expect(
        page.getByRole('option', { name: `Create "${attached.Name}"`, exact: true }),
      ).toHaveCount(0);
      expect(errors).toEqual([]);
    } finally {
      await apiClient.deleteGroup(group.ID);
      await apiClient.deleteTag(attached.ID);
    }
  });

  test('choosing a tag adds it in place, with no submit button and no reload', async ({
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

      // The explicit button, and the form it lived in, are gone.
      await expect(page.locator('form[action*="addTags"]')).toHaveCount(0);
      await expect(page.getByRole('button', { name: 'Add Tags' })).toHaveCount(0);

      // A page-lifetime marker survives only if the add does not reload the page.
      await page.evaluate(() => {
        (window as unknown as { __inPlace?: boolean }).__inPlace = true;
      });

      const field = page.locator('[data-selector-field="editedId"] input[role="combobox"]');
      await field.fill(`ws3 attach ${runId}`);
      const added = page.waitForResponse(
        (r) => r.url().includes('/v1/groups/addTags') && r.request().method() === 'POST',
      );
      await page.locator(`[role="option"]:has-text("ws3 attach ${runId}")`).first().click();
      await added;

      await expect
        .poll(async () => {
          const fresh = (await apiClient.getGroup(group.ID)) as unknown as {
            Tags?: { ID: number }[];
          };
          return (fresh.Tags ?? []).some((t) => t.ID === tag.ID);
        })
        .toBe(true);

      expect(
        await page.evaluate(() => (window as unknown as { __inPlace?: boolean }).__inPlace),
      ).toBe(true);
      expect(errors).toEqual([]);
    } finally {
      await apiClient.deleteGroup(group.ID);
      await apiClient.deleteTag(tag.ID);
    }
  });

  test('creating a new tag from the sidebar adds it in place', async ({ page, apiClient }) => {
    const errors: string[] = [];
    failOnPageError(page, errors);

    const group = await apiClient.createGroup({ name: `ws3 createtag ${runId}`, categoryId });
    const tagName = `ws3 newtag ${runId}`;
    let createdId: number | undefined;

    try {
      await page.goto(`/group?id=${group.ID}`);
      await page.waitForLoadState('load');

      await page.evaluate(() => {
        (window as unknown as { __inPlace?: boolean }).__inPlace = true;
      });

      const field = page.locator('[data-selector-field="editedId"] input[role="combobox"]');
      await field.fill(tagName);
      const createRow = page.getByRole('option', { name: `Create "${tagName}"`, exact: true });
      await expect(createRow).toBeVisible();

      // Choosing the virtual create row creates the tag and persists the
      // association in one step, in place.
      const tagCreated = page.waitForResponse(
        (r) => new URL(r.url()).pathname === '/v1/tag' && r.request().method() === 'POST',
      );
      const added = page.waitForResponse(
        (r) => r.url().includes('/v1/groups/addTags') && r.request().method() === 'POST',
      );
      await createRow.click();
      await tagCreated;
      await added;

      await expect
        .poll(async () => {
          const fresh = (await apiClient.getGroup(group.ID)) as unknown as {
            Tags?: { Name: string }[];
          };
          return (fresh.Tags ?? []).some((t) => t.Name === tagName);
        })
        .toBe(true);
      expect(
        await page.evaluate(() => (window as unknown as { __inPlace?: boolean }).__inPlace),
      ).toBe(true);
      createdId = (await apiClient.getTags()).find((t) => t.Name === tagName)?.ID;
      expect(errors).toEqual([]);
    } finally {
      await apiClient.deleteGroup(group.ID);
      if (createdId) await apiClient.deleteTag(createdId);
    }
  });

  test('removing a tag from a chip persists it, and the chip links to the tag', async ({
    page,
    apiClient,
  }) => {
    const errors: string[] = [];
    failOnPageError(page, errors);

    const group = await apiClient.createGroup({ name: `ws3 remove ${runId}`, categoryId });
    const tag = await apiClient.createTag(`ws3 removable ${runId}`);
    await apiClient.addTagsToGroups([group.ID], [tag.ID]);

    try {
      await page.goto(`/group?id=${group.ID}`);
      await page.waitForLoadState('load');

      // The chip is still a link to the tag's own page...
      const tagEditor = page.locator('[data-selector-field="editedId"]');
      const chip = tagEditor.getByRole('link', { name: tag.Name, exact: true });
      await expect(chip).toHaveAttribute('href', `/tag?id=${tag.ID}`);

      // ...with a remove control that persists the removal in place.
      const removed = page.waitForResponse(
        (r) => r.url().includes('/v1/groups/removeTags') && r.request().method() === 'POST',
      );
      await page.getByRole('button', { name: `Remove ${tag.Name}`, exact: true }).click();
      await removed;

      await expect
        .poll(async () => {
          const fresh = (await apiClient.getGroup(group.ID)) as unknown as {
            Tags?: { ID: number }[];
          };
          return (fresh.Tags ?? []).some((t) => t.ID === tag.ID);
        })
        .toBe(false);
      await expect(chip).toHaveCount(0);
      expect(errors).toEqual([]);
    } finally {
      await apiClient.deleteGroup(group.ID);
      await apiClient.deleteTag(tag.ID);
    }
  });

  test('a failed add rolls the chip back and announces it', async ({ page, apiClient }) => {
    const errors: string[] = [];
    failOnPageError(page, errors);

    const group = await apiClient.createGroup({ name: `ws3 fail ${runId}`, categoryId });
    const tag = await apiClient.createTag(`ws3 failing ${runId}`);

    try {
      await page.goto(`/group?id=${group.ID}`);
      await page.waitForLoadState('load');

      await page.route('**/v1/groups/addTags', (route) =>
        route.fulfill({ status: 500, contentType: 'application/json', body: '{"error":"nope"}' }),
      );

      const field = page.locator('[data-selector-field="editedId"] input[role="combobox"]');
      await field.fill(tag.Name);
      await page.locator(`[role="option"]:has-text("${tag.Name}")`).first().click();

      // The optimistic chip is rolled back, and the reader is told rather than
      // being left with the live region's optimistic "Added".
      await expect(page.getByRole('link', { name: tag.Name, exact: true })).toHaveCount(0);
      await expect(
        page.locator('[data-selector-field="editedId"]').locator('[role="status"]'),
      ).toContainText(/could not update tags/i, { timeout: 5000 });
      expect(errors).toEqual([]);

      await page.unroute('**/v1/groups/addTags');
    } finally {
      await apiClient.deleteGroup(group.ID);
      await apiClient.deleteTag(tag.ID);
    }
  });
});
