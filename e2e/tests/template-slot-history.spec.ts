/**
 * Slot history popup on the template editors: each slot cluster's History
 * button lists the revisions that changed that slot, diffs one against what is
 * in the editor now (unsaved edits included), previews the old version, and
 * applies it to the editor without saving.
 */
import { test, expect } from '../fixtures/base.fixture';
import type { Page } from '@playwright/test';

async function postForm(page: Page, path: string, form: Record<string, string>) {
  const resp = await page.request.post(path, { form });
  expect(resp.ok(), `${path}: ${resp.status()} ${await resp.text()}`).toBeTruthy();
}

async function listRevisions(page: Page, prefix: string, id: number) {
  const resp = await page.request.get(`${prefix}/revisions?id=${id}`);
  expect(resp.ok()).toBeTruthy();
  return (await resp.json()) as { number: number; applied?: string[]; changedSlots?: string[] }[];
}

// Revisions: #1 create (header one), #2 header two + its CSS, #3 sidebar only,
// #4 header three.
async function seedCategory(page: Page, apiClient: { createCategory: Function; createGroup: Function }) {
  const name = `Slot History ${Date.now()}`;
  const cat = await apiClient.createCategory(name, undefined, { CustomHeader: '<p>one</p>' });
  const id = String(cat.ID);
  await postForm(page, '/v1/category', { ID: id, name, CustomHeader: '<p>two</p>', CustomHeaderCSS: '.two { color: rgb(1, 2, 3); }' });
  await postForm(page, '/v1/category', { ID: id, name, CustomSidebar: '<aside>side</aside>' });
  await postForm(page, '/v1/category', { ID: id, name, CustomHeader: '<p>three</p>' });
  await apiClient.createGroup({ name: `Slot History Target ${Date.now()}`, categoryId: cat.ID });
  return cat as { ID: number };
}

const dialog = (page: Page) => page.getByRole('dialog', { name: /^History of / });

async function openHistory(page: Page, slot: string) {
  await page.getByTestId(`slot-history-${slot}`).click();
  await expect(dialog(page)).toBeVisible();
  await expect(dialog(page).getByTestId('slot-history-loading')).toBeHidden();
}

