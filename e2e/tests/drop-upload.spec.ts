import { test, expect } from '../fixtures/base.fixture';
import type { Page } from '@playwright/test';

/**
 * E2E tests for drag-and-drop upload onto pages that already accept paste.
 *
 * A browser cannot be handed a real OS folder, so the drop carries a fake
 * DataTransfer whose items expose the same webkitGetAsEntry() surface a real
 * drop does. The directory entries are mocks that page through readEntries in
 * batches of two, so the walker's read-until-empty loop is exercised too.
 */

type Node =
  | { type: 'file'; name: string; body: string; mime?: string }
  | { type: 'dir'; name: string; children: Node[] };

const file = (name: string, body: string, mime = 'text/plain'): Node => ({ type: 'file', name, body, mime });
const dir = (name: string, children: Node[]): Node => ({ type: 'dir', name, children });

/** Dispatch dragenter + dragover + drop carrying `tree`. Returns whether drop was default-prevented. */
async function drop(page: Page, tree: Node[], opts: { hold?: boolean; delayMs?: number } = {}): Promise<boolean> {
  return page.evaluate(
    ({ tree, hold, delayMs }) => {
      const mkFile = (n: any) => new File([n.body], n.name, { type: n.mime || 'text/plain' });
      const mkEntry = (n: any): any => {
        if (n.type === 'file') {
          return { isFile: true, isDirectory: false, name: n.name, file: (ok: any) => ok(mkFile(n)) };
        }
        return {
          isFile: false,
          isDirectory: true,
          name: n.name,
          createReader() {
            let at = 0;
            return {
              readEntries(ok: any) {
                const batch = n.children.slice(at, at + 2).map(mkEntry);
                at += 2;
                setTimeout(() => ok(batch), delayMs);
              },
            };
          },
        };
      };
      const dt: any = {
        types: ['Files'],
        dropEffect: 'none',
        items: tree.map((n) => ({
          kind: 'file',
          webkitGetAsEntry: () => mkEntry(n),
          getAsFile: () => (n.type === 'file' ? mkFile(n) : null),
        })),
        files: tree.filter((n: any) => n.type === 'file').map(mkFile),
      };
      const fire = (type: string) => {
        const ev = new Event(type, { bubbles: true, cancelable: true });
        Object.defineProperty(ev, 'dataTransfer', { value: dt });
        document.body.dispatchEvent(ev);
        return ev;
      };
      fire('dragenter');
      fire('dragover');
      if (hold) return false;
      return fire('drop').defaultPrevented;
    },
    { tree, hold: !!opts.hold, delayMs: opts.delayMs ?? 0 },
  );
}

const MODAL = '[role="dialog"][aria-labelledby="paste-upload-title"]';

