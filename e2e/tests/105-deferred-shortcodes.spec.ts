/**
 * E2E tests for the [lazy] and [details] deferred-render shortcodes.
 *
 * Both defer their body to POST /v1/shortcodes/deferred on the main display
 * pages: [lazy] fetches when it scrolls into view, [details] fetches the first
 * time its native <details> is opened (keyboard/screen-reader accessible).
 */
import { test, expect } from '../fixtures/base.fixture';

test.describe('Deferred shortcodes ([lazy] / [details])', () => {
  test('[lazy] loads on view and [details] loads on open', async ({ apiClient, page }) => {
    const stamp = Date.now();

    const sidebar = [
      `<div class="lz-wrap">[lazy]<div class="lz-payload">LAZY-${stamp}</div>[/lazy]</div>`,
      `<div class="dt-wrap">[details summary="Show more ${stamp}"]<div class="dt-payload">DETAILS-${stamp}</div>[/details]</div>`,
    ].join('\n');

    const cat = await apiClient.createCategory(`Defer Cat ${stamp}`, 'defer', {
      CustomSidebar: sidebar,
    });
    const group = await apiClient.createGroup({ name: `Defer Group ${stamp}`, categoryId: cat.ID });

    const deferredPosts: string[] = [];
    page.on('request', (req) => {
      if (req.url().includes('/v1/shortcodes/deferred') && req.method() === 'POST') {
        deferredPosts.push(req.url());
      }
    });

    await page.goto(`/group?id=${group.ID}`);
    await page.waitForLoadState('load');

    // [lazy] emits a placeholder custom element rather than inline HTML...
    await expect(page.locator('lazy-shortcode')).toHaveCount(1);
    // ...and its body is fetched + injected when it comes into view.
    await page.locator('lazy-shortcode').scrollIntoViewIfNeeded();
    await expect(page.locator('.lz-payload')).toContainText(`LAZY-${stamp}`, { timeout: 8000 });

    // [details] is collapsed: its body has NOT been fetched or rendered yet.
    await expect(page.locator('.dt-payload')).toHaveCount(0);

    const summary = page.locator('details.details-shortcode > summary');
    await expect(summary).toHaveText(`Show more ${stamp}`);

    // Opening the disclosure triggers the on-open deferred fetch.
    const respPromise = page.waitForResponse(
      (r) => r.url().includes('/v1/shortcodes/deferred') && r.request().method() === 'POST',
    );
    await summary.click();
    await respPromise;
    await expect(page.locator('.dt-payload')).toContainText(`DETAILS-${stamp}`);

    // Both blocks went through the deferred endpoint (one for lazy, one for details).
    expect(deferredPosts.length).toBeGreaterThanOrEqual(2);

    await apiClient.deleteGroup(group.ID);
    await apiClient.deleteCategory(cat.ID);
  });

  test('[details] is operable with the keyboard', async ({ apiClient, page }) => {
    const stamp = Date.now();
    const sidebar = `<div class="kb-wrap">[details summary="Keyboard ${stamp}"]<div class="kb-payload">KB-${stamp}</div>[/details]</div>`;

    const cat = await apiClient.createCategory(`Defer KB Cat ${stamp}`, 'deferkb', { CustomSidebar: sidebar });
    const group = await apiClient.createGroup({ name: `Defer KB Group ${stamp}`, categoryId: cat.ID });

    await page.goto(`/group?id=${group.ID}`);
    await page.waitForLoadState('load');

    await expect(page.locator('.kb-payload')).toHaveCount(0);

    // Focus the native <summary> and open it with the keyboard.
    const summary = page.locator('details.details-shortcode > summary');
    await summary.focus();
    await page.keyboard.press('Enter');

    await expect(page.locator('.kb-payload')).toContainText(`KB-${stamp}`, { timeout: 8000 });

    await apiClient.deleteGroup(group.ID);
    await apiClient.deleteCategory(cat.ID);
  });

  test('inline fallback in live preview (no deferral)', async ({ apiClient, page }) => {
    const stamp = Date.now();

    // The live template preview does not install a signer, so [lazy]/[details]
    // render inline there — the author sees the content, and [details] stays a
    // plain collapsible <details>.
    const cat = await apiClient.createCategory(`Defer Prev Cat ${stamp}`, 'deferprev', {});
    const group = await apiClient.createGroup({ name: `Defer Prev Group ${stamp}`, categoryId: cat.ID });

    const resp = await page.request.post('/v1/category/previewTemplate', {
      data: {
        entityId: group.ID,
        content: `[lazy]<span class="prev-lazy">PREVLAZY-${stamp}</span>[/lazy][details summary="s"]<span class="prev-det">PREVDET-${stamp}</span>[/details]`,
      },
    });
    expect(resp.ok()).toBeTruthy();
    const body = await resp.json();
    // Inline: content is present in the returned HTML, no placeholder element.
    expect(body.html).toContain(`PREVLAZY-${stamp}`);
    expect(body.html).toContain(`PREVDET-${stamp}`);
    expect(body.html).not.toContain('<lazy-shortcode');
    expect(body.html).toContain('<details');

    await apiClient.deleteGroup(group.ID);
    await apiClient.deleteCategory(cat.ID);
  });
});
