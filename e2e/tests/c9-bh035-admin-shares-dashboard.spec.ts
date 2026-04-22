/**
 * BH-035: centralized /admin/shares dashboard with shareCreatedAt tracking.
 *
 * The dashboard lists every note currently holding a share token. Single
 * revoke is a per-row form against /v1/admin/shares/bulk-revoke; bulk revoke
 * is the same endpoint, checking the row checkboxes first.
 */
import { test, expect } from '../fixtures/base.fixture';

test.describe('BH-035: admin shares dashboard', () => {
  test('/admin/shares lists only notes with active tokens', async ({ page, apiClient }) => {
    const suffix = Date.now();
    const shared = await apiClient.createNote({ name: `BH035-shared-${suffix}` });
    const unshared = await apiClient.createNote({ name: `BH035-unshared-${suffix}` });
    await apiClient.shareNote(shared.ID);

    await page.goto('/admin/shares');
    await expect(page.getByTestId('admin-shares-table')).toBeVisible();

    // Shared note must appear as a row (locate via data-share-note-id attr).
    await expect(page.locator(`[data-share-note-id="${shared.ID}"]`)).toBeVisible();
    // Unshared note must not appear.
    await expect(page.locator(`[data-share-note-id="${unshared.ID}"]`)).toHaveCount(0);
  });

  test('bulk revoke clears share tokens for every checked row', async ({ page, apiClient }) => {
    const suffix = Date.now();
    const a = await apiClient.createNote({ name: `BH035-bulk-a-${suffix}` });
    const b = await apiClient.createNote({ name: `BH035-bulk-b-${suffix}` });
    const c = await apiClient.createNote({ name: `BH035-bulk-c-${suffix}` });
    await apiClient.shareNote(a.ID);
    await apiClient.shareNote(b.ID);
    await apiClient.shareNote(c.ID);

    await page.goto('/admin/shares');

    // Check a and b, leave c unchecked. Suppress the confirm() dialog so
    // the form submits unattended.
    page.once('dialog', (dialog) => dialog.accept());
    await page.locator(`[data-share-note-id="${a.ID}"] input[name="ids"]`).check();
    await page.locator(`[data-share-note-id="${b.ID}"] input[name="ids"]`).check();
    await page.getByTestId('admin-shares-bulk-revoke').click();
    await page.waitForURL('**/admin/shares');

    // After submit, the page reloads. a and b gone; c still present.
    await expect(page.locator(`[data-share-note-id="${a.ID}"]`)).toHaveCount(0);
    await expect(page.locator(`[data-share-note-id="${b.ID}"]`)).toHaveCount(0);
    await expect(page.locator(`[data-share-note-id="${c.ID}"]`)).toBeVisible();
  });

  test('revoke single share via per-row form removes the row', async ({ page, apiClient }) => {
    const note = await apiClient.createNote({ name: `BH035-revoke-${Date.now()}` });
    await apiClient.shareNote(note.ID);

    await page.goto('/admin/shares');
    const row = page.locator(`[data-share-note-id="${note.ID}"]`);
    await expect(row).toBeVisible();

    page.once('dialog', (dialog) => dialog.accept());
    await row.getByTestId('admin-share-revoke').click();
    await page.waitForURL('**/admin/shares');

    await expect(page.locator(`[data-share-note-id="${note.ID}"]`)).toHaveCount(0);
  });

  test('empty state renders when no notes are shared', async ({ page, apiClient }) => {
    // Revoke any pre-existing shares to guarantee empty state.
    const existing = await apiClient.getSharedNotes();
    for (const n of existing) {
      await apiClient.unshareNote(n.ID);
    }
    await page.goto('/admin/shares');
    await expect(page.getByTestId('admin-shares-empty')).toBeVisible();
    await expect(page.getByTestId('admin-shares-table')).toHaveCount(0);
  });
});
