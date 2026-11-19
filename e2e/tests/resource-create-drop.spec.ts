import { test, expect } from '../fixtures/base.fixture';
import type { Page } from '@playwright/test';

/**
 * Dropping files on the resource creation form.
 *
 * The form's file picker owns every file drop on its page: a drop anywhere on
 * it adds the files to the picker, the way a paste does. Before, a drop beside
 * the picker either went to the browser, which navigated away from the form to
 * show the file, or (on `/resource/new?ownerId=N`, where a group's Add link
 * lands) was taken by the page-wide paste-upload dialog, which ignored the form.
 *
 * Drops carry a real DataTransfer, so `input.files` can be filled from it.
 * Folders use mock entries with the webkitGetAsEntry() surface a real drop has.
 */

type Node =
  | { type: 'file'; name: string; body: string }
  | { type: 'dir'; name: string; children: Node[] };

const MODAL = '[role="dialog"][aria-labelledby="paste-upload-title"]';

/** Fire dragenter/dragover/drop on `selector`. Returns what each one said. */
async function dropOn(page: Page, selector: string, tree: Node[]) {
  return page.evaluate(async ({ selector, tree }: { selector: string; tree: Node[] }) => {
    const mkFile = (n: any) => new File([n.body], n.name, { type: 'text/plain' });
    const mkEntry = (n: any): any => n.type === 'file'
      ? { isFile: true, isDirectory: false, name: n.name, file: (ok: any) => ok(mkFile(n)) }
      : {
        isFile: false, isDirectory: true, name: n.name,
        createReader() {
          let done = false;
          return { readEntries(ok: any) { ok(done ? [] : n.children.map(mkEntry)); done = true; } };
        },
      };
    const real = new DataTransfer();
    for (const n of tree as any[]) if (n.type === 'file') real.items.add(mkFile(n));
    const dt: any = {
      types: ['Files'],
      dropEffect: 'none',
      items: tree.map((n: any) => ({ kind: 'file', webkitGetAsEntry: () => mkEntry(n), getAsFile: () => (n.type === 'file' ? mkFile(n) : null) })),
      files: real.files,
    };
    const target = document.querySelector(selector)!;
    const fire = (type: string) => {
      const ev = new Event(type, { bubbles: true, cancelable: true });
      Object.defineProperty(ev, 'dataTransfer', { value: dt });
      target.dispatchEvent(ev);
      return ev;
    };
    fire('dragenter');
    const over = fire('dragover');
    // Alpine applies x-show on its next tick.
    await new Promise((r) => setTimeout(r, 50));
    const overlay = document.querySelector('[data-testid="drop-overlay"]') as HTMLElement | null;
    const overlayShown = !!overlay && getComputedStyle(overlay).display !== 'none';
    const drop = fire('drop');
    return { overPrevented: over.defaultPrevented, dropEffect: dt.dropEffect, overlayShown, dropPrevented: drop.defaultPrevented };
  }, { selector, tree });
}

const pickedNames = (page: Page) => page.evaluate(
  () => [...(document.getElementById('resource') as HTMLInputElement).files!].map((f) => f.name),
);

