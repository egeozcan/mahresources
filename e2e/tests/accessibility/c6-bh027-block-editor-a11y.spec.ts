/**
 * BH-027: Block editor WCAG-A violations
 *
 * 1. Gallery <img> elements have no alt attribute (axe-critical image-alt)
 * 2. Heading-level <select> has no accessible name (axe-critical select-name)
 * 3. Move-up/move-down/delete icon buttons rely on title= only (serious)
 * 4. Add-Block picker has no aria-expanded / aria-haspopup / role="listbox" (serious)
 */
import { test, expect } from '../../fixtures/a11y.fixture';
import AxeBuilder from '@axe-core/playwright';

test.describe('BH-027: block editor a11y', () => {
  test('axe finds zero Critical violations on the block editor', async ({ page, apiClient }) => {
    const note = await apiClient.createNote({ Name: `bh027-${Date.now()}` });

    // Block editor is on the note display page, not the edit form
    await page.goto(`/note?id=${note.ID}`);
    await page.waitForSelector('.block-editor');

    // Enter edit mode so all controls are visible
    const editBtn = page.locator('.block-editor button', { hasText: 'Edit Blocks' });
    await editBtn.waitFor({ state: 'visible' });
    await editBtn.click();

    const scan = await new AxeBuilder({ page })
      .include('.block-editor')
      .analyze();

    const critical = scan.violations.filter(v => v.impact === 'critical');
    expect(critical, JSON.stringify(critical, null, 2)).toEqual([]);

    // cleanup
    await apiClient.deleteNote(note.ID);
  });

  test('heading-level select has accessible name', async ({ page, apiClient }) => {
    const note = await apiClient.createNote({ Name: `bh027-h-${Date.now()}` });
    await page.goto(`/note?id=${note.ID}`);
    await page.waitForSelector('.block-editor');

    // Enter edit mode
    const editBtn = page.locator('.block-editor button', { hasText: 'Edit Blocks' });
    await editBtn.waitFor({ state: 'visible' });
    await editBtn.click();

    // Open add-block picker
    const trigger = page.locator('[data-testid="add-block-trigger"]');
    await trigger.waitFor({ state: 'visible' });
    await trigger.click();

    // Click the Heading option
    const headingOption = page.locator('[role="listbox"] [data-block-type="heading"]');
    await headingOption.waitFor({ state: 'visible' });
    await headingOption.click();

    // Wait for the heading block to appear (edit mode shows the select)
    // The heading-level select is inside the block editor in edit mode
    const levelSelect = page.locator('.block-editor select[aria-label]').last();
    await levelSelect.waitFor({ state: 'visible' });

    const ariaLabel = await levelSelect.getAttribute('aria-label');
    expect(ariaLabel).toMatch(/heading level/i);

    // cleanup
    await apiClient.deleteNote(note.ID);
  });

  test('move + delete buttons have aria-labels', async ({ page, apiClient }) => {
    const note = await apiClient.createNote({ Name: `bh027-ctrl-${Date.now()}` });
    await page.goto(`/note?id=${note.ID}`);
    await page.waitForSelector('.block-editor');

    // Enter edit mode
    const editBtn = page.locator('.block-editor button', { hasText: 'Edit Blocks' });
    await editBtn.waitFor({ state: 'visible' });
    await editBtn.click();

    // Add two text blocks so move-up/down appear
    const trigger = page.locator('[data-testid="add-block-trigger"]');
    const listbox = page.locator('[role="listbox"][aria-label="Block types"]');
    for (let i = 0; i < 2; i++) {
      // Make sure picker is closed first
      await listbox.waitFor({ state: 'hidden' });
      await trigger.click();
      // Wait for listbox to open
      await listbox.waitFor({ state: 'visible' });
      const textOption = page.locator('[role="listbox"] [data-block-type="text"]');
      await textOption.waitFor({ state: 'visible' });
      await textOption.click();
      // Wait for the listbox to close after selection
      await listbox.waitFor({ state: 'hidden' });
    }

    const buttons = page.locator('[data-block-control]');
    const count = await buttons.count();
    expect(count).toBeGreaterThan(0);

    for (let i = 0; i < count; i++) {
      const aria = await buttons.nth(i).getAttribute('aria-label');
      expect(aria, `block control #${i} missing aria-label`).toBeTruthy();
    }

    // cleanup
    await apiClient.deleteNote(note.ID);
  });

  test('add-block picker exposes disclosure + listbox semantics', async ({ page, apiClient }) => {
    const note = await apiClient.createNote({ Name: `bh027-picker-${Date.now()}` });
    await page.goto(`/note?id=${note.ID}`);
    await page.waitForSelector('.block-editor');

    // Enter edit mode
    const editBtn = page.locator('.block-editor button', { hasText: 'Edit Blocks' });
    await editBtn.waitFor({ state: 'visible' });
    await editBtn.click();

    const trigger = page.locator('[data-testid="add-block-trigger"]');
    await trigger.waitFor({ state: 'visible' });

    await expect(trigger).toHaveAttribute('aria-haspopup', 'listbox');
    await expect(trigger).toHaveAttribute('aria-expanded', 'false');

    await trigger.click();
    await expect(trigger).toHaveAttribute('aria-expanded', 'true');

    const list = page.locator('[role="listbox"][aria-label="Block types"]');
    await expect(list).toBeVisible();

    // cleanup
    await apiClient.deleteNote(note.ID);
  });
});
