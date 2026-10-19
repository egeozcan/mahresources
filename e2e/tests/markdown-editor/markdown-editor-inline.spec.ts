/**
 * The markdown editor inside double-click description editing on detail
 * pages (docs/plans/2026-10-19-markdown-description-editor-design.md,
 * decisions 4 and 9, slice 4). The existing commit paths stay: Ctrl/Cmd+Enter
 * and Save reload, Tab out saves without reloading, Escape cancels, but an
 * open completion popup takes the first Escape.
 */
import type { Page } from '@playwright/test';
import { test, expect } from '../../fixtures/base.fixture';

const mod = 'ControlOrMeta';

function region(page: Page) {
  return page.locator('[data-description-editor]');
}
function editor(page: Page) {
  return region(page).locator('.cm-content');
}

async function open(page: Page) {
  await page.locator('[data-description-display]').first().dblclick();
  await expect(editor(page)).toBeVisible();
}

test.describe('Markdown editor in inline description editing', () => {
  test('double-click opens a focused markdown editor and Ctrl/Cmd+Enter saves', async ({ page, apiClient }) => {
    const tag = await apiClient.createTag(`inline-md-${Date.now()}`, 'start');
    await page.goto(`/tag?id=${tag.ID}`);
    await open(page);
    await expect(editor(page)).toBeFocused();
    await expect(region(page).getByRole('toolbar')).toBeVisible();
    await editor(page).press(`${mod}+a`);
    await page.keyboard.type('**bold** words');
    await page.keyboard.press(`${mod}+Enter`);
    await expect(page.locator('[data-description-display] strong')).toHaveText('bold');
    expect((await apiClient.getTags()).find((t) => t.ID === tag.ID)?.Description).toBe('**bold** words');
  });

  test('Escape closes an open mention popup first, then cancels', async ({ page, apiClient }) => {
    const name = `InlineMention ${Date.now()}`;
    const category = await apiClient.createCategory(`inl cat ${Date.now()}`, '');
    const group = await apiClient.createGroup({ name, categoryId: category.ID });
    const tag = await apiClient.createTag(`inline-esc-${Date.now()}`, 'keep me');
    await page.goto(`/tag?id=${tag.ID}`);
    await open(page);
    await editor(page).press('End');
    await page.keyboard.type(' @InlineMent');
    await expect(page.getByRole('option', { name: new RegExp(name) })).toBeVisible();
    await page.keyboard.press('Escape');
    await expect(page.getByRole('option', { name: new RegExp(name) })).toHaveCount(0);
    await expect(editor(page)).toBeVisible();
    await page.keyboard.press('Escape');
    await expect(region(page)).toHaveCount(0);
    await expect(page.locator('[data-description-display]')).toContainText('keep me');
    expect((await apiClient.getTags()).find((t) => t.ID === tag.ID)?.Description).toBe('keep me');
    await apiClient.deleteGroup(group.ID);
    await apiClient.deleteCategory(category.ID);
  });

  test('tabbing out of the editor saves without reloading', async ({ page, apiClient }) => {
    const tag = await apiClient.createTag(`inline-tab-${Date.now()}`, 'old');
    await page.goto(`/tag?id=${tag.ID}`);
    await open(page);
    await editor(page).press(`${mod}+a`);
    await page.keyboard.type('tabbed away');
    let reloaded = false;
    page.on('framenavigated', () => { reloaded = true; });
    // Editor -> Save -> Cancel -> out of the region.
    await page.keyboard.press('Tab');
    await page.keyboard.press('Tab');
    await page.keyboard.press('Tab');
    await expect.poll(async () => (await apiClient.getTags()).find((t) => t.ID === tag.ID)?.Description).toBe('tabbed away');
    expect(reloaded).toBe(false);
  });

  test('preview renders shortcodes against the entity', async ({ page, apiClient }) => {
    const note = await apiClient.createNote({ name: `Inline Preview ${Date.now()}`, description: 'x' });
    await page.goto(`/note?id=${note.ID}`);
    await open(page);
    await editor(page).press(`${mod}+a`);
    await page.keyboard.type('[property path="Name"]');
    await region(page).getByRole('button', { name: 'Preview' }).click();
    await expect(page.frameLocator('[data-description-editor] iframe[data-markdown-preview]').locator('body')).toContainText(note.Name);
    await apiClient.deleteNote(note.ID);
  });

  test('closing the editor before CodeMirror loads leaves nothing behind', async ({ page, apiClient, request }) => {
    const main = await (await request.get('/public/dist/main.js')).text();
    const eager = new Set([...main.matchAll(/from\s*"\.\/assets\/([^"]+)"/g)].map((m) => m[1]));
    let release!: () => void;
    const held = new Promise<void>((r) => { release = r; });
    await page.route(/\/public\/dist\/assets\//, async (route) => {
      const file = route.request().url().split('/assets/')[1].split('?')[0];
      if (!eager.has(file)) await held;
      await route.continue();
    });
    const errors: string[] = [];
    page.on('pageerror', (e) => errors.push(e.message));
    const tag = await apiClient.createTag(`inline-early-${Date.now()}`, 'unchanged');
    await page.goto(`/tag?id=${tag.ID}`);
    // markdownEditor adds a live region to <body> on init and removes it in
    // destroy(); destroy() is also what stops the pending mount.
    const liveRegions = () => page.locator('body > [role="status"][aria-live="polite"]').count();
    const before = await liveRegions();
    await page.locator('[data-description-display]').first().dblclick();
    await expect(region(page).locator('textarea[name="description"]')).toBeVisible();
    expect(await liveRegions()).toBe(before + 1);
    await page.keyboard.press('Escape');
    await expect(region(page)).toHaveCount(0);
    expect(await liveRegions()).toBe(before);
    release();
    await page.waitForTimeout(500);
    await expect(page.locator('.cm-editor')).toHaveCount(0);
    expect(errors).toEqual([]);
  });
});
