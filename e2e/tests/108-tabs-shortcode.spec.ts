/**
 * E2E tests for the [tabs] shortcode.
 *
 * The server renders every panel stacked under a hidden tab list; the
 * <tabs-shortcode> element upgrades that to the WAI-ARIA tabs pattern. A
 * lazy="true" tab is a [lazy] block, fetched from POST /v1/shortcodes/deferred
 * only once its panel is shown. These run on a group page, whose CustomHeader
 * holds one tab set with an eager tab, an eager tab containing a region
 * [reload], and a lazy tab with its own [reload].
 */
import { test, expect } from '../fixtures/base.fixture';
import type { Page, Request } from '@playwright/test';

const isDeferredPost = (r: Request) => r.url().includes('/v1/shortcodes/deferred') && r.method() === 'POST';

const HEADER = [
  '[tabs label="Project views"]',
  '[tab label="Overview"]<p class="tb-over">OVERVIEW</p>[/tab]',
  '[tab label="Notes"]<p class="tb-notes">NOTES-[meta path="status" inline="true"]</p>[reload label="Refresh notes"][/tab]',
  '[tab label="Files" lazy="true"]<p class="tb-lazy">LAZY-[meta path="status" inline="true"]</p>[reload label="Refresh files"][/tab]',
  '[/tabs]',
].join('\n');

/** 1x1 PNG, pasted to drive paste-upload's whole-page morph. */
const TINY_PNG_B64 = 'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAIAAACQd1PeAAAADElEQVR4nGP4z8AAAAMBAQDJ/pLvAAAAAElFTkSuQmCC';

async function pasteImage(page: Page) {
  await page.evaluate((b64) => {
    const binary = atob(b64);
    const bytes = new Uint8Array(binary.length);
    for (let i = 0; i < binary.length; i++) bytes[i] = binary.charCodeAt(i);
    const dt = new DataTransfer();
    dt.items.add(new File([bytes], 'tabs-paste.png', { type: 'image/png' }));
    const event = new Event('paste', { bubbles: true, cancelable: true });
    Object.defineProperty(event, 'clipboardData', { value: dt });
    window.dispatchEvent(event);
  }, TINY_PNG_B64);
}

