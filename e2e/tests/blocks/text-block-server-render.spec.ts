/**
 * Text blocks render on the server, like the note's Description
 * (docs/plans/2026-10-19-markdown-description-editor-design.md, decision 13):
 * full markdown, mentions and shortcodes against the note, on page load, after
 * leaving edit mode, and after the block list reloads.
 */
import type { Page } from '@playwright/test';
import { test, expect } from '../../fixtures/base.fixture';

function card(page: Page, blockId: number) {
  return page.locator(`[data-block-id="${blockId}"]`);
}
function rendered(page: Page, blockId: number) {
  return card(page, blockId).locator('[data-text-block-rendered]');
}

test.describe('Text blocks render on the server', () => {
  let categoryId: number;
  let ownerId: number;
  let noteId: number;
  let noteName: string;

  test.beforeAll(async ({ apiClient }) => {
    categoryId = (await apiClient.createCategory(`TB render ${Date.now()}`, '')).ID;
    ownerId = (await apiClient.createGroup({ name: `TB owner ${Date.now()}`, categoryId })).ID;
  });

  test.beforeEach(async ({ apiClient }) => {
    noteName = `TB Note ${Date.now()}`;
    noteId = (await apiClient.createNote({ name: noteName, ownerId })).ID;
  });

  test.afterEach(async ({ apiClient }) => {
    if (noteId) await apiClient.deleteNote(noteId);
  });

  test.afterAll(async ({ apiClient }) => {
    if (ownerId) await apiClient.deleteGroup(ownerId);
    if (categoryId) await apiClient.deleteCategory(categoryId);
  });

  test('on page load: headings, lists, mentions and shortcodes', async ({ page, apiClient }) => {
    const md = (await apiClient.createBlock(noteId, 'text', 'd', { text: '# Big Heading\n\n- one\n- two' })).id;
    const sc = (await apiClient.createBlock(noteId, 'text', 'h', {
      text: `[property path="Name"] and @[group:${ownerId}:Owner]`,
    })).id;
    await page.goto(`/note?id=${noteId}`);
    await expect(rendered(page, md).locator('h1')).toHaveText('Big Heading');
    await expect(rendered(page, md).locator('li')).toHaveCount(2);
    await expect(rendered(page, sc)).toContainText(noteName);
    await expect(rendered(page, sc).locator(`a[href*="/group?id=${ownerId}"]`)).toHaveCount(1);
  });

  test('after editing, Done shows the server rendering of the new text', async ({ page, apiClient }) => {
    const block = (await apiClient.createBlock(noteId, 'text', 'd', { text: 'plain' })).id;
    await page.goto(`/note?id=${noteId}`);
    await page.getByText('Edit Blocks').click();
    await card(page, block).locator('textarea').fill('## Now a heading');
    await page.getByRole('button', { name: 'Done' }).click();
    await expect(rendered(page, block).locator('h2')).toHaveText('Now a heading');
  });

  test('a block showing the Description updates when the first block changes it', async ({ page, apiClient }) => {
    const first = (await apiClient.createBlock(noteId, 'text', 'd', { text: 'old text' })).id;
    const second = (await apiClient.createBlock(noteId, 'text', 'h', { text: 'Desc: [property path="Description"]' })).id;
    await page.goto(`/note?id=${noteId}`);
    await expect(rendered(page, second)).toContainText('Desc: old text');
    await page.getByText('Edit Blocks').click();
    await card(page, first).locator('textarea').fill('new text');
    await page.getByRole('button', { name: 'Done' }).click();
    await expect(rendered(page, second)).toContainText('Desc: new text');
  });

  test('adding a block keeps the other blocks rendered', async ({ page, apiClient }) => {
    const block = (await apiClient.createBlock(noteId, 'text', 'd', { text: '**kept**' })).id;
    await page.goto(`/note?id=${noteId}`);
    await page.getByText('Edit Blocks').click();
    await page.locator('[data-testid="add-block-trigger"]').click();
    await page.locator('[role="option"][data-block-type="divider"]').click();
    await expect(page.locator('.block-editor [data-block-id]')).toHaveCount(2);
    await page.getByRole('button', { name: 'Done' }).click();
    await expect(rendered(page, block).locator('strong')).toHaveText('kept');
  });

  test('scripts in a block run, as they do in a Description', async ({ page, apiClient }) => {
    const block = (await apiClient.createBlock(noteId, 'text', 'd', {
      // The script on its own line is a raw HTML block; inline after other
      // markup, markdown would escape its quotes, in a Description too.
      text: '<span class="probe">loading</span>\n\n<script>document.querySelectorAll(".probe").forEach((e) => { e.textContent = "ready"; });</script>',
    })).id;
    await page.goto(`/note?id=${noteId}`);
    await expect(rendered(page, block).locator('.probe')).toHaveText('ready');
  });

  test('a failed render says so and shows the saved text', async ({ page, apiClient }) => {
    const block = (await apiClient.createBlock(noteId, 'text', 'd', { text: 'before' })).id;
    await page.goto(`/note?id=${noteId}`);
    await page.route(/\/v1\/note\/renderedTextBlocks/, (route) =>
      route.fulfill({ status: 500, contentType: 'text/plain', body: 'boom' }));
    await page.getByText('Edit Blocks').click();
    await card(page, block).locator('textarea').fill('after');
    await page.getByRole('button', { name: 'Done' }).click();
    await expect(page.locator('.block-editor [role="alert"]')).toContainText(/render/i);
    await expect(rendered(page, block)).toHaveText('after');
  });

  test('deleting a block outside edit mode refreshes the blocks that show the Description', async ({ page, apiClient }) => {
    const first = (await apiClient.createBlock(noteId, 'text', 'd', { text: 'first text' })).id;
    const second = (await apiClient.createBlock(noteId, 'text', 'h', { text: 'Desc: [property path="Description"]' })).id;
    await page.goto(`/note?id=${noteId}`);
    await expect(rendered(page, second)).toContainText('Desc: first text');
    // The window.mahBlock bridge and plugin code can delete without edit mode.
    await page.evaluate((id) => {
      const root = document.querySelector('.block-editor') as any;
      return (window as any).Alpine.$data(root).deleteBlock(id);
    }, first);
    await expect(rendered(page, second)).not.toContainText('first text');
  });
});
