/**
 * Tests that clicking a navigation link while editing a description inline
 * does NOT block the navigation.
 *
 * Bug: The description inline-edit uses @click.away to save changes via fetch,
 * then calls location.reload() on success. When the user clicks a nav link
 * while editing, @click.away fires first and saves the description, but then
 * location.reload() overrides the pending navigation, leaving the user stuck
 * on the current page instead of navigating to the clicked link's destination.
 *
 * Expected: Clicking a nav link while editing should save the description AND
 * navigate to the target page.
 *
 * Actual: The page reloads (location.reload()) instead of navigating to the
 * target page, blocking the user's intended navigation.
 */
import { test, expect } from '../fixtures/base.fixture';

test.describe('Description click-away should not block navigation', () => {
  let noteId: number;

  test.beforeAll(async ({ apiClient }) => {
    const note = await apiClient.createNote({
      name: 'Nav Block Test Note',
      description: 'Original description for nav block test',
    });
    noteId = note.ID;
  });

  test('clicking nav link while editing description should navigate away', async ({
    page,
  }) => {
    // Go to the note detail page
    await page.goto(`/note?id=${noteId}`);
    await page.waitForLoadState('load');

    // Find and double-click the description to enter edit mode
    const descriptionArea = page.locator('.description').first();
    await expect(descriptionArea).toBeVisible({ timeout: 5000 });
    await descriptionArea.dblclick();

    // Verify the textarea appeared
    const textarea = page.locator('textarea[name="description"]');
    await expect(textarea).toBeVisible({ timeout: 3000 });

    // Modify the description
    await textarea.fill('Modified description text');

    // Click the "Groups" link in the navigation to navigate away
    const navLink = page.getByRole('link', { name: 'Groups' });
    await navLink.click();

    // Wait for navigation to complete
    await page.waitForLoadState('load');

    // The user should have navigated to the /groups page
    // BUG: Instead, location.reload() fires and the page stays on /note?id=...
    expect(page.url()).toContain('/groups');
    expect(page.url()).not.toContain(`/note`);
  });

  test('clicking nav link while editing description should still save the description', async ({
    page,
  }) => {
    // Reset the description first
    const note = await page.request.post(`/v1/note/editDescription?id=${noteId}`, {
      multipart: { description: 'Reset description for second test' },
    });
    expect(note.ok()).toBeTruthy();

    // Go to the note detail page
    await page.goto(`/note?id=${noteId}`);
    await page.waitForLoadState('load');

    // Double-click to edit the description
    const descriptionArea = page.locator('.description').first();
    await expect(descriptionArea).toBeVisible({ timeout: 5000 });
    await descriptionArea.dblclick();

    const textarea = page.locator('textarea[name="description"]');
    await expect(textarea).toBeVisible({ timeout: 3000 });

    // Modify the description
    await textarea.fill('Saved while navigating away');

    // Click a nav link to navigate away
    const navLink = page.getByRole('link', { name: 'Tags' });
    await navLink.click();

    // Wait for any navigation/reload to settle
    await page.waitForLoadState('load');

    // Verify the description was still saved despite navigating away
    const resp = await page.request.get(`/v1/note?id=${noteId}`, {
      headers: { Accept: 'application/json' },
    });
    const data = await resp.json();
    expect(data.Description).toBe('Saved while navigating away');
  });

  test.afterAll(async ({ apiClient }) => {
    if (noteId) await apiClient.deleteNote(noteId);
  });
});
