import { test, expect } from '../../fixtures/base.fixture';
import * as path from 'path';
import type { Page } from '@playwright/test';

/**
 * Product decision 145: the resource detail page's main preview opens the
 * lightbox, and an explicit "Open original" link keeps the workflow the old
 * behaviour was the only route to.
 *
 * Two identical-looking images used to behave differently — a thumbnail on this
 * same page called $store.lightbox.openFromClick, while the main preview 302'd
 * straight to /files/…. Both workflows have to survive: the lightbox is where
 * zoom, crop, rotate and navigate live, and "click the picture to get the file"
 * (save-as, copy URL, open elsewhere) is a real thing people do.
 *
 * The load-bearing case is the standalone fallback. This resource is absent from
 * the page's lightbox item list by construction (GetSeriesSiblings excludes
 * itself, and the sidebar is not a scanned container), so before the fallback
 * existed openFromClick would have found nothing and done nothing — a dead click,
 * which is worse than the plain link it replaced.
 */

const IMAGE = path.join(__dirname, '../../test-assets/sample-image.png');

// The same locator tests/lightbox/lightbox.spec.ts uses: the lightbox is an
// aria-modal dialog, distinguished from the other overlays by what it is not.
// The confirm dialog is role="alertdialog", so it never collides with this.
const lightbox = (page: Page) =>
  page.locator(
    '[role="dialog"][aria-modal="true"]:not([aria-labelledby="paste-upload-title"]):not([aria-labelledby="entity-picker-title"])'
  );

test.describe('resource main preview', () => {
  test('opens the lightbox rather than navigating to the file', async ({ page, apiClient }) => {
    const resource = await apiClient.createResource({ filePath: IMAGE, name: `preview-lightbox-${Date.now()}` });

    await page.goto(`/resource?id=${resource.ID}`);
    const preview = page.locator('[data-testid="resource-main-preview"]');
    await expect(preview).toBeVisible({ timeout: 5000 });

    await preview.click();

    // The lightbox opened, and the page did not navigate away.
    await expect(lightbox(page)).toBeVisible({
      timeout: 5000,
    });
    expect(page.url()).toContain(`/resource?id=${resource.ID}`);
  });

  test('"Open original" still reaches the file itself', async ({ page, apiClient }) => {
    const resource = await apiClient.createResource({ filePath: IMAGE, name: `preview-original-${Date.now()}` });

    await page.goto(`/resource?id=${resource.ID}`);
    const original = page.locator('[data-testid="resource-open-original"]');
    await expect(original).toBeVisible({ timeout: 5000 });

    // Same tab, no target="_blank" — a forced new window would be a regression.
    expect(await original.getAttribute('target')).toBeNull();

    const href = await original.getAttribute('href');
    expect(href).toContain(`/v1/resource/view?id=${resource.ID}`);

    // It resolves to the actual file rather than to an error page.
    const response = await page.request.get(href!);
    expect(response.ok()).toBe(true);
  });

  test('its accessible name contains its visible text (WCAG 2.5.3)', async ({ page, apiClient }) => {
    const name = `preview-label-${Date.now()}`;
    const resource = await apiClient.createResource({ filePath: IMAGE, name });

    await page.goto(`/resource?id=${resource.ID}`);
    const original = page.locator('[data-testid="resource-open-original"]');
    await expect(original).toBeVisible({ timeout: 5000 });

    const visible = (await original.textContent())?.trim() ?? '';
    const accessible = (await original.getAttribute('aria-label')) ?? '';
    expect(visible).toBe('Open original');
    expect(accessible.toLowerCase()).toContain(visible.toLowerCase());
  });

  test('closing a standalone preview leaves the page gallery navigable', async ({ page, apiClient }) => {
    // The regression this guards: a standalone open used to replace `items`, and
    // without the restore in close() every later thumbnail click on the page fell
    // into the standalone branch too, losing sibling-to-sibling navigation.
    const resource = await apiClient.createResource({ filePath: IMAGE, name: `preview-restore-${Date.now()}` });

    await page.goto(`/resource?id=${resource.ID}`);
    await page.locator('[data-testid="resource-main-preview"]').click();
    await expect(lightbox(page)).toBeVisible({
      timeout: 5000,
    });

    await page.keyboard.press('Escape');
    await expect(lightbox(page)).toBeHidden({
      timeout: 5000,
    });

    const parked = await page.evaluate(
      () => (window as any).Alpine?.store('lightbox')?._itemsBeforeStandalone ?? null
    );
    expect(parked).toBeNull();
  });
});
