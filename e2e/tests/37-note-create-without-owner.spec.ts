/**
 * Tests that a note can be created via the UI without selecting an Owner.
 *
 * Bug: The Owner autocompleter on the note create form has min=1, making it
 * required. But OwnerId is nullable (*uint) in the database — the Owner
 * should be optional, just like it is on the Group form.
 */
import { test, expect } from '../fixtures/base.fixture';

test.describe('Note creation without owner', () => {
  let createdNoteId: number | null = null;

  test('should allow creating a note without selecting an owner', async ({
    page,
  }) => {
    await page.goto('/note/new');
    await page.waitForLoadState('load');

    // Fill in just the name — leave Owner empty
    const nameInput = page.locator('input[name="Name"]');
    await nameInput.fill('Note Without Owner');

    // Submit the form
    await page.locator('button[type="submit"]').click();

    // Should redirect to the note display page, NOT stay on the form
    await page.waitForURL(/\/note\?id=/, { timeout: 5000 });

    // Extract the note ID from the URL
    const url = new URL(page.url());
    const idStr = url.searchParams.get('id');
    expect(idStr).toBeTruthy();
    createdNoteId = parseInt(idStr!, 10);
    expect(createdNoteId).toBeGreaterThan(0);

    // Verify the note title is displayed
    await expect(page.locator('h1')).toContainText('Note Without Owner');
  });

  test.afterAll(async ({ apiClient }) => {
    if (createdNoteId) {
      await apiClient.deleteNote(createdNoteId);
    }
  });
});