test.describe('[tabs] shortcode', () => {
  let categoryId: number;
  let groupId: number;
  let deferredPosts: number;

  test.beforeEach(async ({ apiClient, page }) => {
    const stamp = `${Date.now()}-${Math.floor(Math.random() * 1e6)}`;
    categoryId = (await apiClient.createCategory(`Tabs Cat ${stamp}`, 'tabs', { CustomHeader: HEADER })).ID;
    groupId = (await apiClient.createGroup({
      name: `Tabs Group ${stamp}`,
      categoryId,
      meta: JSON.stringify({ status: 'before' }),
    })).ID;
    deferredPosts = 0;
    page.on('request', (r) => {
      if (isDeferredPost(r)) deferredPosts++;
    });
  });

  test.afterEach(async ({ apiClient }) => {
    if (groupId) await apiClient.deleteGroup(groupId).catch(() => {});
    if (categoryId) await apiClient.deleteCategory(categoryId).catch(() => {});
  });

  const tablist = (page: Page) => page.getByRole('tablist', { name: 'Project views' });
  const tab = (page: Page, name: string) => tablist(page).getByRole('tab', { name, exact: true });

  test('the keyboard reaches one tab stop, moves without opening, and a lazy tab loads when opened', async ({ page }) => {
    await page.goto(`/group?id=${groupId}`);
    await expect(tablist(page)).toBeVisible();
    await expect(tablist(page).getByRole('tab')).toHaveCount(3);
    await expect(page.getByRole('tabpanel')).toHaveCount(1);
    await expect(page.locator('.tb-over')).toBeVisible();
    await expect(page.locator('.tb-notes')).toBeHidden();
    await expect(page.locator('lazy-shortcode')).toBeHidden();

    // Tab from the top of the page: the set is one stop, on the selected tab.
    let reached = false;
    for (let i = 0; i < 80 && !reached; i++) {
      await page.keyboard.press('Tab');
      const focused = await page.evaluate(() => document.activeElement?.getAttribute('role') === 'tab' ? document.activeElement.textContent : null);
      if (focused) {
        expect(focused).toBe('Overview');
        reached = true;
      }
    }
    expect(reached).toBe(true);
    expect(deferredPosts).toBe(0);

    // Arrows move focus without selecting, so arrowing past the lazy tab loads nothing.
    await page.keyboard.press('ArrowRight');
    await expect(tab(page, 'Notes')).toBeFocused();
    await page.keyboard.press('ArrowRight');
    await expect(tab(page, 'Files')).toBeFocused();
    await expect(tab(page, 'Overview')).toHaveAttribute('aria-selected', 'true');
    await page.waitForTimeout(300);
    expect(deferredPosts).toBe(0);

    const loaded = page.waitForResponse((r) => isDeferredPost(r.request()));
    await page.keyboard.press('Enter');
    await loaded;
    await expect(tab(page, 'Files')).toHaveAttribute('aria-selected', 'true');
    await expect(page.locator('.tb-lazy')).toHaveText('LAZY-before');
    await expect(page.locator('.tb-over')).toBeHidden();
    expect(deferredPosts).toBe(1);

    // Tab moves into the open panel; Shift+Tab comes back to the selected tab.
    await page.keyboard.press('Tab');
    await expect(page.getByRole('button', { name: 'Refresh files' })).toBeFocused();
    await page.keyboard.press('Shift+Tab');
    await expect(tab(page, 'Files')).toBeFocused();

    // The arrows wrap, and Home and End reach the ends.
    await page.keyboard.press('ArrowRight');
    await expect(tab(page, 'Overview')).toBeFocused();
    await page.keyboard.press('End');
    await expect(tab(page, 'Files')).toBeFocused();
    await page.keyboard.press('Home');
    await expect(tab(page, 'Overview')).toBeFocused();
    await page.keyboard.press('Space');
    await expect(tab(page, 'Overview')).toHaveAttribute('aria-selected', 'true');
    await expect(page.locator('.tb-over')).toBeVisible();
  });

  test('a [reload] in a lazy tab refreshes that tab, which stays open', async ({ apiClient, page }) => {
    await page.goto(`/group?id=${groupId}`);
    await tab(page, 'Files').click();
    await expect(page.locator('.tb-lazy')).toHaveText('LAZY-before');

    await apiClient.editMeta('group', groupId, 'status', JSON.stringify('after'));
    const reloaded = page.waitForResponse((r) => isDeferredPost(r.request()));
    await page.getByRole('button', { name: 'Refresh files' }).click();
    await reloaded;

    await expect(page.locator('.tb-lazy')).toHaveText('LAZY-after');
    await expect(tab(page, 'Files')).toHaveAttribute('aria-selected', 'true');
    // Only the lazy block re-rendered: the eager tab still shows the page-load value.
    await expect(page.locator('.tb-notes')).toHaveText('NOTES-before');
    expect(deferredPosts).toBe(2);
  });

  test('a [reload] in an eager tab re-renders the slot and keeps that tab open and focused', async ({ apiClient, page }) => {
    await page.goto(`/group?id=${groupId}`);
    await tab(page, 'Notes').click();
    await expect(page.locator('.tb-notes')).toHaveText('NOTES-before');

    await apiClient.editMeta('group', groupId, 'status', JSON.stringify('after'));
    const button = page.getByRole('button', { name: 'Refresh notes' });
    await button.focus();
    const reloaded = page.waitForResponse((r) => isDeferredPost(r.request()));
    await page.keyboard.press('Enter');
    await reloaded;

    await expect(page.locator('.tb-notes')).toHaveText('NOTES-after');
    await expect(tab(page, 'Notes')).toHaveAttribute('aria-selected', 'true');
    await expect(page.locator('.tb-over')).toBeHidden();
    await expect(page.getByRole('button', { name: 'Refresh notes' })).toBeFocused();
    // The lazy tab came back unrevealed and stays unfetched while hidden.
    await page.waitForTimeout(300);
    expect(deferredPosts).toBe(1);
  });

  test('a page morph keeps the open tab and its focus, and does not re-fetch a lazy tab the reader left', async ({ page }) => {
    await page.goto(`/group?id=${groupId}`);
    await tab(page, 'Files').click();
    await expect(page.locator('.tb-lazy')).toHaveText('LAZY-before');
    expect(deferredPosts).toBe(1);
    await tab(page, 'Overview').click();
    await expect(tab(page, 'Overview')).toBeFocused();

    // Paste-upload re-fetches the page and morphs .main in place. The dialog
    // hands focus back to the Overview tab when it closes, before the morph.
    await pasteImage(page);
    const modal = page.locator('[role="dialog"][aria-labelledby="paste-upload-title"]');
    await expect(modal).toBeVisible({ timeout: 5000 });
    const refetched = page.waitForResponse((r) => r.url().includes(`/group?id=${groupId}`) && r.request().method() === 'GET');
    await modal.getByRole('button', { name: 'Upload' }).click();
    await refetched;
    await expect(modal).toBeHidden({ timeout: 10000 });
    await expect(page.getByRole('link', { name: 'tabs-paste.png' }).first()).toBeAttached({ timeout: 10000 });

    await expect(tab(page, 'Overview')).toHaveAttribute('aria-selected', 'true');
    await expect(tab(page, 'Overview')).toBeFocused();
    await expect(page.locator('.tb-over')).toBeVisible();
    await expect(page.locator('lazy-shortcode')).toBeHidden();
    await page.waitForTimeout(500);
    expect(deferredPosts).toBe(1);

    const loaded = page.waitForResponse((r) => isDeferredPost(r.request()));
    await tab(page, 'Files').click();
    await loaded;
    await expect(page.locator('.tb-lazy')).toHaveText('LAZY-before');
    expect(deferredPosts).toBe(2);
  });

  test('the live preview renders a lazy tab inline', async ({ page }) => {
    const resp = await page.request.post('/v1/category/previewTemplate', {
      data: { entityId: groupId, content: '[tabs][tab label="A"]eager[/tab][tab label="B" lazy="true"]<span class="pv">INLINE</span>[/tab][/tabs]' },
    });
    expect(resp.ok()).toBeTruthy();
    const { html } = await resp.json();
    expect(html).toContain('<tabs-shortcode');
    expect(html).toContain('<div class="lazy-content"><span class="pv">INLINE</span></div>');
    expect(html).not.toContain('<lazy-shortcode');
  });

  test('long labels wrap at 320 px instead of scrolling sideways', async ({ apiClient, page }) => {
    const long = 'A rather long tab label that cannot fit on one narrow line';
    await apiClient.updateCategory(categoryId, {
      name: `Tabs Narrow ${Date.now()}`,
      CustomHeader: `[tabs label="Narrow"][tab label="${long} one"]a[/tab][tab label="${long} two"]b[/tab][tab label="Short"]c[/tab][/tabs]`,
    });
    await page.setViewportSize({ width: 320, height: 800 });
    await page.goto(`/group?id=${groupId}`);
    const host = page.locator('tabs-shortcode');
    await expect(page.getByRole('tablist', { name: 'Narrow' })).toBeVisible();

    const fits = await host.evaluate((el) => el.scrollWidth <= el.clientWidth);
    expect(fits).toBe(true);
    for (const t of await page.getByRole('tablist', { name: 'Narrow' }).getByRole('tab').all()) {
      const box = (await t.boundingBox())!;
      expect(box.x).toBeGreaterThanOrEqual(0);
      expect(box.x + box.width).toBeLessThanOrEqual(320);
    }
  });
});
