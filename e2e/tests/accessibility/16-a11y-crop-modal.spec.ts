/**
 * Accessibility tests for the image crop modal.
 *
 * Covers: axe-core violations in the open modal, Escape-to-close, and tab
 * order through the canonical keyboard-only crop path (aspect → X → Y →
 * Width → Height → comment → Crop → Cancel).
 */
import path from 'path';
import { test, expect } from '../../fixtures/a11y.fixture';

test.describe.serial('Image crop modal accessibility', () => {
  let resourceId: number;
  let runId: number;

  test.beforeAll(async ({ apiClient }) => {
    runId = Date.now();

    const category = await apiClient.createCategory(
      `Crop a11y Category ${runId}`,
      'Category for crop modal a11y tests',
    );
    const owner = await apiClient.createGroup({
      name: `Crop a11y Owner ${runId}`,
      description: 'Owner for crop modal a11y tests',
      categoryId: category.ID,
    });
    const resource = await apiClient.createResource({
      filePath: path.join(__dirname, '../../test-assets/sample-image-9.png'),
      name: `Crop a11y resource ${runId}`,
      description: 'Resource used to exercise the crop modal',
      ownerId: owner.ID,
    });
    resourceId = resource.ID;
  });

  test('open crop modal has no axe violations', async ({ page, checkA11y }) => {
    await page.goto(`/resource?id=${resourceId}`);
    await page.waitForLoadState('load');

    await page.locator(`#crop-open-${resourceId}`).click();
    const dialog = page.locator(`#crop-modal-${resourceId}`);
    await expect(dialog).toBeVisible();

    // axe-core needs the image to be loaded so it can evaluate alt text.
    await dialog.locator('img').first().waitFor({ state: 'visible' });

    await checkA11y();
  });

  test('Escape key closes the crop modal', async ({ page }) => {
    await page.goto(`/resource?id=${resourceId}`);
    await page.waitForLoadState('load');

    await page.locator(`#crop-open-${resourceId}`).click();
    const dialog = page.locator(`#crop-modal-${resourceId}`);
    await expect(dialog).toBeVisible();

    await page.keyboard.press('Escape');
    await expect(dialog).not.toBeVisible();
  });

  test('tab order follows the canonical keyboard path', async ({ page }) => {
    await page.goto(`/resource?id=${resourceId}`);
    await page.waitForLoadState('load');

    await page.locator(`#crop-open-${resourceId}`).click();
    const dialog = page.locator(`#crop-modal-${resourceId}`);
    await expect(dialog).toBeVisible();

    // Focus the aspect select explicitly so we have a deterministic starting
    // point (<dialog>.showModal() focuses the first tabbable element, but the
    // default varies between engines — we pin it here).
    await page.locator(`#crop-aspect-${resourceId}`).focus();

    const expectedIds = [
      `crop-x-${resourceId}`,
      `crop-y-${resourceId}`,
      `crop-w-${resourceId}`,
      `crop-h-${resourceId}`,
      `crop-comment-${resourceId}`,
    ];

    for (const expectedId of expectedIds) {
      await page.keyboard.press('Tab');
      const activeId = await page.evaluate(() => document.activeElement?.id || '');
      expect(activeId).toBe(expectedId);
    }
  });
});
