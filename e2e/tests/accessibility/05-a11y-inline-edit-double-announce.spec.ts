/**
 * Accessibility test: inline-edit must not cause double-announcement
 *
 * Bug: The inline-edit web component copies its light DOM text content into a
 * shadow DOM span. Both the original text node and the shadow copy are exposed
 * to assistive technology, so screen readers announce the entity name twice.
 * For example, a group heading reads "TestGroup1 TestGroup1 Edit name" instead
 * of "TestGroup1 Edit name".
 *
 * WCAG 4.1.2 (Name, Role, Value) requires components to expose a single,
 * correct accessible name.
 */
import { test, expect } from '../../fixtures/a11y.fixture';

test.describe('Inline Edit - Double Announcement', () => {
  test('Group heading inline-edit should not expose light DOM text to assistive technology', async ({ page, a11yTestData }) => {
    await page.goto(`/group?id=${a11yTestData.groupId}`);
    await page.waitForLoadState('load');

    // Find the inline-edit element inside the heading
    const inlineEdit = page.locator('h1 inline-edit').first();
    await expect(inlineEdit).toBeVisible();

    // Check whether the light DOM text is consumed by a hidden slot.
    // Without the fix, the light DOM text is a bare text node that AT can read
    // alongside the shadow DOM span, causing double announcement.
    // With the fix, a <slot> with display:none consumes the light DOM content,
    // hiding it from AT while keeping textContent readable for JS.
    const hasHiddenSlot = await inlineEdit.evaluate((el) => {
      const shadow = el.shadowRoot;
      if (!shadow) return false;
      const slot = shadow.querySelector('slot');
      if (!slot) return false;
      // The slot must be hidden to prevent the light DOM content from being rendered/announced
      const style = window.getComputedStyle(slot);
      return style.display === 'none';
    });

    expect(
      hasHiddenSlot,
      'inline-edit shadow DOM must contain a hidden <slot> to consume light DOM text. ' +
      'Without it, screen readers announce the entity name twice (once from light DOM, ' +
      'once from the shadow DOM span). WCAG 4.1.2.'
    ).toBe(true);
  });

  test('Note heading inline-edit should not expose light DOM text to assistive technology', async ({ page, a11yTestData }) => {
    await page.goto(`/note?id=${a11yTestData.noteId}`);
    await page.waitForLoadState('load');

    const inlineEdit = page.locator('h1 inline-edit').first();
    await expect(inlineEdit).toBeVisible();

    const hasHiddenSlot = await inlineEdit.evaluate((el) => {
      const shadow = el.shadowRoot;
      if (!shadow) return false;
      const slot = shadow.querySelector('slot');
      if (!slot) return false;
      const style = window.getComputedStyle(slot);
      return style.display === 'none';
    });

    expect(
      hasHiddenSlot,
      'inline-edit shadow DOM must contain a hidden <slot> to consume light DOM text. ' +
      'Without it, screen readers announce the entity name twice (once from light DOM, ' +
      'once from the shadow DOM span). WCAG 4.1.2.'
    ).toBe(true);
  });

  test('inline-edit should still display the correct name in shadow DOM', async ({ page, a11yTestData }) => {
    await page.goto(`/group?id=${a11yTestData.groupId}`);
    await page.waitForLoadState('load');

    const inlineEdit = page.locator('h1 inline-edit').first();
    await expect(inlineEdit).toBeVisible();

    // The shadow DOM span should contain the entity name
    const shadowText = await inlineEdit.evaluate((el) => {
      const shadow = el.shadowRoot;
      if (!shadow) return '';
      const span = shadow.querySelector('span span');
      return span?.textContent?.trim() || '';
    });

    // The light DOM textContent should still hold the value (used as data store)
    const lightText = await inlineEdit.evaluate((el) => el.textContent?.trim() || '');

    // Both should be non-empty and match
    expect(shadowText.length).toBeGreaterThan(0);
    expect(lightText.length).toBeGreaterThan(0);
    expect(shadowText).toBe(lightText);
  });
});
