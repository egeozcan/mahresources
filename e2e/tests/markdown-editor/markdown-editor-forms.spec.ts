/**
 * The markdown editor on create/edit forms
 * (docs/plans/2026-10-19-markdown-description-editor-design.md, slice 2).
 *
 * Every Description field is a CodeMirror markdown editor with a toolbar,
 * @-mention completion, and shortcode completion only on the entity types
 * whose Descriptions run shortcodes. The textarea stays the submitted value:
 * text typed before the editor loads survives, and a failed load leaves a
 * working textarea.
 */
import type { Page } from '@playwright/test';
import { test, expect } from '../../fixtures/base.fixture';

const FORMS = [
  '/note/new',
  '/group/new',
  '/resource/new',
  '/tag/new',
  '/category/new',
  '/resourceCategory/new',
  '/noteType/new',
  '/query/new',
  '/relationType/new',
  '/templatePartial/new',
];

function field(page: Page) {
  return page.locator('[data-markdown-field="Description"]');
}

function editor(page: Page) {
  return field(page).locator('.cm-content');
}

// Playwright resolves this to Meta on macOS and Control elsewhere.
const mod = 'ControlOrMeta';

// The chunks main.js imports statically; everything else under assets/ is
// loaded lazily (CodeMirror among it).
async function eagerChunks(request: import('@playwright/test').APIRequestContext) {
  const main = await (await request.get('/public/dist/main.js')).text();
  return new Set([...main.matchAll(/from\s*"\.\/assets\/([^"]+)"/g)].map((m) => m[1]));
}

function chunkName(url: string) {
  return url.split('/assets/')[1].split('?')[0];
}

test.describe('Markdown editor on forms', () => {
  for (const url of FORMS) {
    test(`${url}: the Description field is a markdown editor`, async ({ page }) => {
      await page.goto(url);
      await expect(editor(page)).toBeVisible();
      await expect(field(page).getByRole('toolbar')).toBeVisible();
      await expect(field(page).getByRole('button', { name: 'Bold' })).toBeVisible();
      // The source textarea stays as the submitted value, hidden once mounted.
      await expect(field(page).locator('textarea[name="Description"]')).toBeHidden();
    });
  }

  test('the editable element is a labelled multiline textbox', async ({ page }) => {
    await page.goto('/tag/new');
    const box = page.getByRole('textbox', { name: 'Description' });
    await expect(box).toBeVisible();
    await expect(box).toHaveAttribute('aria-multiline', 'true');
    await expect(box).toHaveClass(/cm-content/);
  });

  test('other textareas on the same form stay textareas', async ({ page }) => {
    await page.goto('/resourceCategory/new');
    await expect(page.locator('textarea[name="AutoDetectRules"]')).toBeVisible();
  });

  test('what is typed is what is saved', async ({ page, apiClient }) => {
    const name = `md-tag-${Date.now()}`;
    await page.goto('/tag/new');
    await page.locator('input[name="name"]').fill(name);
    await editor(page).fill('# Title\n\n**bold** text');
    await page.locator('button[type="submit"]').first().click();
    await page.waitForURL(/\/tag\?id=\d+/);
    const tag = (await apiClient.getTags()).find((t) => t.Name === name);
    // Form submission sends textarea line breaks as CRLF, as it always has.
    expect(tag?.Description?.replace(/\r\n/g, '\n')).toBe('# Title\n\n**bold** text');
  });

  test('toolbar and shortcuts insert markdown', async ({ page }) => {
    await page.goto('/tag/new');
    await editor(page).fill('word');
    await editor(page).press(`${mod}+a`);
    await field(page).getByRole('button', { name: 'Bold' }).click();
    await expect(editor(page)).toHaveText('**word**');

    await editor(page).fill('word');
    await editor(page).press(`${mod}+a`);
    await editor(page).press(`${mod}+i`);
    await expect(editor(page)).toHaveText('*word*');

    await editor(page).fill('site');
    await editor(page).press(`${mod}+a`);
    await editor(page).press(`${mod}+Shift+k`);
    await expect(editor(page)).toHaveText('[site](url)');
  });

  test('Ctrl/Cmd+K still opens global search from inside the editor', async ({ page }) => {
    await page.goto('/tag/new');
    await editor(page).click();
    await page.keyboard.press(`${mod}+k`);
    await expect(page.getByRole('dialog', { name: /search/i })).toBeVisible();
  });

  test('@-mention completion inserts a mention marker', async ({ page, apiClient }) => {
    const groupName = `Mentionable ${Date.now()}`;
    const category = await apiClient.createCategory(`Mention cat ${Date.now()}`, '');
    const group = await apiClient.createGroup({ name: groupName, categoryId: category.ID });
    await page.goto('/tag/new');
    await editor(page).click();
    await page.keyboard.type('see @Mentionab');
    const option = page.getByRole('option', { name: new RegExp(groupName) });
    await expect(option).toBeVisible();
    // Finding 133 carried over: the editable element names the listbox it
    // drives and the active option, as the old mention textarea did.
    const box = editor(page);
    await expect(box).toHaveAttribute('aria-autocomplete', 'list');
    const listboxId = await box.getAttribute('aria-controls');
    expect(listboxId).toBeTruthy();
    await expect(page.locator(`[id="${listboxId}"]`)).toHaveAttribute('role', 'listbox');
    await expect(box).toHaveAttribute('aria-activedescendant', /.+/);
    // CodeMirror ignores Enter for 75ms after the list opens (interactionDelay).
    await page.waitForTimeout(150);
    await page.keyboard.press('Enter');
    await expect(editor(page)).toHaveText(`see @[group:${group.ID}:${groupName}]`);
    await apiClient.deleteGroup(group.ID);
    await apiClient.deleteCategory(category.ID);
  });

  test('shortcode completion appears only where Descriptions run shortcodes', async ({ page }) => {
    await page.goto('/note/new');
    await editor(page).click();
    await page.keyboard.type('[mrq');
    await expect(page.getByRole('option', { name: /^mrql/ })).toBeVisible();

    await page.goto('/tag/new');
    await editor(page).click();
    await page.keyboard.type('[mrq');
    await page.waitForTimeout(500);
    await expect(page.getByRole('option', { name: /^mrql/ })).toHaveCount(0);
  });

  test('text typed before the editor loads is kept', async ({ page, request }) => {
    // Hold only the chunks main.js loads lazily (CodeMirror among them); its
    // static imports must load or the page never finishes.
    const eager = await eagerChunks(request);
    let release!: () => void;
    const held = new Promise<void>((r) => { release = r; });
    await page.route(/\/public\/dist\/assets\//, async (route) => {
      if (!eager.has(chunkName(route.request().url()))) await held;
      await route.continue();
    });
    await page.goto('/tag/new');
    const source = field(page).locator('textarea[name="Description"]');
    await source.fill('typed early');
    // The race really happened: the editor had not mounted yet.
    await expect(editor(page)).toHaveCount(0);
    release();
    await expect(editor(page)).toHaveText('typed early');
  });

  test('if the editor fails to load, the textarea still works', async ({ page, apiClient, request }) => {
    // Only the lazy chunks fail, so Alpine runs and markdownEditor's own
    // failure handling is what keeps the textarea.
    const eager = await eagerChunks(request);
    await page.route(/\/public\/dist\/assets\//, (route) =>
      eager.has(chunkName(route.request().url())) ? route.continue() : route.abort());
    const name = `md-fallback-${Date.now()}`;
    await page.goto('/tag/new');
    await page.locator('input[name="name"]').fill(name);
    const source = field(page).locator('textarea[name="Description"]');
    await expect(source).toBeVisible();
    await source.fill('plain fallback');
    await page.locator('button[type="submit"]').first().click();
    await page.waitForURL(/\/tag\?id=\d+/);
    const tag = (await apiClient.getTags()).find((t) => t.Name === name);
    expect(tag?.Description).toBe('plain fallback');
  });
});

