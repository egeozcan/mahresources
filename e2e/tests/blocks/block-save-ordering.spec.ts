/**
 * Block content saves and block-list reloads must land in the order the user
 * made them (docs/plans/2026-10-19-markdown-description-editor-design.md,
 * decision 13).
 *
 * Each test holds one request with page.route so a response arrives after a
 * later one, the interleaving the editor has to survive:
 *  - two content saves for the same block, the older one slower;
 *  - a save that fails while the user clicks Done;
 *  - a block-list reload whose snapshot predates a save that then succeeds;
 *  - a block-list reload whose snapshot predates a delete that then succeeds.
 */
import type { Page, Route } from '@playwright/test';
import { test, expect } from '../../fixtures/base.fixture';
import { acceptConfirm } from '../../helpers/confirm-dialog';

const delay = (ms: number) => new Promise((r) => setTimeout(r, ms));

function deferred() {
  let resolve!: () => void;
  const promise = new Promise<void>((r) => { resolve = r; });
  return { promise, resolve };
}

function card(page: Page, blockId: number) {
  return page.locator(`[data-block-id="${blockId}"]`);
}

async function blurEditor(page: Page) {
  await page.locator('h1').first().click();
}

test.describe('Block save ordering', () => {
  let categoryId: number;
  let ownerGroupId: number;
  let noteId: number;
  let blockA: number;
  let blockB: number;
  let blockC: number;

  test.beforeAll(async ({ apiClient }) => {
    const category = await apiClient.createCategory('Block Order Category', 'block save ordering');
    categoryId = category.ID;
    const owner = await apiClient.createGroup({ name: 'Block Order Owner', categoryId });
    ownerGroupId = owner.ID;
  });

  test.beforeEach(async ({ apiClient }) => {
    const note = await apiClient.createNote({ name: 'Block Order Note', ownerId: ownerGroupId });
    noteId = note.ID;
    blockA = (await apiClient.createBlock(noteId, 'text', 'd', { text: 'old A' })).id;
    blockB = (await apiClient.createBlock(noteId, 'text', 'h', { text: 'old B' })).id;
    blockC = (await apiClient.createBlock(noteId, 'text', 'p', { text: 'old C' })).id;
  });

  test.afterEach(async ({ apiClient }) => {
    if (noteId) await apiClient.deleteNote(noteId);
  });

  test.afterAll(async ({ apiClient }) => {
    if (ownerGroupId) await apiClient.deleteGroup(ownerGroupId);
    if (categoryId) await apiClient.deleteCategory(categoryId);
  });

  test('a slow older save does not overwrite a newer one', async ({ page, apiClient }) => {
    const firstPutDone = deferred();
    let puts = 0;
    await page.route(/\/v1\/note\/block\?/, async (route: Route) => {
      if (route.request().method() !== 'PUT') return route.fallback();
      puts++;
      if (puts === 1) {
        await delay(1500);
        const resp = await route.fetch();
        await route.fulfill({ response: resp });
        firstPutDone.resolve();
        return;
      }
      await route.fallback();
    });

    await page.goto(`/note?id=${noteId}`);
    await page.getByText('Edit Blocks').click();
    const textarea = card(page, blockA).locator('textarea');
    await textarea.fill('first');
    await blurEditor(page);
    await textarea.fill('first second');
    await blurEditor(page);

    await firstPutDone.promise;
    await expect
      .poll(async () => (await apiClient.getBlock(blockA)).content.text, { timeout: 5000 })
      .toBe('first second');
    // Nothing older lands afterwards.
    await delay(1000);
    expect((await apiClient.getBlock(blockA)).content.text).toBe('first second');
  });

  test('a failed save keeps the note in edit mode with the error shown', async ({ page, apiClient }) => {
    await page.route(/\/v1\/note\/block\?/, async (route: Route) => {
      if (route.request().method() !== 'PUT') return route.fallback();
      await route.fulfill({ status: 500, contentType: 'application/json', body: JSON.stringify({ error: 'disk full' }) });
    });

    await page.goto(`/note?id=${noteId}`);
    await page.getByText('Edit Blocks').click();
    const textarea = card(page, blockA).locator('textarea');
    await textarea.fill('unsaved text');
    await page.getByRole('button', { name: 'Done' }).click();

    await expect(page.locator('.block-editor [role="alert"]')).toContainText('disk full');
    await expect(page.getByRole('button', { name: 'Done' })).toBeVisible();
    await expect(textarea).toHaveValue('unsaved text');
    expect((await apiClient.getBlock(blockA)).content.text).toBe('old A');
  });

  test('a reload that snapshotted before a save does not bring back the old text', async ({ page, apiClient }) => {
    const putDone = deferred();
    const getFulfilled = deferred();
    let heldGet = false;
    await page.route(/\/v1\/note\/block\?/, async (route: Route) => {
      if (route.request().method() !== 'PUT') return route.fallback();
      await delay(300);
      const resp = await route.fetch();
      await route.fulfill({ response: resp });
      putDone.resolve();
    });
    await page.route(/\/v1\/note\/blocks\?/, async (route: Route) => {
      if (heldGet) return route.fallback();
      heldGet = true;
      const resp = await route.fetch(); // snapshot taken now, before the PUT lands
      await putDone.promise;
      // Past any trailing debounced PUT too, so nothing repairs the text after.
      await delay(1200);
      await route.fulfill({ response: resp });
      getFulfilled.resolve();
    });

    await page.goto(`/note?id=${noteId}`);
    await page.getByText('Edit Blocks').click();
    await card(page, blockA).locator('textarea').fill('new A');
    // Clicking Move down blurs the textarea (save) and reorders (reload).
    await card(page, blockA).locator('[data-block-control="move-down"]').click();

    await getFulfilled.promise;
    await expect.poll(async () => (await apiClient.getBlock(blockA)).content.text).toBe('new A');
    await expect(page.getByText('Loading blocks...')).toBeHidden();

    // Re-entering the editor and blurring must not write the stale text back.
    await page.getByRole('button', { name: 'Done' }).click();
    await expect(card(page, blockA).locator('.block-content .prose')).toHaveText('new A');
    await page.getByText('Edit Blocks').click();
    await card(page, blockA).locator('textarea').focus();
    await blurEditor(page);
    await delay(800);
    expect((await apiClient.getBlock(blockA)).content.text).toBe('new A');
  });

  test('a reload that snapshotted before a delete does not bring the block back', async ({ page, apiClient }) => {
    const deleteDone = deferred();
    let heldGet = false;
    await page.route(/\/v1\/note\/block\?/, async (route: Route) => {
      if (route.request().method() !== 'DELETE') return route.fallback();
      await delay(500);
      const resp = await route.fetch();
      await route.fulfill({ response: resp });
      deleteDone.resolve();
    });
    await page.route(/\/v1\/note\/blocks\?/, async (route: Route) => {
      if (heldGet) return route.fallback();
      heldGet = true;
      const resp = await route.fetch(); // still lists block C
      await deleteDone.promise;
      await delay(200);
      await route.fulfill({ response: resp });
    });

    await page.goto(`/note?id=${noteId}`);
    await page.getByText('Edit Blocks').click();
    await card(page, blockC).locator('[data-block-control="delete"]').click();
    await acceptConfirm(page);
    // The DELETE is still in flight; reorder A, which reloads the list.
    await card(page, blockA).locator('[data-block-control="move-down"]').click();

    await deleteDone.promise;
    await expect.poll(async () => (await apiClient.getBlocks(noteId)).map((b) => b.id).sort())
      .toEqual([blockA, blockB].sort());
    await delay(800);
    await expect(card(page, blockC)).toHaveCount(0);
    await expect(page.locator('.block-editor [data-block-id]')).toHaveCount(2);
  });
});
