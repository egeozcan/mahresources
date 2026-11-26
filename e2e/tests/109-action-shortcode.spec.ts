/**
 * E2E tests for the [action] shortcode.
 *
 * The server renders <action-shortcode> around a real <button>. Activating it
 * runs the plugin action through POST /v1/jobs/action/run, reports the outcome
 * next to the button and through the page's live region, and, after a
 * successful run, refreshes the [autorefresh] blocks for the entity. The role
 * checks (a guest sees no button) are in auth/action-shortcode-roles.spec.ts,
 * because this project's server runs with login off.
 */
import { test, expect } from '../fixtures/base.fixture';
import type { Page } from '@playwright/test';
import path from 'path';

const runPost = (r: { url(): string; request(): { method(): string } }) =>
  r.url().includes('/v1/jobs/action/run') && r.request().method() === 'POST';

const deferredPost = (r: { url(): string; request(): { method(): string } }) =>
  r.url().includes('/v1/shortcodes/deferred') && r.request().method() === 'POST';

// What document.activeElement settles on, once it has stopped changing for
// 600 ms: a refresh moves focus in steps, and an early sample can catch the
// state before the last one (docs/lessons.md, "expect.poll is satisfied by a
// transient early state").
async function settledFocus(page: Page): Promise<string> {
  let last = '';
  let stableSince = Date.now();
  const deadline = Date.now() + 10000;
  while (Date.now() < deadline) {
    const now = await page.evaluate(() => {
      const el = document.activeElement as HTMLElement | null;
      if (!el) return 'none';
      return `${el.tagName}|${el.className}|${(el as any).__actionMarker ? 'old' : 'new'}|${el.isConnected}`;
    });
    if (now !== last) {
      last = now;
      stableSince = Date.now();
    } else if (Date.now() - stableSince >= 600) {
      return last;
    }
    await page.waitForTimeout(100);
  }
  return last;
}

test.describe('[action] shortcode', () => {
  test.beforeAll(async ({ apiClient }) => {
    await apiClient.enablePlugin('test-action-button');
    await apiClient.enablePlugin('test-actions');
  });

  test('runs by keyboard after the confirmation, refreshes its block in place and keeps focus', async ({
    apiClient,
    page,
  }) => {
    const stamp = Date.now();
    const rc = await apiClient.createResourceCategory(`Action Cat ${stamp}`, 'action', {
      // The button comes first in the block, so focus returns to it.
      CustomSidebar:
        '<div class="act-wrap">[autorefresh][action plugin="test-action-button" name="mark-reviewed"] ' +
        '<span class="act-desc">[property path="Description"]</span>[/autorefresh]</div>',
    });
    const resource = await apiClient.createResource({
      filePath: path.join(__dirname, '../test-assets/sample-image-21.png'),
      name: `Action Resource ${stamp}`,
      description: 'fresh',
      resourceCategoryId: rc.ID,
    });

    try {
      await page.goto(`/resource?id=${resource.ID}`);
      await page.waitForLoadState('load');
      const url = page.url();

      const button = page.locator('.act-wrap action-shortcode button');
      await expect(button).toHaveText('Mark reviewed');
      await expect(page.locator('.act-desc')).toHaveText('fresh');

      await page.evaluate(() => {
        (window as any).__actionProbe = 'kept';
        (document.querySelector('.act-wrap action-shortcode button') as any).__actionMarker = true;
      });

      await button.focus();
      await page.keyboard.press('Enter');

      const dialog = page.getByRole('alertdialog');
      await expect(dialog).toBeVisible();
      await expect(dialog).toContainText('Mark as reviewed?');

      const ran = page.waitForResponse(runPost);
      const refreshed = page.waitForResponse(deferredPost);
      await dialog.getByRole('button', { name: 'Mark reviewed' }).click();
      expect((await ran).status()).toBe(200);
      await refreshed;

      await expect(page.locator('.act-desc')).toHaveText('reviewed');
      expect(page.url()).toBe(url);
      expect(await page.evaluate(() => (window as any).__actionProbe)).toBe('kept');

      // Announced through the page's live region, which outlives the refresh.
      await expect(
        page.locator('body > [aria-live="polite"]').filter({ hasText: `Marked resource ${resource.ID} reviewed` }),
      ).toHaveCount(1);

      // Focus ends on the re-rendered button, a new node, not on <body>.
      expect(await settledFocus(page)).toBe('BUTTON|action-shortcode-button|new|true');
    } finally {
      await apiClient.deleteResource(resource.ID).catch(() => {});
      await apiClient.deleteResourceCategory(rc.ID).catch(() => {});
    }
  });

  test('a declined confirmation runs nothing', async ({ apiClient, page }) => {
    const stamp = Date.now();
    const rc = await apiClient.createResourceCategory(`Action Decline Cat ${stamp}`, 'action', {
      CustomSidebar: '<div class="act-wrap">[action plugin="test-action-button" name="mark-reviewed"]</div>',
    });
    const resource = await apiClient.createResource({
      filePath: path.join(__dirname, '../test-assets/sample-image-21.png'),
      name: `Action Decline ${stamp}`,
      description: 'fresh',
      resourceCategoryId: rc.ID,
    });

    try {
      await page.goto(`/resource?id=${resource.ID}`);
      await page.waitForLoadState('load');
      let posted = false;
      page.on('request', (r) => {
        if (runPost(r)) posted = true;
      });

      const button = page.locator('.act-wrap action-shortcode button');
      await button.click();
      const dialog = page.getByRole('alertdialog');
      await expect(dialog).toBeVisible();
      await page.keyboard.press('Escape');
      await expect(dialog).toHaveCount(0);
      await expect(button).toBeFocused();
      await page.waitForTimeout(300);
      expect(posted).toBe(false);
      await expect(page.locator('.act-wrap .action-shortcode-status')).toHaveText('');
    } finally {
      await apiClient.deleteResource(resource.ID).catch(() => {});
      await apiClient.deleteResourceCategory(rc.ID).catch(() => {});
    }
  });

  test('follows an async action to the end of its Job', async ({ apiClient, page }) => {
    const stamp = Date.now();
    const rc = await apiClient.createResourceCategory(`Action Async Cat ${stamp}`, 'action', {
      CustomSidebar: '<div class="act-async">[action plugin="test-actions" name="async-demo" label="Run demo"]</div>',
    });
    const resource = await apiClient.createResource({
      filePath: path.join(__dirname, '../test-assets/sample-image-21.png'),
      name: `Action Async ${stamp}`,
      resourceCategoryId: rc.ID,
    });

    try {
      await page.goto(`/resource?id=${resource.ID}`);
      await page.waitForLoadState('load');
      const host = page.locator('.act-async action-shortcode');
      await expect(host).toHaveAttribute('data-async', 'true');

      const ran = page.waitForResponse(runPost);
      await host.getByRole('button', { name: 'Run demo' }).click();
      expect((await ran).status()).toBe(202);

      await expect(host.locator('.action-shortcode-status')).toHaveText('Succeeded', { timeout: 20000 });
      await expect(host.getByRole('button', { name: 'Run demo' })).not.toHaveAttribute('aria-disabled', 'true');
      await expect(
        page.locator('body > [aria-live="polite"]').filter({ hasText: 'Run demo: Succeeded' }),
      ).toHaveCount(1);
    } finally {
      await apiClient.deleteResource(resource.ID).catch(() => {});
      await apiClient.deleteResourceCategory(rc.ID).catch(() => {});
    }
  });
});