test.describe('Markdown editor layout', () => {
  // The textarea is sized like the editor, so mounting CodeMirror over it does
  // not move what is below: a click aimed there right after load must land.
  for (const lines of [1, 30]) {
    test(`mounting keeps the field's height (${lines} line description)`, async ({ page, apiClient, request }) => {
      const text = Array.from({ length: lines }, (_, i) => `line ${i + 1}`).join('\n');
      const tag = await apiClient.createTag(`md-layout-${lines}-${Date.now()}`, text);
      const eager = await eagerChunks(request);
      let release!: () => void;
      const held = new Promise<void>((r) => { release = r; });
      await page.route(/\/public\/dist\/assets\//, async (route) => {
        if (!eager.has(chunkName(route.request().url()))) await held;
        await route.continue();
      });
      await page.goto(`/tag/edit?id=${tag.ID}`);
      await expect(field(page).locator('textarea[name="Description"]')).toBeVisible();
      const before = (await field(page).boundingBox())!.height;
      release();
      await expect(editor(page)).toBeVisible();
      const after = (await field(page).boundingBox())!.height;
      expect(Math.abs(after - before), `${before}px became ${after}px`).toBeLessThanOrEqual(6);
      await page.unrouteAll({ behavior: 'ignoreErrors' });
    });
  }
});
