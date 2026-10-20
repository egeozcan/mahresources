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
    await card(page, block).locator('.cm-content').fill('## Now a heading');
    await page.getByRole('button', { name: 'Done' }).click();
    await expect(rendered(page, block).locator('h2')).toHaveText('Now a heading');
  });

  test('a block showing the Description updates when the first block changes it', async ({ page, apiClient }) => {
    const first = (await apiClient.createBlock(noteId, 'text', 'd', { text: 'old text' })).id;
    const second = (await apiClient.createBlock(noteId, 'text', 'h', { text: 'Desc: [property path="Description"]' })).id;
    await page.goto(`/note?id=${noteId}`);
    await expect(rendered(page, second)).toContainText('Desc: old text');
    await page.getByText('Edit Blocks').click();
    await card(page, first).locator('.cm-content').fill('new text');
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
    await card(page, block).locator('.cm-content').fill('**after**');
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
    await card(page, other).locator('.cm-content').fill('other changed');
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
      await card(page, other).locator('.cm-content').fill('other changed');
      await page.getByRole('button', { name: 'Done' }).click();
      const draft = rendered(page, block).locator('input.draft');
      await draft.fill('my draft');
      release();
      await expect(rendered(page, other)).toHaveText('other changed');
      await expect(draft).toHaveValue('my draft');
      await expect(draft).toBeFocused();
      // A block the user touched stays theirs after they leave it too: its
      // rendering is of its current text, and a widget in it may still be
      // saving. A failure is reported, not painted over it.
      await page.locator('h1').first().click();
      await page.waitForTimeout(500);
      await expect(draft).toHaveValue('my draft');
      if (outcome === 'a failed render') {
        await expect(page.locator('.block-editor [role="alert"]')).toContainText(/render/i);
      }
    });
  }

  test('replacing the block objects does not rebuild a block someone is typing in', async ({ page, apiClient }) => {
    const block = (await apiClient.createBlock(noteId, 'text', 'd', {
      text: '<div x-data="{ draft: \'\' }"><input class="draft" x-model="draft"></div>',
    })).id;
    await page.goto(`/note?id=${noteId}`);
    const draft = rendered(page, block).locator('input.draft');
    await draft.fill('my draft');
    // A list reload (and any per-block state update) replaces the block objects.
    await page.evaluate(() => {
      const root = document.querySelector('.block-editor') as any;
      const editor = (window as any).Alpine.$data(root);
      editor.blocks = editor.blocks.map((b: any) => ({ ...b }));
    });
    await page.waitForTimeout(200);
    await expect(draft).toHaveValue('my draft');
    await expect(draft).toBeFocused();
  });

  test('a held rendering waits for a click inside the block (WebKit does not focus buttons)', async ({ page, apiClient }) => {
    const block = (await apiClient.createBlock(noteId, 'text', 'd', {
      text: '<div x-data="{ draft: \'\', saved: \'\' }"><input class="draft" x-model="draft"><button type="button" class="save" @click="saved = draft" x-text="saved || \'save\'"></button></div>\n\n[reload label="Refresh"]',
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
    await card(page, other).locator('.cm-content').fill('other changed');
    await page.getByRole('button', { name: 'Done' }).click();
    const draft = rendered(page, block).locator('input.draft');
    await draft.fill('kept');
    release();
    await expect(rendered(page, other)).toHaveText('other changed');
    // Simulate WebKit: the button press moves focus to <body>, not the button.
    await rendered(page, block).locator('button.save').evaluate((button) => {
      button.addEventListener('mousedown', (e) => { e.preventDefault(); (document.activeElement as HTMLElement)?.blur(); }, { once: true });
    });
    await rendered(page, block).locator('button.save').click();
    await expect(rendered(page, block).locator('button.save')).toHaveText('kept');
  });

  test('a recovery identical to what is shown cancels a held failure fallback', async ({ page, apiClient }) => {
    const block = (await apiClient.createBlock(noteId, 'text', 'd', {
      text: '<div x-data="{ draft: \'\' }"><input class="draft" x-model="draft"></div>',
    })).id;
    const other = (await apiClient.createBlock(noteId, 'text', 'h', { text: 'other' })).id;
    await page.goto(`/note?id=${noteId}`);
    let fail = true;
    await page.route(/\/v1\/note\/renderedTextBlocks/, (route) =>
      fail ? route.fulfill({ status: 500, contentType: 'text/plain', body: 'boom' }) : route.continue());
    const draft = rendered(page, block).locator('input.draft');
    await draft.fill('x');
    // A render that fails while the widget has focus: its fallback is held.
    await page.evaluate(() => (window as any).Alpine.$data(document.querySelector('.block-editor')).refreshRenderedText());
    await expect(page.locator('.block-editor [role="alert"]')).toContainText(/render/i);
    fail = false;
    await page.evaluate(() => (window as any).Alpine.$data(document.querySelector('.block-editor')).refreshRenderedText());
    await expect(page.locator('.block-editor [role="alert"]')).toBeHidden();
    // Leaving the block must not apply the stale failure fallback.
    await rendered(page, other).click();
    await expect(rendered(page, block).locator('input.draft')).toHaveCount(1);
  });

  test('a widget save during a pending render is not undone by the older rendering', async ({ page, apiClient }) => {
    // The button stands in for an inline [meta] editor's Save: it changes
    // server state the block displays, and shows the new value at once.
    const block = (await apiClient.createBlock(noteId, 'text', 'd', {
      text: `<span class="name">[property path="Name"]</span> <button type="button" class="rename" x-on:click="fetch('/v1/note/editName?id=${noteId}', { method: 'POST', body: new URLSearchParams({ name: 'Renamed B' }) }).then(() => { $el.previousElementSibling.textContent = 'Renamed B'; })">rename</button>\n\n[reload label="Refresh"]`,
    })).id;
    const other = (await apiClient.createBlock(noteId, 'text', 'h', { text: 'other' })).id;
    await page.goto(`/note?id=${noteId}`);
    let release!: () => void;
    const held = new Promise<void>((r) => { release = r; });
    let first = true;
    await page.route(/\/v1\/note\/renderedTextBlocks/, async (route) => {
      if (!first) return route.continue();
      first = false;
      const resp = await route.fetch(); // rendered before the rename
      await held;
      await route.fulfill({ response: resp });
    });
    await page.getByText('Edit Blocks').click();
    await card(page, other).locator('.cm-content').fill('other changed');
    await page.getByRole('button', { name: 'Done' }).click();
    await rendered(page, block).locator('button.rename').click();
    await expect(rendered(page, block).locator('.name')).toHaveText('Renamed B');
    release();
    await expect(rendered(page, other)).toHaveText('other changed');
    await page.locator('h1').first().click();
    await page.waitForTimeout(500);
    await expect(rendered(page, block).locator('.name')).toHaveText('Renamed B');
  });

  test('a widget save after a rendering was held is not undone on leaving the block', async ({ page, apiClient }) => {
    const block = (await apiClient.createBlock(noteId, 'text', 'd', {
      text: `<span class="name">[property path="Name"]</span> <input class="draft"> <button type="button" class="rename" x-on:click="fetch('/v1/note/editName?id=${noteId}', { method: 'POST', body: new URLSearchParams({ name: 'Renamed C' }) }).then(() => { $el.parentElement.querySelector('.name').textContent = 'Renamed C'; })">rename</button>\n\n[reload label="Refresh"]`,
    })).id;
    const other = (await apiClient.createBlock(noteId, 'text', 'h', { text: 'other' })).id;
    await page.goto(`/note?id=${noteId}`);
    let release!: () => void;
    const held = new Promise<void>((r) => { release = r; });
    let first = true;
    await page.route(/\/v1\/note\/renderedTextBlocks/, async (route) => {
      if (!first) return route.continue();
      first = false;
      await held;
      await route.continue();
    });
    await page.getByText('Edit Blocks').click();
    await card(page, other).locator('.cm-content').fill('other changed');
    await page.getByRole('button', { name: 'Done' }).click();
    // Focus sits in the block (no interaction yet) while the rendering lands: it is held.
    await rendered(page, block).locator('input.draft').focus();
    release();
    await expect(rendered(page, other)).toHaveText('other changed');
    // Now save from inside the block, then leave it.
    await rendered(page, block).locator('button.rename').click();
    await expect(rendered(page, block).locator('.name')).toHaveText('Renamed C');
    await page.locator('h1').first().click();
    await page.waitForTimeout(500);
    await expect(rendered(page, block).locator('.name')).toHaveText('Renamed C');
  });

  test('a rendering that arrives while a widget save is in flight does not replace the widget', async ({ page, apiClient }) => {
    // The save button disables itself while saving, which drops focus to
    // <body>, as an inline [meta] editor's Save does; the request then fails.
    const block = (await apiClient.createBlock(noteId, 'text', 'd', {
      text: `<div x-data="{ draft: '', saving: false, error: '' }"><input class="draft" x-model="draft"> <button type="button" class="save" :disabled="saving" x-on:click="saving = true; fetch('/v1/held-save', { method: 'POST' }).then((r) => { if (!r.ok) throw new Error('nope'); }).catch(() => { error = 'Could not save'; }).finally(() => { saving = false; })">save</button> <span class="error" x-text="error"></span></div>\n\n[reload label="Refresh"]`,
    })).id;
    const other = (await apiClient.createBlock(noteId, 'text', 'h', { text: 'other' })).id;
    await page.goto(`/note?id=${noteId}`);
    let releaseRender!: () => void;
    const renderHeld = new Promise<void>((r) => { releaseRender = r; });
    await page.route(/\/v1\/note\/renderedTextBlocks/, async (route) => {
      await renderHeld;
      await route.continue();
    });
    let releaseSave!: () => void;
    const saveHeld = new Promise<void>((r) => { releaseSave = r; });
    await page.route(/\/v1\/held-save/, async (route) => {
      await saveHeld;
      await route.fulfill({ status: 500, body: 'boom' });
    });
    await page.getByText('Edit Blocks').click();
    await card(page, other).locator('.cm-content').fill('other changed');
    await page.getByRole('button', { name: 'Done' }).click();
    await rendered(page, block).locator('input.draft').fill('my draft');
    await rendered(page, block).locator('button.save').click();
    await expect(rendered(page, block).locator('button.save')).toBeDisabled();
    releaseRender();
    await expect(rendered(page, other)).toHaveText('other changed');
    await page.waitForTimeout(300);
    releaseSave();
    await expect(rendered(page, block).locator('.error')).toHaveText('Could not save');
    await expect(rendered(page, block).locator('input.draft')).toHaveValue('my draft');
  });

  test('a keyboard-only widget save is not undone by a held rendering', async ({ page, apiClient }) => {
    // Arrow keys change and save the value with no click, input or change
    // event, as the [pills] widget does.
    const block = (await apiClient.createBlock(noteId, 'text', 'd', {
      text: `<span class="name">[property path="Name"]</span> <button type="button" class="pill" x-on:keydown.arrow-right="fetch('/v1/note/editName?id=${noteId}', { method: 'POST', body: new URLSearchParams({ name: 'Renamed D' }) }).then(() => { $el.parentElement.querySelector('.name').textContent = 'Renamed D'; })">pill</button>\n\n[reload label="Refresh"]`,
    })).id;
    const other = (await apiClient.createBlock(noteId, 'text', 'h', { text: 'other' })).id;
    await page.goto(`/note?id=${noteId}`);
    let release!: () => void;
    const held = new Promise<void>((r) => { release = r; });
    let first = true;
    await page.route(/\/v1\/note\/renderedTextBlocks/, async (route) => {
      if (!first) return route.continue();
      first = false;
      await held;
      await route.continue();
    });
    await page.getByText('Edit Blocks').click();
    await card(page, other).locator('.cm-content').fill('other changed');
    await page.getByRole('button', { name: 'Done' }).click();
    await rendered(page, block).locator('button.pill').focus();
    release();
    await expect(rendered(page, other)).toHaveText('other changed');
    await page.keyboard.press('ArrowRight');
    await expect(rendered(page, block).locator('.name')).toHaveText('Renamed D');
    await page.locator('h1').first().click();
    await page.waitForTimeout(500);
    await expect(rendered(page, block).locator('.name')).toHaveText('Renamed D');
  });

  test('an edited block shows its old rendering inert until the new one arrives', async ({ page, apiClient }) => {
    const block = (await apiClient.createBlock(noteId, 'text', 'd', {
      text: '<button type="button" class="old">old widget</button>',
    })).id;
    await page.goto(`/note?id=${noteId}`);
    let release!: () => void;
    const held = new Promise<void>((r) => { release = r; });
    await page.route(/\/v1\/note\/renderedTextBlocks/, async (route) => {
      await held;
      await route.continue();
    });
    await page.getByText('Edit Blocks').click();
    await card(page, block).locator('.cm-content').fill('<button type="button" class="new">new widget</button>');
    await page.getByRole('button', { name: 'Done' }).click();
    await expect(rendered(page, block).locator('button.old')).toBeVisible();
    await expect(rendered(page, block)).toHaveAttribute('aria-busy', 'true');
    expect(await rendered(page, block).evaluate((el) => (el as HTMLElement).inert)).toBe(true);
    release();
    await expect(rendered(page, block).locator('button.new')).toBeVisible();
    await expect(rendered(page, block)).not.toHaveAttribute('aria-busy', 'true');
    expect(await rendered(page, block).evaluate((el) => (el as HTMLElement).inert)).toBe(false);
  });

  test('a draft typed inside an iframe in a block survives a background render', async ({ page, apiClient }) => {
    const block = (await apiClient.createBlock(noteId, 'text', 'd', {
      text: '<iframe class="frame" srcdoc="<input class=draft placeholder=Draft>"></iframe>\n\n[reload label="Refresh"]',
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
    await card(page, other).locator('.cm-content').fill('other changed');
    await page.getByRole('button', { name: 'Done' }).click();
    const draft = rendered(page, block).frameLocator('iframe.frame').locator('input.draft');
    await draft.fill('iframe draft');
    // Then leave the frame: the block stays touched.
    await page.locator('h1').first().click();
    release();
    await expect(rendered(page, other)).toHaveText('other changed');
    await page.waitForTimeout(300);
    await expect(draft).toHaveValue('iframe draft');
  });

  test('a draft typed after moving straight from one block iframe to another survives', async ({ page, apiClient }) => {
    const frameText = '<iframe class="frame" srcdoc="<input class=draft placeholder=Draft>"></iframe>\n\n[reload label="Refresh"]';
    const a = (await apiClient.createBlock(noteId, 'text', 'd', { text: frameText })).id;
    const b = (await apiClient.createBlock(noteId, 'text', 'h', { text: frameText })).id;
    const other = (await apiClient.createBlock(noteId, 'text', 'p', { text: 'other' })).id;
    await page.goto(`/note?id=${noteId}`);
    let release!: () => void;
    const held = new Promise<void>((r) => { release = r; });
    await page.route(/\/v1\/note\/renderedTextBlocks/, async (route) => {
      await held;
      await route.continue();
    });
    await page.getByText('Edit Blocks').click();
    await card(page, other).locator('.cm-content').fill('other changed');
    await page.getByRole('button', { name: 'Done' }).click();
    const draftA = rendered(page, a).frameLocator('iframe.frame').locator('input.draft');
    const draftB = rendered(page, b).frameLocator('iframe.frame').locator('input.draft');
    await draftA.fill('draft A');
    // Frame to frame: the page sees no event at all.
    await draftB.fill('draft B');
    await page.locator('h1').first().click();
    release();
    await expect(rendered(page, other)).toHaveText('other changed');
    await page.waitForTimeout(300);
    await expect(draftA).toHaveValue('draft A');
    await expect(draftB).toHaveValue('draft B');
  });

  test('entering a frame does not keep an edited block on its outdated rendering', async ({ page, apiClient }) => {
    const a = (await apiClient.createBlock(noteId, 'text', 'd', {
      text: '<iframe class="frame" srcdoc="<p>old</p>"></iframe>',
    })).id;
    const b = (await apiClient.createBlock(noteId, 'text', 'h', {
      text: '<iframe class="frame" srcdoc="<input class=draft>"></iframe>',
    })).id;
    await page.goto(`/note?id=${noteId}`);
    let release!: () => void;
    const held = new Promise<void>((r) => { release = r; });
    await page.route(/\/v1\/note\/renderedTextBlocks/, async (route) => {
      await held;
      await route.continue();
    });
    await page.getByText('Edit Blocks').click();
    await card(page, a).locator('.cm-content').fill('**new text**');
    await page.getByRole('button', { name: 'Done' }).click();
    await expect(rendered(page, a)).toHaveAttribute('aria-busy', 'true');
    await rendered(page, b).frameLocator('iframe.frame').locator('input.draft').fill('typing');
    release();
    await expect(rendered(page, a).locator('strong')).toHaveText('new text');
    await expect(rendered(page, a)).not.toHaveAttribute('aria-busy', 'true');
  });

  test('a render made while a save is in flight is not taken for a rendering of the unsaved text', async ({ page, apiClient }) => {
    const block = (await apiClient.createBlock(noteId, 'text', 'd', { text: 'one' })).id;
    await page.goto(`/note?id=${noteId}`);
    await page.getByText('Edit Blocks').click();
    let releasePut!: () => void;
    const putHeld = new Promise<void>((r) => { releasePut = r; });
    let putSeen = false;
    await page.route(/\/v1\/note\/block\?/, async (route) => {
      if (route.request().method() !== 'PUT') return route.fallback();
      putSeen = true;
      await putHeld;
      await route.continue();
    });
    await card(page, block).locator('.cm-content').fill('two');
    await expect.poll(() => putSeen).toBe(true);
    // A refresh while the save is pending: the server renders the saved 'one'.
    await page.evaluate(() => (window as any).Alpine.$data(document.querySelector('.block-editor')).refreshRenderedText());
    await page.waitForTimeout(300);
    releasePut();
    await expect.poll(async () => (await apiClient.getBlock(block)).content.text).toBe('two');
    let releaseRender!: () => void;
    const renderHeld = new Promise<void>((r) => { releaseRender = r; });
    await page.route(/\/v1\/note\/renderedTextBlocks/, async (route) => {
      await renderHeld;
      await route.continue();
    });
    await page.getByRole('button', { name: 'Done' }).click();
    // What is shown is the rendering of 'one': outdated, so inert.
    await expect(rendered(page, block)).toHaveText('one');
    await expect(rendered(page, block)).toHaveAttribute('aria-busy', 'true');
    releaseRender();
    await expect(rendered(page, block)).toHaveText('two');
    await expect(rendered(page, block)).not.toHaveAttribute('aria-busy', 'true');
  });

  test('an edit that renders to the same HTML does not leave the block inert', async ({ page, apiClient }) => {
    const block = (await apiClient.createBlock(noteId, 'text', 'd', { text: '**bold** [link](/notes)' })).id;
    await page.goto(`/note?id=${noteId}`);
    await page.getByText('Edit Blocks').click();
    await card(page, block).locator('.cm-content').fill('__bold__ [link](/notes)');
    await page.getByRole('button', { name: 'Done' }).click();
    await expect(rendered(page, block).locator('strong')).toHaveText('bold');
    await expect(rendered(page, block)).not.toHaveAttribute('aria-busy', 'true');
    expect(await rendered(page, block).evaluate((el) => (el as HTMLElement).inert)).toBe(false);
  });
});