test.describe('Slot history popup', () => {
  test('lists only the revisions that changed the slot', async ({ page, apiClient }) => {
    const cat = await seedCategory(page, apiClient);
    await page.goto(`/category/edit?id=${cat.ID}`);

    await openHistory(page, 'CustomHeader');
    await expect(dialog(page)).toHaveAccessibleName('History of Custom Header');
    const items = dialog(page).locator('[data-testid^="slot-history-rev-"]');
    await expect(items).toHaveCount(3);
    await expect(items.nth(0)).toHaveAttribute('data-testid', 'slot-history-rev-4');
    await expect(items.nth(1)).toHaveAttribute('data-testid', 'slot-history-rev-2');
    await expect(items.nth(2)).toHaveAttribute('data-testid', 'slot-history-rev-1');
    await page.keyboard.press('Escape');

    await openHistory(page, 'CustomSidebar');
    await expect(dialog(page).locator('[data-testid^="slot-history-rev-"]')).toHaveCount(1);
    await expect(dialog(page).getByTestId('slot-history-rev-3')).toBeVisible();
  });

  test('diffs against unsaved editor content and previews the old version', async ({ page, apiClient }) => {
    const cat = await seedCategory(page, apiClient);
    await page.goto(`/category/edit?id=${cat.ID}`);
    await page.locator('.cm-content[aria-label="Custom Header"]').fill('<p>draft</p>');

    await openHistory(page, 'CustomHeader');
    await dialog(page).getByTestId('slot-history-rev-1').check();
    const unified = dialog(page).getByTestId('slot-history-unified-CustomHeader');
    await expect(unified.locator('[data-op="delete"]')).toContainText('<p>one</p>');
    await expect(unified.locator('[data-op="insert"]')).toContainText('<p>draft</p>');

    await dialog(page).getByTestId('slot-history-layout-side').check();
    await expect(dialog(page).getByTestId('slot-history-side-CustomHeader').locator('td').first()).toContainText('<p>one</p>');

    const frame = page.frameLocator('iframe[title="Preview of revision #1"]');
    await expect(frame.locator('body')).toContainText('one');
    // A picture of the old version: not reachable by keyboard or pointer.
    const iframe = dialog(page).locator('iframe[title="Preview of revision #1"]');
    await expect(iframe).toHaveAttribute('tabindex', '-1');
    expect(await iframe.evaluate((el) => (el as HTMLElement).inert)).toBe(true);
  });

  test('a revision the same as the editor says so', async ({ page, apiClient }) => {
    const cat = await seedCategory(page, apiClient);
    await page.goto(`/category/edit?id=${cat.ID}`);
    await openHistory(page, 'CustomHeader');
    await dialog(page).getByTestId('slot-history-rev-4').check();
    await expect(dialog(page).getByTestId('slot-history-same')).toContainText('Same as the editor');
  });

  test('apply writes the slot, optionally its CSS, and the save records it', async ({ page, apiClient }) => {
    const cat = await seedCategory(page, apiClient);
    await page.goto(`/category/edit?id=${cat.ID}`);
    const css = page.locator('input[name="CustomHeaderCSS"]');
    const header = page.locator('input[name="CustomHeader"]');
    await expect(css).toHaveValue('.two { color: rgb(1, 2, 3); }');

    // Without its CSS: only the markup changes.
    await page.locator('.cm-content[aria-label="Custom Header CSS"]').fill('.mine {}');
    await openHistory(page, 'CustomHeader');
    await dialog(page).getByTestId('slot-history-rev-1').check();
    await dialog(page).getByTestId('slot-history-include-css').uncheck();
    await dialog(page).getByRole('button', { name: 'Apply #1 to the editor' }).click();
    await expect(dialog(page)).toHaveCount(0);
    await expect(header).toHaveValue('<p>one</p>');
    await expect(css).toHaveValue('.mine {}');
    // Focus stays at the slot: back on its History button, with the load
    // announced in the slot's preview. The form's notice keeps the record.
    await expect(page.getByTestId('slot-history-CustomHeader')).toBeFocused();
    const status = page.getByTestId('slot-applied-CustomHeader');
    await expect(status).toHaveAttribute('role', 'status');
    await expect(status).toContainText('Loaded CustomHeader from revision #1');
    await expect(status).toContainText('Nothing is saved yet');
    await expect(page.getByTestId('revision-load-notice')).toContainText('Loaded CustomHeader from revision #1');

    // With it (the default).
    await openHistory(page, 'CustomHeader');
    await dialog(page).getByTestId('slot-history-rev-2').check();
    await expect(dialog(page).getByTestId('slot-history-include-css')).toBeChecked();
    await dialog(page).getByRole('button', { name: 'Apply #2 to the editor' }).click();
    await expect(header).toHaveValue('<p>two</p>');
    await expect(css).toHaveValue('.two { color: rgb(1, 2, 3); }');

    // Applying the same revision again is announced again: the status empties first.
    await page.locator('.cm-content[aria-label="Custom Header"]').fill('<p>edited</p>');
    await expect(status).toContainText('revision #2');
    await openHistory(page, 'CustomHeader');
    await dialog(page).getByTestId('slot-history-rev-2').check();
    await status.evaluate((el) => {
      const seen: string[] = [];
      (window as unknown as { __statusTexts: string[] }).__statusTexts = seen;
      new MutationObserver(() => seen.push(el.textContent || '')).observe(el, { childList: true, characterData: true, subtree: true });
    });
    await dialog(page).getByRole('button', { name: 'Apply #2 to the editor' }).click();
    await expect(header).toHaveValue('<p>two</p>');
    await expect
      .poll(() => page.evaluate(() => (window as unknown as { __statusTexts: string[] }).__statusTexts))
      .toEqual(['', 'Loaded CustomHeader from revision #2. Review the form, then save. Nothing is saved yet.']);

    // Nothing was saved until now.
    expect((await listRevisions(page, '/v1/category', cat.ID))[0].number).toBe(4);
    await page.getByRole('button', { name: 'Save', exact: true }).click();
    await page.waitForURL(/\/category\?id=/);
    const newest = (await listRevisions(page, '/v1/category', cat.ID))[0];
    expect(newest.number).toBe(5);
    expect(newest.applied).toEqual(expect.arrayContaining(['loaded:1:CustomHeader', 'loaded:2:CustomHeader']));
  });

  test('a late response for an earlier selection never replaces the current one', async ({ page, apiClient }) => {
    const cat = await seedCategory(page, apiClient);
    let release!: () => void;
    const held = new Promise<void>((r) => { release = r; });
    await page.route(/\/v1\/category\/revisions\/1\?/, async (route) => {
      await held;
      await route.continue();
    });
    await page.goto(`/category/edit?id=${cat.ID}`);
    await openHistory(page, 'CustomHeader');
    await dialog(page).getByTestId('slot-history-rev-1').check();
    await dialog(page).getByTestId('slot-history-rev-2').check();
    const unified = dialog(page).getByTestId('slot-history-unified-CustomHeader');
    await expect(unified.locator('[data-op="delete"]')).toContainText('<p>two</p>');
    release();
    await page.waitForTimeout(500);
    await expect(unified.locator('[data-op="delete"]')).toContainText('<p>two</p>');
    await expect(dialog(page).getByRole('button', { name: 'Apply #2 to the editor' })).toBeEnabled();
    await expect(dialog(page).getByRole('button', { name: 'Apply #1 to the editor' })).toHaveCount(0);
  });

  test('Escape and Close return focus to the History button; Tab stays inside', async ({ page, apiClient }) => {
    const cat = await seedCategory(page, apiClient);
    await page.goto(`/category/edit?id=${cat.ID}`);
    const opener = page.getByTestId('slot-history-CustomHeader');

    await opener.focus();
    await page.keyboard.press('Enter');
    await expect(dialog(page)).toBeVisible();
    await dialog(page).getByTestId('slot-history-rev-2').check();
    for (let i = 0; i < 15; i++) {
      await page.keyboard.press('Tab');
      expect(await dialog(page).evaluate((el) => el.contains(document.activeElement))).toBe(true);
    }
    await page.keyboard.press('Escape');
    await expect(dialog(page)).toHaveCount(0);
    await expect(opener).toBeFocused();

    // Opened by a pointer click on a button that did not hold focus.
    await page.locator('body').click({ position: { x: 1, y: 1 } });
    await opener.click();
    await expect(dialog(page)).toBeVisible();
    await dialog(page).getByRole('button', { name: 'Close' }).click();
    await expect(dialog(page)).toHaveCount(0);
    await expect(opener).toBeFocused();
  });

  test('Custom CSS history shows a diff and no preview frame', async ({ page, apiClient }) => {
    const name = `CSS History ${Date.now()}`;
    const cat = await apiClient.createCategory(name, undefined, { CustomCSS: '.a { color: red; }' });
    await postForm(page, '/v1/category', { ID: String(cat.ID), name, CustomCSS: '.b { color: blue; }' });
    await page.goto(`/category/edit?id=${cat.ID}`);

    await openHistory(page, 'CustomCSS');
    await dialog(page).getByTestId('slot-history-rev-1').check();
    await expect(dialog(page).getByTestId('slot-history-unified-CustomCSS').locator('[data-op="delete"]')).toContainText('.a { color: red; }');
    await expect(dialog(page).getByTestId('slot-history-css-note')).toContainText('no markup');
    await expect(dialog(page).locator('iframe')).toHaveCount(0);

    // Custom CSS has no preview, and still announces its Apply beside its button.
    await dialog(page).getByRole('button', { name: 'Apply #1 to the editor' }).click();
    await expect(page.getByTestId('slot-history-CustomCSS')).toBeFocused();
    await expect(page.getByTestId('slot-applied-CustomCSS')).toContainText('Loaded CustomCSS from revision #1');
    await expect(page.locator('input[name="CustomCSS"]')).toHaveValue('.a { color: red; }');
  });

  test('the create form has no History buttons', async ({ page }) => {
    await page.goto('/category/new');
    await expect(page.locator('[data-testid^="slot-history-"]')).toHaveCount(0);
  });

  test('note type and resource category editors have slot history too', async ({ page, apiClient }) => {
    const nt = await apiClient.createNoteType(`NT History ${Date.now()}`, undefined, { CustomSidebar: '<p>nt-one</p>' });
    await page.goto(`/noteType/edit?id=${nt.ID}`);
    await openHistory(page, 'CustomSidebar');
    await expect(dialog(page).getByTestId('slot-history-rev-1')).toBeVisible();
    await page.keyboard.press('Escape');

    const rc = await apiClient.createResourceCategory(`RC History ${Date.now()}`, undefined, { CustomHeader: '<p>rc-one</p>' });
    await page.goto(`/resourceCategory/edit?id=${rc.ID}`);
    await expect(page.getByTestId('slot-history-CustomPreview')).toBeVisible();
    await openHistory(page, 'CustomHeader');
    await expect(dialog(page).getByTestId('slot-history-rev-1')).toBeVisible();
  });
});
