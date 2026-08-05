/**
 * BH-035: centralized /admin/shares dashboard with shareCreatedAt tracking.
 *
 * The dashboard lists every note currently holding a share token. Single
 * revoke is a per-row form against /v1/admin/shares/bulk-revoke; bulk revoke
 * is the same endpoint, checking the row checkboxes first.
 */
import { test, expect } from '../../fixtures/base.fixture';

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
    await expect(page.getByTestId('admin-shares-table')).toBeVisible();

    // Check a and b, leave c unchecked, then submit from script rather than
    // clicking "Revoke Selected": the default a11y test rendering of the page
    // has overlay DOM (downloadCockpit, pluginActionModal, pasteUpload) whose
    // backdrops can intercept pointer events even when x-show is false, before
    // Alpine finishes initializing. A scripted submit reaches the server
    // regardless of which element is on top.
    //
    // That scripted submit also means no confirmation is answered here. The
    // form carries x-data="confirmAction(...)" x-bind="events", which hangs the
    // in-app confirm dialog off the @submit event — and form.submit() dispatches
    // no submit event at all. This test has always taken that bypass (it read
    // the same way when the confirm was window.confirm); it covers the revoke
    // reaching the server, not the confirmation standing in front of it.
    //
    // Use page.evaluate so we set .checked on the actual DOM checkboxes and
    // call form.submit() in the same turn, avoiding an overlay-intercept
    // race between the click-to-check and the click-to-submit.
    await page.evaluate(
      ({ ids }) => {
        for (const id of ids) {
          const cb = document.querySelector(
            `[data-share-note-id="${id}"] input[name="ids"]`,
          ) as HTMLInputElement | null;
          if (!cb) throw new Error('checkbox missing for ' + id);
          cb.checked = true;
        }
        const form = document.querySelector(
          '[data-testid="admin-shares-form"]',
        ) as HTMLFormElement | null;
        if (!form) throw new Error('admin-shares-form missing');
        // submit(), not requestSubmit(): submit() fires no submit event, so the
        // confirmAction handler — and the confirm dialog a browser user is shown
        // — never runs. Intended UX there, just noise in an automated test.
        form.submit();
      },
      { ids: [a.ID, b.ID] },
    );
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

    // Submit the hidden per-row revoke form directly (its id is
    // admin-share-revoke-form-<noteId>) to avoid the overlay-click
    // interception that sometimes happens while Alpine is still
    // initializing the layout's lightbox / paste-upload / download-cockpit
    // modals.
    //
    // form.submit() fires no submit event, so it also sidesteps the
    // confirmAction @submit handler that opens the in-app confirm dialog — the
    // same bypass this test has always taken, back when that confirm was
    // window.confirm. Nothing to accept, and nothing left open to hang on.
    await page.evaluate((noteId) => {
      const form = document.getElementById(`admin-share-revoke-form-${noteId}`) as HTMLFormElement | null;
      if (!form) throw new Error('per-row form missing');
      form.submit();
    }, note.ID);
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
