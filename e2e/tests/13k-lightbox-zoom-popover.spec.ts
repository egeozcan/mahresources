import { test, expect } from '../fixtures/base.fixture';
import type { Page } from '@playwright/test';
import path from 'path';

const LIGHTBOX =
  '[role="dialog"][aria-modal="true"]:not([aria-labelledby="paste-upload-title"]):not([aria-labelledby="entity-picker-title"])';

test.describe('Lightbox zoom popover', () => {
  let categoryId: number;
  let ownerGroupId: number;
  const createdResourceIds: number[] = [];
  const testRunId = Date.now();

  test.beforeAll(async ({ apiClient }) => {
    const category = await apiClient.createCategory(
      `Zoom Popover Category ${testRunId}`,
      'Category for lightbox zoom popover tests'
    );
    categoryId = category.ID;

    const ownerGroup = await apiClient.createGroup({
      name: `Zoom Popover Owner ${testRunId}`,
      description: 'Owner for lightbox zoom popover resources',
      categoryId,
    });
    ownerGroupId = ownerGroup.ID;

    const imageFiles = [
      path.join(__dirname, '../test-assets/sample-image-13.png'),
      path.join(__dirname, '../test-assets/sample-image-2.png'),
    ];
    for (let i = 0; i < imageFiles.length; i++) {
      const resource = await apiClient.createResource({
        filePath: imageFiles[i],
        name: `Zoom Popover Image ${i + 1} - ${testRunId}`,
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

  async function openZoomPopover(page: Page) {
    await page.goto(`/resources?OwnerId=${ownerGroupId}&sort=ID&order=desc`);
    await page.waitForLoadState('load');
    await page.locator(`[data-lightbox-item][data-resource-id="${createdResourceIds[1]}"]`).click();

    const lightbox = page.locator(LIGHTBOX);
    await expect(lightbox).toBeVisible();
    await expect(lightbox.locator('img')).toBeVisible();
    await page.waitForFunction(() => {
      const img = document.querySelector('[role="dialog"] img') as HTMLImageElement | null;
      return img && img.complete && img.naturalWidth > 0 && img.clientWidth > 0;
    });

    const zoomButton = lightbox.locator('button[title="Choose zoom level"]');
    await expect(zoomButton).toBeVisible();
    await zoomButton.click();

    const popover = page.locator('#zoom-preset-popover');
    await expect(popover).toBeVisible();
    return { lightbox, popover };
  }

  test('slider changes zoom without closing the popover', async ({ page }) => {
    const { popover } = await openZoomPopover(page);

    await expect(popover.getByRole('button', { name: /^Fit \(/ })).toBeVisible();
    const slider = popover.getByRole('slider', { name: 'Zoom level' });
    await expect(slider).toBeVisible();

    const before = await page.evaluate(() => (window as any).Alpine.store('lightbox').zoomLevel);
    await slider.evaluate((node: HTMLInputElement) => {
      const min = Number(node.min);
      const max = Number(node.max);
      const target = Math.min(max, Math.max(min, Number(node.value) + 25));
      node.value = String(target);
      node.dispatchEvent(new Event('input', { bubbles: true }));
      node.dispatchEvent(new Event('change', { bubbles: true }));
    });

    await expect
      .poll(() => page.evaluate(() => (window as any).Alpine.store('lightbox').zoomLevel))
      .toBeGreaterThan(before);
    await expect(popover).toBeVisible();

    const toolbarPercent = await page
      .locator(`${LIGHTBOX} button[title="Choose zoom level"]`)
      .innerText();
    const sliderPercent = await slider.inputValue();
    expect(toolbarPercent.trim()).toBe(`${Math.round(Number(sliderPercent))}%`);
  });

  test('focused slider arrow keys do not navigate the lightbox', async ({ page }) => {
    const { lightbox, popover } = await openZoomPopover(page);

    const counter = lightbox.locator('div.bg-black\\/50:has-text("/")').first();
    await expect(counter).toContainText('1 /');

    const slider = popover.getByRole('slider', { name: 'Zoom level' });
    await slider.focus();
    await page.keyboard.press('ArrowRight');

    await expect(counter).toContainText('1 /');
    await expect(popover).toBeVisible();
  });

  test('fit preset resets zoom and closes the popover', async ({ page }) => {
    const { popover } = await openZoomPopover(page);

    await page.evaluate(() => (window as any).Alpine.store('lightbox').setZoomLevel(2));
    await expect
      .poll(() => page.evaluate(() => (window as any).Alpine.store('lightbox').zoomLevel))
      .toBeGreaterThan(1);

    await popover.getByRole('button', { name: /^Fit \(/ }).click();

    await expect
      .poll(() => page.evaluate(() => (window as any).Alpine.store('lightbox').zoomLevel))
      .toBe(1);
    await expect(popover).toBeHidden();
  });
});