test.describe.serial('Drop Upload', () => {
  const uid = Date.now() + Math.floor(Math.random() * 100000);
  let categoryId: number;
  let folderCategoryId: number;
  let folderCategoryName: string;
  let groupId: number;
  let groupName: string;
  const createdGroupIds: number[] = [];
  const createdResourceIds: number[] = [];

  async function findGroups(page: Page, name: string): Promise<any[]> {
    const resp = await page.request.get(`/v1/groups?Name=${encodeURIComponent(name)}`);
    return resp.json();
  }
  async function findResource(page: Page, name: string): Promise<any> {
    const resp = await page.request.get(`/v1/resources?Name=${encodeURIComponent(name)}`);
    const list = await resp.json();
    expect(list, `resource ${name}`).toHaveLength(1);
    createdResourceIds.push(list[0].ID);
    return list[0];
  }
  async function track(page: Page, name: string): Promise<any[]> {
    const groups = await findGroups(page, name);
    for (const g of groups) createdGroupIds.push(g.ID);
    return groups;
  }

  test.beforeAll(async ({ apiClient }) => {
    categoryId = (await apiClient.createCategory(`DropTest Category ${uid}`)).ID;
    folderCategoryName = `DropTest Folder Category ${uid}`;
    folderCategoryId = (await apiClient.createCategory(folderCategoryName)).ID;
    groupName = `DropTest Group ${uid}`;
    groupId = (await apiClient.createGroup({ name: groupName, categoryId })).ID;
  });

  test('dropping loose files opens the modal and uploads them to the group', async ({ page, groupPage }) => {
    await groupPage.gotoDisplay(groupId);
    expect(await drop(page, [file(`loose-a-${uid}.txt`, `a-${uid}`), file(`loose-b-${uid}.txt`, `b-${uid}`)])).toBe(true);

    const modal = page.locator(MODAL);
    await expect(modal).toBeVisible();
    await expect(page.locator('#paste-upload-title')).toContainText(groupName);
    await expect(modal.locator('input[aria-label^="Name for item"]')).toHaveCount(2);
    // No folder was dropped, so none of the folder controls appear.
    await expect(modal.getByLabel('Keep folder structure')).toHaveCount(0);
    await expect(modal.getByLabel('Search group categories')).toHaveCount(0);

    await modal.getByRole('button', { name: 'Upload' }).click();
    await expect(modal).not.toBeVisible({ timeout: 10000 });

    for (const n of ['a', 'b']) {
      const r = await findResource(page, `loose-${n}-${uid}.txt`);
      expect(r.OwnerId).toBe(groupId);
    }
  });

  test('dropping a folder tree creates nested groups with the chosen category', async ({ page, groupPage }) => {
    const root = `photos-${uid}`;
    await groupPage.gotoDisplay(groupId);
    await drop(page, [
      dir(root, [
        file(`top-${uid}.txt`, `top-${uid}`),
        dir('2024', [file(`jan-${uid}.txt`, `jan-${uid}`), file(`feb-${uid}.txt`, `feb-${uid}`)]),
        dir('junk', [file('.DS_Store', 'x'), file('Thumbs.db', 'x')]),
        dir('empty', []),
      ]),
    ]);

    const modal = page.locator(MODAL);
    await expect(modal).toBeVisible();
    await expect(modal.locator('input[aria-label^="Name for item"]')).toHaveCount(3);
    const keep = modal.getByLabel('Keep folder structure');
    await expect(keep).toBeChecked();
    // The folder plus 2024; junk and empty contribute nothing.
    await expect(modal.getByText('2 groups will be created')).toBeVisible();

    const catInput = modal.getByLabel('Search group categories');
    await catInput.fill(folderCategoryName);
    await modal
      .locator('#paste-upload-group-category-listbox [role="option"]', { hasText: folderCategoryName })
      .click();

    await modal.getByRole('button', { name: 'Upload' }).click();
    await expect(modal).not.toBeVisible({ timeout: 15000 });

    const rootGroups = await track(page, root);
    expect(rootGroups).toHaveLength(1);
    expect(rootGroups[0].OwnerId).toBe(groupId);
    expect(rootGroups[0].CategoryId).toBe(folderCategoryId);

    const sub = (await track(page, '2024')).filter((g) => g.OwnerId === rootGroups[0].ID);
    expect(sub).toHaveLength(1);
    expect(sub[0].CategoryId).toBe(folderCategoryId);
    expect(await findGroups(page, 'junk')).toEqual([]);
    expect(await findGroups(page, 'empty')).toEqual([]);

    expect((await findResource(page, `top-${uid}.txt`)).OwnerId).toBe(rootGroups[0].ID);
    expect((await findResource(page, `jan-${uid}.txt`)).OwnerId).toBe(sub[0].ID);
    expect((await findResource(page, `feb-${uid}.txt`)).OwnerId).toBe(sub[0].ID);
  });

  test('turning off keep folder structure uploads flat into the current group', async ({ page, groupPage }) => {
    const root = `flat-${uid}`;
    await groupPage.gotoDisplay(groupId);
    await drop(page, [dir(root, [file(`flat-one-${uid}.txt`, `flat-${uid}`)])]);

    const modal = page.locator(MODAL);
    await expect(modal).toBeVisible();
    await modal.getByLabel('Keep folder structure').uncheck();
    await expect(modal.getByText(/will be created/)).toHaveCount(0);
    await expect(modal.getByLabel('Search group categories')).toBeHidden();

    await modal.getByRole('button', { name: 'Upload' }).click();
    await expect(modal).not.toBeVisible({ timeout: 10000 });

    expect(await findGroups(page, root)).toEqual([]);
    expect((await findResource(page, `flat-one-${uid}.txt`)).OwnerId).toBe(groupId);
  });

  test('the same folder name dropped twice makes two groups, never a merge', async ({ page, groupPage }) => {
    const root = `twice-${uid}`;
    await groupPage.gotoDisplay(groupId);
    await drop(page, [dir(root, [file(`twice-1-${uid}.txt`, `t1-${uid}`)])]);
    const modal = page.locator(MODAL);
    await expect(modal).toBeVisible();
    await drop(page, [dir(root, [file(`twice-2-${uid}.txt`, `t2-${uid}`)])]);
    await expect(modal.locator('input[aria-label^="Name for item"]')).toHaveCount(2);
    await expect(modal.getByText('2 groups will be created')).toBeVisible();

    await modal.getByRole('button', { name: 'Upload' }).click();
    await expect(modal).not.toBeVisible({ timeout: 15000 });

    const groups = await track(page, root);
    expect(groups).toHaveLength(2);
    const r1 = await findResource(page, `twice-1-${uid}.txt`);
    const r2 = await findResource(page, `twice-2-${uid}.txt`);
    expect(r1.OwnerId).not.toBe(r2.OwnerId);
  });

  test('retrying after a failed file reuses the groups already created', async ({ page, groupPage }) => {
    const root = `retry-${uid}`;
    await groupPage.gotoDisplay(groupId);
    await drop(page, [dir(root, [file(`retry-1-${uid}.txt`, `r1-${uid}`), file(`retry-2-${uid}.txt`, `r2-${uid}`)])]);
    const modal = page.locator(MODAL);
    await expect(modal).toBeVisible();

    let calls = 0;
    await page.route('**/v1/resource', (route) => {
      if (route.request().method() === 'POST' && ++calls === 1) return route.abort();
      return route.continue();
    });
    await modal.getByRole('button', { name: 'Upload' }).click();
    await expect(modal.getByRole('button', { name: 'Retry', exact: true })).toBeVisible({ timeout: 10000 });
    expect(await track(page, root)).toHaveLength(1);

    await modal.getByRole('button', { name: 'Retry', exact: true }).click();
    await expect(modal).not.toBeVisible({ timeout: 10000 });
    expect(await findGroups(page, root)).toHaveLength(1);
    const g = (await findGroups(page, root))[0];
    expect((await findResource(page, `retry-1-${uid}.txt`)).OwnerId).toBe(g.ID);
    expect((await findResource(page, `retry-2-${uid}.txt`)).OwnerId).toBe(g.ID);
  });

  test('two same-named folders in one drop make two groups', async ({ page, groupPage }) => {
    const root = `dup-root-${uid}`;
    await groupPage.gotoDisplay(groupId);
    await drop(page, [
      dir(root, [file(`dr-1-${uid}.txt`, `dr1-${uid}`)]),
      dir(root, [file(`dr-2-${uid}.txt`, `dr2-${uid}`)]),
    ]);
    const modal = page.locator(MODAL);
    await expect(modal).toBeVisible();
    await expect(modal.getByText('2 groups will be created')).toBeVisible();
    await modal.getByRole('button', { name: 'Upload' }).click();
    await expect(modal).not.toBeVisible({ timeout: 15000 });

    expect(await track(page, root)).toHaveLength(2);
    const r1 = await findResource(page, `dr-1-${uid}.txt`);
    const r2 = await findResource(page, `dr-2-${uid}.txt`);
    expect(r1.OwnerId).not.toBe(r2.OwnerId);
  });

  test('a drop that finishes reading mid-upload leaves the running batch alone', async ({ page, groupPage }) => {
    const late = `late-${uid}`;
    await groupPage.gotoDisplay(groupId);
    await drop(page, [file(`running-${uid}.txt`, `running-${uid}`)]);
    const modal = page.locator(MODAL);
    await expect(modal).toBeVisible();

    await page.route('**/v1/resource', async (route) => {
      await new Promise((r) => setTimeout(r, 2500));
      await route.continue();
    });
    // The drop starts reading while nothing is uploading; Upload is clicked
    // before the read ends, so the guard before the walk cannot catch it.
    await drop(page, [dir(late, [file(`late-1-${uid}.txt`, `l1-${uid}`)])], { delayMs: 1200 });
    await modal.getByRole('button', { name: 'Upload' }).click();
    await expect.poll(() => page.evaluate(() => (window as any).Alpine.store('pasteUpload').state)).toBe('uploading');

    await expect(page.getByRole('status').filter({ hasText: /wait for the current upload/i })).toBeVisible();
    await expect(modal).not.toBeVisible({ timeout: 15000 });
    expect((await findResource(page, `running-${uid}.txt`)).OwnerId).toBe(groupId);
    expect(await findGroups(page, late)).toEqual([]);
  });

  test('cancelling while a drop is still being read does not bring the modal back', async ({ page, groupPage }) => {
    await groupPage.gotoDisplay(groupId);
    await drop(page, [file(`first-${uid}.txt`, `first-${uid}`)]);
    const modal = page.locator(MODAL);
    await expect(modal).toBeVisible();

    await drop(page, [dir(`cancelled-${uid}`, [file(`cancelled-1-${uid}.txt`, `c1-${uid}`)])], { delayMs: 800 });
    await modal.getByRole('button', { name: 'Cancel' }).click();
    await expect(modal).not.toBeVisible();
    await page.waitForTimeout(2000);
    await expect(modal).not.toBeVisible();
    expect(await page.evaluate(() => (window as any).Alpine.store('pasteUpload').items.length)).toBe(0);
  });

  test('the group category does not outlive the folders it was chosen for', async ({ page, groupPage }) => {
    const root = `stale-${uid}`;
    await groupPage.gotoDisplay(groupId);
    await drop(page, [dir(root, [file(`stale-in-${uid}.txt`, `si-${uid}`)]), file(`stale-loose-${uid}.txt`, `sl-${uid}`)]);
    const modal = page.locator(MODAL);
    await expect(modal).toBeVisible();

    await modal.getByLabel('Search group categories').fill(folderCategoryName);
    await modal
      .locator('#paste-upload-group-category-listbox [role="option"]', { hasText: folderCategoryName })
      .click();
    await expect.poll(() => page.evaluate(() => (window as any).Alpine.store('pasteUpload').groupCategoryId)).toBe(folderCategoryId);

    // Only the loose file fails, so the folder's file is removed from the batch.
    await page.route('**/v1/resource', (route) =>
      route.request().postData()?.includes(`stale-loose-${uid}`) ? route.abort() : route.continue());
    await modal.getByRole('button', { name: 'Upload' }).click();
    await expect(modal.getByRole('button', { name: 'Retry', exact: true })).toBeVisible({ timeout: 10000 });
    await track(page, root);

    expect(await page.evaluate(() => (window as any).Alpine.store('pasteUpload').groupCategoryId)).toBeNull();
    await expect(modal.getByLabel('Search group categories')).toHaveCount(0);
  });

  test('a drop another handler already consumed does not open the modal', async ({ page, groupPage }) => {
    await groupPage.gotoDisplay(groupId);
    await page.evaluate(() => {
      const zone = document.createElement('div');
      zone.id = 'local-drop-zone';
      zone.textContent = 'local zone';
      zone.addEventListener('drop', (e) => e.preventDefault());
      document.body.prepend(zone);
    });
    await page.evaluate(() => {
      const f = new File(['local-' + Date.now()], 'local.txt');
      const dt: any = {
        types: ['Files'],
        items: [{ kind: 'file', webkitGetAsEntry: () => null, getAsFile: () => f }],
        files: [f],
      };
      const fire = (type: string, target: Element) => {
        const ev = new Event(type, { bubbles: true, cancelable: true });
        Object.defineProperty(ev, 'dataTransfer', { value: dt });
        target.dispatchEvent(ev);
      };
      const zone = document.getElementById('local-drop-zone')!;
      fire('dragenter', zone);
      fire('drop', zone);
    });
    await page.waitForTimeout(500);
    await expect(page.locator(MODAL)).not.toBeVisible();
    await expect(page.getByTestId('drop-overlay')).toBeHidden();
  });

  test('crossing a file input mid-drag keeps the overlay up', async ({ page, groupPage }) => {
    await groupPage.gotoDisplay(groupId);
    await page.evaluate(() => {
      const input = document.createElement('input');
      input.type = 'file';
      input.id = 'stray-file-input';
      document.body.prepend(input);
    });
    await page.evaluate(() => {
      const dt: any = { types: ['Files'], items: [], files: [] };
      const fire = (type: string, target: Element) => {
        const ev = new Event(type, { bubbles: true, cancelable: true });
        Object.defineProperty(ev, 'dataTransfer', { value: dt });
        target.dispatchEvent(ev);
      };
      const input = document.getElementById('stray-file-input')!;
      fire('dragenter', document.body);
      fire('dragenter', input);
      fire('dragleave', input);
    });
    await expect(page.getByTestId('drop-overlay')).toBeVisible();
  });

  test('a folder dropped inside the success window keeps the category the picker shows', async ({ page, groupPage }) => {
    const first = `win1-${uid}`;
    const second = `win2-${uid}`;
    await groupPage.gotoDisplay(groupId);
    await drop(page, [dir(first, [file(`win1-${uid}.txt`, `w1-${uid}`)])]);
    const modal = page.locator(MODAL);
    await expect(modal).toBeVisible();
    await modal.getByLabel('Search group categories').fill(folderCategoryName);
    await modal
      .locator('#paste-upload-group-category-listbox [role="option"]', { hasText: folderCategoryName })
      .click();
    await modal.getByRole('button', { name: 'Upload' }).click();
    await expect.poll(() => page.evaluate(() => (window as any).Alpine.store('pasteUpload').state)).toBe('success');

    await drop(page, [dir(second, [file(`win2-${uid}.txt`, `w2-${uid}`)])]);
    await expect(modal.locator('input[aria-label^="Name for item"]')).toHaveCount(1);
    // What the picker shows and what the store will send must agree.
    const shown = await modal.locator('#paste-upload-group-category-listbox').count();
    expect(shown).toBeGreaterThan(0);
    const stored = await page.evaluate(() => (window as any).Alpine.store('pasteUpload').groupCategoryId);
    await expect(modal.getByText(folderCategoryName).first()).toBeVisible();
    expect(stored).toBe(folderCategoryId);

    await modal.getByRole('button', { name: 'Upload' }).click();
    await expect(modal).not.toBeVisible({ timeout: 15000 });
    await track(page, first);
    const g = (await track(page, second))[0];
    expect(g.CategoryId).toBe(folderCategoryId);
    await findResource(page, `win1-${uid}.txt`);
    await findResource(page, `win2-${uid}.txt`);
  });

  test('a folder with only ignored content opens nothing and says why', async ({ page, groupPage }) => {
    await groupPage.gotoDisplay(groupId);
    await drop(page, [dir('only-junk', [file('.DS_Store', 'x'), dir('empty', [])])]);
    await expect(page.locator(MODAL)).not.toBeVisible();
    await expect(page.getByRole('status').filter({ hasText: /nothing to upload/i })).toBeVisible();
  });

  test('the overlay shows during a file drag, stays away for text drags, and clears on drop', async ({ page, groupPage }) => {
    await groupPage.gotoDisplay(groupId);
    const overlay = page.getByTestId('drop-overlay');
    await expect(overlay).toBeHidden();

    const prevented = await page.evaluate(() => {
      const dt: any = { types: ['text/plain'], items: [], files: [] };
      const ev = new Event('dragover', { bubbles: true, cancelable: true });
      Object.defineProperty(ev, 'dataTransfer', { value: dt });
      document.body.dispatchEvent(ev);
      return ev.defaultPrevented;
    });
    expect(prevented).toBe(false);
    await expect(overlay).toBeHidden();

    await drop(page, [file(`ov-${uid}.txt`, `ov-${uid}`)], { hold: true });
    await expect(overlay).toBeVisible();
    await expect(overlay).toContainText(groupName);
    await expect(overlay).toHaveAttribute('aria-hidden', 'true');

    await drop(page, [file(`ov-${uid}.txt`, `ov-${uid}`)]);
    await expect(overlay).toBeHidden();
    await page.locator(MODAL).getByRole('button', { name: 'Cancel' }).click();
  });

  test('a page with no upload target leaves the drop to the browser', async ({ page }) => {
    await page.goto('/groups');
    expect(await drop(page, [file(`none-${uid}.txt`, `none-${uid}`)])).toBe(false);
    await expect(page.locator(MODAL)).not.toBeVisible();
  });

  test.afterAll(async ({ apiClient }) => {
    for (const id of createdResourceIds) await apiClient.deleteResource(id).catch(() => {});
    for (const id of [...createdGroupIds].reverse()) await apiClient.deleteGroup(id).catch(() => {});
    if (groupId) await apiClient.deleteGroup(groupId).catch(() => {});
    for (const id of [categoryId, folderCategoryId]) {
      if (id) await apiClient.deleteCategory(id).catch(() => {});
    }
  });
});
