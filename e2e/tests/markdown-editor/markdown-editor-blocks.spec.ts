/**
 * The markdown editor in a note's text blocks
 * (docs/plans/2026-10-19-markdown-description-editor-design.md, decision 14,
 * slice 6): toolbar, @-mentions and shortcode completion, no Write/Preview
 * toggle. Autosave stays; the immediate save fires when focus leaves the
 * block's editor, not when it moves into the toolbar or a completion popup.
 */
import type { Page } from '@playwright/test';
import { test, expect } from '../../fixtures/base.fixture';

const mod = 'ControlOrMeta';

function card(page: Page, blockId: number) {
  return page.locator(`[data-block-id="${blockId}"]`);
}
function editor(page: Page, blockId: number) {
  return card(page, blockId).locator('.cm-content');
}

test.describe('Markdown editor in text blocks', () => {
  let categoryId: number;
  let ownerId: number;
  let noteId: number;

  test.beforeAll(async ({ apiClient }) => {
    categoryId = (await apiClient.createCategory(`MD blocks ${Date.now()}`, '')).ID;
    ownerId = (await apiClient.createGroup({ name: `MD blocks owner ${Date.now()}`, categoryId })).ID;
  });

  test.beforeEach(async ({ apiClient }) => {
    noteId = (await apiClient.createNote({ name: `MD blocks note ${Date.now()}`, ownerId })).ID;
  });

  test.afterEach(async ({ apiClient }) => {
    if (noteId) await apiClient.deleteNote(noteId);
  });

  test.afterAll(async ({ apiClient }) => {
    if (ownerId) await apiClient.deleteGroup(ownerId);
    if (categoryId) await apiClient.deleteCategory(categoryId);
  });

  async function openEditor(page: Page) {
    await page.goto(`/note?id=${noteId}`);
    await page.getByText('Edit Blocks').click();
    await expect(page.getByRole('button', { name: 'Done' })).toBeVisible();
  }

  test('a text block edits in a labelled markdown editor with a toolbar and no preview toggle', async ({ page, apiClient }) => {
    const block = (await apiClient.createBlock(noteId, 'text', 'd', { text: 'start' })).id;
    await openEditor(page);
    await expect(editor(page, block)).toBeVisible();
    await expect(editor(page, block)).toHaveText('start');
    await expect(editor(page, block)).toHaveAttribute('aria-label', 'Text block');
    await expect(editor(page, block)).toHaveAttribute('aria-multiline', 'true');
    await expect(card(page, block).getByRole('toolbar')).toBeVisible();
    await expect(card(page, block).getByRole('button', { name: 'Preview' })).toHaveCount(0);
    await expect(card(page, block).locator('textarea')).toBeHidden();

    await editor(page, block).click();
    await page.keyboard.press(`${mod}+a`);
    await page.keyboard.type('**bold** text');
    await page.getByRole('button', { name: 'Done' }).click();
    await expect(card(page, block).locator('[data-text-block-rendered] strong')).toHaveText('bold');
    expect((await apiClient.getBlock(block)).content.text).toBe('**bold** text');
  });

  test('typing autosaves without leaving the block', async ({ page, apiClient }) => {
    const block = (await apiClient.createBlock(noteId, 'text', 'd', { text: '' })).id;
    await openEditor(page);
    await editor(page, block).click();
    await page.keyboard.type('autosaved');
    await expect.poll(async () => (await apiClient.getBlock(block)).content.text).toBe('autosaved');
    await expect(editor(page, block)).toBeFocused();
  });

  test('a toolbar button keeps focus in the editor and does not count as leaving it', async ({ page, apiClient }) => {
    const block = (await apiClient.createBlock(noteId, 'text', 'd', { text: 'word' })).id;
    await openEditor(page);
    await editor(page, block).click();
    await page.keyboard.press(`${mod}+a`);
    let saves = 0;
    page.on('request', (req) => {
      if (req.method() === 'PUT' && /\/v1\/note\/block\b/.test(req.url())) saves++;
    });
    await card(page, block).getByRole('button', { name: 'Bold' }).click();
    await expect(editor(page, block)).toHaveText('**word**');
    await expect(editor(page, block)).toBeFocused();
    // Leaving the block saves now, without waiting for the autosave delay.
    await page.locator('h1').first().click();
    await expect.poll(async () => (await apiClient.getBlock(block)).content.text, { timeout: 400 }).toBe('**word**');
    expect(saves).toBeGreaterThan(0);
  });

  test('tabbing out of the block saves it immediately', async ({ page, apiClient }) => {
    const block = (await apiClient.createBlock(noteId, 'text', 'd', { text: '' })).id;
    await openEditor(page);
    await editor(page, block).click();
    await page.keyboard.type('tabbed');
    await page.keyboard.press('Tab');
    await expect.poll(async () => (await apiClient.getBlock(block)).content.text, { timeout: 400 }).toBe('tabbed');
  });

  test('@-mentions offer notes, and picking one does not save half-typed text', async ({ page, apiClient }) => {
    const target = await apiClient.createNote({ name: `MentionTarget ${Date.now()}`, ownerId });
    const block = (await apiClient.createBlock(noteId, 'text', 'd', { text: '' })).id;
    await openEditor(page);
    await editor(page, block).click();
    await page.keyboard.type('see @MentionTarg');
    const option = page.getByRole('option', { name: new RegExp(target.Name) });
    await expect(option).toBeVisible();
    // CodeMirror ignores Enter for 75ms after the list opens (interactionDelay).
    await page.waitForTimeout(150);
    await page.keyboard.press('Enter');
    await expect(editor(page, block)).toHaveText(`see @[note:${target.ID}:${target.Name}]`);
    await expect(editor(page, block)).toBeFocused();
    await page.getByRole('button', { name: 'Done' }).click();
    // Done leaves edit mode only once the save has landed.
    await expect(page.getByRole('button', { name: 'Edit Blocks' })).toBeVisible();
    expect((await apiClient.getBlock(block)).content.text).toBe(`see @[note:${target.ID}:${target.Name}]`);
    await apiClient.deleteNote(target.ID);
  });

  test('shortcode completion is offered in a text block', async ({ page, apiClient }) => {
    const block = (await apiClient.createBlock(noteId, 'text', 'd', { text: '' })).id;
    await openEditor(page);
    await editor(page, block).click();
    await page.keyboard.type('[mrq');
    await expect(page.getByRole('option', { name: /^mrql/ })).toBeVisible();
  });

  test('two text blocks each get their own editor', async ({ page, apiClient }) => {
    const a = (await apiClient.createBlock(noteId, 'text', 'd', { text: 'first' })).id;
    const b = (await apiClient.createBlock(noteId, 'text', 'h', { text: 'second' })).id;
    await openEditor(page);
    await expect(editor(page, a)).toHaveText('first');
    await expect(editor(page, b)).toHaveText('second');
    await editor(page, b).click();
    await page.keyboard.press('End');
    await page.keyboard.type(' more');
    await page.getByRole('button', { name: 'Done' }).click();
    await expect(page.getByRole('button', { name: 'Edit Blocks' })).toBeVisible();
    expect((await apiClient.getBlock(a)).content.text).toBe('first');
    expect((await apiClient.getBlock(b)).content.text).toBe('second more');
  });
});
