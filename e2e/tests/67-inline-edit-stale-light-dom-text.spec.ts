/**
 * Tests that inline-editing a name updates the light DOM text content of the
 * <inline-edit> element, not just the shadow DOM display text.
 *
 * Bug: After a successful inline edit, only the shadow DOM's displayText span
 * is updated with the new value. The light DOM child text node (the original
 * server-rendered name) remains unchanged. This causes the <h1> heading to
 * contain both the old and new names — the old name from the light DOM text
 * node and the new name from the shadow DOM. Screen readers and accessibility
 * tools see both values in the heading, creating confusion.
 *
 * Example: editing "Original Name" to "New Name" results in the h1 accessible
 * name being "Tag Original Name New Name Edit name" instead of the expected
 * "Tag New Name Edit name" (or ideally just "Tag New Name Edit name" once).
 */
import { test, expect } from '../fixtures/base.fixture';

test.describe('Inline-edit updates light DOM text after save', () => {
  let tagId: number;

  test.beforeAll(async ({ apiClient }) => {
    const tag = await apiClient.createTag(
      'LightDomOriginal',
      'Tag for light DOM text test',
    );
    tagId = tag.ID;
  });

  test('editing tag name should update the light DOM text content of <inline-edit>', async ({
    page,
  }) => {
    await page.goto(`/tag?id=${tagId}`);
    await page.waitForLoadState('load');

    const inlineEdit = page.locator('inline-edit').first();
    await expect(inlineEdit).toBeVisible({ timeout: 5000 });

    // Verify initial state: light DOM text matches original name
    const originalLightDom = await inlineEdit.textContent();
    expect(originalLightDom?.trim()).toBe('LightDomOriginal');

    // Click the edit button inside the shadow DOM
    const editButton = inlineEdit.locator('button.edit-button');
    await editButton.click();

    // Fill in the new name
    const input = inlineEdit.locator('input');
    await expect(input).toBeVisible({ timeout: 2000 });
    await input.fill('LightDomRenamed');
    await input.press('Enter');

    // Wait for the fetch to complete and the display to settle
    await page.waitForTimeout(500);

    // The light DOM textContent of <inline-edit> should now be the NEW name.
    // BUG: it still contains the old name "LightDomOriginal" because the
    // component only updates shadow DOM displayText, not the element's own
    // text content.
    const updatedLightDom = await inlineEdit.textContent();
    expect(updatedLightDom?.trim()).not.toContain('LightDomOriginal');
    expect(updatedLightDom?.trim()).toContain('LightDomRenamed');
  });

  test.afterAll(async ({ apiClient }) => {
    if (tagId) await apiClient.deleteTag(tagId);
  });
});
