/**
 * E2E tests for the [lazy] and [details] deferred-render shortcodes.
 *
 * Both defer their body to POST /v1/shortcodes/deferred on the main display
 * pages: [lazy] fetches when it scrolls into view, [details] fetches the first
 * time its native <details> is opened (keyboard/screen-reader accessible).
 */
import { test, expect } from '../fixtures/base.fixture';
import path from 'path';

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

  test('[lazy] and editable [meta] custom resource summary survive lightbox refresh morph', async ({ apiClient, page }) => {
    const stamp = Date.now();
    let resourceId: number | undefined;
    let categoryId: number | undefined;
    const initialStatus = `pending-${stamp}`;
    const nextStatus = `active-${stamp}`;

    try {
      const category = await apiClient.createResourceCategory(`Defer Morph RC ${stamp}`, 'defermorph', {
        CustomSummary: [
          `<div class="morph-lazy-wrap">[lazy]<span class="morph-lazy-payload">Lazy name: [property path="Name"]</span>[/lazy]</div>`,
          `<div class="morph-meta-wrap" style="display: flex; gap: 1rem;">`,
          `  <div style="flex: 1 1 200px;">`,
          `    <label>Status</label>`,
          `    <div style="border: 1px solid #d1d5db; border-radius: 0.375rem; padding: 0.5rem; background: #f9fafb;">[meta path="status" editable="true"]</div>`,
          `  </div>`,
          `</div>`,
        ].join('\n'),
      });
      categoryId = category.ID;

      const resource = await apiClient.createResource({
        filePath: path.join(__dirname, '../test-assets/sample-image-21.png'),
        name: `Defer Morph Resource ${stamp}`,
        resourceCategoryId: category.ID,
        meta: JSON.stringify({ status: initialStatus }),
      });
      resourceId = resource.ID;

      await page.goto(`/resources?ResourceCategoryId=${category.ID}`);
      await page.waitForLoadState('load');

      await expect(page.locator('lazy-shortcode')).toHaveCount(1);
      await page.locator('lazy-shortcode').scrollIntoViewIfNeeded();
      await expect(page.locator('.morph-lazy-payload')).toContainText(`Defer Morph Resource ${stamp}`, {
        timeout: 8000,
      });
      const metaWrap = page.locator('.morph-meta-wrap');
      await expect(metaWrap.locator('meta-shortcode')).toContainText(initialStatus);
      await expect(metaWrap.getByRole('button', { name: 'Edit Status' })).toBeVisible();

      await page.locator(`[data-lightbox-item][data-resource-id="${resource.ID}"]`).click();
      const lightbox = page.locator('[role="dialog"][aria-modal="true"]:not([aria-labelledby="paste-upload-title"]):not([aria-labelledby="entity-picker-title"])');
      await expect(lightbox).toBeVisible();

      await lightbox.locator('button[title="Resource info"]').click();
      const editPanel = lightbox.locator('[data-edit-panel]');
      await expect(editPanel).toBeVisible();

      const nameInput = editPanel.locator('#lightbox-edit-name');
      await expect(nameInput).toBeVisible();
      const nextName = `Defer Morph Renamed ${stamp}`;
      const saveResponse = page.waitForResponse(
        (r) => r.url().includes('/v1/resource/editName') && r.request().method() === 'POST',
      );
      await nameInput.fill(nextName);
      await nameInput.blur();
      await saveResponse;
      await apiClient.editMeta('resource', resource.ID, 'status', JSON.stringify(nextStatus));

      const refreshResponse = page.waitForResponse(
        (r) => r.url().includes('/resources.body') && r.request().method() === 'GET',
      );
      await page.keyboard.press('e');
      await refreshResponse;
      await expect(editPanel).toBeHidden();

      await expect(page.locator('.morph-lazy-payload')).toContainText(nextName, { timeout: 8000 });
      await expect(page.locator('lazy-shortcode noscript')).toHaveCount(0);
      await expect(metaWrap.locator('meta-shortcode')).toContainText(nextStatus);
      await expect(metaWrap.getByRole('button', { name: 'Edit Status' })).toBeVisible();
    } finally {
      if (resourceId) await apiClient.deleteResource(resourceId).catch(() => {});
      if (categoryId) await apiClient.deleteResourceCategory(categoryId).catch(() => {});
    }
  });
});
