import { test, expect } from '../fixtures/base.fixture';
import type { Page } from '@playwright/test';
import path from 'path';

/**
 * Regression tests for the lightbox adversarial review fixes.
 * Each test targets a specific confirmed finding (referenced by its BH id).
 */
test.describe('Lightbox adversary-review fixes', () => {
  let categoryId: number;
  let ownerGroupId: number;
  const createdResourceIds: number[] = [];
  const testRunId = Date.now();

  const LIGHTBOX =
    '[role="dialog"][aria-modal="true"]:not([aria-labelledby="paste-upload-title"]):not([aria-labelledby="entity-picker-title"])';

  test.beforeAll(async ({ apiClient }) => {
    const category = await apiClient.createCategory(
      `Lightbox Fixes Category ${testRunId}`,
      'Category for lightbox fix regression tests'
    );
    categoryId = category.ID;

    const ownerGroup = await apiClient.createGroup({
      name: `Lightbox Fixes Owner ${testRunId}`,
      description: 'Owner for lightbox fix regression resources',
      categoryId: categoryId,
    });
    ownerGroupId = ownerGroup.ID;

    const testImageFiles = [
      path.join(__dirname, '../test-assets/sample-image-13.png'),
      path.join(__dirname, '../test-assets/sample-image-2.png'),
      path.join(__dirname, '../test-assets/sample-image-3.png'),
    ];
    for (let i = 0; i < testImageFiles.length; i++) {
      const resource = await apiClient.createResource({
        filePath: testImageFiles[i],
        name: `Lightbox Fix Image ${i + 1} - ${testRunId}`,
        description: `Test image ${i + 1}`,
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
        /* ignore */
      }
    }
    if (ownerGroupId) await apiClient.deleteGroup(ownerGroupId);
    if (categoryId) await apiClient.deleteCategory(categoryId);
  });

  async function openLightbox(page: Page) {
    await page.goto('/resources');
    await page.waitForLoadState('load');
    await page.locator('[data-lightbox-item]').first().click();
    const lightbox = page.locator(LIGHTBOX);
    await expect(lightbox).toBeVisible();
    return lightbox;
  }

  // BH M8: the loading spinner must expose its loading state to assistive tech.
  test('loading spinner is exposed as a status region with a label', async ({ page }) => {
    const lightbox = await openLightbox(page);

    const spinner = lightbox.locator('[role="status"]');
    await expect(spinner).toHaveAttribute('role', 'status');
    // The sr-only text is present in the DOM regardless of the spinner's visibility.
    await expect(spinner.locator('.sr-only')).toHaveText('Loading media');
  });

  // BH H2 / M3: Space must activate the focused control (here, Close), not navigate.
  test('Space activates a focused button instead of navigating', async ({ page }) => {
    const lightbox = await openLightbox(page);

    const counter = lightbox.locator('div.bg-black\\/50:has-text("/")').first();
    await expect(counter).toContainText('1 /');

    // Focus the Close button explicitly, then press Space.
    const closeButton = lightbox.locator('button[aria-label="Close"]');
    await closeButton.focus();
    await page.keyboard.press('Space');

    // The button should have activated (closing the lightbox) rather than advancing.
    await expect(lightbox).toBeHidden();
  });

  // BH H3: Space (and other shortcuts) must not navigate while focus is inside a panel.
  test('Space does not navigate while focus is inside the quick-tag panel', async ({ page }) => {
    const lightbox = await openLightbox(page);

    const counter = lightbox.locator('div.bg-black\\/50:has-text("/")').first();
    await expect(counter).toContainText('1 /');

    // Open the quick-tag panel via keyboard (nothing focused → shortcut fires).
    await page.evaluate(() => (document.activeElement as HTMLElement)?.blur());
    await page.keyboard.press('t');

    const panel = page.locator('[data-quick-tag-panel]');
    await expect(panel).toBeVisible();

    // Focus a control inside the panel and press Space.
    await panel.locator('button').first().focus();
    await page.keyboard.press('Space');

    // The image position must be unchanged and the lightbox still open.
    await expect(counter).toContainText('1 /');
    await expect(lightbox).toBeVisible();
  });

  // BH M4: holding the panel-toggle key must not thrash the panel via key auto-repeat.
  test('repeated keydown of the info-panel key (auto-repeat) toggles only once', async ({ page }) => {
    await openLightbox(page);
    await page.evaluate(() => (document.activeElement as HTMLElement)?.blur());

    const panel = page.locator('[data-edit-panel]');

    // Simulate an OS key-repeat burst: the first event has repeat=false, the rest repeat=true.
    await page.evaluate(() => {
      const fire = (repeat: boolean) =>
        window.dispatchEvent(new KeyboardEvent('keydown', { key: 'e', repeat, bubbles: true }));
      fire(false);
      for (let i = 0; i < 5; i++) fire(true);
    });

    // Net effect of the burst is a single open (repeats are ignored), so the panel is open.
    await expect(panel).toBeVisible();
  });
});
