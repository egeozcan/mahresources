/**
 * UI bug hunt 2026-07-29, findings 21 / 88 / 152 (WS2).
 *
 * Bug: when an inline rename was rejected, src/webcomponents/inlineedit.js:247
 * announced "Could not save <label>" through window.mahAnnounce only. That
 * region is created by src/utils/ariaLiveRegion.js:12-22 at width:1px,
 * height:1px, clip:rect(0,0,0,0) — so the sole signal a sighted user got was a
 * one-second #fee2e2 flash, and the editor had already closed and thrown away
 * what they typed. The component also discarded the server's explanation
 * (finding 152): the API answers with a usable reason and the UI said
 * "Could not save name".
 *
 * The server half is covered by
 * server/api_tests/edit_name_error_messages_test.go, which makes the message
 * worth showing; this spec covers showing it.
 */
import type { Page } from '@playwright/test';
import { test, expect } from '../../fixtures/base.fixture';
import type { ApiClient } from '../../helpers/api-client';

test.describe('A rejected inline rename is visible, keeps the text, and says why', () => {
  let runId: string;
  let occupiedName: string;
  let occupiedTagId: number;
  let victimTagId: number;
  let victimName: string;

  test.beforeEach(async ({ apiClient }) => {
    runId = `${Date.now()}-${Math.random().toString(36).substring(2, 8)}`;
    occupiedName = `ws2 taken ${runId}`;
    victimName = `ws2 victim ${runId}`;
    occupiedTagId = (await apiClient.createTag(occupiedName, 'finding 152')).ID;
    victimTagId = (await apiClient.createTag(victimName, 'finding 152')).ID;
  });

  test.afterEach(async ({ apiClient }) => {
    if (victimTagId) await apiClient.deleteTag(victimTagId);
    if (occupiedTagId) await apiClient.deleteTag(occupiedTagId);
  });

  async function currentName(apiClient: ApiClient, id: number) {
    const tags = await apiClient.getTags();
    return tags.find((t: { ID: number }) => t.ID === id)?.Name ?? '';
  }

  async function openRenameEditor(page: Page, tagId: number) {
    await page.goto(`/tag?id=${tagId}`);
    await page.waitForLoadState('load');
    const inlineEdit = page.locator('h1 inline-edit').first();
    await expect(inlineEdit).toBeVisible();
    await inlineEdit.locator('button.edit-button').click();
    const input = inlineEdit.locator('input');
    await expect(input).toBeFocused();
    return { inlineEdit, input };
  }

  test('a duplicate name shows the server reason and keeps the editor open', async ({
    page,
    apiClient,
  }) => {
    const { inlineEdit, input } = await openRenameEditor(page, victimTagId);

    await input.fill(occupiedName);
    await input.press('Enter');

    const error = inlineEdit.locator('[data-inline-edit-error]');
    await expect(error).toBeVisible();
    await expect(error).toContainText('already exists');
    await expect(error).toContainText(occupiedName);

    // The editor must still be open holding what the user typed — reverting to
    // the old name is what made the failure invisible.
    await expect(inlineEdit.locator('input')).toBeVisible();
    await expect(inlineEdit.locator('input')).toHaveValue(occupiedName);

    // And nothing was written.
    expect(await currentName(apiClient, victimTagId)).toBe(victimName);
  });

  test('an empty name shows the server reason', async ({ page, apiClient }) => {
    const { inlineEdit, input } = await openRenameEditor(page, victimTagId);

    await input.fill('   ');
    await input.press('Enter');

    const error = inlineEdit.locator('[data-inline-edit-error]');
    await expect(error).toBeVisible();
    await expect(error).toContainText('must not be empty');

    expect(await currentName(apiClient, victimTagId)).toBe(victimName);
  });

  test('correcting a rejected name clears the error', async ({ page, apiClient }) => {
    const { inlineEdit, input } = await openRenameEditor(page, victimTagId);

    await input.fill(occupiedName);
    await input.press('Enter');
    await expect(inlineEdit.locator('[data-inline-edit-error]')).toBeVisible();

    const corrected = `ws2 corrected ${runId}`;
    await inlineEdit.locator('input').fill(corrected);
    await inlineEdit.locator('input').press('Enter');

    await expect
      .poll(() => currentName(apiClient, victimTagId), { timeout: 10000 })
      .toBe(corrected);

    // A stale message under a successfully renamed entity reads as a failure.
    await expect(inlineEdit.locator('[data-inline-edit-error]')).toHaveCount(0);
    await expect(inlineEdit.locator('[aria-invalid="true"]')).toHaveCount(0);
  });

  test('a valid rename still succeeds and shows no error (control)', async ({
    page,
    apiClient,
  }) => {
    const { inlineEdit, input } = await openRenameEditor(page, victimTagId);
    const accepted = `ws2 accepted ${runId}`;

    await input.fill(accepted);
    await input.press('Enter');

    await expect
      .poll(() => currentName(apiClient, victimTagId), { timeout: 10000 })
      .toBe(accepted);

    await expect(inlineEdit.locator('[data-inline-edit-error]')).toHaveCount(0);
    await expect(inlineEdit).toContainText(accepted);
  });
});
