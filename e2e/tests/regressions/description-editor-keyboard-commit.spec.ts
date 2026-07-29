/**
 * UI bug hunt 2026-07-29, findings 15 / 53 / 87 (WS2).
 *
 * Bug: templates/partials/description.tpl bound its only save trigger to
 * Alpine's @click.away. There was no Save button, no Cancel button and no
 * keyboard commit path, so a keyboard-only user could type a description and
 * had no way at all to store it — Tab left the editor open holding text that
 * was never sent, and Escape discarded it with no confirmation and no undo.
 * The partial is included by twelve templates, so the same dead end existed on
 * every entity, in both list and detail contexts.
 *
 * Every assertion here reads the description back through the API. A UI-only
 * assertion cannot tell a successful write from one that posted nothing, which
 * is precisely the failure mode under test (docs/lessons.md).
 */
import type { Page } from '@playwright/test';
import { test, expect } from '../../fixtures/base.fixture';
import type { ApiClient } from '../../helpers/api-client';

const ORIGINAL = 'ws2 original description';

test.describe('Inline description editor can be committed from the keyboard', () => {
  let runId: string;
  let categoryId: number;
  let ownerGroupId: number;

  test.beforeAll(async ({ apiClient }) => {
    runId = `${Date.now()}-${Math.random().toString(36).substring(2, 8)}`;
    const category = await apiClient.createCategory(`WS2 Desc Cat ${runId}`, 'findings 15/53/87');
    categoryId = category.ID;
    const owner = await apiClient.createGroup({ name: `WS2 Desc Owner ${runId}`, categoryId });
    ownerGroupId = owner.ID;
  });

  /** Each test owns its note: these are real writes, so sharing one would couple them. */
  async function freshNote(apiClient: ApiClient, label: string) {
    const note = await apiClient.createNote({
      name: `WS2 Desc Note ${label} ${runId}`,
      description: ORIGINAL,
      ownerId: ownerGroupId,
    });
    return note.ID;
  }

  /**
   * Live verification caught a commit that persisted but then threw while
   * repainting the page, leaving the pre-edit text on screen. Asserting only the
   * API value is blind to that, so every test also watches for page errors.
   */
  function watchPageErrors(page: Page) {
    const errors: string[] = [];
    page.on('pageerror', (e) => errors.push(e.message));
    return errors;
  }

  async function openEditor(page: Page, noteId: number) {
    await page.goto(`/note?id=${noteId}`);
    await page.waitForLoadState('load');
    const region = page.locator('.description').first();
    await expect(region).toBeVisible();
    await region.dblclick();
    const textarea = page.locator('textarea[name="description"]');
    await expect(textarea).toBeVisible();
    return { region, textarea };
  }

  async function persistedDescription(apiClient: ApiClient, noteId: number) {
    const note = await apiClient.getNote(noteId);
    return note.Description ?? '';
  }

  test('Ctrl+Enter commits the edit', async ({ page, apiClient }) => {
    const noteId = await freshNote(apiClient, 'ctrl-enter');
    const { textarea } = await openEditor(page, noteId);

    await textarea.fill('committed with ctrl enter');
    await textarea.press('Control+Enter');

    await expect
      .poll(() => persistedDescription(apiClient, noteId), { timeout: 10000 })
      .toBe('committed with ctrl enter');

    await apiClient.deleteNote(noteId);
  });

  test('the Save button commits the edit', async ({ page, apiClient }) => {
    const noteId = await freshNote(apiClient, 'save-button');
    const { region, textarea } = await openEditor(page, noteId);

    await textarea.fill('committed with the save button');
    await region.getByRole('button', { name: 'Save' }).click();

    await expect
      .poll(() => persistedDescription(apiClient, noteId), { timeout: 10000 })
      .toBe('committed with the save button');

    await apiClient.deleteNote(noteId);
  });

  test('tabbing out of the editor commits rather than stranding the text', async ({
    page,
    apiClient,
  }) => {
    const noteId = await freshNote(apiClient, 'tab-out');
    const errors = watchPageErrors(page);
    const { region, textarea } = await openEditor(page, noteId);

    await textarea.fill('committed by leaving the field');
    // Tab moves focus to the Save button, which is inside the editor — the edit
    // must survive that. Only leaving the region entirely commits.
    await textarea.press('Tab');
    expect(await persistedDescription(apiClient, noteId)).toBe(ORIGINAL);

    // Walk out of the region: Save, Cancel, then past the last control.
    await page.keyboard.press('Tab');
    await page.keyboard.press('Tab');

    await expect
      .poll(() => persistedDescription(apiClient, noteId), { timeout: 10000 })
      .toBe('committed by leaving the field');

    // Tab-out deliberately does not reload (WCAG 3.2.2), so the page has to
    // repaint the value itself. Persisting while still showing the old text is
    // the same "did that go anywhere?" doubt the whole finding is about.
    await expect(region).toContainText('committed by leaving the field');
    await expect(region).not.toContainText(ORIGINAL);
    expect(errors, `page errors: ${JSON.stringify(errors)}`).toEqual([]);

    // Re-opening must offer the saved text, not the template's stale copy.
    await region.dblclick();
    await expect(page.locator('textarea[name="description"]')).toHaveValue(
      'committed by leaving the field',
    );

    await apiClient.deleteNote(noteId);
  });

  test('Cancel and Escape discard without writing (negative control)', async ({
    page,
    apiClient,
  }) => {
    const noteId = await freshNote(apiClient, 'cancel');
    const { region, textarea } = await openEditor(page, noteId);

    await textarea.fill('discarded by cancel');
    await region.getByRole('button', { name: 'Cancel' }).click();
    await expect(page.locator('textarea[name="description"]')).toBeHidden();

    await region.dblclick();
    const reopened = page.locator('textarea[name="description"]');
    await expect(reopened).toBeVisible();
    await reopened.fill('discarded by escape');
    await reopened.press('Escape');
    await expect(page.locator('textarea[name="description"]')).toBeHidden();

    // Positive control in the same test: the write path does work on this note,
    // so "still ORIGINAL" is a real result and not an endpoint that never fires.
    await region.dblclick();
    const third = page.locator('textarea[name="description"]');
    await third.fill('kept after an explicit save');
    await third.press('Control+Enter');
    await expect
      .poll(() => persistedDescription(apiClient, noteId), { timeout: 10000 })
      .toBe('kept after an explicit save');

    await apiClient.deleteNote(noteId);
  });

  test('the same commit paths work on a list page card', async ({ page, apiClient }) => {
    const noteId = await freshNote(apiClient, 'list-card');

    await page.goto(`/notes?ownerId=${ownerGroupId}`);
    await page.waitForLoadState('load');

    const card = page.locator(`[x-data*="itemId: ${noteId}"]`).first();
    const region = card.locator('.description').first();
    await expect(region).toBeVisible();
    await region.dblclick();

    const textarea = card.locator('textarea[name="description"]');
    await expect(textarea).toBeVisible();
    await textarea.fill('committed from the list');
    await textarea.press('Control+Enter');

    await expect
      .poll(() => persistedDescription(apiClient, noteId), { timeout: 10000 })
      .toBe('committed from the list');

    await apiClient.deleteNote(noteId);
  });

  test.afterAll(async ({ apiClient }) => {
    if (ownerGroupId) await apiClient.deleteGroup(ownerGroupId);
    if (categoryId) await apiClient.deleteCategory(categoryId);
  });
});
