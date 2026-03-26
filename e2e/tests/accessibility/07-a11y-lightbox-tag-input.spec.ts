/**
 * Accessibility test: Lightbox tag search input must have aria-label
 *
 * Bug: The tag search input in the lightbox quick-tag panel has role="combobox"
 * and full ARIA combobox attributes but is missing aria-label. The visible
 * <label> above it ("Tags") is not programmatically associated. WCAG 4.1.2
 * requires an accessible name on all interactive elements.
 */
import { test, expect } from '../../fixtures/a11y.fixture';

test.describe('Lightbox Tag Input - Missing aria-label', () => {
  // We need a resource with an image to open the lightbox.
  const testRunId = Date.now() + Math.floor(Math.random() * 100000);
  let categoryId: number;
  let groupId: number;
  let resourceId: number;

  test.beforeAll(async ({ apiClient }) => {
    const category = await apiClient.createCategory(
      `Lightbox A11y Category ${testRunId}`,
      'For lightbox a11y tests'
    );
    categoryId = category.ID;

    const group = await apiClient.createGroup({
      name: `Lightbox A11y Group ${testRunId}`,
      categoryId: category.ID,
    });
    groupId = group.ID;

    const path = await import('path');
    const resource = await apiClient.createResource({
      filePath: path.join(__dirname, '../../test-assets/sample-image.png'),
      name: `Lightbox A11y Resource ${testRunId}`,
      ownerId: group.ID,
    });
    resourceId = resource.ID;
  });

  test('Tag search input in lightbox should have an aria-label', async ({ page }) => {
    // Navigate to the group that owns our test resource
    await page.goto(`/group?id=${groupId}`);
    await page.waitForLoadState('load');

    // Click on the resource thumbnail to open the lightbox
    const thumbnail = page.locator(`[data-lightbox-item][data-resource-id="${resourceId}"]`).first();
    const hasThumbnail = await thumbnail.isVisible({ timeout: 3000 }).catch(() => false);
    test.skip(!hasThumbnail, 'Resource thumbnail not visible on group page');

    await thumbnail.click();

    // Wait for lightbox to be visible
    const lightboxOverlay = page.locator('[x-show="$store.lightbox.isOpen"]').first();
    await lightboxOverlay.waitFor({ state: 'visible', timeout: 5000 });

    // Press 't' to open the quick tag panel (keyboard shortcut from template)
    await page.keyboard.press('t');

    // Wait for the tag input to appear
    const tagInput = page.locator('[data-tag-editor-input]');
    await tagInput.waitFor({ state: 'visible', timeout: 5000 });

    const ariaLabel = await tagInput.getAttribute('aria-label');
    expect(
      ariaLabel,
      'Lightbox tag search input has role="combobox" but no aria-label. ' +
      'The visible "Tags" label above is not programmatically associated. ' +
      'WCAG 4.1.2 requires an accessible name.'
    ).toBeTruthy();
  });

  test.afterAll(async ({ apiClient }) => {
    try { if (resourceId) await apiClient.deleteResource(resourceId); } catch { /* ignore */ }
    try { if (groupId) await apiClient.deleteGroup(groupId); } catch { /* ignore */ }
    try { if (categoryId) await apiClient.deleteCategory(categoryId); } catch { /* ignore */ }
  });
});
