import { test, expect } from '../../fixtures/base.fixture';
import type { Page } from '@playwright/test';
import path from 'path';

const LIGHTBOX =
  '[role="dialog"][x-show="$store.lightbox.isOpen"]';

/**
 * Keyboard and focus handling inside the lightbox: keys pressed in a panel or a
 * popover act on that control and never leak to the viewer's window shortcuts.
 */
test.describe('Lightbox keyboard and focus fixes', () => {
  let categoryId: number;
  let ownerGroupId: number;
  const createdResourceIds: number[] = [];
  const testRunId = Date.now();

  test.beforeAll(async ({ apiClient }) => {
    const category = await apiClient.createCategory(
      `Lightbox Keys Category ${testRunId}`,
      'Category for lightbox keyboard tests'
    );
    categoryId = category.ID;

    const ownerGroup = await apiClient.createGroup({
      name: `Lightbox Keys Owner ${testRunId}`,
      description: 'Owner for lightbox keyboard test resources',
      categoryId,
    });
    ownerGroupId = ownerGroup.ID;

    const imageFiles = [
      path.join(__dirname, '../../test-assets/sample-image-13.png'),
      path.join(__dirname, '../../test-assets/sample-image-2.png'),
      path.join(__dirname, '../../test-assets/sample-image-3.png'),
    ];
    for (let i = 0; i < imageFiles.length; i++) {
      const resource = await apiClient.createResource({
        filePath: imageFiles[i],
        name: `Lightbox Keys Image ${i + 1} - ${testRunId}`,
        description: `Keys description ${i + 1}`,
        ownerId: ownerGroupId,
      });
      createdResourceIds.push(resource.ID);
    }
  });

  test.afterAll(async ({ apiClient }) => {
    for (const resourceId of createdResourceIds) {
      try {
        await apiClient.deleteResource(resourceId);
      } catch {
        /* ignore cleanup errors */
      }
    }
    if (ownerGroupId) await apiClient.deleteGroup(ownerGroupId);
    if (categoryId) await apiClient.deleteCategory(categoryId);
  });

  async function openLightbox(page: Page) {
    await page.goto(`/resources?OwnerId=${ownerGroupId}`);
    await page.waitForLoadState('load');
    await page.locator('[data-lightbox-item]').first().click();
    const lightbox = page.locator(LIGHTBOX);
    await expect(lightbox).toBeVisible();
    return lightbox;
  }

  const store = (page: Page) =>
    page.evaluate(() => {
      const s = (window as any).Alpine.store('lightbox');
      return { isOpen: s.isOpen, currentIndex: s.currentIndex, isFullscreen: s.isFullscreen };
    });

  async function openInfoPanel(page: Page, lightbox: ReturnType<Page['locator']>) {
    await lightbox.locator('button[title="Resource info"]').click();
    const name = page.locator('#lightbox-edit-name');
    await expect(name).toHaveValue(/Lightbox Keys Image \d/);
    // openEditPanel moves focus to its close button a frame after the details land; wait for
    // it, or that move can steal focus from the field a test is about to type into.
    await expect(page.getByRole('button', { name: 'Close info panel' })).toBeFocused();
    const id: number = await page.evaluate(() => (window as any).Alpine.store('lightbox').getCurrentItem().id);
    return { name, id };
  }

  test('Escape in the Name field reverts the edit and keeps the viewer open', async ({ page, apiClient }) => {
    const lightbox = await openLightbox(page);
    const { name, id } = await openInfoPanel(page, lightbox);
    const original = await name.inputValue();

    await name.fill('should not be saved');
    await name.press('Escape');

    await expect(name).toHaveValue(original);
    expect((await store(page)).isOpen).toBe(true);
    const resource = await apiClient.getResource(id);
    expect(resource.Name).toBe(original);
  });

  test('Enter in the Name field saves without toggling fullscreen', async ({ page, apiClient }) => {
    const lightbox = await openLightbox(page);
    const { name, id } = await openInfoPanel(page, lightbox);
    const original = await name.inputValue();
    const renamed = `${original} renamed`;

    await page.evaluate(() => {
      const s = (window as any).Alpine.store('lightbox');
      (window as any).__fullscreenToggles = 0;
      const orig = s.toggleFullscreen;
      s.toggleFullscreen = function () { (window as any).__fullscreenToggles++; return orig.call(this); };
    });

    await name.fill(renamed);
    await name.press('Enter');

    await expect.poll(async () => (await apiClient.getResource(id)).Name).toBe(renamed);
    expect(await page.evaluate(() => (window as any).__fullscreenToggles)).toBe(0);

    // Restore for the other tests.
    await name.fill(original);
    await name.press('Enter');
    await expect.poll(async () => (await apiClient.getResource(id)).Name).toBe(original);
  });

  test('PageDown in the Description field does not change the item', async ({ page }) => {
    const lightbox = await openLightbox(page);
    await openInfoPanel(page, lightbox);
    const start = (await store(page)).currentIndex;
    const description = page.locator('#lightbox-edit-description');
    await description.focus();
    await page.keyboard.press('PageDown');
    expect((await store(page)).currentIndex).toBe(start);

    // Outside a field PageDown still pages.
    await lightbox.locator('button[aria-label="Close info panel"]').click();
    await page.keyboard.press('PageDown');
    await expect.poll(async () => (await store(page)).currentIndex).toBe(start + 1);
  });

  test('Escape on the "Add tag?" confirmation cancels it without closing the viewer', async ({ page }) => {
    const lightbox = await openLightbox(page);
    await lightbox.locator('button[title="Edit tags"]').click();
    const input = page.locator('[data-quick-tag-panel] [data-tag-editor-input]');
    await expect(input).toBeVisible();

    // Enter on the active "Create" row creates outright; the confirmation is the core's
    // request-create-confirmation state, so enter it the way the adapter's unit tests do
    // (selectorFieldAdapter.test.ts). If that command is renamed, this fails loudly here.
    await input.evaluate((el, label) => {
      (window as any).Alpine.$data(el)._core.dispatch({ type: 'request-create-confirmation', label });
    }, `brand-new-tag-${testRunId}`);
    const confirm = page.locator('[data-tag-confirm-ui] button').first();
    await expect(confirm).toBeFocused();

    await page.keyboard.press('Escape');

    await expect(page.locator('[data-tag-confirm-ui]')).toHaveCount(0);
    expect((await store(page)).isOpen).toBe(true);
    await expect(page.locator('[data-quick-tag-panel] [data-tag-editor-input]')).toBeFocused();
  });

  test('closing the tags panel returns focus to its toggle', async ({ page }) => {
    const lightbox = await openLightbox(page);
    await lightbox.locator('button[title="Edit tags"]').click();
    await page.locator('button[aria-label="Close edit tags panel"]').click();
    await expect(lightbox.locator('button[title="Edit tags"]')).toBeFocused();
  });

  test('closing the info panel returns focus to its toggle', async ({ page }) => {
    const lightbox = await openLightbox(page);
    await lightbox.locator('button[title="Resource info"]').click();
    await page.locator('button[aria-label="Close info panel"]').click();
    await expect(lightbox.locator('button[title="Resource info"]')).toBeFocused();
  });

  test('Escape in the zoom popover closes only the popover', async ({ page }) => {
    const lightbox = await openLightbox(page);
    await page.waitForFunction(() => {
      const img = document.querySelector('[role="dialog"] img') as HTMLImageElement | null;
      return img && img.complete && img.naturalWidth > 0 && img.clientWidth > 0;
    });
    const zoomButton = lightbox.locator('button[title="Choose zoom level"]');
    await expect(zoomButton).toBeVisible();
    await expect(zoomButton).toHaveAttribute('aria-expanded', 'false');
    await zoomButton.click();

    const popover = page.locator('#zoom-preset-popover');
    await expect(popover).toBeVisible();
    await expect(zoomButton).toHaveAttribute('aria-expanded', 'true');
    // Focus moves into the popover on open.
    expect(await popover.evaluate(p => p.contains(document.activeElement))).toBe(true);

    // From a preset button (the slider stops its own keys).
    await popover.getByRole('button').first().focus();
    await page.keyboard.press('Escape');

    await expect(popover).toBeHidden();
    expect((await store(page)).isOpen).toBe(true);
    await expect(zoomButton).toBeFocused();
    await expect(zoomButton).toHaveAttribute('aria-expanded', 'false');
  });

  test('Escape straight after opening the zoom popover closes only the popover', async ({ page }) => {
    const lightbox = await openLightbox(page);
    await page.waitForFunction(() => {
      const img = document.querySelector('[role="dialog"] img') as HTMLImageElement | null;
      return img && img.complete && img.naturalWidth > 0 && img.clientWidth > 0;
    });
    const zoomButton = lightbox.locator('button[title="Choose zoom level"]');
    await zoomButton.click();
    const popover = page.locator('#zoom-preset-popover');
    await expect(popover).toBeVisible();

    // Focus is wherever opening put it (the slider when there is one).
    await page.keyboard.press('Escape');

    await expect(popover).toBeHidden();
    expect((await store(page)).isOpen).toBe(true);
    await expect(zoomButton).toBeFocused();
  });
});