test.describe('Dropping files on the resource creation form', () => {
  const uid = `${Date.now()}-${Math.floor(Math.random() * 100000)}`;
  let ownerId: number;
  let categoryId: number;

  test.beforeAll(async ({ apiClient }) => {
    categoryId = (await apiClient.createCategory(`CreateDrop Category ${uid}`)).ID;
    ownerId = (await apiClient.createGroup({ name: `CreateDrop Owner ${uid}`, categoryId })).ID;
  });

  test.afterAll(async ({ apiClient }) => {
    await apiClient.deleteGroup(ownerId).catch(() => {});
    await apiClient.deleteCategory(categoryId).catch(() => {});
  });

  for (const query of ['', '?ownerId=OWNER']) {
    test(`a file dropped beside the picker is added to it (${query || 'no owner'})`, async ({ page }) => {
      await page.goto(`/resource/new${query.replace('OWNER', String(ownerId))}`);
      const result = await dropOn(page, 'form [name="Name"]', [{ type: 'file', name: `beside-${uid}.txt`, body: `beside-${uid}` }]);
      // Taken, so the browser does not navigate away to show the file.
      expect(result.dropPrevented).toBe(true);
      expect(result.overPrevented).toBe(true);
      expect(result.dropEffect).toBe('copy');
      // The page-wide upload overlay and dialog stay out of the form's way.
      expect(result.overlayShown).toBe(false);
      await expect.poll(() => pickedNames(page)).toEqual([`beside-${uid}.txt`]);
      await page.waitForTimeout(300);
      await expect(page.locator(MODAL)).not.toBeVisible();
    });
  }

  test('a drop onto the picker itself adds to the selection instead of replacing it', async ({ page }) => {
    await page.goto('/resource/new');
    await page.locator('#resource').setInputFiles({ name: `first-${uid}.txt`, mimeType: 'text/plain', buffer: Buffer.from(`first-${uid}`) });
    await dropOn(page, '#resource', [{ type: 'file', name: `second-${uid}.txt`, body: `second-${uid}` }]);
    await expect.poll(() => pickedNames(page)).toEqual([`first-${uid}.txt`, `second-${uid}.txt`]);
  });

  test('a text file dropped into the Description editor stays the editor\'s', async ({ page }) => {
    await page.goto('/resource/new');
    await expect(page.locator('.cm-content').first()).toBeVisible();
    await dropOn(page, '.cm-content', [{ type: 'file', name: `prose-${uid}.txt`, body: `prose-body-${uid}` }]);
    await expect(page.locator('.cm-content').first()).toContainText(`prose-body-${uid}`);
    expect(await pickedNames(page)).toEqual([]);
  });

  test('a dropped folder adds the files inside it, skipping hidden ones', async ({ page }) => {
    await page.goto('/resource/new');
    await dropOn(page, 'form', [{
      type: 'dir', name: `folder-${uid}`, children: [
        { type: 'file', name: `a-${uid}.txt`, body: `a-${uid}` },
        { type: 'file', name: '.DS_Store', body: 'x' },
        { type: 'dir', name: 'inner', children: [{ type: 'file', name: `b-${uid}.txt`, body: `b-${uid}` }] },
      ],
    }]);
    await expect.poll(() => pickedNames(page)).toEqual([`a-${uid}.txt`, `b-${uid}.txt`]);
  });

  test('a dropped file is what Save uploads, to the owner the form was opened for', async ({ page, apiClient }) => {
    const name = `saved-drop-${uid}.txt`;
    await page.goto(`/resource/new?ownerId=${ownerId}`);
    await dropOn(page, 'form', [{ type: 'file', name, body: `saved-drop-body-${uid}` }]);
    await expect.poll(() => pickedNames(page)).toEqual([name]);
    await page.locator('form button[type="submit"]').first().click();
    await page.waitForURL(/\/resource\?id=\d+/);
    const id = Number(new URL(page.url()).searchParams.get('id'));
    const resource = await (await page.request.get(`/v1/resource?id=${id}`)).json();
    expect(resource.Name).toBe(name);
    expect(resource.OwnerId).toBe(ownerId);
    await apiClient.deleteResource(id).catch(() => {});
  });
});

test.describe('Owner-filtered pages without a paste context', () => {
  test('a drop on /note/new?ownerId=N offers to upload to that group', async ({ page, apiClient }) => {
    const name = `NoteNew Drop Owner ${Date.now()}`;
    const category = await apiClient.createCategory(`${name} Category`);
    const group = await apiClient.createGroup({ name, categoryId: category.ID });
    try {
      await page.goto(`/note/new?ownerId=${group.ID}`);
      await dropOn(page, 'main', [{ type: 'file', name: `note-new-${group.ID}.txt`, body: `note-new-${group.ID}` }]);
      const modal = page.locator(MODAL);
      await expect(modal).toBeVisible();
      await expect(modal).toContainText(name);
      await modal.getByRole('button', { name: 'Cancel' }).click();
    } finally {
      await apiClient.deleteGroup(group.ID).catch(() => {});
      await apiClient.deleteCategory(category.ID).catch(() => {});
    }
  });
});
