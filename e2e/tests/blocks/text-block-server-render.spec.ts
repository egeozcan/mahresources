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

  test('Alpine markup in a block works; a <script> in a block does not run', async ({ page, apiClient }) => {
    const block = (await apiClient.createBlock(noteId, 'text', 'd', {
      text: '<div x-data="{ n: 0 }"><button type="button" class="counter" @click="n++" x-text="n">initial</button></div>\n\n<script>window.__blockScriptRan = true;</script>',
    })).id;
    await page.goto(`/note?id=${noteId}`);
    const counter = rendered(page, block).locator('button.counter');
    await expect(counter).toHaveText('0');
    await counter.click();
    await expect(counter).toHaveText('1');
    // Re-rendering in place (Done) would re-run scripts on every refresh, so a
    // block runs none; the design doc records this difference from a Description.
    expect(await page.evaluate(() => (window as any).__blockScriptRan)).toBeUndefined();
    await page.getByText('Edit Blocks').click();
    await page.getByRole('button', { name: 'Done' }).click();
    await expect(counter).toHaveText('0');
    await counter.click();
    await expect(counter).toHaveText('1');
  });

  test('a failed render says so and shows the saved text; a later render clears it', async ({ page, apiClient }) => {
    const block = (await apiClient.createBlock(noteId, 'text', 'd', { text: 'before' })).id;
    await page.goto(`/note?id=${noteId}`);
    await page.route(/\/v1\/note\/renderedTextBlocks/, (route) =>
      route.fulfill({ status: 500, contentType: 'text/plain', body: 'boom' }));
    await page.getByText('Edit Blocks').click();
    await card(page, block).locator('textarea').fill('**after**');
    await page.getByRole('button', { name: 'Done' }).click();
    await expect(page.locator('.block-editor [role="alert"]')).toContainText(/render/i);
    await expect(rendered(page, block)).toHaveText('**after**');

    await page.unroute(/\/v1\/note\/renderedTextBlocks/);
    await page.getByText('Edit Blocks').click();
    await page.getByRole('button', { name: 'Done' }).click();
    await expect(rendered(page, block).locator('strong')).toHaveText('after');
    await expect(page.locator('.block-editor [role="alert"]')).toBeHidden();
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

  test('a refresh leaves an unchanged block alone, so a widget draft in it survives', async ({ page, apiClient }) => {
    const block = (await apiClient.createBlock(noteId, 'text', 'd', {
      text: '<div x-data="{ draft: \'\' }"><input class="draft" x-model="draft"></div>',
    })).id;
    const other = (await apiClient.createBlock(noteId, 'text', 'h', { text: 'other' })).id;
    await page.goto(`/note?id=${noteId}`);
    let release!: () => void;
    const held = new Promise<void>((r) => { release = r; });
    await page.route(/\/v1\/note\/renderedTextBlocks/, async (route) => {
      await held;
      await route.continue();
    });
    await page.getByText('Edit Blocks').click();
    await card(page, other).locator('textarea').fill('other changed');
    await page.getByRole('button', { name: 'Done' }).click();
    const draft = rendered(page, block).locator('input.draft');
    await draft.fill('my draft');
    release();
    await expect(rendered(page, other)).toHaveText('other changed');
    await expect(draft).toHaveValue('my draft');
    await expect(draft).toBeFocused();
  });

  for (const outcome of ['a fresh token', 'a failed render'] as const) {
    test(`${outcome} does not replace the block someone is typing in`, async ({ page, apiClient }) => {
      // [reload] mints a new token on every render, so this block's HTML
      // changes on each refresh even though its text does not.
      const block = (await apiClient.createBlock(noteId, 'text', 'd', {
        text: '<div x-data="{ draft: \'\' }"><input class="draft" x-model="draft"></div>\n\n[reload label="Refresh"]',
      })).id;
      const other = (await apiClient.createBlock(noteId, 'text', 'h', { text: 'other' })).id;
      await page.goto(`/note?id=${noteId}`);
      let release!: () => void;
      const held = new Promise<void>((r) => { release = r; });
      await page.route(/\/v1\/note\/renderedTextBlocks/, async (route) => {
        await held;
        if (outcome === 'a failed render') {
          await route.fulfill({ status: 500, contentType: 'text/plain', body: 'boom' });
        } else {
          await route.continue();
        }
      });
      await page.getByText('Edit Blocks').click();
      await card(page, other).locator('textarea').fill('other changed');
      await page.getByRole('button', { name: 'Done' }).click();
      const draft = rendered(page, block).locator('input.draft');
      await draft.fill('my draft');
      release();
      await expect(rendered(page, other)).toHaveText('other changed');
      await expect(draft).toHaveValue('my draft');
      await expect(draft).toBeFocused();
      // Held, not dropped: once focus leaves the block, the update lands.
      await page.locator('h1').first().click();
      if (outcome === 'a failed render') {
        await expect(rendered(page, block)).toContainText('[reload label="Refresh"]');
      } else {
        await expect(rendered(page, block).locator('input.draft')).toHaveValue('');
      }
    });
  }
});
